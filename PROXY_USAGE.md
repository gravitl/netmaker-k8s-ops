# Netmaker K8s Proxy Usage Guide

## Overview

The Netmaker K8s Proxy is a reverse proxy that allows secure access to Kubernetes API servers through WireGuard tunnels. It works alongside a `netclient` sidecar container that establishes WireGuard connectivity, enabling remote access to Kubernetes clusters.

## Features

- **WireGuard Integration**: Works with netclient sidecar container for WireGuard connectivity
- **Smart Binding**: Automatically binds to WireGuard interface IP for secure tunnel access
- **Network Isolation**: Ensures all traffic flows through the WireGuard tunnel
- **Dual Authentication Modes**: Auth mode (impersonation) and NoAuth mode (passthrough)
- **User Impersonation**: Auth mode impersonates WireGuard peers for granular RBAC
- **Dynamic User Mapping**: Server-managed IP-to-user/group mapping for flexible access control
- **External API Integration**: Automatic sync with external APIs for centralized user management
- **Multi-Authentication Support**: Bearer tokens, client certificates, basic auth
- **WireGuard Compatible**: Works with WireGuard IP addresses in kubeconfig
- **Health Checks**: Built-in health and readiness endpoints
- **Netclient Status Monitoring**: Real-time status of WireGuard connections
- **CORS Support**: Cross-origin resource sharing for web clients
- **Comprehensive Logging**: Detailed request logging and authentication tracking
- **Configurable**: Environment-based configuration
- **Process Monitoring**: Automatic restart of failed netclient processes

## WireGuard Binding

The proxy automatically detects and binds to the WireGuard interface IP address. This ensures:

1. **Secure Communication**: All traffic flows through the WireGuard tunnel
2. **Network Isolation**: The proxy is only accessible via the WireGuard network
3. **Automatic Detection**: No manual configuration needed

### How It Works

1. **Netclient Sidecar**: Creates WireGuard interface (`wg0`, `netmaker`, etc.) with assigned IP
2. **Interface Detection**: Proxy scans for WireGuard interfaces created by the sidecar
3. **IP Extraction**: Uses `ip addr show` to get the interface IP address
4. **Smart Binding**: Binds the proxy to `WIREGUARD_IP:8085` instead of `:8085`
5. **Fallback**: If no WireGuard interface is found, binds to all interfaces

### Manual Override

You can override the binding IP using the `PROXY_BIND_IP` environment variable:

```bash
export PROXY_BIND_IP=10.0.0.1  # Bind to specific IP
export PROXY_BIND_IP=""        # Bind to all interfaces
```

## Authentication Modes

The proxy supports two distinct authentication modes to handle different security requirements:

### Auth Mode (Default)

In **auth mode**, requests from the WireGuard network are impersonated using the sender's WireGuard peer identity. This enables granular RBAC control based on WireGuard peer identities.

**How it works:**
1. **WireGuard Peer Detection**: Proxy identifies the source WireGuard peer
2. **User Impersonation**: Requests are impersonated as a specific user (default: `wireguard-peer`)
3. **Group Assignment**: Requests are assigned to specific groups (default: `system:authenticated`, `wireguard-peers`)
4. **RBAC Enforcement**: Kubernetes RBAC policies can be configured for these identities

**Configuration:**
```bash
export PROXY_MODE="auth"
export PROXY_IMPERSONATE_USER="wireguard-peer"
export PROXY_IMPERSONATE_GROUPS="system:authenticated,wireguard-peers"
```

**Use cases:**
- Multi-tenant environments where different WireGuard peers need different permissions
- Security-sensitive deployments requiring identity-based access control
- Compliance requirements for audit trails and user attribution

### NoAuth Mode

In **noauth mode**, requests from the WireGuard network are proxied to the Kubernetes API server without authentication. This mode can be combined with external authentication mechanisms.

