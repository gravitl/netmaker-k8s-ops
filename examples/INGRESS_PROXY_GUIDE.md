# Netmaker Ingress Proxy Guide

## Overview

This guide explains how to expose Kubernetes cluster workloads to your Netmaker network using ingress proxy Services. This enables cluster ingress - allowing devices on your Netmaker network to access services running in your Kubernetes cluster.

## How It Works

When you create a Kubernetes Service with ingress annotations, the operator sets up an ingress proxy that listens on the Netmaker network and forwards traffic to your Kubernetes Service. Devices on the Netmaker network can then access your Kubernetes services using the Netmaker IP address or DNS name.

## Prerequisites

1. **Netmaker Network**: A configured Netmaker network
2. **Netclient Sidecar**: The operator must be running with netclient sidecar enabled
3. **Service Annotations**: Services must be annotated to enable ingress

## Configuration

### Service Annotations

To configure a Service as an ingress proxy, add the following annotations:

```yaml
metadata:
  annotations:
    netmaker.io/ingress: "enabled"
    # Optional: Specify the Netmaker IP address to bind to
    # If not specified, the proxy will automatically detect the WireGuard IP dynamically
    # This is recommended since Netmaker assigns IPs dynamically
    # netmaker.io/ingress-bind-ip: "10.0.0.50"
    # Optional: Specify the Netmaker DNS name for this service
    netmaker.io/ingress-dns-name: "my-app.netmaker.internal"
    # Optional: Custom secret configuration for netclient token
    # Note: Secrets are always read from operator namespace (netmaker-k8s-ops-system) for security
    # netmaker.io/secret-name: "custom-netclient-token"  # Default: netclient-token
    # netmaker.io/secret-key: "token"                    # Default: token
```

**Dynamic IP Detection**: By default (when `netmaker.io/ingress-bind-ip` is not specified), the ingress proxy automatically detects the WireGuard IP assigned by Netmaker. This works because:
1. The proxy container shares the network namespace with the netclient sidecar
2. The proxy waits for the WireGuard interface to be ready (up to 60 seconds)
3. It detects the IP from common interface names (`netmaker`, `wg0`, `wg1`)
4. Falls back to detecting any private IP if the interface name is unknown
5. Only binds to `0.0.0.0` as a last resort (with a warning)

### Service Configuration

The Service should be configured normally - the ingress proxy will forward traffic to it:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-k8s-service
  namespace: default
  annotations:
    netmaker.io/ingress: "enabled"
    netmaker.io/ingress-dns-name: "my-app.netmaker.internal"
spec:
  ports:
  - port: 8080
    targetPort: 8080
    protocol: TCP
  selector:
    app: my-app
  type: ClusterIP
```

## Examples

### Example 1: Expose Kubernetes API Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-api-service
  namespace: default
  annotations:
    netmaker.io/ingress: "enabled"
    netmaker.io/ingress-dns-name: "api.k8s.netmaker.internal"
spec:
  ports:
  - name: http
    port: 80
    targetPort: 8080
    protocol: TCP
  selector:
    app: my-api
  type: ClusterIP
```

**Access from Netmaker network:**
```bash
curl http://api.k8s.netmaker.internal
# or using the WireGuard IP
curl http://10.0.0.50:80
```

### Example 2: Expose Kubernetes Database

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-db-service
  namespace: default
  annotations:
    netmaker.io/ingress: "enabled"
spec:
  ports:
  - name: postgres
    port: 5432
    targetPort: 5432
    protocol: TCP
  selector:
    app: postgres
  type: ClusterIP
```

**Access from Netmaker network:**
```bash
psql -h <ingress-proxy-netmaker-ip> -p 5432 -U postgres
```

### Example 3: Expose Multiple Ports

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-app-service
  namespace: default
  annotations:
    netmaker.io/ingress: "enabled"
    netmaker.io/ingress-dns-name: "app.netmaker.internal"
spec:
  ports:
  - name: http
    port: 80
    targetPort: 8080
    protocol: TCP
  - name: https
    port: 443
    targetPort: 8443
    protocol: TCP
  selector:
    app: my-app
  type: ClusterIP
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Netmaker Network                         │
│                                                               │
│  ┌──────────────────────────────────────────────────────┐  │
│  │         Netmaker Device                              │  │
│  │         (wants to access K8s service)                │  │
│  └──────────────────┬───────────────────────────────────┘  │
└─────────────────────┼───────────────────────────────────────┘
                      │ WireGuard Tunnel
                      ▼
┌─────────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                        │
│                                                               │
│  ┌──────────────────────────────────────────────────────┐  │
│  │         Ingress Proxy Pod                             │  │
│  │  ┌──────────────┐  ┌──────────────────────────────┐  │  │
│  │  │   Netclient  │  │   Proxy Container (socat)   │  │  │
│  │  │   Sidecar    │  │   Listens on Netmaker IP    │  │  │
│  │  └──────────────┘  └──────────┬───────────────────┘  │  │
│  └────────────────────────────────┼───────────────────────┘  │
│                                   │                          │
│                                   ▼                          │
│                          ┌──────────────┐                    │
│                          │ K8s Service  │                    │
│                          │  (ClusterIP) │                    │
│                          └──────┬───────┘                    │
│                                 │                            │
│                                 ▼                            │
│                          ┌──────────────┐                    │
│                          │  Your Pods   │                    │
│                          └──────────────┘                    │
└─────────────────────────────────────────────────────────────┘
```

