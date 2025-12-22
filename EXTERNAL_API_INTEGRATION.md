# External API Integration

This document describes how to integrate the Netmaker K8s Proxy with an external API to automatically fetch user IP mappings.

## Overview

The proxy can automatically fetch user IP mappings from an external API endpoint `/api/users/network_ip` and periodically sync them. This allows for centralized management of user mappings without manual configuration.

## Configuration

### Environment Variables

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `API_SERVER_DOMAIN` | Domain of the external API server | - | Yes |
| `API_TOKEN` | Bearer token for API authentication | - | Yes |
| `API_SYNC_INTERVAL` | How often to sync with external API (seconds) | `300` | No |

### Example Configuration

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: netmaker-k8s-ops
spec:
  template:
    spec:
      containers:
      - name: manager
        env:
        - name: API_SERVER_DOMAIN
          value: "api.example.com"
        - name: API_TOKEN
          value: "your-api-token-here"
        - name: API_SYNC_INTERVAL
          value: "300"  # seconds
```

## External API Specification

### Endpoint

**GET** `https://{API_SERVER_DOMAIN}/api/users/network_ip`

### Authentication

The proxy sends a Bearer token in the Authorization header:

```
Authorization: Bearer {API_TOKEN}
```

### Request Headers

```
Authorization: Bearer {API_TOKEN}
Content-Type: application/json
```

### Response Format

The API should return a JSON response with the following structure:

```json
{
  "mappings": {
    "10.0.0.1": {
      "user": "alice",
      "groups": ["system:authenticated", "developers"]
    },
    "10.0.0.2": {
      "user": "bob",
      "groups": ["system:authenticated", "admins"]
    }
  }
}
```

### Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `mappings` | Object | Map of IP addresses to user mappings |
| `mappings[ip].user` | String | Kubernetes username to impersonate |
| `mappings[ip].groups` | Array | List of groups to assign to the user |

## How It Works

### Automatic Sync

1. **Startup**: On proxy startup, fetches initial user mappings from external API
2. **Periodic Sync**: Every `API_SYNC_INTERVAL`, fetches updated mappings
3. **Update Mappings**: Replaces all existing mappings with the new ones from API
4. **Impersonation**: Incoming requests use the latest mappings for user impersonation

### Manual Sync

You can manually trigger a sync using the admin API:

```bash
curl -X POST http://localhost:8085/admin/sync-external-api
```

### Fallback Behavior

- If external API is not configured, proxy uses default user/groups
- If API is unavailable, proxy continues with existing mappings
- If API returns invalid data, proxy logs error and continues with existing mappings

## Example Implementations

### Simple Node.js API

```javascript
const express = require('express');
const app = express();

// Mock user data
const mappings = {
  '10.0.0.1': { user: 'alice', groups: ['system:authenticated', 'developers'] },
  '10.0.0.2': { user: 'bob', groups: ['system:authenticated', 'admins'] },
  '10.0.0.3': { user: 'charlie', groups: ['system:authenticated', 'readonly-users'] }
};

// Middleware to verify Bearer token
const verifyToken = (req, res, next) => {
  const token = req.headers.authorization?.replace('Bearer ', '');
  if (token !== process.env.API_TOKEN) {
    return res.status(401).json({ error: 'Unauthorized' });
  }
  next();
};

// Endpoint for user mappings
app.get('/api/users/network_ip', verifyToken, (req, res) => {
  res.json({ mappings });
});

app.listen(3000, () => {
  console.log('API server running on port 3000');
});
```

### Python Flask API

```python
from flask import Flask, jsonify, request
import os

app = Flask(__name__)

# Mock user data
mappings = {
    "10.0.0.1": {"user": "alice", "groups": ["system:authenticated", "developers"]},
    "10.0.0.2": {"user": "bob", "groups": ["system:authenticated", "admins"]},
    "10.0.0.3": {"user": "charlie", "groups": ["system:authenticated", "readonly-users"]}
}

def verify_token(f):
    def decorated_function(*args, **kwargs):
        token = request.headers.get('Authorization', '').replace('Bearer ', '')
        if token != os.getenv('API_TOKEN'):
            return jsonify({'error': 'Unauthorized'}), 401
        return f(*args, **kwargs)
    return decorated_function

@app.route('/api/users/network_ip', methods=['GET'])
@verify_token
def get_user_mappings():
    return jsonify({"mappings": mappings})

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=3000)
```

### Go HTTP Server

```go
package main

import (
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
)

type UserMapping struct {
    IP     string   `json:"ip"`
    User   string   `json:"user"`
    Groups []string `json:"groups"`
}

type APIResponse struct {
    Users []UserMapping `json:"users"`
}

func main() {
    // Mock user data
    mappings := map[string]UserMapping{
        "10.0.0.1": {User: "alice", Groups: []string{"system:authenticated", "developers"}},
        "10.0.0.2": {User: "bob", Groups: []string{"system:authenticated", "admins"}},
        "10.0.0.3": {User: "charlie", Groups: []string{"system:authenticated", "readonly-users"}},
    }

    http.HandleFunc("/api/users/network_ip", func(w http.ResponseWriter, r *http.Request) {
        // Verify Bearer token
        token := r.Header.Get("Authorization")
        if token != "Bearer "+os.Getenv("API_TOKEN") {
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]interface{}{"mappings": mappings})
    })

    log.Fatal(http.ListenAndServe(":3000", nil))
}
```

