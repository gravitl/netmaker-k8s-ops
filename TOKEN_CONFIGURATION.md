# Netmaker Token Configuration Guide

This guide explains all the ways to pass the Netmaker network join token to the operator and netclient sidecars.

## Overview

The Netmaker token is required for:
1. **Operator Manager**: The netclient sidecar in the operator pod
2. **Webhook-Injected Sidecars**: Netclient sidecars injected by the mutating webhook into your application pods
3. **Egress Proxy Pods**: Netclient sidecars in egress proxy pods created by the egress proxy controller
4. **Ingress Proxy Pods**: Netclient sidecars in ingress proxy pods created by the ingress proxy controller

## Method 1: Direct Environment Variable (Operator Manager Only)

For the operator manager's netclient sidecar, you can set the token directly as an environment variable in `config/manager/manager.yaml`:

```yaml
# In config/manager/manager.yaml, find the netclient container section
containers:
- name: netclient
  image: gravitl/netclient:v1.1.0
  env:
  - name: TOKEN
    value: "eyJzZXJ2ZXIiOiJhcGkubm0uMTg4LTE2Ni0xODEtMjA1Lm5pcC5pbyIsInZhbHVlIjoiR0NRU0pJRUhaNFJVT1pQQk1JWjRZNk9WN0JLV0NYT1MifQ=="  # Your Netmaker token
```

**Pros**: Simple, direct configuration  
**Cons**: Token is stored in plain text in the YAML file (not recommended for production)

## Method 2: Kubernetes Secret (Recommended)

### For Operator Manager

1. **Create a Secret** with your Netmaker token:

```bash
kubectl create secret generic netclient-token \
  --from-literal=token="your-netmaker-token-here" \
  --namespace=netmaker-k8s-ops-system
```

2. **Update `config/manager/manager.yaml`** to use the secret:

```yaml
containers:
- name: netclient
  image: gravitl/netclient:v1.1.0
  env:
  - name: TOKEN
    valueFrom:
      secretKeyRef:
        name: netclient-token
        key: token
```

### For Webhook-Injected Sidecars

The webhook automatically reads tokens from Kubernetes secrets. This is the **recommended method** for all sidecars.

### For Egress/Ingress Proxy Pods

The egress and ingress proxy controllers automatically read tokens from Kubernetes secrets. This is the **recommended method** for proxy pods.

**Important**: 
- **Secrets can ONLY be read from the operator namespace** (`netmaker-k8s-ops-system` by default) for security
- The controllers will check in this order:
  1. **Service annotations** (highest priority) - `netmaker.io/secret-name` and `netmaker.io/secret-key`
  2. **Environment variables** - `NETCLIENT_SECRET_NAME`, `NETCLIENT_SECRET_KEY` (defaults: `netclient-token`, `token`)
  3. **Default secret** - `netclient-token` in the operator namespace

**Note**: 
- Secrets are required - if the secret is not found, the proxy pod will fail to start
- The `netmaker.io/secret-namespace` annotation is ignored - secrets are always read from the operator namespace
- Default secret name is `netclient-token` in operator namespace

#### Method 1: Service Annotations (Per-Service Configuration)

You can specify a custom secret name/key per Service using annotations. **Note**: Secrets are always read from the operator namespace for security.

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-egress-service
  annotations:
    netmaker.io/egress: "enabled"
    netmaker.io/egress-target-dns: "api.netmaker.internal"
    # Custom secret configuration (secret must be in operator namespace)
    netmaker.io/secret-name: "custom-netclient-token"      # Secret name (default: netclient-token)
    netmaker.io/secret-key: "token"                        # Key in secret (default: token)
spec:
  ports:
  - port: 80
    targetPort: 8080
```

**Use cases**:
- Different services connecting to different Netmaker networks (using different secret names)
- Per-service token management
- **Important**: All secrets must be created in the operator namespace (`netmaker-k8s-ops-system`)

#### Method 2: Default Secret (Operator Namespace)

Create the default secret in the operator namespace:

```bash
# Create secret in the operator namespace (required)
kubectl create secret generic netclient-token \
  --from-literal=token="your-netmaker-token-here" \
  --namespace=netmaker-k8s-ops-system