**How it works:**
1. **Passthrough**: Requests are forwarded directly to the Kubernetes API server
2. **No Impersonation**: No user impersonation headers are added
3. **External Auth**: Relies on external authentication (IDP, cloud provider, etc.)
4. **Simple Proxy**: Acts as a basic reverse proxy

**Configuration:**
```bash
export PROXY_MODE="noauth"
```

**Use cases:**
- Integration with external identity providers (OIDC, LDAP, etc.)
- Cloud provider authentication (AWS IAM, Azure AD, GCP IAM)
- Simple proxy scenarios where authentication is handled upstream
- Development and testing environments

### Mode Comparison

| Feature | Auth Mode | NoAuth Mode |
|---------|-----------|-------------|
| **User Impersonation** | ✅ Yes | ❌ No |
| **RBAC Integration** | ✅ Full | ❌ None |
| **External Auth** | ❌ Not needed | ✅ Required |
| **Audit Trail** | ✅ User attribution | ❌ Limited |
| **Security** | ✅ High | ⚠️ Depends on external |
| **Complexity** | ⚠️ Medium | ✅ Low |
| **Use Case** | Production, Multi-tenant | Simple proxy, External auth |

## User IP Mapping (Auth Mode)

In auth mode, the proxy supports dynamic user IP mapping, allowing different WireGuard peers to be impersonated as different Kubernetes users with different group memberships.

### How It Works

1. **Server Management**: User mappings are managed through admin API endpoints
2. **Dynamic Lookup**: For each request, the proxy looks up the client IP in the mapping
3. **Impersonation**: If a mapping exists, uses the mapped user/groups; otherwise falls back to defaults
4. **RBAC Integration**: Kubernetes RBAC policies can be configured for the mapped users/groups

### Admin API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/admin/user-mappings` | GET | Get all current mappings |
| `/admin/user-mappings` | POST | Add a new mapping |
| `/admin/user-mappings/{ip}` | DELETE | Remove a mapping |

### Example Usage

```bash
# Add a mapping for Alice (IP 10.0.0.1) with developer access
curl -X POST http://localhost:8085/admin/user-mappings \
  -H "Content-Type: application/json" \
  -d '{
    "ip": "10.0.0.1",
    "user": "alice",
    "groups": ["system:authenticated", "developers"]
  }'

# Add a mapping for Bob (IP 10.0.0.2) with admin access
curl -X POST http://localhost:8085/admin/user-mappings \
  -H "Content-Type: application/json" \
  -d '{
    "ip": "10.0.0.2", 
    "user": "bob",
    "groups": ["system:authenticated", "admins"]
  }'

# View all mappings
curl http://localhost:8085/admin/user-mappings

# Remove a mapping
curl -X DELETE http://localhost:8085/admin/user-mappings/10.0.0.1
```

### Integration Examples

#### Netmaker Integration
```bash
#!/bin/bash
# Sync Netmaker peer IPs with user mappings
PEERS=$(curl -s "https://your-netmaker-server/api/v1/networks/your-network/peers" \
  -H "Authorization: Bearer $NETMAKER_TOKEN")

echo "$PEERS" | jq -r '.[] | @base64' | while read peer; do
  PEER_DATA=$(echo "$peer" | base64 -d)
  IP=$(echo "$PEER_DATA" | jq -r '.address')
  USER=$(echo "$PEER_DATA" | jq -r '.name')
  
  curl -X POST http://localhost:8085/admin/user-mappings \
    -H "Content-Type: application/json" \
    -d "{\"ip\": \"$IP\", \"user\": \"$USER\", \"groups\": [\"system:authenticated\", \"wireguard-peers\"]}"
done
```

For detailed API documentation, see [USER_IP_MAPPING_API.md](USER_IP_MAPPING_API.md).

## External API Integration

The proxy can automatically fetch user mappings from an external API, enabling centralized management of user access control.

### Configuration

Set the following environment variables:

