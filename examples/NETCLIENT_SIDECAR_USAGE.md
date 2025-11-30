# Netclient Sidecar Usage Guide

This guide explains how to use the netclient sidecar injection feature to enable connectivity for Kubernetes workloads through the private WireGuard network.

## Overview

The netclient sidecar can be automatically injected into Kubernetes resources (Pods, Deployments, StatefulSets, DaemonSets, Jobs, ReplicaSets) to provide:

- **Ingress**: Allows external access to your Kubernetes applications from the private network
- **Egress**: Allows your Kubernetes applications to access other services on the private network

The netclient sidecar enables both ingress and egress connectivity automatically when injected.

## Prerequisites

1. **Netmaker Server**: A running Netmaker server with a configured network
2. **Netclient Token Secret**: A Kubernetes secret containing the netclient join token
3. **Webhook Enabled**: The mutating webhook must be deployed and running

### Creating the Netclient Token Secret

```bash
# Create a secret with your netclient token
kubectl create secret generic netclient-token \
  --from-literal=token=your-netclient-token-here \
  --namespace=default
```

## Usage

### Basic Configuration

To enable netclient sidecar injection, add the following label to your resource:

```yaml
metadata:
  labels:
    netmaker.io/netclient: enabled
```

The label must be present on:
- The resource metadata (Deployment, StatefulSet, etc.)
- The pod template metadata (spec.template.metadata.labels)

### Custom Secret Configuration

By default, the webhook looks for a secret named `netclient-token` in the same namespace with a key `token`. You can customize this using annotations:

```yaml
metadata:
  annotations:
    netmaker.io/secret-name: my-custom-secret
    netmaker.io/secret-key: my-token-key
    netmaker.io/secret-namespace: my-namespace
```

## Examples

### 1. Deployment with Netclient Sidecar

Allows your application to be accessed from and access services on the private network:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-with-netclient
  labels:
    app: nginx
    netmaker.io/netclient: enabled
spec:
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

See `examples/deployment-with-netclient-ingress.yaml` for a complete example.

### 2. StatefulSet with Netclient Sidecar

Database accessible from the private network:

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: database-with-netclient
  labels:
    app: database
    netmaker.io/netclient: enabled
spec:
  template:
    metadata:
      labels:
        app: database
        netmaker.io/netclient: enabled
    spec:
      containers:
      - name: database
        image: postgres:14
```

See `examples/statefulset-with-netclient.yaml` for a complete example.

### 3. Job with Netclient Sidecar

One-time job that needs to access services on the private network:

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: data-sync-job-with-netclient
  labels:
    app: data-sync
    netmaker.io/netclient: enabled
spec:
  template:
    metadata:
      labels:
        app: data-sync
        netmaker.io/netclient: enabled
    spec:
      containers:
      - name: data-sync
        image: my-sync-job:latest
      restartPolicy: Never
```

See `examples/job-with-netclient-egress.yaml` for a complete example.

## How It Works

1. **Webhook Injection**: When you create or update a resource with the `netmaker.io/netclient: enabled` label, the mutating webhook intercepts the request.

2. **Sidecar Addition**: The webhook automatically adds a netclient sidecar container to your pod spec with:
   - WireGuard configuration
   - Required security contexts (privileged, NET_ADMIN, SYS_MODULE)
   - Appropriate volumes
   
   Note: Containers in a pod share the network namespace, so the WireGuard interface created by the netclient sidecar is automatically available to all containers in the pod without requiring hostNetwork.

3. **Network Connectivity**: Once the sidecar starts, it establishes a WireGuard connection to your Netmaker network, enabling both:
   - **Ingress**: Services can be accessed from the private network
   - **Egress**: Pods can access services on the private network

## Verification

### Check if Sidecar is Injected

```bash
# Check deployment
kubectl get deployment my-app -o yaml | grep -A 10 netclient

# Check pod
kubectl get pod <pod-name> -o yaml | grep -A 10 netclient
```

### Check Netclient Logs

```bash
# View netclient sidecar logs
kubectl logs <pod-name> -c netclient
```

### Test Connectivity

**Test Ingress (from private network):**
```bash
# From a machine on the private network
curl http://<pod-wireguard-ip>:<service-port>
```

**Test Egress (from pod):**
```bash
# From inside the pod
kubectl exec <pod-name> -c <app-container> -- curl http://<private-service-ip>:<port>
```

## Important Notes

1. **Shared Network Namespace**: Containers in a pod share the network namespace, so the WireGuard interface created by the netclient sidecar is automatically accessible to all containers in the pod. Host networking is not required.

2. **Privileged Containers**: The netclient sidecar runs with privileged access and requires `NET_ADMIN` and `SYS_MODULE` capabilities.

3. **Resource Requirements**: The netclient sidecar has default resource limits:
   - CPU: 50m request, 200m limit
   - Memory: 64Mi request, 128Mi limit

4. **Readiness Probe**: The netclient sidecar includes a readiness probe that checks for the WireGuard interface. Pods will not be ready until the interface is established.

5. **Secret Access**: Ensure the webhook service account has permissions to read secrets in the target namespace.

## Troubleshooting

### Sidecar Not Injected

1. Verify the label is present on both the resource and pod template:
   ```bash
   kubectl get deployment <name> -o jsonpath='{.metadata.labels.netmaker\.io/netclient}'
   kubectl get deployment <name> -o jsonpath='{.spec.template.metadata.labels.netmaker\.io/netclient}'
   ```

2. Check webhook logs:
   ```bash
   kubectl logs -n netmaker-k8s-ops-system -l control-plane=controller-manager
   ```

3. Verify webhook is registered:
   ```bash
   kubectl get mutatingwebhookconfiguration netclient-sidecar-webhook
   ```

### Connectivity Issues

1. Check netclient logs for connection errors
2. Verify the token secret is correct
3. Ensure the Netmaker server is accessible
4. Check WireGuard interface status:
   ```bash
   kubectl exec <pod-name> -c netclient -- ip link show netmaker
   kubectl exec <pod-name> -c netclient -- ip addr show netmaker
   ```

## Supported Resource Types

- Pods
- Deployments
- StatefulSets
- DaemonSets
- Jobs
- ReplicaSets

## Security Considerations

- The netclient sidecar requires privileged access
- Ensure proper RBAC for secret access
- Use network policies to restrict traffic if needed
- Regularly rotate netclient tokens
- Monitor netclient logs for suspicious activity

