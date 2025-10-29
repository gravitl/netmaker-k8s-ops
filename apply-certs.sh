#!/bin/bash

# Apply certificates and check status
set -e

echo "🔧 Applying certificate configuration..."

# Apply the certificates
kubectl apply -k config/certmanager

echo "⏳ Waiting for root CA certificate..."
kubectl -n netmaker-k8s-ops-system wait certificate/netmaker-webhook-root-ca --for=condition=Ready --timeout=60s

echo "⏳ Waiting for webhook certificate..."
kubectl -n netmaker-k8s-ops-system wait certificate/webhook-server-cert --for=condition=Ready --timeout=60s

echo "✅ Certificates are ready!"

# Check the secrets
echo "🔍 Checking secrets..."
kubectl -n netmaker-k8s-ops-system get secret netmaker-webhook-root-ca
kubectl -n netmaker-k8s-ops-system get secret webhook-server-cert

echo "🎉 Webhook certificate is ready!"