```bash
export EXTERNAL_API_SERVER_DOMAIN="api.example.com"
export EXTERNAL_API_TOKEN="your-api-token"
export EXTERNAL_API_SYNC_INTERVAL="5m"
```

### How It Works

1. **Automatic Sync**: Proxy fetches user mappings from external API on startup and periodically
2. **API Endpoint**: `GET https://{domain}/api/users/network_ip`
3. **Authentication**: Bearer token authentication
4. **Response Format**: JSON with user IP mappings

### Example API Response

```json
{
  "mappings": {
    "10.0.0.1": {
      "user": "alice",
      "groups": ["system:authenticated", "developers"]
    },
    "10.0.0.2": {
      "user": "bob",
      "groups": ["system:authenticated", "admins"]
    }
  }
}
```

### Manual Sync

Trigger a manual sync using the admin API:

```bash
curl -X POST http://localhost:8085/admin/sync-external-api
```

### Integration Examples

#### Netmaker Integration
```bash
# Create API endpoint that serves Netmaker peer data
curl -s "https://your-netmaker-server/api/v1/networks/your-network/peers" \
  -H "Authorization: Bearer $NETMAKER_TOKEN" | \
jq '{
  mappings: [.[] | {key: .address, value: {user: .name, groups: ["system:authenticated", "wireguard-peers"]}}] | from_entries
}'
```

For detailed integration guide, see [EXTERNAL_API_INTEGRATION.md](EXTERNAL_API_INTEGRATION.md).

## Quick Start

### 1. Configure WireGuard (Required)

Before deploying, you need to set up WireGuard connectivity:

1. **Get your Netmaker token** from your Netmaker server
2. **Update the deployment** with your token:

```bash
# Edit config/manager/manager.yaml
# Set your Netmaker token in both places:
# - NETCLIENT_TOKEN environment variable
# - netclient container TOKEN environment variable
```

3. **Deploy the operator** (see step 2 below)

For detailed WireGuard setup instructions, see [WIREGUARD_SETUP.md](WIREGUARD_SETUP.md).

### 2. Build the Operator

```bash
go build -o bin/netmaker-k8s-ops cmd/main.go
```

### 2. Configure Your Kubeconfig

Your kubeconfig should point to the WireGuard IP address where the proxy is running:

```yaml
apiVersion: v1
clusters:
- cluster:
    server: https://10.0.0.1:8085  # WireGuard IP + proxy port
    insecure-skip-tls-verify: true  # For testing
  name: netmaker-cluster
contexts:
- context:
    cluster: netmaker-cluster
    user: netmaker-user
  name: netmaker-context
current-context: netmaker-context
users:
- name: netmaker-user
  user:
    token: your-bearer-token-here
```

### 3. Start the Proxy

```bash
# Using environment variables
export KUBECONFIG=/path/to/your/kubeconfig
export PROXY_PORT=8085  # Optional: customize proxy port
export GIN_MODE=debug

./bin/netmaker-k8s-ops
```

### 4. Test the Proxy

