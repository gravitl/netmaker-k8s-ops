# Netclient Sidecar Webhook

The Netclient Sidecar Webhook automatically adds a WireGuard netclient sidecar to any pod with the appropriate label, enabling secure connectivity to your Kubernetes cluster through Netmaker.

## How It Works

1. **Label Detection**: The webhook watches for pods with the label `netmaker.io/netclient: enabled`
2. **Namespace Filtering**: Only applies to namespaces with the label `netmaker.io/webhook: enabled`
3. **Sidecar Injection**: Automatically adds a netclient container with proper configuration
4. **Network Setup**: Adds required volumes for WireGuard. Note: `hostNetwork` is not required since containers in a pod share the network namespace.

## Prerequisites

1. **Netmaker Server**: You need a running Netmaker server
2. **Webhook Enabled**: The webhook must be deployed and running
3. **Namespace Labels**: Target namespaces must have the webhook label

## Configuration

### Environment Variables

The webhook uses these environment variables for netclient configuration:

```yaml
env:
- name: NETCLIENT_IMAGE
  value: "gravitl/netclient:v1.1.0"  # Netclient image
- name: NETCLIENT_SERVER
  value: "api.netmaker.example.com" # Optional: Netmaker server
- name: NETCLIENT_NETWORK
  value: "your-network-name"        # Optional: Network name
- name: NETCLIENT_SECRET_NAME
  value: "netclient-token"          # Secret name containing the token
- name: NETCLIENT_SECRET_KEY
  value: "token"                    # Secret key containing the token
```

### Secret Configuration

The webhook reads the netclient join token from a Kubernetes secret. Create a secret with your netclient token:

```bash
# Create the secret
kubectl create secret generic netclient-token \
  --from-literal=token=your-netclient-token \
  --namespace=your-namespace
```

Or apply the example secret:

```bash
# Edit the secret with your token
kubectl apply -f examples/netclient-secret.yaml
```

### Webhook Configuration

The webhook is configured to:
- **Operations**: CREATE, UPDATE
- **Resources**: pods
- **Failure Policy**: Fail (reject if webhook fails)
- **Side Effects**: None

## Usage

### 1. Enable Webhook for Namespace

Add the webhook label to your namespace:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: my-app
  labels:
    netmaker.io/webhook: enabled
```

### 2. Label Your Pods

Add the netclient label to any pod you want to have WireGuard connectivity:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-app
  labels:
    netmaker.io/netclient: enabled
spec:
  containers:
  - name: my-app
    image: my-app:latest
```

#### Custom Secret Configuration

You can specify custom secret configuration using pod labels:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-app
  labels:
    netmaker.io/netclient: enabled
    # Optional: Custom secret configuration
    netmaker.io/secret-name: my-custom-token
    netmaker.io/secret-key: join-token
    netmaker.io/secret-namespace: netmaker-system
spec:
  containers:
  - name: my-app
    image: my-app:latest
```

**Available Labels:**
- `netmaker.io/secret-name`: Secret name (default: `netclient-token`)
- `netmaker.io/secret-key`: Secret key (default: `token`)
- `netmaker.io/secret-namespace`: Secret namespace (default: pod's namespace)

#### Configuration Priority

The webhook uses the following priority for secret configuration:

1. **Pod Labels** (highest priority)
   - `netmaker.io/secret-name`
   - `netmaker.io/secret-key`
   - `netmaker.io/secret-namespace`

2. **Environment Variables** (fallback)
   - `NETCLIENT_SECRET_NAME`
   - `NETCLIENT_SECRET_KEY`
   - Uses pod's namespace

3. **Defaults** (lowest priority)
   - Secret name: `netclient-token`
   - Secret key: `token`
   - Secret namespace: pod's namespace

#### Example Configurations

**Default Configuration:**
```yaml
labels:
  netmaker.io/netclient: enabled
# Uses secret "netclient-token" with key "token" in same namespace
```

**Custom Secret Name:**
```yaml
labels:
  netmaker.io/netclient: enabled
  netmaker.io/secret-name: production-token
# Uses secret "production-token" with key "token" in same namespace
```

**Custom Secret Key:**
```yaml
labels:
  netmaker.io/netclient: enabled
  netmaker.io/secret-key: auth-token
# Uses secret "netclient-token" with key "auth-token" in same namespace
```

**Custom Namespace:**
```yaml
labels:
  netmaker.io/netclient: enabled
  netmaker.io/secret-namespace: shared-secrets
# Uses secret "netclient-token" with key "token" in "shared-secrets" namespace
```

**Full Custom Configuration:**
```yaml
labels:
  netmaker.io/netclient: enabled
  netmaker.io/secret-name: my-custom-token
  netmaker.io/secret-key: join-token
  netmaker.io/secret-namespace: netmaker-system
