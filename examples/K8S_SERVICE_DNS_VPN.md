# Accessing Kubernetes Service DNS from VPN

This guide explains how to access Kubernetes internal service names (e.g., `my-service.default.svc.cluster.local`) from devices connected to the Netmaker VPN.

## Current Behavior

### Pods with Netclient Sidecars
Pods that have the netclient sidecar injected can:
- ✅ Access the VPN network
- ✅ Resolve and access Kubernetes service DNS names (e.g., `my-service.default.svc.cluster.local`)
- ✅ Access services via ClusterIP

This works because:
- Pods use the cluster's CoreDNS for DNS resolution
- Pods are on the cluster network where service IPs are routable

### External VPN Clients
Devices connected to the Netmaker VPN (but outside the cluster) can:
- ✅ Reach pods via their WireGuard IP addresses
- ❌ **Cannot** resolve Kubernetes service DNS names
- ❌ **Cannot** access ClusterIP services directly

## Enabling Kubernetes Service DNS from VPN

To enable external VPN clients to resolve and access Kubernetes services, you need to:

### Option 1: Expose CoreDNS and Configure DNS Forwarding

1. **Find the Kubernetes DNS IP**

   First, find your cluster's DNS IP (usually CoreDNS):

   ```bash
   # Get the DNS service IP
   kubectl get svc -n kube-system kube-dns -o jsonpath='{.spec.clusterIP}'
   
   # Or if using CoreDNS directly
   kubectl get svc -n kube-system coredns -o jsonpath='{.spec.clusterIP}'
   
   # Common default: 10.96.0.10
   ```

   See [FIND_K8S_DNS.md](./FIND_K8S_DNS.md) for detailed methods to find the DNS IP.

2. **Expose CoreDNS to the VPN Network**

   The CoreDNS service already exists, but you need to ensure it's accessible from the VPN. You can:

   **Option A: Use existing CoreDNS service** (if service CIDR is routable from VPN)
   
   **Option B: Create a new service** that's accessible via VPN:

   ```yaml
   apiVersion: v1
   kind: Service
   metadata:
     name: coredns-vpn
     namespace: kube-system
   spec:
     selector:
       k8s-app: kube-dns
     ports:
     - name: dns
       port: 53
       targetPort: 53
       protocol: UDP
     - name: dns-tcp
       port: 53
       targetPort: 53
       protocol: TCP
     type: ClusterIP
     # Note: You'll need to ensure this service IP is routable from VPN
   ```

2. **Configure Netmaker DNS Forwarding**

   In your Netmaker server configuration, add DNS forwarding for `.svc.cluster.local`:

   - Forward `.svc.cluster.local` queries to CoreDNS service IP
   - Or configure Netmaker to use CoreDNS as a nameserver for cluster domains

3. **Make Service IPs Routable**

   You have several options:

   **Option A: Use NodePort Services**
   ```yaml
   apiVersion: v1
   kind: Service
   metadata:
     name: my-service
   spec:
     type: NodePort
     ports:
     - port: 80
       targetPort: 8080
   ```

   **Option B: Use LoadBalancer Services**
   ```yaml
   apiVersion: v1
   kind: Service
   metadata:
     name: my-service
   spec:
     type: LoadBalancer
     ports:
     - port: 80
       targetPort: 8080
   ```

   **Option C: Configure Route to Service CIDR**
   - Add routes in Netmaker to route service CIDR (typically `10.96.0.0/12`) through a pod with netclient
   - This requires network configuration on the Netmaker server

### Option 2: Use a DNS Proxy/Forwarder Pod

Deploy a DNS forwarder pod with netclient sidecar that forwards DNS queries:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dns-forwarder
  labels:
    netmaker.io/netclient: enabled
spec:
  replicas: 1
  selector:
    matchLabels:
      app: dns-forwarder
  template:
    metadata:
      labels:
        app: dns-forwarder
        netmaker.io/netclient: enabled
    spec:
      containers:
      - name: dns-forwarder
        image: coredns/coredns:latest
        args:
        - -conf
        - /etc/coredns/Corefile
        volumeMounts:
        - name: config
          mountPath: /etc/coredns
      volumes:
      - name: config
        configMap:
          name: dns-forwarder-config
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: dns-forwarder-config
data:
  Corefile: |
    .:53 {
        forward . 10.96.0.10:53  # CoreDNS service IP
        cache
        errors
        log
    }
```

Then configure Netmaker to use this DNS forwarder's WireGuard IP.

## Recommended Approach

For most use cases, **accessing pods directly via their WireGuard IPs** is simpler and more secure:

1. **Expose services via NodePort or LoadBalancer** if you need external access
2. **Use pod WireGuard IPs** for direct pod access from VPN
3. **Use port-forwarding** for development/testing

## Finding Service DNS Names

To find available service DNS names in your cluster:

```bash
# List all services with their DNS names
kubectl get svc --all-namespaces

# Get DNS name for a specific service
kubectl get svc <service-name> -n <namespace> -o jsonpath='{.metadata.name}.{.metadata.namespace}.svc.cluster.local{"\n"}'

# List all service DNS names
kubectl get svc --all-namespaces -o json | \
  jq -r '.items[] | "\(.metadata.name).\(.metadata.namespace).svc.cluster.local"'
```

Service DNS names follow the format: `<service-name>.<namespace>.svc.cluster.local`

See [FIND_K8S_SERVICE_DNS.md](./FIND_K8S_SERVICE_DNS.md) for detailed methods to find service DNS names.

## Testing

### From a Pod with Netclient Sidecar

```bash
# Test DNS resolution
kubectl exec <pod-name> -c <app-container> -- nslookup my-service.default.svc.cluster.local

# Test service access
kubectl exec <pod-name> -c <app-container> -- curl http://my-service.default.svc.cluster.local:80
```

### From External VPN Client

```bash
# Test pod access via WireGuard IP
curl http://<pod-wireguard-ip>:<port>

# Test DNS (will fail unless configured)
nslookup my-service.default.svc.cluster.local
```

## Security Considerations

1. **Network Policies**: Ensure network policies allow traffic from VPN IPs
2. **Service Exposure**: Be careful when exposing services to VPN - use proper authentication
3. **DNS Security**: If exposing CoreDNS, consider DNS-over-TLS or restrict access
4. **Service Mesh**: Consider using a service mesh (Istio, Linkerd) for better security and routing

## Troubleshooting

### DNS Not Resolving from VPN

1. Check if CoreDNS is accessible from VPN network
2. Verify DNS forwarding is configured in Netmaker
3. Check firewall rules allow DNS traffic (port 53)
4. Verify service IPs are routable from VPN

### Services Not Accessible

1. Check if service type allows external access (NodePort/LoadBalancer)
2. Verify network policies allow traffic
3. Check service endpoints are healthy
4. Verify routing is configured correctly

