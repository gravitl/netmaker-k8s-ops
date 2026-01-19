# Netmaker Kubernetes Operator

A Kubernetes operator that seamlessly integrates Netmaker WireGuard networks with Kubernetes clusters, enabling secure bidirectional connectivity between Kubernetes workloads and Netmaker networks.

## Overview

The Netmaker K8s Operator provides multiple features to bridge Kubernetes clusters with Netmaker WireGuard networks:

- **Egress Proxy**: Expose Netmaker services to Kubernetes cluster workloads
- **Ingress Proxy**: Expose Kubernetes services to Netmaker network devices
- **API Proxy**: Secure access to Kubernetes API through WireGuard tunnels

## Use Cases

### 1. Egress Proxy (Cluster Egress)

Expose services that are external to your Kubernetes cluster but available in your Netmaker network, making them accessible to your Kubernetes workloads.

**Use Case**: Allow Kubernetes applications to access Netmaker services (APIs, databases, etc.) using standard Kubernetes Service names.

**Example**:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: netmaker-api-egress
  annotations:
    netmaker.io/egress: "enabled"
    netmaker.io/egress-target-ip: "100.93.135.2"
spec:
  ports:
  - port: 80
    targetPort: 8080  # Port on Netmaker device
```

**How it works**:
- Creates a proxy pod with netclient sidecar
- Proxy listens on Kubernetes Service port
- Forwards traffic to Netmaker device via WireGuard
- Kubernetes workloads access via Service name

**Documentation**: [Egress Proxy Guide](examples/EGRESS_PROXY_GUIDE.md)

### 2. Ingress Proxy (Cluster Ingress)

Expose Kubernetes services to devices on your Netmaker network, allowing Netmaker devices to access Kubernetes workloads.

**Use Case**: Enable Netmaker network devices to access Kubernetes services (APIs, databases, web apps) using Netmaker IPs or DNS names.

**Example**:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-api-ingress
  annotations:
    netmaker.io/ingress: "enabled"
spec:
  ports:
  - port: 80
    targetPort: 8080
  selector:
    app: my-api
```

**How it works**:
- Creates a proxy pod with netclient sidecar
- Proxy listens on Netmaker network IP (WireGuard interface)
- Forwards traffic to Kubernetes Service
- Netmaker devices access via Netmaker IP or DNS

**Documentation**: [Ingress Proxy Guide](examples/INGRESS_PROXY_GUIDE.md)

### 3. API Proxy

Secure reverse proxy for accessing Kubernetes API servers through WireGuard tunnels with user impersonation and RBAC support.

**Use Case**: Remote access to Kubernetes clusters via WireGuard with authentication and authorization.

**Documentation**: [Proxy Usage Guide](PROXY_USAGE.md)

## Key Differences: Ingress vs Egress

| Feature | **Ingress Proxy** | **Egress Proxy** |
|---------|------------------|------------------|
| **Direction** | Netmaker → Kubernetes | Kubernetes → Netmaker |
| **Use Case** | Expose K8s services to Netmaker network | Access Netmaker services from K8s |
| **Proxy Listens On** | Netmaker network IP (WireGuard interface) | Kubernetes Service port |
| **Proxy Forwards To** | Kubernetes Service (ClusterIP) | Netmaker device (IP/DNS) |
| **Access Method** | Netmaker devices use Netmaker IP/DNS | K8s workloads use Service name |
| **Annotation** | `netmaker.io/ingress: "enabled"` | `netmaker.io/egress: "enabled"` |
| **Target Config** | `netmaker.io/ingress-dns-name` (optional) | `netmaker.io/egress-target-ip` or `netmaker.io/egress-target-dns` |
| **Port Config** | Uses Service `port` and `targetPort` | Uses Service `targetPort` (port on Netmaker device) |

### Visual Comparison

**Egress Proxy Flow** (K8s → Netmaker):
```
K8s Pod → K8s Service → Egress Proxy Pod → WireGuard → Netmaker Device
```

**Ingress Proxy Flow** (Netmaker → K8s):
```
Netmaker Device → WireGuard → Ingress Proxy Pod → K8s Service → K8s Pod
```

## Quick Start

### Prerequisites

- Go v1.22.0+
- Docker v17.03+
- kubectl v1.11.3+
- Kubernetes cluster v1.11.3+
- Netmaker server with network configured
- Netmaker token for joining the network

### Installation

#### Option 1: Helm Chart (Recommended)

**Install from Helm repository (recommended):**
```bash
# Add the Helm repository (replace with your DigitalOcean Spaces endpoint)
helm repo add netmaker-k8s-ops https://downloads.netmaker.io/charts/
helm repo update

# Install the chart
helm install netmaker-k8s-ops netmaker-k8s-ops/netmaker-k8s-ops \
  --namespace netmaker-k8s-ops-system \
  --create-namespace \
  --version 1.0.0 \
  --set image.repository=gravitl/netmaker-k8s-ops \
  --set image.tag=latest \
  --set netclient.token="YOUR_NETMAKER_TOKEN_HERE"
```


