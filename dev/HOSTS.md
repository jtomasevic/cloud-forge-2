# Local DNS Setup

Add these entries to `/etc/hosts` for local development:

```text
127.0.0.1  api.cloudforge.local
127.0.0.1  auth.cloudforge.local
127.0.0.1  gateway.cloudforge.local
```

After applying Envoy Gateway resources, local access is:

- CloudForge API: `http://api.cloudforge.local:18080/v1/accounts`
- CloudForge Swagger: `http://api.cloudforge.local:18080/swagger/`
- Keycloak admin: `http://auth.cloudforge.local:18080/auth/admin`

HTTPS is also available on the k3d load balancer HTTPS port:

- CloudForge API: `https://api.cloudforge.local:18443/v1/accounts`
- CloudForge Swagger: `https://api.cloudforge.local:18443/swagger/`
- Keycloak admin: `https://auth.cloudforge.local:18443/auth/admin`

The HTTPS certificate is self-signed for local development, so clients may need
to trust the local cert-manager certificate or use `curl -k`.

Setup flow:

```bash
make dev
```

To prepare the cluster and gateway resources without starting Tilt, run
`make dev-setup`. To reapply only the Envoy Gateway resources, run
`make gateway-apply`.

The k3d HTTP/HTTPS ports default to `18080` and `18443`; override them with
`CF_K3D_LB_HTTP_PORT` and `CF_K3D_LB_HTTPS_PORT` before creating the cluster.