```bash
# Test health endpoint
curl http://localhost:8085/health  # or use service: kubectl port-forward svc/netmaker-k8s-ops-controller-manager-proxy-service 8085:8085

# Test readiness endpoint
curl http://localhost:8085/ready

# Test netclient status endpoint
curl http://localhost:8085/netclient/status

# Test Kubernetes API (requires proper authentication)
curl -H "Authorization: Bearer YOUR_TOKEN" http://localhost:8085/api/v1/namespaces

# Test Kubernetes version
curl http://localhost:8085/version

# Test Kubernetes metrics
curl http://localhost:8085/metrics
```

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `KUBECONFIG` | `~/.kube/config` | Path to kubeconfig file |
| `PROXY_PORT` | `8085` | Port for the proxy server |
| `PROXY_BIND_IP` | Auto-detected | IP address to bind the proxy to (defaults to WireGuard interface IP) |
| `PROXY_MODE` | `auth` | Proxy authentication mode (`auth` or `noauth`) |
| `PROXY_IMPERSONATE_USER` | `wireguard-peer` | Username to impersonate for WireGuard peers (auth mode only) |
| `PROXY_IMPERSONATE_GROUPS` | `system:authenticated,wireguard-peers` | Groups to impersonate for WireGuard peers (auth mode only) |
| `PROXY_SKIP_TLS_VERIFY` | `true` | Skip TLS verification for proxy connections |
| `EXTERNAL_API_SERVER_DOMAIN` | - | External API server domain for user mapping sync |
| `EXTERNAL_API_TOKEN` | - | Bearer token for external API authentication |
| `EXTERNAL_API_SYNC_INTERVAL` | `5m` | How often to sync with external API |
| `GIN_MODE` | `debug` | Gin framework mode (debug/release) |
| `IN_CLUSTER` | `false` | Use in-cluster configuration |

### Netclient Sidecar Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `NETCLIENT_TOKEN` | - | Netmaker client token (required for sidecar) |
| `NETCLIENT_SERVER` | - | Netmaker server URL |
| `NETCLIENT_NETWORK` | - | Netmaker network name |
| `NETCLIENT_CONFIG_DIR` | `/etc/netclient` | Directory for netclient configuration |
| `NETCLIENT_LOG_DIR` | `/var/log/netclient` | Directory for netclient logs |
| `NETCLIENT_DISABLED` | `false` | Set to `true` to disable netclient sidecar |

### Authentication Methods

The proxy supports multiple authentication methods:

1. **Bearer Token**: Set in kubeconfig or passed via Authorization header
2. **Client Certificates**: Configured in kubeconfig
3. **Basic Authentication**: Username/password in kubeconfig
4. **Existing Headers**: Passes through existing Authorization headers

## WireGuard Integration

### Setting Up WireGuard Access

1. **Configure Netmaker**: Set up your Netmaker network with remote access gateway
2. **Get WireGuard Config**: Download the WireGuard configuration from Netmaker
3. **Connect to WireGuard**: Import the config into your WireGuard client
4. **Update Kubeconfig**: Point your kubeconfig to the WireGuard IP address

### Example WireGuard Setup

```bash
# Install WireGuard (Ubuntu/Debian)
sudo apt install wireguard

# Import Netmaker config
sudo wg-quick up netmaker-config.conf

# Verify connection
ip addr show wg0
```

## API Endpoints

The proxy provides several endpoints for monitoring and health checks:

### Health Endpoints

- **`GET /health`** - Basic health check
  ```json
  {
    "status": "healthy",
    "proxy": "k8s-api-proxy"
  }
  ```

- **`GET /ready`** - Readiness check
  ```json
  {
    "status": "ready",
    "proxy": "k8s-api-proxy"
  }
  ```

### Netclient Status Endpoint

- **`GET /netclient/status`** - Checks for WireGuard interfaces (indicates netclient is running)
  
  When WireGuard interface is detected:
  ```json
  {
    "status": "netclient_status",
    "data": {
      "running": true,
      "type": "wireguard_interface_check",
      "interface": "wg0",
      "message": "WireGuard interface wg0 detected"
    }
  }
  ```
  
  When no WireGuard interface is found:
  ```json
  {
    "status": "netclient_status",
    "data": {
      "running": false,
      "type": "wireguard_interface_check",
      "message": "Checking for WireGuard interfaces"
    }
  }
  ```
  
  **Note**: This endpoint only checks for WireGuard network interfaces since the official netclient doesn't provide a status API.

### Kubernetes API Proxy

The proxy handles specific Kubernetes API endpoints:

- **`ANY /api/*path`** - Proxies requests to Kubernetes core API
- **`ANY /apis/*path`** - Proxies requests to Kubernetes extension APIs
- **`ANY /version`** - Proxies requests to Kubernetes version endpoint
- **`ANY /metrics`** - Proxies requests to Kubernetes metrics endpoint

