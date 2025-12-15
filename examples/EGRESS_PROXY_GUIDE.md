# Netmaker Egress Proxy Guide

## Overview

This guide explains how to expose Netmaker services to your Kubernetes cluster workloads using egress proxy Services. This enables cluster egress - allowing your Kubernetes applications to access services that are external to your cluster but available in your Netmaker network.

## Status

✅ **Implemented**: The egress proxy controller is now implemented and watches for Services with `netmaker.io/egress: "enabled"` annotation. It automatically:
1. Creates egress proxy pods with netclient sidecar
2. Configures nginx proxy to route traffic to Netmaker devices
3. Updates Service endpoints to route traffic to proxy pods
4. Cleans up resources when Services are deleted or egress is disabled

## How It Works

When you create a Kubernetes Service with specific annotations, the operator sets up an in-cluster egress proxy that routes traffic to a Netmaker device. Your cluster workloads can then access the Netmaker service using the Kubernetes Service name, without needing to know the underlying Netmaker IP or DNS.

## Prerequisites

1. **Netmaker Network**: A configured Netmaker network with devices/services you want to expose
2. **Netclient Sidecar**: The operator must be running with netclient sidecar enabled
3. **Service Annotations**: Services must be annotated to specify the Netmaker target

## Configuration

### Service Annotations

To configure a Service as an egress proxy to a Netmaker device, add the following annotations:

**Important**: Target ports are specified using the standard Kubernetes `targetPort` in the Service spec, not via annotations. This is simpler and follows Kubernetes conventions.

#### Option 1: Using Netmaker IP Address

```yaml
metadata:
  annotations:
    netmaker.io/egress: "enabled"
    netmaker.io/egress-target-ip: "10.0.0.100"  # Netmaker device IP
spec:
  ports:
  - port: 8080
    targetPort: 8080  # Port on the Netmaker device (standard Kubernetes way)
```

#### Option 2: Using Netmaker DNS Name

```yaml
metadata:
  annotations:
    netmaker.io/egress: "enabled"
    netmaker.io/egress-target-dns: "api.internal.netmaker"  # Netmaker device DNS name
    # Optional: Custom secret configuration for netclient token
    # Note: Secrets are always read from operator namespace (netmaker-k8s-ops-system) for security
    # netmaker.io/secret-name: "custom-netclient-token"  # Default: netclient-token
    # netmaker.io/secret-key: "token"                    # Default: token
spec:
  ports:
  - port: 8080
    targetPort: 8080  # Port on the Netmaker device
```

### Service Configuration

The Service should be configured as a ClusterIP service (default) or NodePort/LoadBalancer if needed:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-netmaker-service
  namespace: default
  annotations:
    netmaker.io/egress: "enabled"
    netmaker.io/egress-target-ip: "10.0.0.100"  # Netmaker device IP
spec:
  ports:
  - port: 8080                    # Port exposed by the Service
    targetPort: 8080              # Port on the Netmaker device (standard Kubernetes way)
    protocol: TCP
  selector:
    # This selector will be used to route to the egress proxy
    app: netmaker-egress-proxy
  type: ClusterIP
```

## Examples

### Example 1: Expose Netmaker API Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: netmaker-api
  namespace: default
  annotations:
    netmaker.io/egress: "enabled"
    netmaker.io/egress-target-dns: "api.netmaker.internal"
    netmaker.io/egress-target-port: "443"
spec:
  ports:
  - name: https
    port: 443
    targetPort: 443
    protocol: TCP
  type: ClusterIP
```

**Usage in Pod:**
```bash
curl https://netmaker-api.default.svc.cluster.local/api/health
```

### Example 2: Expose Netmaker Database

```yaml
apiVersion: v1
kind: Service
metadata:
  name: netmaker-db
  namespace: default
  annotations:
    netmaker.io/egress: "enabled"
    netmaker.io/egress-target-ip: "10.0.0.50"
spec:
  ports:
  - name: postgres
    port: 5432
    targetPort: 5432  # Port on the Netmaker database device
    protocol: TCP
  type: ClusterIP
```

**Usage in Pod:**
```bash
psql -h netmaker-db.default.svc.cluster.local -U postgres
```

### Example 3: Expose Multiple Ports

```yaml
apiVersion: v1
kind: Service
metadata:
  name: netmaker-app
  namespace: default
  annotations:
    netmaker.io/egress: "enabled"
    netmaker.io/egress-target-ip: "10.0.0.200"
spec:
  ports:
  - name: http
    port: 80
    targetPort: 80
    protocol: TCP
  - name: https
    port: 443
    targetPort: 443
    protocol: TCP
  type: ClusterIP
```