**Or install from local chart:**
```bash
# Basic installation with default values
# Note: If namespace already exists, either omit --create-namespace or set namespace.create=false
helm install netmaker-k8s-ops netmaker-k8s-ops/netmaker-k8s-ops \
  --namespace netmaker-k8s-ops-system \
  --create-namespace \
  --set image.repository=gravitl/netmaker-k8s-ops \
  --set image.tag=latest \
  --set netclient.token="YOUR_NETMAKER_TOKEN_HERE"
```

   **If namespace already exists**, use:
```bash
helm install netmaker-k8s-ops netmaker-k8s-ops/netmaker-k8s-ops \
  --namespace netmaker-k8s-ops-system \
  --set namespace.create=false \
  --set image.repository=gravitl/netmaker-k8s-ops \
  --set image.tag=latest \
  --set netclient.token="YOUR_NETMAKER_TOKEN_HERE"
```

   **With K8s Proxy configuration (PRO netmaker server needed)** (need netmaker API integration in auth mode for users sync):

   **Auth MODE**
```bash
helm install netmaker-k8s-ops netmaker-k8s-ops/netmaker-k8s-ops \
  --namespace netmaker-k8s-ops-system \
  --create-namespace \
  --set image.repository=gravitl/netmaker-k8s-ops \
  --set image.tag=latest \
  --set netclient.token="YOUR_NETMAKER_TOKEN_HERE" \
  --set manager.configMap.proxyMode="auth" \
  --set service.proxy.enabled=true \
  --set api.enabled=true \
  --set api.serverDomain="api.example.com" \
  --set api.token="your-api-token-here" \
  --set api.syncInterval="10"
```
  **NOAUTH MODE**
helm install netmaker-k8s-ops netmaker-k8s-ops/netmaker-k8s-ops \
  --namespace netmaker-k8s-ops-system \
  --create-namespace \
  --set image.repository=gravitl/netmaker-k8s-ops \
  --set image.tag=latest \
  --set netclient.token="YOUR_NETMAKER_TOKEN_HERE" \
  --set manager.configMap.proxyMode="noauth" \
  --set service.proxy.enabled=true
```
   **Or using a values file** (for better organization):
```bash
# Create a values file (values-custom.yaml)
cat > values-custom.yaml <<EOF
image:
  repository: <your-registry>/netmaker-k8s-ops
  tag: <tag>
netclient:
  token: "YOUR_NETMAKER_TOKEN_HERE"
manager:
  env:
    - name: IN_CLUSTER
      value: "true"
    - name: ENABLE_LEADER_ELECTION
      value: "true"
    - name: PROXY_SKIP_TLS_VERIFY
      value: "true"
    - name: PROXY_MODE
      value: "auth"
    - name: API_SERVER_DOMAIN
      value: "api.example.com"
    - name: API_TOKEN
      value: "your-api-token-here"
    - name: API_SYNC_INTERVAL
      value: "300"
EOF

helm install netmaker-k8s-ops ./deploy/netmaker-k8s-ops \
  --namespace netmaker-k8s-ops-system \
  --create-namespace \
  --values values-custom.yaml
```



2. **Using Kubernetes Secret for token** (recommended for production):
```bash
# Create secret
kubectl create secret generic netclient-token \
  --from-literal=token="YOUR_NETMAKER_TOKEN_HERE" \
  --namespace netmaker-k8s-ops-system

# Install with secret reference (requires modifying values.yaml or using --set-file)
helm install netmaker-k8s-ops ./deploy/netmaker-k8s-ops \
  --namespace netmaker-k8s-ops-system \
  --create-namespace \
  --set image.repository=<your-registry>/netmaker-k8s-ops \
  --set image.tag=<tag>
```

   Then update the deployment to use the secret (see [Token Configuration Guide](TOKEN_CONFIGURATION.md)).

3. **Verify deployment**:
```bash
kubectl get pods -n netmaker-k8s-ops-system
helm status netmaker-k8s-ops -n netmaker-k8s-ops-system
```

**Uninstall with Helm**:
```bash
helm uninstall netmaker-k8s-ops -n netmaker-k8s-ops-system
```

**Note**: CRDs are NOT automatically removed by Helm uninstall (this is Helm's default behavior to prevent data loss). To remove CRDs manually:
```bash
kubectl delete crd netmakerops.network.netmaker.io
```

#### Option 2: Make/Kustomize

1. **Build and push the operator image**:
```bash
make docker-build docker-push IMG=<your-registry>/netmaker-k8s-ops:tag
```

2. **Install CRDs**:
```bash
make install
```

3. **Deploy the operator**:
```bash
make deploy IMG=<your-registry>/netmaker-k8s-ops:tag
```

4. **Configure Netmaker token**:

   **Option A: Direct environment variable** (quick start, not recommended for production):
   ```yaml
   # In config/manager/manager.yaml, netclient container section
   env:
   - name: TOKEN
     value: "YOUR_NETMAKER_TOKEN_HERE"
   ```

   **Option B: Kubernetes Secret** (recommended for production):
   ```bash
   # Create secret
   kubectl create secret generic netclient-token \
     --from-literal=token="YOUR_NETMAKER_TOKEN_HERE" \
     --namespace=netmaker-k8s-ops-system
   
   # Update config/manager/manager.yaml to use secret:
   env:
   - name: TOKEN
     valueFrom:
       secretKeyRef:
         name: netclient-token
         key: token
   ```

   See [Token Configuration Guide](TOKEN_CONFIGURATION.md) for all methods and best practices.

5. **Verify deployment**:
```bash
kubectl get pods -n netmaker-k8s-ops-system
```


### Example: Egress Proxy

Create a Service to access a Netmaker API:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: netmaker-api-egress
  annotations:
    netmaker.io/egress: "enabled"
    netmaker.io/egress-target-dns: "api.netmaker.internal"
spec:
  ports:
  - port: 80
    targetPort: 8080
```