## Integration with Existing Systems

### Netmaker Integration

```bash
#!/bin/bash
# Script to create an API endpoint that serves Netmaker peer data

# Get peers from Netmaker API
PEERS=$(curl -s "https://your-netmaker-server/api/v1/networks/your-network/peers" \
  -H "Authorization: Bearer $NETMAKER_TOKEN")

# Transform to the expected format
echo "$PEERS" | jq '{
  users: [.[] | {
    ip: .address,
    user: .name,
    groups: ["system:authenticated", "wireguard-peers"]
  }]
}'
```

### LDAP Integration

```python
import ldap
from flask import Flask, jsonify, request
import os

app = Flask(__name__)

def get_ldap_users():
    # Connect to LDAP
    conn = ldap.initialize('ldap://your-ldap-server')
    conn.simple_bind_s('cn=admin,dc=example,dc=com', 'password')
    
    # Search for users with IP addresses
    result = conn.search_s('ou=users,dc=example,dc=com', ldap.SCOPE_SUBTREE, 
                          '(objectClass=person)', ['cn', 'ipAddress', 'memberOf'])
    
    users = []
    for dn, attrs in result:
        if 'ipAddress' in attrs:
            user = {
                'ip': attrs['ipAddress'][0].decode(),
                'user': attrs['cn'][0].decode(),
                'groups': ['system:authenticated', 'ldap-users']
            }
            users.append(user)
    
    return users

@app.route('/api/users/network_ip', methods=['GET'])
def get_user_mappings():
    # Verify token
    token = request.headers.get('Authorization', '').replace('Bearer ', '')
    if token != os.getenv('API_TOKEN'):
        return jsonify({'error': 'Unauthorized'}), 401
    
    users = get_ldap_users()
    return jsonify({'users': users})

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=3000)
```

## Monitoring and Debugging

### Check Sync Status

```bash
# Check if external API is configured
kubectl exec deployment/netmaker-k8s-ops -- env | grep API_

# View proxy logs for sync activity
kubectl logs -f deployment/netmaker-k8s-ops | grep "external API"

# Check current mappings
curl http://localhost:8085/admin/user-mappings
```

### Manual Sync

```bash
# Trigger manual sync
curl -X POST http://localhost:8085/admin/sync-external-api

# Check sync result
curl http://localhost:8085/admin/user-mappings
```

### Test External API

```bash
# Test external API directly
curl -H "Authorization: Bearer your-token" \
     https://api.example.com/api/users/network_ip
```

## Error Handling

### Common Issues

1. **API Not Configured**: Check `API_SERVER_DOMAIN` and `API_TOKEN`
2. **Authentication Failed**: Verify API token is correct
3. **Network Issues**: Check connectivity to external API server
4. **Invalid Response**: Ensure API returns correct JSON format

### Debug Commands

```bash
# Check proxy configuration
kubectl exec deployment/netmaker-k8s-ops -- env | grep API_

# View sync logs
kubectl logs deployment/netmaker-k8s-ops | grep "external API"

# Test API connectivity from proxy pod
kubectl exec deployment/netmaker-k8s-ops -- curl -H "Authorization: Bearer $API_TOKEN" \
  https://$API_SERVER_DOMAIN/api/users/network_ip
```

## Security Considerations

1. **API Token Security**: Store API tokens in Kubernetes secrets
2. **TLS Verification**: Consider enabling TLS verification for production
3. **Network Security**: Ensure external API is accessible from proxy
4. **Token Rotation**: Implement token rotation for long-running deployments

### Using Kubernetes Secrets

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: external-api-secret
type: Opaque
data:
  token: <base64-encoded-token>

---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: netmaker-k8s-ops
spec:
  template:
    spec:
      containers:
      - name: manager
        env:
        - name: API_TOKEN
          valueFrom:
            secretKeyRef:
              name: external-api-secret
              key: token
```

## Performance Considerations

1. **Sync Frequency**: Balance between freshness and API load
2. **Caching**: Mappings are cached in memory between syncs
3. **Error Handling**: Failed syncs don't affect existing mappings
4. **Concurrent Access**: Thread-safe mapping updates

## Troubleshooting

### Sync Not Working

1. Check environment variables are set correctly
2. Verify API endpoint is accessible
3. Check authentication token is valid
4. Review proxy logs for error messages

### Mappings Not Applied

1. Verify API returns correct format
2. Check IP addresses match exactly
3. Ensure proxy is in auth mode
4. Review RBAC configuration

### Performance Issues

1. Increase sync interval if API is slow
2. Optimize external API response
3. Monitor memory usage
4. Check network latency

## Next Steps

1. **Implement API**: Create your external API endpoint
2. **Configure Proxy**: Set environment variables
3. **Test Integration**: Verify sync is working
4. **Monitor**: Set up monitoring and alerting
5. **Scale**: Consider multiple proxy instances
