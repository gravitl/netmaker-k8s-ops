# Netmaker K8s Proxy Usage Guide

## Overview

The Netmaker K8s Proxy is a secure reverse proxy that enables remote access to Kubernetes API servers through WireGuard tunnels. It provides a secure way to access your Kubernetes cluster from devices on your Netmaker network without exposing the Kubernetes API server directly to the internet.

## Features

- **WireGuard Integration**: Works seamlessly with Netmaker WireGuard networks
- **Dual Authentication Modes**: Auth mode (with RBAC) and NoAuth mode (for external auth)
- **Automatic Mode Selection**: Automatically switches to noauth mode if server is not pro
- **Smart Binding**: Automatically binds to WireGuard interface IP for secure tunnel access
- **User Impersonation**: Auth mode supports user impersonation with granular RBAC control
- **Dynamic User Mapping**: Server-managed IP-to-user/group mapping (Pro feature)
- **External API Integration**: Automatic sync with external APIs for centralized user management
- **Health Checks**: Built-in health and readiness endpoints
- **Netclient Status Monitoring**: Real-time status of WireGuard connections

## How It Works

1. **Netclient Sidecar**: Establishes WireGuard connection to Netmaker network
2. **Proxy Server**: Starts on WireGuard interface IP (or all interfaces if not found)
3. **Request Forwarding**: Proxies Kubernetes API requests through WireGuard tunnel
4. **Authentication**: Applies authentication based on configured mode (auth/noauth)

### Architecture

```
Netmaker Device → WireGuard Tunnel → Proxy Server → Kubernetes API Server
```

## Installation

### Helm Installation

#### Enable Proxy Mode

**Auth Mode (requires Netmaker Pro server):**
```bash
helm install netmaker-k8s-ops netmaker-k8s-ops/netmaker-k8s-ops \
  --namespace netmaker-k8s-ops-system \
  --create-namespace \
  --set image.repository=gravitl/netmaker-k8s-ops \
  --set image.tag=latest \
  --set netclient.token="YOUR_NETMAKER_TOKEN_HERE" \
  --set manager.configMap.proxyMode="auth" \
  --set service.proxy.enabled=true \
  --set api.enabled=true \
  --set api.serverDomain="api.example.com" \
  --set api.token="your-api-token-here" \
  --set api.syncInterval="10"
```

**NoAuth Mode:**
```bash
helm install netmaker-k8s-ops netmaker-k8s-ops/netmaker-k8s-ops \
  --namespace netmaker-k8s-ops-system \
  --create-namespace \
  --set image.repository=gravitl/netmaker-k8s-ops \
  --set image.tag=latest \
  --set netclient.token="YOUR_NETMAKER_TOKEN_HERE" \
  --set manager.configMap.proxyMode="noauth" \
  --set service.proxy.enabled=true
```

### Automatic Mode Selection

**Important**: If you set `PROXY_MODE="auth"` but your Netmaker server is not a Pro server, the operator will automatically switch to `"noauth"` mode with a warning log message. This ensures the proxy continues to function even without Pro features.

**Behavior:**
- `PROXY_MODE="auth"` + Pro server → Runs in auth mode
- `PROXY_MODE="auth"` + Non-Pro server → Automatically switches to noauth mode
- `PROXY_MODE="noauth"` → Always runs in noauth mode (regardless of server type)
- Connection error checking Pro status → Disables proxy mode (if auth was requested)

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PROXY_MODE` | - | Proxy authentication mode: `"auth"` or `"noauth"` |
| `PROXY_PORT` | `8085` | Port for the proxy server |
| `PROXY_BIND_IP` | Auto-detected | IP address to bind the proxy to (defaults to WireGuard interface IP) |
| `PROXY_IMPERSONATE_USER` | `wireguard-peer` | Username to impersonate for WireGuard peers (auth mode only) |
| `PROXY_IMPERSONATE_GROUPS` | `system:authenticated,wireguard-peers` | Groups to impersonate (auth mode only) |
| `PROXY_SKIP_TLS_VERIFY` | `true` | Skip TLS verification for proxy connections |
| `SERVER_HOST` | - | Netmaker server hostname (for pro status check) |
| `API_SERVER_DOMAIN` | - | External API server domain for user mapping sync |
| `API_TOKEN` | - | Bearer token for external API authentication |
| `API_SYNC_INTERVAL` | `300` | How often to sync with external API (seconds) |

### Helm Values Configuration

```yaml
manager:
  configMap:
    proxyMode: "auth"  # or "noauth"
    proxySkipTLSVerify: "true"
    
