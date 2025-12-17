# Cross-Cluster Connectivity Guide

This guide demonstrates how to set up cross-cluster connectivity between Kubernetes clusters using Netmaker. You'll learn how to:

1. Expose a database in Cluster A to the Netmaker network
2. Connect an application server in Cluster B to that database
3. Test and verify the connectivity

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                    Netmaker Network (VPN)                        │
│                                                                   │
│  ┌──────────────────────┐         ┌──────────────────────┐      │
│  │  Cluster A           │         │  Cluster B           │      │
│  │  (Database)          │         │  (Application)       │      │
│  │                      │         │                      │      │
│  │  ┌──────────────┐   │         │  ┌──────────────┐   │      │
│  │  │ PostgreSQL   │   │         │  │ App Server   │   │      │
│  │  │ Database     │   │         │  │              │   │      │
│  │  └──────┬───────┘   │         │  └──────┬───────┘   │      │
│  │         │            │         │         │            │      │
│  │  ┌─────▼──────┐     │         │  ┌─────▼──────┐     │      │
│  │  │ Ingress    │     │         │  │ Egress     │     │      │
│  │  │ Proxy      │     │         │  │ Proxy      │     │      │
│  │  │ (exposes)  │     │         │  │ (accesses) │     │      │
│  │  └─────┬──────┘     │         │  └─────┬──────┘     │      │
│  │        │             │         │        │            │      │
│  │  ┌─────▼──────┐     │         │  ┌─────▼──────┐     │      │
│  │  │ Netclient  │     │         │  │ Netclient  │     │      │
│  │  │ Sidecar    │     │         │  │ Sidecar    │     │      │
│  │  └─────┬──────┘     │         │  └─────┬──────┘     │      │
│  └────────┼─────────────┘         └────────┼─────────────┘      │
│           │                                │                    │
│           └────────── WireGuard ───────────┘                    │
└─────────────────────────────────────────────────────────────────┘
```

## Prerequisites

1. **Two Kubernetes Clusters** (Cluster A and Cluster B)
2. **Netmaker Network**: A configured Netmaker network with both clusters joined
3. **Netmaker K8s Operator**: Installed in both clusters with netclient sidecar enabled
4. **Netmaker Tokens**: Valid tokens for both clusters to join the Netmaker network

## Step-by-Step Setup

### Step 1: Deploy Database in Cluster A

Deploy the database and expose it to the Netmaker network using an **ingress proxy**:

```bash
# Apply the database example to Cluster A
kubectl apply -f examples/cross-cluster-database-example.yaml --context=cluster-a
```

This creates:
- PostgreSQL StatefulSet
- PersistentVolumeClaim for data storage
- Secret for database credentials
- ClusterIP Service for internal access
- **Ingress Proxy Service** that exposes the database to Netmaker network

**Key Configuration:**
```yaml
annotations:
  netmaker.io/ingress: "enabled"
  netmaker.io/ingress-dns-name: "postgres.cluster-a.netmaker.internal"
```

### Step 2: Verify Database is Exposed

Check that the ingress proxy is running in Cluster A:

```bash
# Check ingress proxy pod
kubectl get pods -l app=netmaker-ingress-proxy --context=cluster-a

# Get the Netmaker IP assigned to the ingress proxy
kubectl exec -it <ingress-proxy-pod> -c netclient -- ip addr show netmaker --context=cluster-a

# Test from within Cluster A
kubectl exec -it postgres-test-client -- psql -h postgres-db.default.svc.cluster.local -U postgres -d mydb --context=cluster-a
```

### Step 3: Deploy Application Server in Cluster B

Deploy the application server and configure it to access the database via an **egress proxy**:

```bash
# Apply the server example to Cluster B
kubectl apply -f examples/cross-cluster-server-example.yaml --context=cluster-b
```

This creates:
- Application Server Deployment
- Secret for database credentials
- ClusterIP Service for the application
- **Egress Proxy Service** that routes traffic to Cluster A's database

**Key Configuration:**
```yaml
annotations:
  netmaker.io/egress: "enabled"
  netmaker.io/egress-target-dns: "postgres.cluster-a.netmaker.internal"
```

### Step 4: Verify Application Can Connect

Test the connection from Cluster B:

```bash
# Check egress proxy pod
kubectl get pods -l app=netmaker-egress-proxy --context=cluster-b

# Test database connection from Cluster B
kubectl exec -it db-test-client -- psql -h postgres-cluster-a.default.svc.cluster.local -U postgres -d mydb --context=cluster-b