## Testing

### Run the Test Script

```bash
# Test without netclient
./test-proxy.sh

# Test with netclient (requires token)
NETCLIENT_TOKEN=your-token ./test-proxy.sh
```

### Manual Testing

```bash
# Start proxy
./bin/netmaker-k8s-ops &

# Test endpoints
curl http://localhost:8085/health  # or use service: kubectl port-forward svc/netmaker-k8s-ops-controller-manager-proxy-service 8085:8085
curl http://localhost:8085/ready

# Test with kubectl
kubectl --kubeconfig=your-wireguard-kubeconfig get nodes
```

## Troubleshooting

### Common Issues

1. **Authentication Failed**
   - Check your kubeconfig has valid credentials
   - Verify the Bearer token is not expired
   - Ensure client certificates are valid

2. **Connection Refused**
   - Verify the proxy is running on the correct port
   - Check firewall rules allow traffic on the proxy port
   - Ensure WireGuard connection is active

3. **TLS Errors**
   - For testing, use `insecure-skip-tls-verify: true` in kubeconfig
   - For production, configure proper TLS certificates

### Debug Mode

Enable debug logging:

```bash
export GIN_MODE=debug
./bin/netmaker-k8s-ops
```

### Logs

The proxy logs all requests with:
- Request method and path
- Client IP address
- User agent
- Authentication method used

## Security Considerations

1. **TLS**: Use proper TLS certificates in production
2. **Authentication**: Always use strong authentication methods
3. **Network**: Ensure WireGuard network is properly secured
4. **Access Control**: Implement proper RBAC in Kubernetes
5. **Monitoring**: Monitor proxy logs for suspicious activity

## Production Deployment

### Kubernetes Deployment with Netclient Sidecar

The recommended approach is to deploy the proxy with the official Netmaker client as a sidecar container:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: netmaker-k8s-proxy
spec:
  replicas: 1
  selector:
    matchLabels:
      app: netmaker-k8s-proxy
  template:
    metadata:
      labels:
        app: netmaker-k8s-proxy
    spec:
      hostNetwork: true  # Required for WireGuard
      containers:
      # Netmaker K8s Proxy
      - name: netmaker-k8s-ops
        image: netmaker-k8s-ops:latest
        ports:
        - containerPort: 8085
        env:
        - name: PROXY_PORT
          value: "8085"
        - name: NETCLIENT_DISABLED
          value: "true"  # Disable built-in sidecar
      # Official Netmaker Client
      - name: netclient
        image: gravitl/netclient:v1.1.0
        env:
        - name: TOKEN
          valueFrom:
            secretKeyRef:
              name: netclient-secret
              key: NETCLIENT_TOKEN
        - name: DAEMON
          value: "on"
        securityContext:
          privileged: true
          capabilities:
            add:
            - NET_ADMIN
            - SYS_MODULE
```

### Docker Deployment (Standalone)

For standalone deployment without Kubernetes:

```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o netmaker-k8s-ops cmd/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/netmaker-k8s-ops .
CMD ["./netmaker-k8s-ops"]
```

### Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: netmaker-k8s-proxy
spec:
  replicas: 1
  selector:
    matchLabels:
      app: netmaker-k8s-proxy
  template:
    metadata:
      labels:
        app: netmaker-k8s-proxy
    spec:
      containers:
      - name: proxy
        image: netmaker-k8s-ops:latest
        ports:
        - containerPort: 8085
        env:
        - name: PROXY_PORT
          value: "8085"
        - name: GIN_MODE
          value: "release"
```

## Next Steps

1. **Deploy to Kubernetes**: Use the provided deployment manifests
2. **Configure Monitoring**: Set up Prometheus metrics and logging
3. **Implement Security**: Add authentication and authorization layers
4. **Scale**: Configure multiple proxy instances for high availability
