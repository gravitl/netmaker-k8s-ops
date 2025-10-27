# Netmaker K8s Operator Testing Guide

This guide provides step-by-step instructions to test the Netmaker K8s operator with the proxy and netclient sidecar functionality.

## Prerequisites

- Go 1.22.0+
- Docker
- kubectl
- Access to a Kubernetes cluster
- Netmaker server and token (optional for full testing)

## Step 1: Build the Operator

```bash
# Navigate to the project directory
cd /Users/abhishekk/go/src/github.com/gravitl/netmaker-k8s-ops

# Build the operator binary
go build -o bin/netmaker-k8s-ops cmd/main.go

# Verify the binary was created
ls -la bin/netmaker-k8s-ops
```

## Step 2: Basic Proxy Testing (Without Netclient)

### 2.1 Test Proxy Health Endpoints

```bash
# Start the proxy in the background
./bin/netmaker-k8s-ops &
PROXY_PID=$!

# Wait for startup
sleep 5

# Test health endpoint
echo "Testing health endpoint..."
curl -s http://localhost:8085/health | jq .

# Test readiness endpoint
echo "Testing readiness endpoint..."
curl -s http://localhost:8085/ready | jq .

# Test netclient status (should show no WireGuard interfaces)
echo "Testing netclient status..."
curl -s http://localhost:8085/netclient/status | jq .

# Clean up
kill $PROXY_PID
```

### 2.2 Test with Kubeconfig

```bash
# Set your kubeconfig
export KUBECONFIG=~/.kube/config

# Start the proxy
./bin/netmaker-k8s-ops &
PROXY_PID=$!

# Wait for startup
sleep 5

# Test Kubernetes API proxy (requires valid kubeconfig)
echo "Testing Kubernetes API proxy..."
curl -s -H "Authorization: Bearer $(kubectl config view --raw -o jsonpath='{.users[0].user.token}')" \
  http://localhost:8085/api/v1/namespaces | jq .

# Test Kubernetes version endpoint
echo "Testing Kubernetes version endpoint..."
curl -s http://localhost:8085/version | jq .

# Test Kubernetes metrics endpoint
echo "Testing Kubernetes metrics endpoint..."
curl -s http://localhost:8085/metrics | head -10

# Clean up
kill $PROXY_PID
```

## Step 3: Test with Netclient Sidecar (Optional)

### 3.1 Using Built-in Sidecar (if you have a Netmaker token)

```bash
# Set your Netmaker token
export NETCLIENT_TOKEN="your-netmaker-token-here"
export NETCLIENT_SERVER="your-netmaker-server.com"
export NETCLIENT_NETWORK="your-network-name"

# Start the proxy with netclient sidecar
./bin/netmaker-k8s-ops &
PROXY_PID=$!

# Wait for startup and WireGuard connection
sleep 15

# Test netclient status (should show WireGuard interface if connected)
echo "Testing netclient status with sidecar..."
curl -s http://localhost:8085/netclient/status | jq .

# Test health endpoint
echo "Testing health endpoint..."
curl -s http://localhost:8085/health | jq .

# Clean up
kill $PROXY_PID
```

### 3.2 Using Separate Netclient Container

```bash
# Start netclient container
docker run -d --name netclient-test \
  --privileged \
  --cap-add=NET_ADMIN \
  --cap-add=SYS_MODULE \
  -e TOKEN="your-netmaker-token-here" \
  -e DAEMON=on \
  gravitl/netclient:v1.1.0

# Wait for WireGuard connection
sleep 10

# Start the proxy (with netclient disabled)
export NETCLIENT_DISABLED=true
./bin/netmaker-k8s-ops &
PROXY_PID=$!

# Wait for startup
sleep 5

# Test netclient status (should detect WireGuard interface from container)
echo "Testing netclient status with separate container..."
curl -s http://localhost:8085/netclient/status | jq .

# Clean up
kill $PROXY_PID
docker stop netclient-test
docker rm netclient-test
```

## Step 4: Kubernetes Deployment Testing

### 4.1 Deploy to Kubernetes

```bash
# Install CRDs
make install

# Deploy the operator
make deploy IMG=netmaker-k8s-ops:latest

# Check if pods are running
kubectl get pods -n system

# Check logs
kubectl logs -n system deployment/controller-manager -c manager
kubectl logs -n system deployment/controller-manager -c netclient
```

### 4.2 Test with Example Configuration

```bash
# Apply the example configuration
kubectl apply -f examples/netclient-sidecar.yaml

# Check the deployment
kubectl get pods -l app=netmaker-k8s-proxy

# Check logs
kubectl logs -l app=netmaker-k8s-proxy -c netmaker-k8s-ops
kubectl logs -l app=netmaker-k8s-proxy -c netclient

# Test the proxy endpoints
kubectl port-forward svc/netmaker-k8s-proxy 8085:8085 &
PORT_FORWARD_PID=$!

# Test endpoints
curl -s http://localhost:8085/health | jq .
curl -s http://localhost:8085/ready | jq .
curl -s http://localhost:8085/netclient/status | jq .

# Clean up
kill $PORT_FORWARD_PID
kubectl delete -f examples/netclient-sidecar.yaml
```

