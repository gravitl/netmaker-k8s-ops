# Netmaker K8s Operator Deployment Guide

This guide explains how to deploy the Netmaker K8s operator as a single replica with WireGuard connectivity.

## Overview

The operator provides:
- **Kubernetes Controller**: Manages Netmaker resources (when implemented)
- **API Proxy**: Reverse proxy for accessing Kubernetes API through WireGuard
- **Netclient Sidecar**: Establishes WireGuard connection to Netmaker network

## Prerequisites

1. **Netmaker Server**: Running Netmaker server with a configured network
2. **Netmaker Token**: Valid token for joining the network
3. **Kubernetes Cluster**: Accessible through WireGuard tunnel
4. **kubectl**: Configured to access your Kubernetes cluster

## Quick Start

### 1. Configure WireGuard

1. **Get your Netmaker token** from your Netmaker server
2. **Update the deployment** with your token:

```bash
# Edit config/manager/manager.yaml
# Set your Netmaker token in both places:
# - NETCLIENT_TOKEN environment variable (line 83)
# - netclient container TOKEN environment variable (line 116)
```

### 2. Build and Deploy

```bash
# Build the operator
make docker-build IMG=controller:latest

# Deploy to Kubernetes
kubectl apply -f config/default/

# Check deployment status
kubectl get pods -n netmaker-k8s-ops-system
```

### 3. Verify Deployment

```bash
# Check pod status
kubectl get pods -n netmaker-k8s-ops-system

# Check netclient logs
kubectl logs -n netmaker-k8s-ops-system -l control-plane=controller-manager -c netclient

# Check manager logs
kubectl logs -n netmaker-k8s-ops-system -l control-plane=controller-manager -c manager

# Test proxy endpoints
kubectl port-forward -n netmaker-k8s-ops-system svc/netmaker-k8s-ops-controller-manager-proxy-service 8085:8085
curl http://localhost:8085/health
curl http://localhost:8085/netclient/status
```

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `NETCLIENT_TOKEN` | - | Netmaker token for WireGuard connection |
| `PROXY_PORT` | `8085` | Port for the proxy server |
| `HEALTH_PROBE_PORT` | `8081` | Port for health probes |
| `ENABLE_LEADER_ELECTION` | `false` | Single replica, no leader election needed |

### Ports

- **8081**: Health and readiness probes
- **8085**: Proxy server (Kubernetes API proxy)

### Security Context

The netclient sidecar runs with:
- `privileged: true` (required for WireGuard)
- `hostNetwork: true` (required for WireGuard interfaces)
- Capabilities: `NET_ADMIN`, `SYS_MODULE`

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Kubernetes Pod                           │
│  ┌─────────────────┐  ┌─────────────────────────────────┐  │
│  │   Manager       │  │        Netclient Sidecar        │  │
│  │   (Controller)  │  │     (WireGuard Client)         │  │
│  │                 │  │                                 │  │
│  │  ┌───────────┐  │  │  ┌─────────────────────────┐   │  │
│  │  │   Proxy   │  │  │  │    WireGuard Interface  │   │  │
│  │  │  :8085    │  │  │  │         (wg0)           │   │  │
│  │  └───────────┘  │  │  └─────────────────────────┘   │  │
│  └─────────────────┘  └─────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
           │                           │
           │                           │
           ▼                           ▼
    Client Requests              WireGuard Tunnel
           │                           │
           │                           ▼
           │                    Netmaker Server
           │                           │
           │                           ▼
           │                    Kubernetes API Server
           │                           │
           └───────────────────────────┘
```

## Troubleshooting

### Common Issues

1. **Pod Not Starting**
   ```bash
   kubectl describe pod -n netmaker-k8s-ops-system -l control-plane=controller-manager
   ```

2. **WireGuard Connection Failed**
   ```bash
   # Check netclient logs
   kubectl logs -n netmaker-k8s-ops-system -l control-plane=controller-manager -c netclient
   
   # Check WireGuard interface
   kubectl exec -n netmaker-k8s-ops-system -l control-plane=controller-manager -c netclient -- ip link show
   ```

3. **Proxy Not Accessible**
   ```bash
   # Check proxy logs
   kubectl logs -n netmaker-k8s-ops-system -l control-plane=controller-manager -c manager | grep proxy
   
   # Test proxy directly
   kubectl port-forward -n netmaker-k8s-ops-system svc/netmaker-k8s-ops-controller-manager-proxy-service 8085:8085
   curl http://localhost:8085/health
   ```

### Debug Commands

```bash
# Check all pods
kubectl get pods -n netmaker-k8s-ops-system -o wide

# Check services
kubectl get svc -n netmaker-k8s-ops-system

# Check logs from both containers
kubectl logs -n netmaker-k8s-ops-system -l control-plane=controller-manager --all-containers=true

# Check WireGuard status
kubectl exec -n netmaker-k8s-ops-system -l control-plane=controller-manager -c netclient -- netclient status

# Check network interfaces
kubectl exec -n netmaker-k8s-ops-system -l control-plane=controller-manager -c netclient -- ip addr show
```

## Using the Proxy

Once deployed, you can use the proxy to access your Kubernetes API:

```bash
# Port forward to access the proxy
kubectl port-forward -n netmaker-k8s-ops-system svc/netmaker-k8s-ops-controller-manager-proxy-service 8085:8085

# Test API access
curl -H "Authorization: Bearer YOUR_TOKEN" http://localhost:8085/api/v1/namespaces
curl http://localhost:8085/version
curl http://localhost:8085/metrics
```

## Security Considerations

1. **Privileged Container**: The netclient runs with elevated privileges
2. **Host Network**: The pod shares the host's network namespace
3. **Token Security**: Store the Netmaker token securely (consider using Kubernetes secrets)

## Next Steps

1. Set your Netmaker token in the deployment
2. Deploy the operator
3. Verify the WireGuard connection
4. Test the proxy functionality
5. Configure your kubeconfig to use the proxy

For more detailed information, see:
- [WIREGUARD_SETUP.md](WIREGUARD_SETUP.md) - Detailed WireGuard setup
- [PROXY_USAGE.md](PROXY_USAGE.md) - Proxy usage guide