```

The controllers will automatically detect and use this secret when creating proxy pods. This is the simplest approach if all services use the same Netmaker network.

#### Method 3: Environment Variables (Global Default)

Set environment variables in the operator deployment to configure default secret name/key:

```yaml
# In config/manager/manager.yaml
env:
- name: NETCLIENT_SECRET_NAME
  value: "netclient-token"  # Default secret name
- name: NETCLIENT_SECRET_KEY
  value: "token"            # Default secret key
```

**Priority Order Summary**:
1. Service annotations (`netmaker.io/secret-name`, `netmaker.io/secret-key`) - **highest priority**
2. Environment variables (`NETCLIENT_SECRET_NAME`, `NETCLIENT_SECRET_KEY`)
3. Default values (`netclient-token`, `token`)

**Security Restriction**:
- **All secrets MUST be in the operator namespace** (`netmaker-k8s-ops-system` by default)
- The `netmaker.io/secret-namespace` annotation is ignored for security
- Secrets are required - if the secret is not found, the proxy pod will fail to start

1. **Create a Secret** in each namespace where you want to use netclient:

```bash
# Create secret in default namespace
kubectl create secret generic netclient-token \
  --from-literal=token="your-netmaker-token-here" \
  --namespace=default

# Create secret in other namespaces as needed
kubectl create secret generic netclient-token \
  --from-literal=token="your-netmaker-token-here" \
  --namespace=production
```

2. **Configure the webhook** to use the secret (via environment variables in the operator deployment):

```yaml
# In config/manager/manager.yaml, manager container env section
env:
- name: NETCLIENT_SECRET_NAME
  value: "netclient-token"  # Name of the secret
- name: NETCLIENT_SECRET_KEY
  value: "token"            # Key in the secret containing the token
```

3. **The webhook will automatically use this secret** when injecting sidecars.

**Pros**: 
- Secure (token stored in Kubernetes Secret)
- Works for all webhook-injected sidecars
- Can use different secrets per namespace

**Cons**: 
- Requires creating secrets in each namespace

## Method 3: Custom Secret per Pod (Webhook)

You can specify a custom secret name, key, or namespace for individual pods using labels:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  template:
    metadata:
      labels:
        netmaker.io/netclient: "enabled"
        # Optional: Custom secret configuration
        netmaker.io/secret-name: "my-custom-netclient-secret"
        netmaker.io/secret-key: "netmaker-token"
        netmaker.io/secret-namespace: "custom-namespace"
    spec:
      containers:
      - name: app
        image: my-app:latest
```