service:
  proxy:
    enabled: true
    port: 8085
    type: ClusterIP
```

## Authentication Modes

### Auth Mode

**Requires**: Netmaker Pro server

In auth mode, requests from WireGuard peers are impersonated using configured user and group identities, enabling granular RBAC control.

**Features:**
- User impersonation with RBAC support
- Dynamic user IP mapping (Pro feature)
- External API integration for user management
- Audit trails and user attribution

**Configuration:**
```bash
--set manager.configMap.proxyMode="auth" \
--set manager.configMap.proxySkipTLSVerify="true"
```

**RBAC Setup Required:**
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: wireguard-peer
rules:
- apiGroups: [""]
  resources: ["pods", "services"]
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: wireguard-peer
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: wireguard-peer
subjects:
- kind: User
  name: wireguard-peer
  apiGroup: rbac.authorization.k8s.io
```

### NoAuth Mode

**Works with**: Any Netmaker server (Pro or Community)

In noauth mode, requests are forwarded directly to the Kubernetes API server without impersonation. Authentication is handled by external mechanisms (OIDC, cloud provider auth, etc.).

**Features:**
- Simple passthrough proxy
- External authentication integration
- No RBAC configuration needed
- Works with any Netmaker server

**Configuration:**
```bash
--set manager.configMap.proxyMode="noauth" \
--set service.proxy.enabled=true
```

**Use Cases:**
- Integration with external identity providers
- Cloud provider authentication (AWS IAM, Azure AD, GCP IAM)
- Simple proxy scenarios
- Development and testing environments

## Usage

### Accessing the Proxy

The proxy is accessible via the WireGuard network IP or through port-forwarding:

**Via WireGuard Network:**
```bash
# From a device on the Netmaker network
curl https://<wireguard-ip>:8085/api/v1/namespaces
```

**Via Port Forward:**
```bash
# Port forward the proxy service
kubectl port-forward -n netmaker-k8s-ops-system \
  svc/netmaker-k8s-ops-controller-manager-proxy-service 8085:8085

# Access locally
curl http://localhost:8085/api/v1/namespaces
```

### Using with kubectl

**Configure kubectl to use the proxy:**

```bash
# Get the proxy service IP
PROXY_IP=$(kubectl get svc -n netmaker-k8s-ops-system \
  netmaker-k8s-ops-controller-manager-proxy-service \
  -o jsonpath='{.spec.clusterIP}')

# Or use WireGuard IP if accessible
PROXY_IP="<wireguard-ip>"

# Configure kubectl
kubectl config set-cluster my-cluster \
  --server=https://${PROXY_IP}:8085 \
  --insecure-skip-tls-verify=true
```

### Health Endpoints

**Health Check:**
```bash
curl http://localhost:8085/health
```

**Readiness Check:**
```bash
curl http://localhost:8085/ready
```

**Netclient Status:**
```bash
curl http://localhost:8085/netclient/status
```

**Kubernetes Version:**
```bash
curl http://localhost:8085/version
```

## Server Pro Status Check

The operator automatically checks if the Netmaker server is a Pro server when proxy mode is enabled:

### Behavior

1. **Pro Server + Auth Mode**: Continues with auth mode
2. **Non-Pro Server + Auth Mode**: Automatically switches to noauth mode with warning
3. **Connection Error**: Disables proxy mode (if auth was requested)
4. **NoAuth Mode**: No check performed, works with any server

### Log Messages

**Pro server detected:**
```
Server is a pro server (is_pro: true). Auth mode requires Netmaker Pro server. Continuing with auth mode.
```

**Non-Pro server detected:**
```
Server is not a pro server (is_pro: false). Switching to auth mode.
Proxy mode set to auth. Operator will continue with proxy in auth mode.
```

**Connection error:**
```
Server pro status check failed, disabling proxy mode
Proxy mode has been disabled due to server pro status check failure
```

## Advanced Configuration

### Custom Binding IP

```bash
--set manager.configMap.proxyBindIP="10.0.0.1"
```

Or via environment variable:
```yaml
env:
- name: PROXY_BIND_IP
  value: "10.0.0.1"
```

### Custom Port

```bash
--set service.proxy.port=9090
```

Or via environment variable:
```yaml
env:
- name: PROXY_PORT
  value: "9090"
```

### External API Integration (Pro Feature)

