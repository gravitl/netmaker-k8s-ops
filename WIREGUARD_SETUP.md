# WireGuard Tunnel Setup Guide

This guide explains how to configure the Netmaker K8s operator to connect to a Kubernetes cluster through a WireGuard tunnel.

## Overview

The operator includes a `netclient` sidecar container that establishes a WireGuard connection to your Netmaker network, allowing the proxy to access the Kubernetes API server through the tunnel.

## Prerequisites

1. **Netmaker Server**: A running Netmaker server with a configured network
2. **Netmaker Token**: A valid token for joining the network
3. **Kubernetes Cluster**: Accessible through the WireGuard tunnel

## Configuration Steps

### 1. Get Your Netmaker Token

1. Log into your Netmaker server
2. Create a new network or use an existing one
3. Generate a token for the Kubernetes cluster
4. Copy the token (it will look like: `eyJzZXJ2ZXIiOi...`)

### 2. Update the Deployment

Edit `config/manager/manager.yaml` and set your Netmaker token:

```yaml
env:
- name: NETCLIENT_TOKEN
  value: "YOUR_NETMAKER_TOKEN_HERE"  # Replace with your actual token
```

### 3. Configure Netclient Sidecar

The netclient sidecar is already configured with:
- **Image**: `gravitl/netclient:v1.1.0`
- **Privileged**: `true` (required for WireGuard)
- **Capabilities**: `NET_ADMIN`, `SYS_MODULE`
- **Host Network**: `true` (required for WireGuard interfaces)

### 4. Deploy the Operator

```bash
# Build and deploy
make docker-build IMG=controller:latest
kubectl apply -f config/default/

# Check the deployment
kubectl get pods -n netmaker-k8s-ops-system
```

### 5. Verify WireGuard Connection

Check if the WireGuard interface is created:

```bash
# Check netclient logs
kubectl logs -n netmaker-k8s-ops-system -l control-plane=controller-manager -c netclient

# Check WireGuard interface status
kubectl exec -n netmaker-k8s-ops-system -l control-plane=controller-manager -c netclient -- ip link show

# Check proxy status
curl http://localhost:8085/netclient/status
```

## Network Configuration

### WireGuard Interface

The netclient will create a WireGuard interface (typically `wg0`) with:
- **IP Address**: Assigned by Netmaker server
- **DNS**: Configured by Netmaker server
- **Routes**: Routes to the Kubernetes cluster through the tunnel

### Kubernetes API Access

The proxy will connect to the Kubernetes API server using:
- **API Server URL**: The WireGuard IP of the Kubernetes API server
- **Authentication**: Uses the same kubeconfig as the operator
- **Tunnel**: All traffic goes through the WireGuard interface

## Troubleshooting

### Common Issues

1. **Token Invalid**
   ```
   Error: Invalid token
   ```
   - Verify the token is correct
   - Check if the token has expired
   - Ensure the token has permission to join the network

2. **WireGuard Interface Not Created**
   ```
   Error: Failed to create WireGuard interface
   ```
   - Check if the pod has `hostNetwork: true`
   - Verify `privileged: true` and capabilities
   - Check netclient logs for detailed errors

3. **Cannot Reach Kubernetes API**
   ```
   Error: Connection refused to K8s API
   ```
   - Verify the Kubernetes API server is accessible via WireGuard
   - Check if the API server IP is correct in kubeconfig
   - Ensure the WireGuard interface has the correct routes

### Debug Commands

```bash
# Check netclient status
kubectl exec -n netmaker-k8s-ops-system -l control-plane=controller-manager -c netclient -- netclient status

# Check WireGuard interface
kubectl exec -n netmaker-k8s-ops-system -l control-plane=controller-manager -c netclient -- ip addr show wg0

# Check routes
kubectl exec -n netmaker-k8s-ops-system -l control-plane=controller-manager -c netclient -- ip route

# Test connectivity to K8s API
kubectl exec -n netmaker-k8s-ops-system -l control-plane=controller-manager -c netclient -- curl -k https://K8S_API_IP:6443/version
```

## Security Considerations

1. **Privileged Container**: The netclient runs with elevated privileges
2. **Host Network**: The pod shares the host's network namespace
3. **Token Security**: Store the Netmaker token securely (consider using Kubernetes secrets)

## Alternative Configurations

### Using Kubernetes Secrets

Instead of hardcoding the token, use a Kubernetes secret:

```yaml
env:
- name: NETCLIENT_TOKEN
  valueFrom:
    secretKeyRef:
      name: netmaker-token
      key: token
```

Create the secret:
```bash
kubectl create secret generic netmaker-token --from-literal=token=YOUR_TOKEN
```

### Custom Netclient Configuration

You can customize the netclient configuration by mounting additional config files:

```yaml
volumeMounts:
- mountPath: /etc/netclient
  name: etc-netclient
- mountPath: /etc/netclient/netclient.conf
  name: netclient-config
  subPath: netclient.conf
```

## Monitoring

The proxy provides several endpoints for monitoring:

- `/health` - Basic health check
- `/ready` - Readiness check
- `/netclient/status` - WireGuard connection status
- `/metrics` - Prometheus metrics

## Next Steps

1. Set your Netmaker token in the deployment
2. Deploy the operator
3. Verify the WireGuard connection
4. Test the proxy functionality
5. Configure your kubeconfig to use the proxy

For more information, see the [PROXY_USAGE.md](PROXY_USAGE.md) guide.
