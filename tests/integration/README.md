# CloudForge Integration Tests

Run the full local flow from the repository root:

```bash
make integration-test
```

The target reuses the existing local development stack:

- Docker Compose for ScyllaDB, OpenBao, and Keycloak
- `cloudforge-dev` k3d cluster
- Cilium, Envoy Gateway, vCluster, cert-manager, and the CloudForge namespace

The tests start `cf-accounts`, `cf-provisioner`, and `cf-router` as real local
Go processes on temporary ports, then drive public HTTP calls through
CF-Router. They are behind the `integration` build tag and `CF_INTEGRATION=1`,
so normal unit tests do not require Docker or k3d.