# Uses secret "my-custom-token" with key "join-token" in "netmaker-system" namespace
```

### 3. Deploy Your Workload

The webhook will automatically:
- Add the netclient sidecar container
- Set `hostNetwork: true`
- Add required volumes (`/etc/netclient`, `/var/log`)
- Configure proper security context and capabilities
- Add readiness probe for WireGuard interface

## Example Deployments

### Deployment with Netclient

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-with-netclient
  namespace: netclient-enabled
spec:
  replicas: 3
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
        netmaker.io/netclient: enabled
    spec:
      containers:
      - name: nginx
        image: nginx:1.21
        ports:
        - containerPort: 80
```

### StatefulSet with Netclient

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: database-with-netclient
  namespace: netclient-enabled
spec:
  serviceName: database
  replicas: 1
  selector:
    matchLabels:
      app: database
  template:
    metadata:
      labels:
        app: database
        netmaker.io/netclient: enabled
    spec:
      containers:
      - name: postgres
        image: postgres:13
        env:
        - name: POSTGRES_PASSWORD
          value: "password"
```

## What Gets Added

When the webhook detects a pod with the netclient label, it automatically adds:

### 1. Netclient Container

```yaml
- name: netclient
  image: gravitl/netclient:v1.1.0
  env:
  - name: TOKEN
    value: "your-netmaker-token"
  - name: DAEMON
    value: "on"
  - name: LOG_LEVEL
    value: "info"
  volumeMounts:
  - name: etc-netclient
    mountPath: /etc/netclient
  - name: log-netclient
    mountPath: /var/log
  securityContext:
    privileged: true
    capabilities:
      add: ["NET_ADMIN", "SYS_MODULE"]
  readinessProbe:
    exec:
      command: ["/bin/sh", "-c", "ip link show netmaker && ip addr show netmaker | grep inet"]
    initialDelaySeconds: 10
    periodSeconds: 5
    failureThreshold: 12
```

### 2. Required Volumes

```yaml
volumes:
- name: etc-netclient
  hostPath:
    path: /etc/netclient
    type: DirectoryOrCreate
- name: log-netclient
  emptyDir:
    medium: Memory
```

### 3. Network Configuration

```yaml
# Note: hostNetwork is not required. Containers in a pod share the network namespace.
```

## Troubleshooting

### Check Webhook Status

```bash
# Check if webhook is running
kubectl get mutatingadmissionwebhook netclient-sidecar-webhook

# Check webhook logs
kubectl logs -n netmaker-k8s-ops-system -l control-plane=controller-manager -c manager | grep webhook
```

### Check Pod Status

```bash
# Check if netclient sidecar was added
kubectl describe pod <pod-name> -n <namespace>

# Check netclient logs
kubectl logs <pod-name> -c netclient -n <namespace>

# Check if WireGuard interface is up
kubectl exec <pod-name> -c netclient -n <namespace> -- ip link show netmaker
```

### Common Issues

1. **Webhook not triggered**: Check namespace has `netmaker.io/webhook: enabled` label
2. **Pod rejected**: Check webhook logs for errors
3. **Netclient not connecting**: Verify the secret exists and contains the correct token
4. **No WireGuard interface**: Check netclient logs for connection issues
5. **Secret not found**: Ensure the secret exists in the specified namespace
6. **Wrong secret used**: Check pod labels for custom secret configuration
7. **Permission denied**: Ensure webhook has RBAC permissions to read secrets in target namespace

## Security Considerations

- **Privileged Containers**: Netclient requires privileged access for WireGuard
- **Host Network**: Pods with netclient use host networking
- **Token Security**: Store Netmaker tokens securely in Kubernetes secrets
- **Namespace Isolation**: Only enable webhook in trusted namespaces
- **Secret Access**: Webhook reads secrets from the same namespace as the pod
- **RBAC**: Ensure webhook has proper RBAC permissions to read secrets

## Advanced Configuration

### Custom Netclient Image

```yaml
env:
- name: NETCLIENT_IMAGE
  value: "your-registry/netclient:custom-tag"
```

### Custom Resource Limits

The webhook uses these default resource limits:

```yaml
resources:
  limits:
    cpu: 200m
    memory: 128Mi
  requests:
    cpu: 50m
    memory: 64Mi
```

### Multiple Networks

To use multiple WireGuard networks, you can:

1. Deploy multiple netclient containers with different configurations
2. Use different labels for different networks
3. Configure routing in your application

## Monitoring

### Webhook Metrics

The webhook exposes metrics for:
- Number of pods processed
- Number of sidecars injected
- Webhook processing time
- Error rates

### Netclient Health

Monitor netclient health using:
- Readiness probe status
- WireGuard interface status
- Netmaker server connectivity
- Log analysis

This webhook makes it easy to add secure WireGuard connectivity to any Kubernetes workload with just a label!
