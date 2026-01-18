# Netmaker Kubernetes Operator - User Guide

## Introduction

### What is the Netmaker Kubernetes Operator?

The Netmaker Kubernetes Operator is a powerful tool that seamlessly bridges your Kubernetes clusters with Netmaker WireGuard networks. It enables secure, bidirectional connectivity between your Kubernetes workloads and devices on your Netmaker network, creating a unified networking layer across your infrastructure.

### Why Use the Netmaker Kubernetes Operator?

Modern infrastructure often spans multiple environments:
- **Kubernetes clusters** running containerized applications
- **Virtual machines** and bare-metal servers
- **Edge devices** and IoT deployments
- **Hybrid cloud** environments

Traditionally, connecting these different environments requires complex VPN configurations, firewall rules, and network routing. The Netmaker Kubernetes Operator simplifies this by:

- **Eliminating Network Complexity**: No need to manually configure VPNs or complex routing rules
- **Enabling Secure Communication**: All traffic flows through encrypted WireGuard tunnels
- **Providing Native Kubernetes Integration**: Use standard Kubernetes Services and annotations
- **Supporting Bidirectional Access**: Kubernetes workloads can reach Netmaker services, and Netmaker devices can reach Kubernetes services

### Key Concepts

Before diving in, it's helpful to understand a few key concepts:

#### Netmaker Network
A Netmaker network is a WireGuard-based virtual network that connects devices across different locations. Devices on the network can communicate securely using private IP addresses assigned by Netmaker.

#### Netclient
Netclient is the agent that runs on devices to connect them to a Netmaker network. The operator uses netclient as a sidecar container to provide WireGuard connectivity to Kubernetes pods.

#### Operator
A Kubernetes operator is a controller that extends Kubernetes functionality. This operator watches for specific Kubernetes resources (like Services with annotations) and automatically configures networking to connect them with Netmaker networks.

#### Egress vs Ingress
- **Egress Proxy**: Allows Kubernetes workloads to access services on the Netmaker network (Kubernetes → Netmaker)
- **Ingress Proxy**: Allows Netmaker devices to access services running in Kubernetes (Netmaker → Kubernetes)

### What Can You Do With the Operator?

The operator provides three main capabilities:

1. **Egress Proxy**: Access Netmaker services (APIs, databases, etc.) from your Kubernetes applications using standard Kubernetes Service names
2. **Ingress Proxy**: Expose your Kubernetes services to devices on your Netmaker network
3. **API Proxy**: Secure access to Kubernetes API servers through Netmaker tunnels with RBAC support

### Use Cases

#### Cross-Environment Database Access
Connect your Kubernetes applications to databases running on servers in your Netmaker network, without exposing them to the public internet.

#### Multi-Cluster Communication
Enable secure communication between workloads in different Kubernetes clusters through a shared Netmaker network.

#### Edge-to-Cloud Connectivity
Connect edge devices and IoT devices in your Netmaker network to services running in your Kubernetes cluster.

#### Secure API Access
Allow remote developers and systems to securely access your Kubernetes API server through WireGuard tunnels.

#### Hybrid Cloud Networking
Unify networking across cloud and on-premises infrastructure through a single Netmaker network.

---

## Getting Started

### Prerequisites

Before installing the Netmaker Kubernetes Operator, ensure you have:

1. **A Kubernetes Cluster** (v1.11.3 or later)
   - Access via `kubectl`
   - Sufficient permissions to create namespaces, deployments, and services

2. **A Netmaker Server**
   - Running and accessible
   - At least one network configured
   - Admin access to generate tokens

3. **A Netmaker Network Token**
   - Generated from your Netmaker server
   - Used to join the Kubernetes cluster to the network
   - Keep this secure - you'll need it during installation

4. **Helm** (v3.0 or later) - Recommended installation method
   - Or `kubectl` and `kustomize` for manual installation

5. **Container Registry Access**
   - The operator image must be available in a registry accessible to your cluster
   - Or use a pre-built image from a public registry

### Installation Methods

The operator can be installed using Helm (recommended) or via Make/Kustomize. We'll focus on the Helm method as it's the easiest for most users.

#### Method 1: Helm Installation (Recommended)

Helm is the recommended installation method as it handles all the complexity of deploying the operator, including CRDs and RBAC.

##### Step 1: Add the Helm Repository

```bash
# Add the Netmaker K8s Operator Helm repository
helm repo add netmaker-k8s-ops https://downloads.netmaker.io/charts/
helm repo update
```

##### Step 2: Prepare Your Configuration

You'll need:
- Your Netmaker network token
- The operator image location (repository and tag)

##### Step 3: Install the Operator

**Basic Installation:**

```bash
helm install netmaker-k8s-ops netmaker-k8s-ops/netmaker-k8s-ops \
  --namespace netmaker-k8s-ops-system \
  --create-namespace \
  --version 1.0.0 \
  --set image.repository=<your-registry>/netmaker-k8s-ops \
  --set image.tag=<tag> \
  --set netclient.token="YOUR_NETMAKER_TOKEN_HERE"
```

Replace:
- `<your-registry>/netmaker-k8s-ops` with your container registry path
- `<tag>` with the image tag/version
- `YOUR_NETMAKER_TOKEN_HERE` with your actual Netmaker network token

**Installation with Custom Values:**

For production deployments, you may want to use a values file:

```bash
# Create a values file
cat > my-values.yaml <<EOF
image:
  repository: <your-registry>/netmaker-k8s-ops
  tag: <tag>
netclient:
  token: "YOUR_NETMAKER_TOKEN_HERE"
service:
  proxy:
    enabled: true
EOF

# Install with values file
helm install netmaker-k8s-ops netmaker-k8s-ops/netmaker-k8s-ops \
  --namespace netmaker-k8s-ops-system \
  --create-namespace \
  --values my-values.yaml
```

