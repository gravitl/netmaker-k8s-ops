# Container Ordering Guide

This guide explains the container ordering strategy for the Netmaker K8s operator to ensure proper WireGuard connectivity.

## Problem

The original deployment had both containers starting simultaneously, which caused issues:
- **Manager container** (proxy) starts and immediately tries to detect WireGuard interface
- **Netclient container** starts and needs time to establish WireGuard connection
- **Race condition**: Proxy fails to find WireGuard interface because it's not ready yet

## Solution: Sidecar Container with Readiness Probe

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Kubernetes Pod                           │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │              Sidecar Container                     │   │
│  │         (netclient)                               │   │
│  │  • Establishes WireGuard connection               │   │
│  │  • Creates WireGuard interface (netmaker)         │   │
│  │  • Maintains connection (daemon mode)             │   │
│  │  • Readiness probe ensures interface is ready     │   │
│  └─────────────────────────────────────────────────────┘   │
│                           │                                │
│                           ▼                                │
│  ┌─────────────────────────────────────────────────────┐   │
│  │              Main Container                         │   │
│  │         (manager)                                   │   │
│  │  • Starts after netclient is ready                 │   │
│  │  • Detects existing WireGuard interface            │   │
│  │  • Binds proxy to WireGuard IP                     │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

### Container Lifecycle

1. **Sidecar Container Phase**:
   - `netclient` starts and runs as daemon
   - Establishes WireGuard connection to Netmaker server
   - Creates WireGuard interface (`netmaker`)
   - Assigns IP address to interface
   - Readiness probe ensures interface is ready before manager starts

2. **Main Container Phase**:
   - `manager` starts after netclient readiness probe passes
   - Waits 5 seconds for interface to be fully ready
   - Scans for WireGuard interface (`netmaker`) with retry logic
   - Uses exponential backoff (2s, 4s, 6s, 8s, 10s, 12s, 14s, 16s, 18s, 20s)
   - Binds proxy to WireGuard IP address
   - Starts serving requests

### Configuration

#### Retry Configuration
The WireGuard interface detection includes configurable retry logic:

```yaml
env:
- name: WIREGUARD_RETRY_MAX_ATTEMPTS
  value: "10"  # Maximum number of retry attempts (default: 10)
- name: WIREGUARD_RETRY_BASE_DELAY_SECONDS
  value: "2"   # Base delay between retries in seconds (default: 2)
- name: WIREGUARD_RETRY_MAX_DELAY_SECONDS
  value: "30"  # Maximum delay between retries in seconds (default: 30)
```

**Retry Schedule** (with defaults):
- Attempt 1: Immediate
- Attempt 2: Wait 2s
- Attempt 3: Wait 4s
- Attempt 4: Wait 6s
- Attempt 5: Wait 8s
- Attempt 6: Wait 10s
- Attempt 7: Wait 12s
- Attempt 8: Wait 14s
- Attempt 9: Wait 16s
- Attempt 10: Wait 18s

Total maximum wait time: ~90 seconds

#### Sidecar Container
```yaml
containers:
- name: netclient
  image: gravitl/netclient:v1.1.0
  env:
  - name: TOKEN
    value: "YOUR_NETMAKER_TOKEN"
  - name: DAEMON
    value: "on"
  - name: LOG_LEVEL
    value: "info"
  readinessProbe:
    exec:
      command:
      - /bin/sh
      - -c
      - "ip link show netmaker && ip addr show netmaker | grep inet"
    initialDelaySeconds: 10
    periodSeconds: 5
    failureThreshold: 12
  securityContext:
    privileged: true
    capabilities:
      add:
      - NET_ADMIN
      - SYS_MODULE
```

#### Main Container
```yaml
containers:
- name: manager
  # ... other config ...
  startupProbe:
    httpGet:
      path: /readyz
      port: 8081
    initialDelaySeconds: 10
    periodSeconds: 5
    failureThreshold: 20  # Wait up to 100 seconds
```

### Benefits

1. **Guaranteed Ordering**: WireGuard connection is established before proxy starts
2. **No Race Conditions**: Manager container only starts after WireGuard is ready
3. **Reliable Detection**: Proxy can reliably find the WireGuard interface
4. **Proper Binding**: Proxy binds to the correct WireGuard IP address
5. **Resource Efficiency**: Init container exits after setup, saving resources

### Startup Sequence

```
1. Pod starts
2. Init container (netclient-init) runs
   ├── Connects to Netmaker server
   ├── Creates WireGuard interface
   ├── Assigns IP address
   └── Exits successfully
3. Main container (manager) starts
   ├── Waits 5 seconds
   ├── Scans for WireGuard interface
   ├── Binds proxy to WireGuard IP
   └── Starts serving requests
```

### Troubleshooting

#### Check Init Container Status
```bash
# Check if init container completed successfully
kubectl describe pod -n netmaker-k8s-ops-system -l control-plane=controller-manager

# Check init container logs
kubectl logs -n netmaker-k8s-ops-system -l control-plane=controller-manager -c netclient-init
```

#### Check WireGuard Interface
```bash
# Check if WireGuard interface exists
kubectl exec -n netmaker-k8s-ops-system -l control-plane=controller-manager -c manager -- ls /sys/class/net/

# Check interface IP
kubectl exec -n netmaker-k8s-ops-system -l control-plane=controller-manager -c manager -- cat /proc/net/fib_trie | grep netmaker
```

#### Check Proxy Binding
```bash
# Check proxy logs for binding information
kubectl logs -n netmaker-k8s-ops-system -l control-plane=controller-manager -c manager | grep -i "binding\|wireguard"

# Expected output:
# INFO: Searching for WireGuard interfaces with retry logic {"interfaces": ["netmaker"]}
# INFO: Attempting to find WireGuard interface {"attempt": 1, "maxRetries": 10}
# INFO: Interface not found yet {"interface": "netmaker", "attempt": 1, "error": "stat /sys/class/net/netmaker: no such file or directory"}
# INFO: Waiting before retry {"delay": "2s", "nextAttempt": 2}
# INFO: Attempting to find WireGuard interface {"attempt": 2, "maxRetries": 10}
# INFO: Found WireGuard interface {"interface": "netmaker", "attempt": 2}
# INFO: Found IP from fib_trie {"interface": "netmaker", "ip": "10.0.0.1", "attempt": 2}
# INFO: Binding proxy to specific IP {"ip": "10.0.0.1", "port": "8085"}
```

### Alternative Approaches

#### Option 1: Sidecar with Readiness Probe
```yaml
containers:
- name: netclient
  # ... config ...
  readinessProbe:
    exec:
      command:
      - /bin/sh
      - -c
      - "ip link show netmaker && ip addr show netmaker | grep inet"
- name: manager
  # ... config ...
  # Depends on netclient being ready
```

#### Option 2: Shared Volume
```yaml
volumes:
- name: wireguard-status
  emptyDir: {}
initContainers:
- name: netclient-init
  # ... config ...
  volumeMounts:
  - name: wireguard-status
    mountPath: /status
  # Write status to volume
containers:
- name: manager
  # ... config ...
  volumeMounts:
  - name: wireguard-status
    mountPath: /status
  # Read status from volume
```

### Best Practices

1. **Use Init Containers** for setup tasks that need to complete before main containers start
2. **Add Startup Probes** to give main containers time to initialize
3. **Include Delays** in application code for additional safety
4. **Monitor Logs** to ensure proper startup sequence
5. **Test Restart Scenarios** to ensure reliability

This approach ensures that the WireGuard connection is fully established before the proxy tries to use it, eliminating race conditions and improving reliability.