# Test application server endpoints
kubectl exec -it app-test-client -- curl http://app-server.default.svc.cluster.local/health --context=cluster-b
kubectl exec -it app-test-client -- curl http://app-server.default.svc.cluster.local/db-test --context=cluster-b
```

## How It Works

### Ingress Proxy (Cluster A)

The ingress proxy:
1. Runs a netclient sidecar that connects to the Netmaker network
2. Receives a Netmaker IP address (e.g., `10.0.0.50`)
3. Listens on that IP for incoming connections
4. Forwards traffic to the PostgreSQL ClusterIP service
5. Makes the database accessible to any device on the Netmaker network

**Traffic Flow:**
```
Netmaker Network → Ingress Proxy (10.0.0.50:5432) → PostgreSQL Service → PostgreSQL Pod
```

### Egress Proxy (Cluster B)

The egress proxy:
1. Runs a netclient sidecar that connects to the Netmaker network
2. Creates a Kubernetes Service that acts as a proxy
3. Routes traffic from Cluster B pods to the database in Cluster A
4. Uses the Netmaker DNS name or IP to reach Cluster A's ingress proxy

**Traffic Flow:**
```
App Pod → Egress Proxy Service → Egress Proxy Pod → Netmaker Network → Ingress Proxy (Cluster A) → Database
```

## Configuration Options

### Using DNS Names (Recommended)

**Cluster A (Ingress):**
```yaml
netmaker.io/ingress-dns-name: "postgres.cluster-a.netmaker.internal"
```

**Cluster B (Egress):**
```yaml
netmaker.io/egress-target-dns: "postgres.cluster-a.netmaker.internal"
```

### Using IP Addresses

**Cluster A (Ingress):**
```yaml
netmaker.io/ingress-bind-ip: "10.0.0.50"  # Optional, auto-detected if not specified
```

**Cluster B (Egress):**
```yaml
netmaker.io/egress-target-ip: "10.0.0.50"  # IP of Cluster A's ingress proxy
```

## Connection String Examples

### From Cluster B Application

```javascript
// Using Kubernetes Service DNS (via egress proxy)
const connectionString = 'postgresql://postgres:changeme123@postgres-cluster-a.default.svc.cluster.local:5432/mydb';
```

### From External Netmaker Device

```bash
# Using Netmaker DNS
psql -h postgres.cluster-a.netmaker.internal -U postgres -d mydb

# Using Netmaker IP
psql -h 10.0.0.50 -U postgres -d mydb
```

## Troubleshooting

### Database Not Accessible from Cluster B

1. **Verify Netmaker Connectivity:**
   ```bash
   # In Cluster A ingress proxy
   kubectl exec -it <ingress-proxy-pod> -c netclient -- ping <cluster-b-netmaker-ip> --context=cluster-a
   
   # In Cluster B egress proxy
   kubectl exec -it <egress-proxy-pod> -c netclient -- ping <cluster-a-netmaker-ip> --context=cluster-b
   ```

2. **Check DNS Resolution:**
   ```bash
   # In Cluster B egress proxy
   kubectl exec -it <egress-proxy-pod> -c netclient -- nslookup postgres.cluster-a.netmaker.internal --context=cluster-b
   ```

3. **Verify Service Endpoints:**
   ```bash
   # Check ingress proxy endpoints in Cluster A
   kubectl get endpoints postgres-db-ingress --context=cluster-a
   
   # Check egress proxy endpoints in Cluster B
   kubectl get endpoints postgres-cluster-a --context=cluster-b
   ```

4. **Check Proxy Logs:**
   ```bash
   # Ingress proxy logs (Cluster A)
   kubectl logs -l app=netmaker-ingress-proxy -c proxy --context=cluster-a
   
   # Egress proxy logs (Cluster B)
   kubectl logs -l app=netmaker-egress-proxy -c proxy --context=cluster-b
   ```

### Connection Timeouts

1. **Verify Firewall Rules:** Ensure Netmaker network allows traffic between clusters
2. **Check Port Configuration:** Ensure port 5432 is correctly configured in both proxies
3. **Verify Database is Listening:** Check that PostgreSQL is listening on the correct interface

### DNS Resolution Issues

1. **Use IP Instead of DNS:** Temporarily use `netmaker.io/egress-target-ip` instead of DNS
2. **Check Netmaker DNS Configuration:** Verify DNS is properly configured in Netmaker
3. **Verify DNS Name Matches:** Ensure ingress DNS name matches egress target DNS

## Security Considerations

1. **Database Credentials:** Use Kubernetes Secrets, never hardcode passwords
2. **Network Policies:** Consider implementing NetworkPolicies to restrict access
3. **TLS/SSL:** Enable TLS for database connections in production
4. **Firewall Rules:** Configure Netmaker firewall rules to restrict access
5. **RBAC:** Use proper RBAC to limit who can create/modify proxy services

## Production Best Practices

1. **Use StatefulSets for Databases:** Ensures stable network identities
2. **Persistent Storage:** Use PersistentVolumeClaims for database data
3. **Backup Strategy:** Implement regular database backups
4. **Monitoring:** Monitor proxy health and database connections
5. **Resource Limits:** Set appropriate resource limits for all components
6. **High Availability:** Consider multiple replicas for critical services
7. **Connection Pooling:** Use connection pooling in application servers

## Example Use Cases

1. **Multi-Region Deployment:** Database in one region, applications in others
2. **Hybrid Cloud:** Database on-premises, applications in cloud
3. **Development/Production:** Share development database across clusters
4. **Disaster Recovery:** Replicate data across clusters
5. **Microservices:** Services in different clusters accessing shared databases

## See Also

- [Ingress Proxy Guide](INGRESS_PROXY_GUIDE.md) - Expose K8s services to Netmaker
- [Egress Proxy Guide](EGRESS_PROXY_GUIDE.md) - Access Netmaker services from K8s
- [Netclient Sidecar Usage](NETCLIENT_SIDECAR_USAGE.md) - Direct pod connectivity
- [WireGuard Setup Guide](../WIREGUARD_SETUP.md) - Netmaker network setup