**Label Options**:
- `netmaker.io/secret-name`: Custom secret name (default: `netclient-token`)
- `netmaker.io/secret-key`: Custom key in secret (default: `token`)
- `netmaker.io/secret-namespace`: Custom namespace for secret (default: pod's namespace)

## Method 4: Environment Variable Fallback (Webhook)

The webhook can also read the token from an environment variable as a fallback, but this is **not recommended** for production:

```yaml
# In config/manager/manager.yaml, manager container env section
env:
- name: NETCLIENT_TOKEN
  value: "your-token-here"  # Fallback if secret not found
```

**Note**: This is only used if the secret lookup fails. Secrets are always preferred.

## Complete Example: Operator Manager with Secret

Here's a complete example for configuring the operator manager with a secret:

### Step 1: Create the Secret

```bash
kubectl create secret generic netclient-token \
  --from-literal=token="eyJzZXJ2ZXIiOiJhcGkubm0uMTg4LTE2Ni0xODEtMjA1Lm5pcC5pbyIsInZhbHVlIjoiR0NRU0pJRUhaNFJVT1pQQk1JWjRZNk9WN0JLV0NYT1MifQ==" \
  --namespace=netmaker-k8s-ops-system
```

### Step 2: Update manager.yaml

```yaml
# config/manager/manager.yaml
spec:
  template:
    spec:
      containers:
      # Manager container
      - name: manager
        env:
        - name: NETCLIENT_SECRET_NAME
          value: "netclient-token"
        - name: NETCLIENT_SECRET_KEY
          value: "token"
      
      # Netclient sidecar container
      - name: netclient
        image: gravitl/netclient:v1.1.0
        env:
        - name: TOKEN
          valueFrom:
            secretKeyRef:
              name: netclient-token
              key: token
```

### Step 3: Deploy

```bash
make deploy IMG=<your-registry>/netmaker-k8s-ops:tag
```

## Complete Example: Webhook-Injected Sidecars

### Step 1: Create Secret in Target Namespace

```bash
# Enable webhook for namespace
kubectl label namespace default netmaker.io/webhook=enabled

# Create secret with token
kubectl create secret generic netclient-token \
  --from-literal=token="your-netmaker-token-here" \
  --namespace=default
```

### Step 2: Configure Webhook (if using custom secret name)

```yaml
# In config/manager/manager.yaml, manager container env section
env:
- name: NETCLIENT_SECRET_NAME
  value: "netclient-token"  # Default secret name
- name: NETCLIENT_SECRET_KEY
  value: "token"            # Default secret key
```

### Step 3: Create Deployment with Netclient Label

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  template:
    metadata:
      labels:
        netmaker.io/netclient: "enabled"
    spec:
      containers:
      - name: app
        image: my-app:latest
```

The webhook will automatically:
1. Detect the `netmaker.io/netclient: "enabled"` label
2. Read the token from the secret (`netclient-token` in the pod's namespace)
3. Inject the netclient sidecar with the token

## Token Format

The Netmaker token is a base64-encoded JSON string that looks like:

```
eyJzZXJ2ZXIiOiJhcGkubm0uZXhhbXBsZS5jb20iLCJ2YWx1ZSI6IllPVVJfVE9LRU5fSEVSRSJ9
```

When decoded, it contains:
```json
{
  "server": "api.netmaker.example.com",
  "value": "YOUR_TOKEN_HERE"
}
```

## Getting Your Netmaker Token

1. **Log into your Netmaker server**
2. **Navigate to Networks** → Select your network
3. **Go to Access Keys** → Create a new access key
4. **Copy the token** (it will be a long base64-encoded string)

Or use the Netmaker CLI:

```bash
netmaker join -t <your-token>
```

## Security Best Practices

1. **Always use Kubernetes Secrets** instead of plain text in YAML files
2. **Use different secrets per namespace** if you have different networks
3. **Rotate tokens regularly** by updating the secret
4. **Use RBAC** to restrict who can read the secrets
5. **Consider using external secret management** (e.g., Sealed Secrets, Vault) for production

## Troubleshooting

### Token Not Working

1. **Verify the secret exists**:
   ```bash
   kubectl get secret netclient-token -n <namespace>
   ```

2. **Check the secret contents** (base64 decode):
   ```bash
   kubectl get secret netclient-token -n <namespace> -o jsonpath='{.data.token}' | base64 -d
   ```

3. **Check netclient logs**:
   ```bash
   kubectl logs <pod-name> -c netclient
   ```

4. **Verify webhook configuration**:
   ```bash
   kubectl get deployment netmaker-k8s-ops-controller-manager -n netmaker-k8s-ops-system -o yaml | grep NETCLIENT
   ```

### Secret Not Found Errors

If you see errors like "secret not found":
1. Ensure the secret exists in the correct namespace
2. Check the secret name matches `NETCLIENT_SECRET_NAME` (default: `netclient-token`)
3. Verify the secret key matches `NETCLIENT_SECRET_KEY` (default: `token`)
4. Check pod labels if using custom secret configuration

### Multiple Networks

If you need to connect to multiple Netmaker networks:

1. **Create separate secrets** for each network:
   ```bash
   kubectl create secret generic netclient-token-network1 \
     --from-literal=token="token-for-network1" \
     --namespace=default
   
   kubectl create secret generic netclient-token-network2 \
     --from-literal=token="token-for-network2" \
     --namespace=default
   ```

2. **Use pod labels** to specify which secret to use:
   ```yaml
   labels:
     netmaker.io/netclient: "enabled"
     netmaker.io/secret-name: "netclient-token-network1"
   ```

## Summary

| Method | Use Case | Security | Recommended |
|--------|----------|----------|-------------|
| Direct Env Var | Quick testing | Low | ❌ No |
| Kubernetes Secret | Production | High | ✅ Yes |
| Custom Secret Labels | Multi-network | High | ✅ Yes |
| Env Var Fallback | Development | Low | ⚠️ Only as fallback |

**Recommended Approach**: Use Kubernetes Secrets for all token storage, with custom labels for multi-network scenarios.

