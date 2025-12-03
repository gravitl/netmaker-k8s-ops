# Finding Kubernetes Cluster DNS IP

This guide shows how to find the DNS IP address for your Kubernetes cluster (CoreDNS).

## Quick Method

### Find CoreDNS Service IP

```bash
# Get the DNS service IP (CoreDNS)
kubectl get svc -n kube-system kube-dns -o jsonpath='{.spec.clusterIP}'

# Or if using CoreDNS directly
kubectl get svc -n kube-system coredns -o jsonpath='{.spec.clusterIP}'

# Alternative: Get all DNS services
kubectl get svc -n kube-system | grep -E 'kube-dns|coredns|dns'
```

### Common DNS IPs

Most Kubernetes clusters use:
- **Default DNS IP**: `10.96.0.10` (most common)
- **Alternative**: `10.0.0.10` or other values in the service CIDR range

## Detailed Methods

### Method 1: Check CoreDNS Service

```bash
# List DNS services in kube-system namespace
kubectl get svc -n kube-system | grep -E 'dns|coredns'

# Get detailed information
kubectl get svc -n kube-system kube-dns -o yaml

# Extract just the ClusterIP
kubectl get svc -n kube-system kube-dns -o jsonpath='{.spec.clusterIP}{"\n"}'
```

### Method 2: Check Pod DNS Configuration

```bash
# Check DNS configuration from a pod
kubectl run -it --rm debug --image=busybox --restart=Never -- cat /etc/resolv.conf

# Or from an existing pod
kubectl exec <pod-name> -- cat /etc/resolv.conf
```

The output will show:
```
nameserver 10.96.0.10
search default.svc.cluster.local svc.cluster.local cluster.local
```

### Method 3: Check CoreDNS Pods

```bash
# List CoreDNS pods
kubectl get pods -n kube-system | grep -E 'coredns|kube-dns'

# Get CoreDNS pod IPs (these are different from service IP)
kubectl get pods -n kube-system -l k8s-app=kube-dns -o wide
```

### Method 4: Check Cluster Configuration

```bash
# Check kubelet configuration (on nodes)
# The DNS IP is usually configured in kubelet config
cat /var/lib/kubelet/config.yaml | grep clusterDNS

# Or check kubeadm config (if using kubeadm)
cat /etc/kubernetes/kubelet.conf | grep clusterDNS
```

## Finding Service CIDR (DNS IP Range)

The DNS service IP is typically the first IP in the service CIDR range:

```bash
# Get service CIDR from cluster info
kubectl cluster-info dump | grep -i service-cluster-ip-range

# Or check kubeadm config
kubectl get configmap -n kube-system kubeadm-config -o yaml | grep serviceSubnet
```

Common service CIDR ranges:
- `10.96.0.0/12` → DNS at `10.96.0.10`
- `10.0.0.0/24` → DNS at `10.0.0.10`
- `172.16.0.0/13` → DNS at `172.16.0.10`

## Verification

### Test DNS Resolution

```bash
# Test DNS from a pod
kubectl run -it --rm test-dns --image=busybox --restart=Never -- nslookup kubernetes.default.svc.cluster.local

# Test with dig (if available)
kubectl run -it --rm test-dns --image=busybox --restart=Never -- nslookup kubernetes.default.svc.cluster.local 10.96.0.10
```

### Check DNS Endpoints

```bash
# Verify DNS service has endpoints
kubectl get endpoints -n kube-system kube-dns

# Check if CoreDNS pods are ready
kubectl get pods -n kube-system -l k8s-app=kube-dns
```

## For VPN Configuration

When configuring Netmaker to forward DNS queries to Kubernetes:

1. **Find the DNS IP** using the methods above
2. **Verify it's accessible** from pods with netclient sidecar
3. **Configure Netmaker** to forward `.svc.cluster.local` queries to this IP

Example:
```bash
# Get DNS IP
DNS_IP=$(kubectl get svc -n kube-system kube-dns -o jsonpath='{.spec.clusterIP}')
echo "Kubernetes DNS IP: $DNS_IP"

# Test from a pod with netclient
kubectl exec <pod-with-netclient> -- nslookup kubernetes.default.svc.cluster.local $DNS_IP
```

## Troubleshooting

### DNS Service Not Found

```bash
# Check if CoreDNS is installed
kubectl get deployment -n kube-system coredns

# Check all services in kube-system
kubectl get svc -n kube-system
```

### DNS Not Working

```bash
# Check CoreDNS logs
kubectl logs -n kube-system -l k8s-app=kube-dns

# Check CoreDNS configuration
kubectl get configmap -n kube-system coredns -o yaml
```

### Multiple DNS Services

Some clusters may have multiple DNS services. Check which one is actually being used:

```bash
# Check which DNS service pods are using
kubectl run -it --rm debug --image=busybox --restart=Never -- cat /etc/resolv.conf

# Match the nameserver IP with service IPs
kubectl get svc -n kube-system -o wide | grep -E 'dns|coredns'
```