## Traffic Flow

1. **Netmaker Device Request**: Device on Netmaker network makes request to ingress proxy IP/DNS
2. **Ingress Proxy Receives**: Proxy pod (with netclient) receives request on Netmaker network
3. **Forward to Service**: Proxy forwards request to Kubernetes Service
4. **Service Routes**: Kubernetes Service routes to your application pods
5. **Response**: Response follows reverse path back to Netmaker device

## How Netmaker Devices Access Kubernetes Services

Once the ingress proxy is created, devices on your Netmaker network can access the Kubernetes service:

### Using Netmaker IP Address:
```bash
curl http://10.0.0.50:80  # Using the ingress proxy's Netmaker IP
```

### Using DNS Name (if configured):
```bash
curl http://my-app.netmaker.internal:80
```

## Implementation Details

### Ingress Proxy Pod

The operator creates an ingress proxy pod that:
- Runs a netclient sidecar for WireGuard connectivity (gets Netmaker IP dynamically)
- Runs a proxy container (socat) that automatically detects and listens on the WireGuard IP
- Forwards traffic to the Kubernetes Service

### Dynamic WireGuard IP Detection

Since Netmaker assigns IP addresses dynamically, the ingress proxy automatically detects the WireGuard IP at runtime:

1. **Interface Detection**: The proxy checks for common WireGuard interface names (`netmaker`, `wg0`, `wg1`)
2. **Wait for Interface**: Waits up to 60 seconds for the interface to be created and brought up
3. **IP Extraction**: Extracts the IP address from the WireGuard interface
4. **Fallback Detection**: If interface name is unknown, searches for any private IP address
5. **Binding**: Binds socat to the detected IP address

**Benefits**:
- Works regardless of what IP Netmaker assigns
- No need to hardcode IP addresses
- Handles interface name variations
- Automatically adapts if IP changes (on pod restart)

**Manual Override**: You can still specify `netmaker.io/ingress-bind-ip` if you want to use a specific IP, but dynamic detection is recommended.

### Service Selector

The ingress proxy forwards to your existing Kubernetes Service, so your Service's selector should match your application pods as usual.

## Best Practices

1. **Use DNS Names**: Configure `netmaker.io/ingress-dns-name` for easier access
2. **Network Isolation**: Only services that need external access should enable ingress
3. **Security**: Consider network policies to restrict access
4. **Resource Limits**: Set appropriate resource limits for ingress proxy pods

## Troubleshooting

### Cannot Access from Netmaker Network

1. Check ingress proxy pod is running:
   ```bash
   kubectl get pods -l app=netmaker-ingress-proxy
   ```

2. Verify netclient connectivity:
   ```bash
   kubectl exec -it <ingress-proxy-pod> -c netclient -- netclient status
   ```

3. Check Netmaker IP assignment:
   ```bash
   kubectl exec -it <ingress-proxy-pod> -c netclient -- ip addr show netmaker
   ```

4. Test connectivity from Netmaker device:
   ```bash
   ping <ingress-proxy-netmaker-ip>
   ```

### Service Not Responding

1. Verify the target Kubernetes Service exists:
   ```bash
   kubectl get svc <service-name>
   ```

2. Check Service endpoints:
   ```bash
   kubectl get endpoints <service-name>
   ```

3. Verify application pods are running:
   ```bash
   kubectl get pods -l <service-selector>
   ```

## Comparison: Ingress vs Egress

| Feature | Ingress | Egress |
|---------|---------|--------|
| Direction | Netmaker → Kubernetes | Kubernetes → Netmaker |
| Use Case | Expose K8s services to Netmaker network | Access Netmaker services from K8s |
| Proxy Listens On | Netmaker network IP | Kubernetes Service port |
| Proxy Forwards To | Kubernetes Service | Netmaker device |

## See Also

- [Egress Proxy Guide](EGRESS_PROXY_GUIDE.md) - Expose Netmaker services to Kubernetes
- [Netclient Sidecar Usage Guide](NETCLIENT_SIDECAR_USAGE.md) - Direct pod connectivity
- [WireGuard Setup Guide](../WIREGUARD_SETUP.md)

