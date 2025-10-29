#!/bin/bash

# Deploy Netmaker K8s Operator with Webhook and cert-manager
# Prerequisites: cert-manager and ClusterIssuer must be installed

set -e

echo "🚀 Deploying Netmaker K8s Operator with Webhook..."

# Check if cert-manager is installed
if ! kubectl get crd certificates.cert-manager.io >/dev/null 2>&1; then
    echo "❌ cert-manager is not installed. Please install cert-manager first."
    echo "   kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml"
    exit 1
fi

# Check if ClusterIssuer exists
if ! kubectl get clusterissuer letsencrypt-prod >/dev/null 2>&1; then
    echo "⚠️  ClusterIssuer 'letsencrypt-prod' not found."
    echo "   Please update config/certmanager/certificate.yaml with your ClusterIssuer name"
    echo "   or create a ClusterIssuer named 'letsencrypt-prod'"
    echo ""
    echo "   Example ClusterIssuer:"
    echo "   kubectl apply -f - <<EOF"
    echo "   apiVersion: cert-manager.io/v1"
    echo "   kind: ClusterIssuer"
    echo "   metadata:"
    echo "     name: letsencrypt-prod"
    echo "   spec:"
    echo "     acme:"
    echo "       server: https://acme-v02.api.letsencrypt.org/directory"
    echo "       email: your-email@example.com"
    echo "       privateKeySecretRef:"
    echo "         name: letsencrypt-prod"
    echo "       solvers:"
    echo "       - http01:"
    echo "           ingress:"
    echo "             class: nginx"
    echo "   EOF"
    exit 1
fi

echo "✅ Prerequisites check passed"

# Deploy the operator
echo "📦 Deploying operator with webhook and cert-manager..."
kubectl apply -k config/default

# Wait for certificate to be ready
echo "⏳ Waiting for webhook certificate to be issued..."
kubectl wait --for=condition=Ready certificate/webhook-server-cert -n netmaker-k8s-ops-system --timeout=300s

# Wait for manager pod to be ready
echo "⏳ Waiting for manager pod to be ready..."
kubectl wait --for=condition=Ready pod -l control-plane=controller-manager -n netmaker-k8s-ops-system --timeout=300s

echo "✅ Deployment complete!"
echo ""
echo "🔍 Verify deployment:"
echo "   kubectl get pods -n netmaker-k8s-ops-system"
echo "   kubectl get certificate -n netmaker-k8s-ops-system"
echo "   kubectl get mutatingwebhookconfiguration netclient-sidecar-webhook"
echo ""
echo "🧪 Test webhook with example:"
echo "   kubectl apply -f examples/pod-with-custom-secret.yaml"
