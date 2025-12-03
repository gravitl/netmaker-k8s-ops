# Finding Kubernetes Service DNS Names

This guide shows how to find and list Kubernetes service DNS names in your cluster.

## Quick Methods

### List All Services with DNS Names

```bash
# List all services in all namespaces with their DNS names
kubectl get svc --all-namespaces -o custom-columns=NAMESPACE:.metadata.namespace,NAME:.metadata.name,DNS:.metadata.name'.'*.metadata.namespace'.svc.cluster.local'

# Simpler: List services and construct DNS names manually
kubectl get svc --all-namespaces
```

### Format: Service DNS Names

Kubernetes service DNS names follow this format:
```
<service-name>.<namespace>.svc.cluster.local
```

For example:
- `kubernetes.default.svc.cluster.local` (default Kubernetes API service)
- `my-app.default.svc.cluster.local` (service `my-app` in `default` namespace)
- `nginx.production.svc.cluster.local` (service `nginx` in `production` namespace)

## Detailed Methods

### Method 1: List Services by Namespace

```bash
# List services in default namespace
kubectl get svc

# List services in a specific namespace
kubectl get svc -n <namespace>

# List services in all namespaces
kubectl get svc --all-namespaces
```

### Method 2: Get Full DNS Names

```bash
# Get service with full DNS name
kubectl get svc <service-name> -n <namespace> -o jsonpath='{.metadata.name}.{.metadata.namespace}.svc.cluster.local{"\n"}'

# Example: Get DNS name for a specific service
kubectl get svc my-service -n default -o jsonpath='{.metadata.name}.{.metadata.namespace}.svc.cluster.local{"\n"}'
# Output: my-service.default.svc.cluster.local
```

### Method 3: List All Services with DNS Names (Script)

```bash
# List all services with their DNS names
kubectl get svc --all-namespaces -o json | \
  jq -r '.items[] | "\(.metadata.name).\(.metadata.namespace).svc.cluster.local"'

# Without jq (using kubectl only)
kubectl get svc --all-namespaces -o custom-columns=DNS:.metadata.name'.'*.metadata.namespace'.svc.cluster.local'
```

### Method 4: Test DNS Resolution

```bash
# Test if a service DNS name resolves
kubectl run -it --rm test-dns --image=busybox --restart=Never -- \
  nslookup <service-name>.<namespace>.svc.cluster.local

# Example
kubectl run -it --rm test-dns --image=busybox --restart=Never -- \
  nslookup kubernetes.default.svc.cluster.local
```

## Common Service DNS Names

### System Services

```bash
# Kubernetes API server
kubernetes.default.svc.cluster.local

# CoreDNS
kube-dns.kube-system.svc.cluster.local
# or
coredns.kube-system.svc.cluster.local

# List all system services
kubectl get svc -n kube-system
```

### Application Services

```bash
# List services in default namespace
kubectl get svc

# List services in a specific namespace
kubectl get svc -n production
kubectl get svc -n staging
```

## Using Short Names

Kubernetes also supports shorter DNS names:

```bash
# Short form (same namespace)
<service-name>

# With namespace
<service-name>.<namespace>

# Full FQDN
<service-name>.<namespace>.svc.cluster.local
```

Example:
- `my-service` (if in same namespace)
- `my-service.default` (explicit namespace)
- `my-service.default.svc.cluster.local` (full FQDN)

## Script to List All Service DNS Names

Create a script to list all service DNS names:

```bash
#!/bin/bash
# list-service-dns.sh

echo "Kubernetes Service DNS Names:"
echo "=============================="
echo ""

kubectl get svc --all-namespaces -o json | \
  jq -r '.items[] | 
    "\(.metadata.name).\(.metadata.namespace).svc.cluster.local | ClusterIP: \(.spec.clusterIP) | Ports: \(.spec.ports[].port)"'

# Or without jq
kubectl get svc --all-namespaces -o custom-columns=\
  DNS:.metadata.name'.'*.metadata.namespace'.svc.cluster.local',\
  CLUSTER-IP:.spec.clusterIP,\
  PORT:.spec.ports[*].port
```

## Finding Services by Label

```bash
# Find services with specific labels
kubectl get svc -l app=my-app

# Find services with multiple labels
kubectl get svc -l app=my-app,env=production

# Show DNS names for labeled services
kubectl get svc -l app=my-app -o jsonpath='{range .items[*]}{.metadata.name}.{.metadata.namespace}.svc.cluster.local{"\n"}{end}'
```

