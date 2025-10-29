#!/bin/bash

# Fix webhook certificate issue
set -e

echo "🔍 Checking prerequisites..."

# Check if cert-manager is installed
if ! kubectl get crd certificates.cert-manager.io >/dev/null 2>&1; then
    echo "❌ cert-manager is not installed"
    exit 1
fi

# Check if ClusterIssuer exists
if ! kubectl get clusterissuer letsencrypt-prod >/dev/null 2>&1; then
    echo "❌ ClusterIssuer 'letsencrypt-prod' not found"
    echo "Available ClusterIssuers:"
    kubectl get clusterissuer
    echo ""
    echo "Please either:"
    echo "1. Create a ClusterIssuer named 'letsencrypt-prod'"
    echo "2. Or update config/certmanager/certificate.yaml with your ClusterIssuer name"
    exit 1
fi

echo "✅ Prerequisites check passed"

# Apply the certificate
echo "📦 Applying certificate..."
bin/kustomize build config/certmanager | kubectl apply -f -

# Wait for certificate to be ready
echo "⏳ Waiting for certificate to be issued..."
kubectl wait --for=condition=Ready certificate/webhook-server-cert -n netmaker-k8s-ops-system --timeout=300s

echo "✅ Certificate is ready!"

# Check the secret
echo "🔍 Checking secret..."
kubectl get secret webhook-server-cert -n netmaker-k8s-ops-system

echo "✅ Webhook certificate is ready!"