**Using Kubernetes Secrets (Recommended for Production):**

For better security, store your Netmaker token in a Kubernetes Secret:

```bash
# Create the secret
kubectl create secret generic netclient-token \
  --from-literal=token="YOUR_NETMAKER_TOKEN_HERE" \
  --namespace netmaker-k8s-ops-system

# Install the operator (you'll need to configure it to use the secret)
# See the Token Configuration Guide for details
```

##### Step 4: Verify Installation

Check that the operator is running:

```bash
# Check pod status
kubectl get pods -n netmaker-k8s-ops-system

# You should see a pod with status "Running"
# Example output:
# NAME                                          READY   STATUS    RESTARTS   AGE
# netmaker-k8s-ops-controller-manager-xxxxx     2/2     Running   0          1m
```

The pod should have 2/2 containers ready (manager + netclient sidecar).

Check the logs to ensure everything is working:

```bash
# Check manager logs
kubectl logs -n netmaker-k8s-ops-system \
  -l control-plane=controller-manager \
  -c manager

# Check netclient logs
kubectl logs -n netmaker-k8s-ops-system \
  -l control-plane=controller-manager \
  -c netclient
```

You should see successful connection messages in the netclient logs indicating it has joined the Netmaker network.

#### Method 2: Manual Installation (Make/Kustomize)

If you prefer to build and deploy manually:

```bash
# 1. Build the operator image
make docker-build docker-push IMG=<your-registry>/netmaker-k8s-ops:tag

# 2. Install CRDs
make install

# 3. Deploy the operator
make deploy IMG=<your-registry>/netmaker-k8s-ops:tag

# 4. Configure the Netmaker token
# Edit config/manager/manager.yaml and set your token
```

### Post-Installation Configuration

#### Configure API Proxy (Optional)

If you want to use the API proxy feature:

```bash
helm upgrade netmaker-k8s-ops netmaker-k8s-ops/netmaker-k8s-ops \
  --namespace netmaker-k8s-ops-system \
  --set api.enabled=true \
  --set api.serverDomain="api.example.com" \
  --set api.token="your-api-token"
```

### Your First Steps

Once the operator is installed and running, try these simple examples:

#### Example 1: Create an Egress Proxy

Access a service on your Netmaker network from Kubernetes:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: netmaker-api
  annotations:
    netmaker.io/egress: "enabled"
    netmaker.io/egress-target-dns: "api.netmaker.internal"
spec:
  ports:
  - port: 80
    targetPort: 8080
```

Then access it from any pod in your cluster:
```bash
curl http://netmaker-api:80
```

#### Example 2: Create an Ingress Proxy

Expose a Kubernetes service to your Netmaker network:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-api
  annotations:
    netmaker.io/ingress: "enabled"
    netmaker.io/ingress-dns-name: "api.k8s.netmaker.internal"
spec:
  ports:
  - port: 80
    targetPort: 8080
  selector:
    app: my-api
```

Devices on your Netmaker network can now access this service using the DNS name or Netmaker IP.

### Troubleshooting

#### Operator Pod Not Starting

```bash
# Check pod status
kubectl describe pod -n netmaker-k8s-ops-system -l control-plane=controller-manager

# Check logs
kubectl logs -n netmaker-k8s-ops-system -l control-plane=controller-manager -c manager
```

Common issues:
- **Image pull errors**: Ensure your cluster can access the container registry
- **RBAC errors**: Ensure you have sufficient permissions
- **Token errors**: Verify your Netmaker token is correct

#### Netclient Not Connecting

```bash
# Check netclient logs
kubectl logs -n netmaker-k8s-ops-system -l control-plane=controller-manager -c netclient

# Check WireGuard interface
kubectl exec -n netmaker-k8s-ops-system -l control-plane=controller-manager -c netclient -- ip link show
```

Common issues:
- **Invalid token**: Verify the token is correct and hasn't expired
- **Network unreachable**: Ensure the Netmaker server is accessible
- **Firewall blocking**: Check that WireGuard ports (UDP 51821) are open

### Next Steps

Now that you have the operator installed, explore these features:

1. **Read the Feature Guides**:
   - [Egress Proxy Guide](../examples/EGRESS_PROXY_GUIDE.md) - Access Netmaker services from K8s
   - [Ingress Proxy Guide](../examples/INGRESS_PROXY_GUIDE.md) - Expose K8s services to Netmaker
   - [API Proxy Guide](../PROXY_USAGE.md) - Secure K8s API access

2. **Explore Examples**:
   - Check the `examples/` directory for ready-to-use configurations
   - Try the cross-cluster connectivity examples
   - Experiment with different proxy configurations

3. **Production Considerations**:
   - Review the [Deployment Guide](../DEPLOYMENT_GUIDE.md) for production best practices
   - Set up proper token management using Kubernetes Secrets
   - Configure resource limits and health checks
   - Enable monitoring and logging

### Getting Help

- **Documentation**: Check the other guides in the repository
- **Issues**: Open an issue on GitHub if you encounter problems
- **Community**: Join the Netmaker community for support

---

## Summary

The Netmaker Kubernetes Operator makes it easy to connect your Kubernetes clusters with Netmaker WireGuard networks. With just a few commands, you can:

- Install the operator using Helm
- Create proxies to bridge Kubernetes and Netmaker services
- Enable secure, encrypted communication across your infrastructure

Start with the simple examples above, then explore the advanced features as your needs grow. Happy networking!