```bash
--set api.enabled=true \
--set api.serverDomain="api.example.com" \
--set api.token="your-api-token" \
--set api.syncInterval="300"
```

## Troubleshooting

### Proxy Not Starting

**Check logs:**
```bash
kubectl logs -n netmaker-k8s-ops-system \
  -l control-plane=controller-manager -c manager
```

**Check if proxy mode is enabled:**
```bash
kubectl get configmap -n netmaker-k8s-ops-system \
  -o jsonpath='{.data.PROXY_MODE}'
```

### Connection Issues

**Verify WireGuard interface:**
```bash
kubectl exec -n netmaker-k8s-ops-system \
  -l control-plane=controller-manager -c netclient -- ip addr show
```

**Check proxy service:**
```bash
kubectl get svc -n netmaker-k8s-ops-system \
  netmaker-k8s-ops-controller-manager-proxy-service
```

### Authentication Issues (Auth Mode)

**Check RBAC:**
```bash
kubectl auth can-i get pods --as=wireguard-peer
kubectl get clusterrolebindings | grep wireguard
```

**Verify impersonation:**
```bash
kubectl logs -n netmaker-k8s-ops-system \
  -l control-plane=controller-manager -c manager | grep impersonate
```

### Server Pro Status Check Issues

**Check server host configuration:**
```bash
kubectl get configmap -n netmaker-k8s-ops-system \
  -o jsonpath='{.data.SERVER_HOST}'
```

**Verify server accessibility:**
```bash
kubectl exec -n netmaker-k8s-ops-system \
  -l control-plane=controller-manager -c manager -- \
  curl -k https://${SERVER_HOST}/api/server/status
```

## Best Practices

1. **Use Secrets for Tokens**: Store Netmaker tokens in Kubernetes secrets
2. **Enable RBAC**: Configure appropriate RBAC for auth mode
3. **Monitor Logs**: Regularly check proxy logs for issues
4. **Network Security**: Ensure WireGuard network is properly secured
5. **Health Monitoring**: Use health endpoints for monitoring
6. **Mode Selection**: Use noauth mode if you don't need Pro features

## Related Documentation

- [Proxy Authentication Modes](PROXY_AUTH_MODES.md) - Detailed auth mode configuration
- [User Guide](docs/USER_GUIDE.md) - General operator usage
- [Token Configuration](TOKEN_CONFIGURATION.md) - Token management
- [Egress Proxy Guide](examples/EGRESS_PROXY_GUIDE.md) - Egress proxy usage
- [Ingress Proxy Guide](examples/INGRESS_PROXY_GUIDE.md) - Ingress proxy usage

## Examples

### Complete Setup with Auth Mode

```bash
# 1. Install with auth mode
helm install netmaker-k8s-ops netmaker-k8s-ops/netmaker-k8s-ops \
  --namespace netmaker-k8s-ops-system \
  --create-namespace \
  --set image.repository=gravitl/netmaker-k8s-ops \
  --set image.tag=latest \
  --set netclient.token="YOUR_NETMAKER_TOKEN" \
  --set manager.configMap.proxyMode="auth" \
  --set service.proxy.enabled=true

# 2. Create RBAC (see examples/rbac-examples.yaml)
kubectl apply -f examples/rbac-examples.yaml

# 3. Test access
kubectl port-forward -n netmaker-k8s-ops-system \
  svc/netmaker-k8s-ops-controller-manager-proxy-service 8085:8085

curl http://localhost:8085/api/v1/namespaces
```

### Complete Setup with NoAuth Mode

```bash
# Install with noauth mode
helm install netmaker-k8s-ops netmaker-k8s-ops/netmaker-k8s-ops \
  --namespace netmaker-k8s-ops-system \
  --create-namespace \
  --set image.repository=gravitl/netmaker-k8s-ops \
  --set image.tag=latest \
  --set netclient.token="YOUR_NETMAKER_TOKEN" \
  --set manager.configMap.proxyMode="noauth" \
  --set service.proxy.enabled=true

# Configure external authentication (OIDC, cloud provider, etc.)
# Then test access
curl http://localhost:8085/api/v1/namespaces
```

## Security Considerations

- **WireGuard Security**: The WireGuard tunnel provides the primary security boundary
- **RBAC**: Configure minimal required permissions in auth mode
- **TLS**: Consider enabling TLS for production deployments
- **Network Isolation**: Ensure proxy is only accessible via WireGuard network
- **Audit Logging**: Enable Kubernetes audit logging for compliance

