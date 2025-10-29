# Proxy Authentication Modes Guide

This guide explains how to configure and use the two authentication modes available in the Netmaker K8s Proxy.

## Overview

The Netmaker K8s Proxy supports two authentication modes:

1. **Auth Mode**: Impersonates WireGuard peers for granular RBAC control
2. **NoAuth Mode**: Proxies requests without authentication (for external auth integration)

## Auth Mode

### How It Works

In auth mode, the proxy:
1. Receives requests from WireGuard peers
2. Impersonates the requests as a specific user (default: `wireguard-peer`)
3. Assigns the requests to specific groups (default: `system:authenticated`, `wireguard-peers`)
4. Forwards the requests to the Kubernetes API server with impersonation headers

### Configuration

Set the following environment variables:

```bash
# Enable auth mode
export PROXY_MODE="auth"

# Configure impersonation user (default: wireguard-peer)
export PROXY_IMPERSONATE_USER="wireguard-peer"

# Configure impersonation groups (comma-separated)
export PROXY_IMPERSONATE_GROUPS="system:authenticated,wireguard-peers"
```

### Kubernetes Configuration

In your Kubernetes deployment:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: netmaker-k8s-ops
spec:
  template:
    spec:
      containers:
      - name: manager
        env:
        - name: PROXY_MODE
          value: "auth"
        - name: PROXY_IMPERSONATE_USER
          value: "wireguard-peer"
        - name: PROXY_IMPERSONATE_GROUPS
          value: "system:authenticated,wireguard-peers"
```

### RBAC Setup

Create RBAC resources for the impersonated user:

```yaml
# ClusterRole for WireGuard peers
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: wireguard-peer
rules:
- apiGroups: [""]
  resources: ["pods", "services", "nodes"]
  verbs: ["get", "list", "watch"]

---
# ClusterRoleBinding for WireGuard peers
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
- kind: Group
  name: wireguard-peers
  apiGroup: rbac.authorization.k8s.io
```

### Advanced Configuration

#### Multiple Users

You can configure different users for different WireGuard peers by using the client IP:

```bash
# Use client IP as username
export PROXY_IMPERSONATE_USER="wireguard-peer-${CLIENT_IP}"

# Or use a mapping service
export PROXY_IMPERSONATE_USER="$(get-user-for-ip ${CLIENT_IP})"
```

#### Dynamic Groups

Configure groups based on WireGuard peer characteristics:

```bash
# Use different groups based on client IP
if [[ "${CLIENT_IP}" =~ ^10\.0\.1\. ]]; then
  export PROXY_IMPERSONATE_GROUPS="system:authenticated,wireguard-peers,admin-group"
else
  export PROXY_IMPERSONATE_GROUPS="system:authenticated,wireguard-peers,user-group"
fi
```

### Use Cases

- **Multi-tenant environments**: Different WireGuard peers get different permissions
- **Security-sensitive deployments**: Identity-based access control
- **Compliance requirements**: Audit trails and user attribution
- **Development teams**: Different access levels for different teams

## NoAuth Mode

### How It Works

In noauth mode, the proxy:
1. Receives requests from WireGuard peers
2. Forwards requests directly to the Kubernetes API server
3. Does not add any impersonation headers
4. Relies on external authentication mechanisms

### Configuration

Set the following environment variable:

```bash
# Enable noauth mode
export PROXY_MODE="noauth"
```

### Kubernetes Configuration

In your Kubernetes deployment:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: netmaker-k8s-ops
spec:
  template:
    spec:
      containers:
      - name: manager
        env:
        - name: PROXY_MODE
          value: "noauth"
```

### External Authentication Integration

#### OIDC Integration

Configure your Kubernetes cluster to use OIDC authentication:

```yaml
# kube-apiserver configuration
apiVersion: v1
kind: ConfigMap
metadata:
  name: kube-apiserver-config
data:
  config.yaml: |
    authentication:
      oidc:
        issuer: https://your-oidc-provider.com
        clientId: your-client-id
        clientSecret: your-client-secret
        usernameClaim: email
        groupsClaim: groups
```

#### AWS IAM Integration

Use AWS IAM for authentication:

```yaml
# kube-apiserver configuration
apiVersion: v1
kind: ConfigMap
metadata:
  name: kube-apiserver-config
data:
  config.yaml: |
    authentication:
      aws:
        clusterID: your-cluster-id
        region: us-west-2
```

#### Azure AD Integration

Use Azure AD for authentication:

```yaml
# kube-apiserver configuration
apiVersion: v1
kind: ConfigMap
metadata:
  name: kube-apiserver-config
data:
  config.yaml: |
    authentication:
      azure:
        tenantId: your-tenant-id
        clientId: your-client-id
        clientSecret: your-client-secret
```

### Use Cases

- **External IDP integration**: OIDC, LDAP, Active Directory
- **Cloud provider authentication**: AWS IAM, Azure AD, GCP IAM
- **Simple proxy scenarios**: Where authentication is handled upstream
- **Development and testing**: Quick setup without complex RBAC

## Mode Selection Guide

### Choose Auth Mode When:

- You need granular RBAC control based on WireGuard peer identity
- You want to implement multi-tenant access control
- You need audit trails and user attribution
- You want to integrate with Kubernetes RBAC directly
- You have security compliance requirements

### Choose NoAuth Mode When:

