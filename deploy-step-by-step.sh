#!/bin/bash

# Deploy Netmaker K8s Operator step by step to avoid webhook ordering issues
set -e

echo "🚀 Deploying Netmaker K8s Operator step by step..."

# Step 1: Deploy CRDs and RBAC
echo "📦 Step 1: Deploying CRDs and RBAC..."
make manifests
bin/kustomize build config/crd | kubectl apply -f -
bin/kustomize build config/rbac | kubectl apply -f -

# Step 2: Deploy manager (without webhook)
echo "📦 Step 2: Deploying manager deployment..."
bin/kustomize build config/manager | kubectl apply -f -

# Step 3: Deploy webhook service
echo "📦 Step 3: Deploying webhook service..."
bin/kustomize build config/webhook | grep -A 20 "kind: Service" | kubectl apply -f -

# Step 4: Wait for manager pod to be ready
echo "⏳ Step 4: Waiting for manager pod to be ready..."
kubectl wait --for=condition=Ready pod -l control-plane=controller-manager -n netmaker-k8s-ops-system --timeout=300s

# Step 5: Deploy certificate
echo "📦 Step 5: Deploying certificate..."
bin/kustomize build config/certmanager | kubectl apply -f -

# Step 6: Wait for certificate to be ready
echo "⏳ Step 6: Waiting for certificate to be issued..."
kubectl wait --for=condition=Ready certificate/webhook-server-cert -n netmaker-k8s-ops-system --timeout=300s

# Step 7: Deploy webhook configuration
echo "📦 Step 7: Deploying webhook configuration..."
bin/kustomize build config/webhook | grep -A 50 "kind: MutatingAdmissionWebhook" | kubectl apply -f -

# Step 8: Deploy remaining services
echo "📦 Step 8: Deploying remaining services..."
bin/kustomize build config/default | grep -E "(kind: Service|kind: ServiceAccount)" | kubectl apply -f -

echo "✅ Deployment complete!"
echo ""
echo "🔍 Verify deployment:"
echo "   kubectl get pods -n netmaker-k8s-ops-system"
echo "   kubectl get certificate -n netmaker-k8s-ops-system"
echo "   kubectl get mutatingwebhookconfiguration netclient-sidecar-webhook"
echo ""
echo "🧪 Test webhook with example:"
echo "   kubectl create namespace test-webhook"
echo "   kubectl label namespace test-webhook netmaker.io/webhook=enabled"
echo "   kubectl create secret generic netclient-token --from-literal=token=your-token -n test-webhook"
echo "   kubectl apply -f examples/pod-with-custom-secret.yaml"
