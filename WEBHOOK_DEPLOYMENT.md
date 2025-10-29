# Webhook Deployment Guide

This guide explains how to deploy the Netmaker K8s Operator with the mutating admission webhook enabled using cert-manager for TLS certificates.

## Prerequisites

1. **Kubernetes cluster** (1.19+)
2. **cert-manager** installed
3. **ClusterIssuer** configured

## Quick Start

### 1. Install cert-manager (if not already installed)

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml
```

### 2. Create a ClusterIssuer

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: your-email@example.com
    privateKeySecretRef:
      name: letsencrypt-prod
    solvers:
    - http01:
        ingress:
          class: nginx
```

### 3. Deploy the Operator

```bash
# Option 1: Use the deployment script
./deploy-with-webhook.sh

# Option 2: Manual deployment
kubectl apply -k config/default
```

### 4. Verify Deployment

```bash
# Check pods
kubectl get pods -n netmaker-k8s-ops-system

# Check certificate
kubectl get certificate -n netmaker-k8s-ops-system

# Check webhook
kubectl get mutatingwebhookconfiguration netclient-sidecar-webhook
```

## Configuration

### Customize ClusterIssuer

Edit `config/certmanager/certificate.yaml`:

```yaml
spec:
  issuerRef:
    name: your-clusterissuer-name  # Change this
    kind: ClusterIssuer
```

### Customize Certificate DNS Names

Edit `config/certmanager/certificate.yaml`:

```yaml
spec:
  dnsNames:
  - webhook-service.netmaker-k8s-ops-system.svc
  - webhook-service.netmaker-k8s-ops-system.svc.cluster.local
  - your-custom-domain.com  # Add custom domains
```

## Testing the Webhook

### 1. Create a test namespace

```bash
kubectl create namespace test-webhook
kubectl label namespace test-webhook netmaker.io/webhook=enabled
```

### 2. Create a test secret

```bash
kubectl create secret generic netclient-token \
  --from-literal=token=your-netclient-token \
  --namespace=test-webhook
```

### 3. Deploy a test pod

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  namespace: test-webhook
  labels:
    netmaker.io/netclient: enabled
spec:
  containers:
  - name: nginx
    image: nginx:1.21
```

### 4. Verify webhook injection

```bash
# Check if netclient sidecar was added
kubectl describe pod test-pod -n test-webhook

# Check netclient logs
kubectl logs test-pod -c netclient -n test-webhook
```

## Troubleshooting

### Certificate Issues

```bash
# Check certificate status
kubectl describe certificate webhook-server-cert -n netmaker-k8s-ops-system

# Check cert-manager logs
kubectl logs -n cert-manager -l app=cert-manager
```

### Webhook Issues

```bash
# Check webhook configuration
kubectl get mutatingwebhookconfiguration netclient-sidecar-webhook -o yaml

# Check manager logs
kubectl logs -n netmaker-k8s-ops-system -l control-plane=controller-manager -c manager
```

### Common Issues

1. **Certificate not ready**: Check ClusterIssuer configuration and cert-manager logs
2. **Webhook not triggered**: Ensure namespace has `netmaker.io/webhook=enabled` label
3. **Pod rejected**: Check webhook logs for errors
4. **Secret not found**: Ensure secret exists in the correct namespace

## Security Considerations

- **TLS Required**: Webhooks must use HTTPS
- **Certificate Rotation**: cert-manager handles automatic certificate rotation
- **RBAC**: Webhook needs permissions to read secrets
- **Namespace Isolation**: Only enable webhook in trusted namespaces

## Advanced Configuration

### Custom Certificate

If you want to use your own certificate instead of cert-manager:

1. Create a TLS secret:
```bash
kubectl create secret tls webhook-server-cert \
  --cert=your-cert.crt \
  --key=your-key.key \
  --namespace=netmaker-k8s-ops-system
```

2. Update the MutatingWebhookConfiguration with your CA bundle:
```bash
kubectl patch mutatingwebhookconfiguration netclient-sidecar-webhook \
  --type='json' -p='[{"op": "replace", "path": "/webhooks/0/clientConfig/caBundle", "value": "YOUR_CA_BUNDLE"}]'
```

### Multiple Webhooks

To add more webhooks, create additional MutatingWebhookConfiguration resources and update the kustomization accordingly.

## Cleanup

```bash
# Remove the operator
kubectl delete -k config/default

# Remove test resources
kubectl delete namespace test-webhook
```