- You want to integrate with external identity providers
- You need cloud provider authentication
- You want a simple proxy without complex RBAC
- You're in a development or testing environment
- You have existing authentication infrastructure

## Security Considerations

### Auth Mode Security

- **User Impersonation**: Ensure the impersonated user has minimal required permissions
- **Group Management**: Use groups to organize permissions logically
- **Audit Logging**: Enable audit logging to track impersonated requests
- **Network Security**: Ensure WireGuard network is properly secured

### NoAuth Mode Security

- **External Authentication**: Ensure external auth is properly configured
- **Network Security**: WireGuard network provides the primary security boundary
- **Access Control**: Implement access control at the network level
- **Monitoring**: Monitor for unauthorized access attempts

## Troubleshooting

### Auth Mode Issues

#### Permission Denied Errors

```bash
# Check if the impersonated user exists and has proper RBAC
kubectl auth can-i get pods --as=wireguard-peer
kubectl auth can-i get pods --as=system:authenticated

# Check ClusterRoleBindings
kubectl get clusterrolebindings | grep wireguard
```

#### Impersonation Not Working

```bash
# Check proxy logs for impersonation headers
kubectl logs -f deployment/netmaker-k8s-ops | grep "impersonate"

# Verify environment variables
kubectl exec deployment/netmaker-k8s-ops -- env | grep PROXY
```

### NoAuth Mode Issues

#### Authentication Failures

```bash
# Check external authentication configuration
kubectl get configmap kube-apiserver-config -o yaml

# Check authentication logs
kubectl logs -f kube-apiserver-* | grep auth
```

#### Proxy Errors

```bash
# Check proxy logs
kubectl logs -f deployment/netmaker-k8s-ops

# Verify mode configuration
kubectl exec deployment/netmaker-k8s-ops -- env | grep PROXY_MODE
```

## Examples

### Complete Auth Mode Setup

```yaml
# 1. Deploy the proxy in auth mode
apiVersion: apps/v1
kind: Deployment
metadata:
  name: netmaker-k8s-ops
spec:
  template:
    spec:
      containers:
      - name: manager
        env:
        - name: PROXY_MODE
          value: "auth"
        - name: PROXY_IMPERSONATE_USER
          value: "wireguard-peer"
        - name: PROXY_IMPERSONATE_GROUPS
          value: "system:authenticated,wireguard-peers"

---
# 2. Create RBAC for WireGuard peers
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: wireguard-peer
rules:
- apiGroups: [""]
  resources: ["pods", "services", "nodes"]
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
- kind: Group
  name: wireguard-peers
  apiGroup: rbac.authorization.k8s.io
```

### Complete NoAuth Mode Setup

```yaml
# 1. Deploy the proxy in noauth mode
apiVersion: apps/v1
kind: Deployment
metadata:
  name: netmaker-k8s-ops
spec:
  template:
    spec:
      containers:
      - name: manager
        env:
        - name: PROXY_MODE
          value: "noauth"

---
# 2. Configure external authentication (example: OIDC)
apiVersion: v1
kind: ConfigMap
metadata:
  name: kube-apiserver-config
data:
  config.yaml: |
    authentication:
      oidc:
        issuer: https://your-oidc-provider.com
        clientId: your-client-id
        clientSecret: your-client-secret
        usernameClaim: email
        groupsClaim: groups
```

## Migration Between Modes

### From NoAuth to Auth Mode

1. **Update proxy configuration**:
   ```bash
   kubectl set env deployment/netmaker-k8s-ops PROXY_MODE=auth
   kubectl set env deployment/netmaker-k8s-ops PROXY_IMPERSONATE_USER=wireguard-peer
   kubectl set env deployment/netmaker-k8s-ops PROXY_IMPERSONATE_GROUPS=system:authenticated,wireguard-peers
   ```

2. **Create RBAC resources**:
   ```bash
   kubectl apply -f examples/rbac-examples.yaml
   ```

3. **Test the configuration**:
   ```bash
   kubectl auth can-i get pods --as=wireguard-peer
   ```

### From Auth to NoAuth Mode

1. **Update proxy configuration**:
   ```bash
   kubectl set env deployment/netmaker-k8s-ops PROXY_MODE=noauth
   ```

2. **Configure external authentication** (if needed)

3. **Test the configuration**:
   ```bash
   kubectl get pods
   ```

## Best Practices

### Auth Mode Best Practices

- **Principle of Least Privilege**: Grant only the minimum required permissions
- **Group-based Access**: Use groups to organize permissions logically
- **Regular Audits**: Regularly review and audit RBAC configurations
- **Monitoring**: Monitor for privilege escalation attempts
- **Documentation**: Document all RBAC changes and their purposes

### NoAuth Mode Best Practices

- **External Auth Security**: Ensure external authentication is properly secured
- **Network Security**: Use WireGuard network as the primary security boundary
- **Access Monitoring**: Monitor for unauthorized access attempts
- **Regular Updates**: Keep external authentication systems updated
- **Backup Authentication**: Have backup authentication methods available

## Conclusion

The choice between auth and noauth modes depends on your specific requirements:

- **Auth mode** provides granular RBAC control and is ideal for production environments with security requirements
- **NoAuth mode** provides simplicity and is ideal for integration with external authentication systems

Choose the mode that best fits your security requirements, infrastructure, and operational needs.
