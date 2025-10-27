#!/bin/bash

# Quick Test Script for Netmaker K8s Operator
# This script runs basic tests to verify the operator is working

set -e

echo "🚀 Netmaker K8s Operator - Quick Test"
echo "======================================"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    if [ $1 -eq 0 ]; then
        echo -e "${GREEN}✅ $2${NC}"
    else
        echo -e "${RED}❌ $2${NC}"
        return 1
    fi
}

print_info() {
    echo -e "${YELLOW}ℹ️  $1${NC}"
}

# Check if binary exists
print_info "Checking if operator binary exists..."
if [ ! -f "bin/netmaker-k8s-ops" ]; then
    print_info "Binary not found. Building..."
    go build -o bin/netmaker-k8s-ops cmd/main.go
    print_status $? "Binary built successfully"
else
    print_status 0 "Binary exists"
fi

# Test 1: Basic proxy startup
print_info "Test 1: Starting proxy server..."
./bin/netmaker-k8s-ops &
PROXY_PID=$!

# Wait for startup
sleep 5

# Test 2: Health endpoint
print_info "Test 2: Testing health endpoint..."
curl -s -f http://localhost:8085/health > /dev/null
print_status $? "Health endpoint working"

# Test 3: Readiness endpoint
print_info "Test 3: Testing readiness endpoint..."
curl -s -f http://localhost:8085/ready > /dev/null
print_status $? "Readiness endpoint working"

# Test 4: Netclient status endpoint
print_info "Test 4: Testing netclient status endpoint..."
curl -s -f http://localhost:8085/netclient/status > /dev/null
print_status $? "Netclient status endpoint working"

# Test 5: Check if jq is available for pretty output
if command -v jq &> /dev/null; then
    print_info "Test 5: Testing endpoints with JSON output..."
    echo "Health response:"
    curl -s http://localhost:8085/health | jq .
    echo "Netclient status response:"
    curl -s http://localhost:8085/netclient/status | jq .
    print_status 0 "JSON output working"
else
    print_info "jq not available, skipping JSON output test"
fi

# Test 6: Graceful shutdown
print_info "Test 6: Testing graceful shutdown..."
kill $PROXY_PID
sleep 2

# Check if process is still running
if kill -0 $PROXY_PID 2>/dev/null; then
    print_status 1 "Process still running after kill signal"
    kill -9 $PROXY_PID 2>/dev/null || true
else
    print_status 0 "Graceful shutdown working"
fi

# Test 7: Check for common issues
print_info "Test 7: Checking for common issues..."

# Check if port is available
if lsof -i :8085 &> /dev/null; then
    print_status 1 "Port 8085 is still in use"
else
    print_status 0 "Port 8085 is available"
fi

# Test 8: Optional - Test with kubeconfig if available
if [ -f "$HOME/.kube/config" ] || [ -n "$KUBECONFIG" ]; then
    print_info "Test 8: Testing with kubeconfig..."
    ./bin/netmaker-k8s-ops &
    PROXY_PID=$!
    sleep 5
    
    # Try to get a token (this might fail, but that's ok)
    TOKEN=$(kubectl config view --raw -o jsonpath='{.users[0].user.token}' 2>/dev/null || echo "")
    if [ -n "$TOKEN" ]; then
        curl -s -f -H "Authorization: Bearer $TOKEN" http://localhost:8085/api/v1/namespaces > /dev/null
        print_status $? "Kubernetes API proxy working"
    else
        print_info "No valid token found, skipping Kubernetes API test"
    fi
    
    kill $PROXY_PID 2>/dev/null || true
    sleep 2
else
    print_info "No kubeconfig found, skipping Kubernetes API test"
fi

echo ""
echo "🎉 Quick test completed!"
echo ""
echo "Next steps:"
echo "1. For full testing, see TESTING_GUIDE.md"
echo "2. To test with netclient: NETCLIENT_TOKEN=your-token ./test-proxy.sh"
echo "3. To deploy to Kubernetes: kubectl apply -f examples/netclient-sidecar.yaml"
echo ""
echo "For detailed testing instructions, run: cat TESTING_GUIDE.md"