Access from Kubernetes pods:
```bash
curl http://netmaker-api-egress:80
```

### Example: Ingress Proxy

Expose a Kubernetes service to Netmaker network:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-api-ingress
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

Access from Netmaker devices:
```bash
curl http://api.k8s.netmaker.internal:80
```

## Documentation

- **[User Guide](docs/USER_GUIDE.md)** - Start here! Introduction and getting started guide for new users
- [Token Configuration Guide](TOKEN_CONFIGURATION.md) - How to pass Netmaker tokens (secrets, env vars, etc.)
- [Egress Proxy Guide](examples/EGRESS_PROXY_GUIDE.md) - Expose Netmaker services to K8s
- [Ingress Proxy Guide](examples/INGRESS_PROXY_GUIDE.md) - Expose K8s services to Netmaker
- [Contributing Guide](CONTRIBUTING.md) - Guidelines for contributing to the project

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Netmaker Network                          │
│                                                               │
│  ┌──────────────┐         ┌──────────────┐                  │
│  │ Netmaker     │         │ Netmaker     │                  │
│  │ Device 1     │         │ Device 2     │                  │
│  └──────┬───────┘         └──────┬───────┘                  │
└─────────┼────────────────────────┼───────────────────────────┘
          │                        │ WireGuard Tunnel
          │                        │
┌─────────┼────────────────────────┼───────────────────────────┐
│         │                        │                           │
│  ┌──────▼────────────────────────▼───────┐                  │
│  │   Kubernetes Cluster                  │                  │
│  │                                        │                  │
│  │  ┌──────────────────────────────────┐ │                  │
│  │  │  Ingress Proxy Pod               │ │                  │
│  │  │  (netclient + socat)             │ │                  │
│  │  └──────────┬───────────────────────┘ │                  │
│  │             │                          │                  │
│  │  ┌──────────▼───────────────────────┐ │                  │
│  │  │  Kubernetes Service              │ │                  │
│  │  └──────────┬───────────────────────┘ │                  │
│  │             │                          │                  │
│  │  ┌──────────▼───────────────────────┐ │                  │
│  │  │  Application Pods                │ │                  │
│  │  │  (with optional netclient)       │ │                  │
│  │  └──────────────────────────────────┘ │                  │
│  │                                        │                  │
│  │  ┌──────────────────────────────────┐ │                  │
│  │  │  Egress Proxy Pod                │ │                  │
│  │  │  (netclient + socat)             │ │                  │
│  │  └──────────────────────────────────┘ │                  │
│  └────────────────────────────────────────┘                  │
└─────────────────────────────────────────────────────────────┘
```

## Features

- ✅ **Egress Proxy**: Access Netmaker services from Kubernetes
- ✅ **Ingress Proxy**: Expose Kubernetes services to Netmaker
- ✅ **RBAC Integration**: Full Kubernetes RBAC support


## Uninstallation

### Helm Installation

**Uninstall using Helm**:
```bash
helm uninstall netmaker-k8s-ops -n netmaker-k8s-ops-system
```

**Delete CRDs** (optional, CRDs are NOT automatically removed by Helm uninstall to prevent data loss):
```bash
kubectl delete crd netmakerops.network.netmaker.io
```

### Make/Kustomize Installation

**Delete CR instances**:
```bash
kubectl delete -k config/samples/
```

**Delete CRDs**:
```bash
make uninstall
```

**Undeploy the operator**:
```bash
make undeploy
```

## Getting Started

### Prerequisites
- go version v1.22.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/netmaker-k8s-ops:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/netmaker-k8s-ops:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following are the steps to build the installer and distribute this project.

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/netmaker-k8s-ops:tag
```

NOTE: The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without
its dependencies.

2. Using the installer

Users can just run kubectl apply -f <URL for YAML BUNDLE> to install the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/netmaker-k8s-ops/<tag or branch>/dist/install.yaml
```

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details on:

- How to get started with development
- Code style and conventions
- Testing requirements
- Pull request process
- And more!

Contributions of all kinds are appreciated - code, documentation, bug reports, feature requests, and feedback.

## License

Copyright 2025 Netmaker, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

See the [LICENSE](LICENSE) file for the full license text.