## How Cluster Workloads Access Netmaker Services

Once the egress proxy Service is created, your Kubernetes workloads can access the Netmaker service using standard Kubernetes Service DNS:

### From within the same namespace:
```bash
curl http://my-netmaker-service:8080
```

### From a different namespace:
```bash
curl http://my-netmaker-service.default.svc.cluster.local:8080
```

### Using FQDN:
```bash
curl http://my-netmaker-service.default.svc.cluster.local:8080
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                        │
│                                                               │
│  ┌──────────────┐         ┌──────────────────────────────┐  │
│  │   Your Pod   │────────▶│  Egress Proxy Service        │  │
│  │              │         │  (Kubernetes Service)        │  │
│  └──────────────┘         └──────────────┬───────────────┘  │
│                                           │                   │
│                                           ▼                   │
│                                  ┌──────────────────┐         │
│                                  │ Egress Proxy Pod │         │
│                                  │ (with netclient) │         │
│                                  └────────┬─────────┘         │
└───────────────────────────────────────────┼───────────────────┘
                                            │ WireGuard Tunnel
                                            ▼
┌─────────────────────────────────────────────────────────────┐
│                    Netmaker Network                         │
│                                                               │
│  ┌──────────────────────────────────────────────────────┐  │
│  │         Netmaker Device/Service                      │  │
│  │         (10.0.0.100:8080 or DNS name)               │  │
│  └──────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

## Implementation Details

### Egress Proxy Pod

The operator creates an egress proxy pod that:
- Runs a netclient sidecar for WireGuard connectivity
- Runs a proxy container that routes traffic to the Netmaker target
- Is automatically managed by the operator

### Service Selector

The Service selector matches the egress proxy pod labels, allowing Kubernetes to route traffic to the proxy pod.

### Traffic Flow

1. **Pod Request**: Your application pod makes a request to the Kubernetes Service
2. **Service Routing**: Kubernetes routes the request to the egress proxy pod
3. **Proxy Processing**: The proxy pod receives the request
4. **WireGuard Routing**: The proxy forwards the request through the WireGuard tunnel
5. **Netmaker Device**: The request reaches the target Netmaker device/service
6. **Response**: The response follows the reverse path back to your pod

## Best Practices

1. **Use DNS Names**: Prefer `netmaker.io/egress-target-dns` over IP addresses for better maintainability
2. **Namespace Isolation**: Create egress Services in the same namespace as your workloads
3. **Port Mapping**: Ensure the Service port matches your application's expectations
4. **Health Checks**: Monitor the egress proxy pod health
5. **Resource Limits**: Set appropriate resource limits for egress proxy pods

## Troubleshooting

### Service Not Routing

1. Check that the egress proxy pod is running:
   ```bash
   kubectl get pods -l app=netmaker-egress-proxy
   ```

2. Verify Service endpoints:
   ```bash
   kubectl get endpoints my-netmaker-service
   ```

3. Check proxy logs:
   ```bash
   kubectl logs -l app=netmaker-egress-proxy -c proxy
   ```

### Cannot Reach Netmaker Device

1. Verify netclient connectivity:
   ```bash
   kubectl exec -it <egress-proxy-pod> -c netclient -- netclient status
   ```

2. Test WireGuard connectivity:
   ```bash
   kubectl exec -it <egress-proxy-pod> -c netclient -- ping <netmaker-device-ip>
   ```

3. Check Netmaker network configuration:
   - Ensure the target device is in the same Netmaker network
   - Verify routes are configured correctly

## Advanced Configuration

### Custom Proxy Image

You can specify a custom proxy image using annotations:

```yaml
metadata:
  annotations:
    netmaker.io/egress: "enabled"
    netmaker.io/egress-target-ip: "10.0.0.100"
    netmaker.io/egress-proxy-image: "custom/proxy:latest"
```

### Multiple Target Ports

For services with multiple ports, each port can map to a different target:

```yaml
spec:
  ports:
  - name: http
    port: 80
    targetPort: 8080
  - name: https
    port: 443
    targetPort: 8443
```

The proxy will route each port to the corresponding target port on the Netmaker device.

## See Also

- [Netclient Sidecar Usage Guide](NETCLIENT_SIDECAR_USAGE.md)
- [Proxy Usage Guide](../PROXY_USAGE.md)
- [WireGuard Setup Guide](../WIREGUARD_SETUP.md)

