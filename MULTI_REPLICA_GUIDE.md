# Multi-Replica Deployment Guide

This guide explains how to deploy the Netmaker K8s operator with multiple replicas for high availability.

## Problem

When running multiple replicas of the operator, you may encounter this error:
```
ERROR setup unable to start manager {"error": "error listening on :8081: listen tcp :8081: bind: address already in use"}
```

This happens because all replicas try to bind to the same health probe port (`:8081`).

## Solution

The operator now automatically handles multi-replica deployments by:

1. **Dynamic Port Assignment**: Uses pod name hash to generate unique health probe ports
2. **Leader Election**: Enables leader election by default when `POD_NAME` is set
3. **Environment Variables**: Supports configuration via environment variables

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `POD_NAME` | - | Pod name (automatically set in Kubernetes) |
| `HEALTH_PROBE_PORT` | Auto-calculated | Health probe port (overrides auto-calculation) |
| `PROXY_PORT` | Auto-calculated | Proxy port (overrides auto-calculation) |
| `ENABLE_LEADER_ELECTION` | Auto-detected | Enable leader election (true if POD_NAME is set) |

### Port Assignment Logic

**Health Probe Port:**
1. If `HEALTH_PROBE_PORT` is set, use that port
2. If `POD_NAME` is available, calculate port as `8081 + (hash % 1000)`
3. Otherwise, use default port `8081`

**Proxy Port:**
1. If `PROXY_PORT` is set, use that port
2. If `POD_NAME` is available, calculate port as `8085 + (hash % 1000)`
3. Otherwise, use default port `8085`

## Deployment Examples

### Single Replica (Default)

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: netmaker-k8s-ops
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: manager
        image: netmaker-k8s-ops:latest
        args:
        # Health probe port will be auto-calculated
```

### Multi-Replica with Auto Port Assignment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: netmaker-k8s-ops-multi
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: manager
        image: netmaker-k8s-ops:latest
        env:
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: ENABLE_LEADER_ELECTION
          value: "true"
        # Health probe port will be auto-calculated
```

### Multi-Replica with Custom Ports

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: netmaker-k8s-ops-multi
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: manager
        image: netmaker-k8s-ops:latest
        env:
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: HEALTH_PROBE_PORT
          value: "8081"  # Will be overridden by pod name hash
```

## Health Probes

### Single Replica
```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: 8081
readinessProbe:
  httpGet:
    path: /readyz
    port: 8081
```

### Multi-Replica
```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: health  # Use named port
readinessProbe:
  httpGet:
    path: /readyz
    port: health  # Use named port
```

## Leader Election

When `POD_NAME` is set (indicating Kubernetes deployment), leader election is automatically enabled:

- Only the leader replica processes Kubernetes events
- Non-leader replicas wait for leadership
- Automatic failover if leader fails
- Prevents duplicate processing

## Troubleshooting

### Port Conflicts

**Problem**: Multiple replicas still have port conflicts

**Solution**: 
1. Ensure `POD_NAME` environment variable is set
2. Check that each pod has a unique name
3. Manually set `HEALTH_PROBE_PORT` for each replica

### Leader Election Issues

**Problem**: No leader is elected

**Solution**:
1. Check RBAC permissions for leader election
2. Ensure `ENABLE_LEADER_ELECTION=true`
3. Check logs for election errors

### Health Probe Failures

**Problem**: Health probes fail on non-leader replicas

**Solution**:
1. Use named ports in health probes
2. Ensure each replica has a unique port
3. Check that ports are not conflicting

## Best Practices

1. **Use Leader Election**: Always enable leader election for multi-replica deployments
2. **Named Ports**: Use named ports in health probes for flexibility
3. **Resource Limits**: Set appropriate resource limits for each replica
4. **Monitoring**: Monitor leader election and replica health
5. **Rolling Updates**: Use rolling updates for zero-downtime deployments

## Example: Complete Multi-Replica Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: netmaker-k8s-ops-multi
spec:
  replicas: 3
  template:
    spec:
      hostNetwork: true
      containers:
      - name: manager
        image: netmaker-k8s-ops:latest
        args:
        - --leader-elect
        - --health-probe-bind-address=:8081
        - --metrics-bind-address=0
        env:
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: ENABLE_LEADER_ELECTION
          value: "true"
        ports:
        - containerPort: 8085
          name: proxy
        - containerPort: 8081
          name: health
        livenessProbe:
          httpGet:
            path: /healthz
            port: health
        readinessProbe:
          httpGet:
            path: /readyz
            port: health
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

## Monitoring

### Check Replica Status
```bash
kubectl get pods -l control-plane=controller-manager
kubectl logs -l control-plane=controller-manager -c manager
```

### Check Leader Election
```bash
kubectl get events --field-selector reason=LeaderElection
```

### Check Health Probes
```bash
kubectl describe pod <pod-name>
```

This configuration ensures that multiple replicas can run without port conflicts while maintaining high availability and proper leader election.