## Finding Services by Type

```bash
# List all ClusterIP services
kubectl get svc --all-namespaces --field-selector spec.type=ClusterIP

# List all NodePort services
kubectl get svc --all-namespaces --field-selector spec.type=NodePort

# List all LoadBalancer services
kubectl get svc --all-namespaces --field-selector spec.type=LoadBalancer
```

## Verify Service DNS Resolution

### From a Pod

```bash
# Test DNS resolution from a pod
kubectl run -it --rm test-dns --image=busybox --restart=Never -- \
  sh -c 'nslookup my-service.default.svc.cluster.local'

# Test with short name (same namespace)
kubectl run -it --rm test-dns --image=busybox --restart=Never -- \
  sh -c 'nslookup my-service'

# Test from a pod with netclient sidecar
kubectl exec <pod-with-netclient> -- nslookup my-service.default.svc.cluster.local
```

### From External VPN Client

```bash
# This will only work if DNS forwarding is configured
# See K8S_SERVICE_DNS_VPN.md for setup instructions
nslookup my-service.default.svc.cluster.local <k8s-dns-ip>
```

## Useful One-Liners

```bash
# Get all service DNS names in a namespace
kubectl get svc -n <namespace> -o jsonpath='{range .items[*]}{.metadata.name}.{.metadata.namespace}.svc.cluster.local{"\n"}{end}'

# Get service DNS name and ClusterIP
kubectl get svc --all-namespaces -o jsonpath='{range .items[*]}{.metadata.name}.{.metadata.namespace}.svc.cluster.local{" -> "}{.spec.clusterIP}{"\n"}{end}'

# Get service DNS name with ports
kubectl get svc --all-namespaces -o jsonpath='{range .items[*]}{.metadata.name}.{.metadata.namespace}.svc.cluster.local{" -> "}{.spec.clusterIP}{":"}{.spec.ports[0].port}{"\n"}{end}'
```

## Finding Services for VPN Access

When configuring VPN access to Kubernetes services:

```bash
# 1. List all services you want to expose
kubectl get svc --all-namespaces

# 2. Get DNS names for specific services
for ns in $(kubectl get ns -o jsonpath='{.items[*].metadata.name}'); do
  echo "=== Namespace: $ns ==="
  kubectl get svc -n $ns -o jsonpath='{range .items[*]}{.metadata.name}.{.metadata.namespace}.svc.cluster.local{"\n"}{end}'
done

# 3. Test DNS resolution from a pod with netclient
kubectl exec <pod-with-netclient> -- nslookup <service-name>.<namespace>.svc.cluster.local
```

## Troubleshooting

### Service Not Found

```bash
# Check if service exists
kubectl get svc <service-name> -n <namespace>

# Check all namespaces
kubectl get svc <service-name> --all-namespaces
```

### DNS Not Resolving

```bash
# Check CoreDNS is running
kubectl get pods -n kube-system -l k8s-app=kube-dns

# Check CoreDNS logs
kubectl logs -n kube-system -l k8s-app=kube-dns

# Test DNS from a pod
kubectl run -it --rm debug --image=busybox --restart=Never -- \
  nslookup kubernetes.default.svc.cluster.local
```

### Service DNS Name Format

Remember the format:
- **Short**: `<service-name>` (same namespace only)
- **With namespace**: `<service-name>.<namespace>`
- **Full FQDN**: `<service-name>.<namespace>.svc.cluster.local`

## Examples

### Example 1: Find All Services in Default Namespace

```bash
kubectl get svc -n default
# Output shows services, construct DNS names as:
# <service-name>.default.svc.cluster.local
```

### Example 2: Get Specific Service DNS Name

```bash
# Service: nginx, Namespace: production
# DNS name: nginx.production.svc.cluster.local

kubectl get svc nginx -n production -o jsonpath='{.metadata.name}.{.metadata.namespace}.svc.cluster.local{"\n"}'
```

### Example 3: List All Service DNS Names

```bash
kubectl get svc --all-namespaces -o json | \
  jq -r '.items[] | "\(.metadata.name).\(.metadata.namespace).svc.cluster.local"'
```

Output example:
```
kubernetes.default.svc.cluster.local
kube-dns.kube-system.svc.cluster.local
nginx.production.svc.cluster.local
my-app.staging.svc.cluster.local
```