## Step 5: Automated Testing

### 5.1 Run the Test Script

```bash
# Make the test script executable
chmod +x test-proxy.sh

# Run basic tests
./test-proxy.sh

# Run tests with netclient (if you have a token)
NETCLIENT_TOKEN="your-token" ./test-proxy.sh
```

### 5.2 Unit Tests

```bash
# Run unit tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific test packages
go test ./internal/controller/...
go test ./internal/proxy/...
```

## Step 6: Integration Testing

### 6.1 Test with Real Netmaker Server

```bash
# Set up your Netmaker environment
export NETCLIENT_TOKEN="your-actual-netmaker-token"
export NETCLIENT_SERVER="your-netmaker-server.com"
export NETCLIENT_NETWORK="your-network"

# Start the proxy
./bin/netmaker-k8s-ops &
PROXY_PID=$!

# Wait for connection
sleep 20

# Test WireGuard connectivity
ip addr show wg0 || echo "No wg0 interface found"

# Test netclient status
curl -s http://localhost:8085/netclient/status | jq .

# Test Kubernetes API through WireGuard
# (Configure your kubeconfig to point to WireGuard IP)
kubectl get nodes

# Clean up
kill $PROXY_PID
```

### 6.2 Test Proxy with WireGuard IP

```bash
# Get your WireGuard IP
WG_IP=$(ip addr show wg0 | grep 'inet ' | awk '{print $2}' | cut -d/ -f1)
echo "WireGuard IP: $WG_IP"

# Update your kubeconfig to use WireGuard IP
kubectl config set-cluster your-cluster --server=https://$WG_IP:8085

# Test kubectl through the proxy
kubectl get nodes
kubectl get pods --all-namespaces
```

## Step 7: Performance Testing

### 7.1 Load Testing

```bash
# Install hey (HTTP load testing tool)
go install github.com/rakyll/hey@latest

# Start the proxy
./bin/netmaker-k8s-ops &
PROXY_PID=$!

# Wait for startup
sleep 5

# Run load test
hey -n 1000 -c 10 http://localhost:8085/health

# Clean up
kill $PROXY_PID
```

### 7.2 Memory and CPU Monitoring

```bash
# Start the proxy
./bin/netmaker-k8s-ops &
PROXY_PID=$!

# Monitor resource usage
top -p $PROXY_PID

# Or use htop for better visualization
htop -p $PROXY_PID

# Clean up
kill $PROXY_PID
```

## Step 8: Troubleshooting

### 8.1 Common Issues

**Proxy won't start:**
```bash
# Check if port is already in use
lsof -i :8085

# Check logs
./bin/netmaker-k8s-ops 2>&1 | tee proxy.log
```

**Netclient connection fails:**
```bash
# Check WireGuard interfaces
ip addr show | grep wg

# Check netclient logs
docker logs netclient-test

# Test connectivity
ping 10.0.0.1  # Replace with your WireGuard gateway IP
```

**Kubernetes API proxy fails:**
```bash
# Check kubeconfig
kubectl config view

# Test direct API access
kubectl get --raw /api/v1/namespaces

# Check authentication
kubectl auth can-i get pods
```

### 8.2 Debug Mode

```bash
# Enable debug logging
export GIN_MODE=debug
./bin/netmaker-k8s-ops
```

## Step 9: Cleanup

### 9.1 Local Testing Cleanup

```bash
# Kill any running processes
pkill -f netmaker-k8s-ops

# Remove test containers
docker rm -f netclient-test 2>/dev/null || true

# Clean up test files
rm -f proxy.log
```

### 9.2 Kubernetes Cleanup

```bash
# Remove example deployment
kubectl delete -f examples/netclient-sidecar.yaml

# Uninstall the operator
make undeploy

# Remove CRDs
make uninstall
```

## Expected Results

### Successful Test Results

1. **Health endpoints return 200 OK**
2. **Netclient status shows WireGuard interface when connected**
3. **Kubernetes API proxy works with valid kubeconfig**
4. **No errors in logs**
5. **Graceful shutdown works**

### Test Checklist

- [ ] Proxy starts without errors
- [ ] Health endpoint responds
- [ ] Readiness endpoint responds
- [ ] Netclient status endpoint works
- [ ] Kubernetes API proxy works (with valid kubeconfig)
- [ ] WireGuard interface detection works
- [ ] Graceful shutdown works
- [ ] No memory leaks during load testing
- [ ] Kubernetes deployment works
- [ ] Logs are informative and error-free

## Next Steps

After successful testing:

1. **Deploy to production** using the Kubernetes manifests
2. **Set up monitoring** with Prometheus and Grafana
3. **Configure alerting** for proxy and netclient health
4. **Implement backup and recovery** procedures
5. **Document operational procedures** for your team
