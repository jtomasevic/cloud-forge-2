# CloudForge Tiltfile
# Modes: CF_DEV_MODE=local (default) | CF_DEV_MODE=k8s

dev_mode = os.getenv("CF_DEV_MODE", "local")
host_kubeconfig = os.getenv("HOST_KUBECONFIG", os.getenv("HOME", "") + "/.kube/config")

if dev_mode not in ["local", "k8s"]:
    fail("CF_DEV_MODE must be either 'local' or 'k8s', got: %s" % dev_mode)

# Backing services (Docker Compose).
docker_compose("dev/docker-compose.yml")

dc_resource("scylladb", labels=["backing-services"])
dc_resource("openbao", labels=["backing-services"])
dc_resource("keycloak", labels=["backing-services"])

# Schema migrations and OpenBao setup.
local_resource(
    "db-migrate",
    cmd="dev/scripts/init-scylladb.sh",
    deps=[
        "dev/scripts/init-scylladb.sh",
        "tools/migrations/main.go",
        "tools/migrations/Makefile",
        "tools/migrations/scripts",
        "libs/scylladb/pkg/migrate",
    ],
    resource_deps=["scylladb"],
    labels=["infra"],
)

local_resource(
    "openbao-init",
    cmd="dev/scripts/init-openbao.sh",
    deps=["dev/scripts/init-openbao.sh"],
    resource_deps=["openbao"],
    labels=["infra"],
)

if dev_mode == "local":
    local_resource(
        "cf-accounts",
        serve_cmd="go run ./services/cf-accounts/...",
        deps=[
            "services/cf-accounts",
            "libs/cloudforge-core",
            "libs/scylladb",
        ],
        env={
            "HTTP_ADDR": ":8081",
            "SCYLLADB_HOSTS": "localhost:9042",
            "SCYLLADB_KEYSPACE": "cloudforge",
        },
        resource_deps=["db-migrate"],
        labels=["cf-services"],
        readiness_probe=probe(
            http_get=http_get_action(port=8081, path="/v1/accounts"),
            period_secs=3,
        ),
    )

    local_resource(
        "cf-provisioner",
        serve_cmd="go run ./services/cf-provisioner/...",
        deps=[
            "services/cf-provisioner",
            "libs/cloudforge-core",
            "libs/scylladb",
            "libs/openbao",
        ],
        env={
            "HTTP_ADDR": ":8082",
            "SCYLLADB_HOSTS": "localhost:9042",
            "SCYLLADB_KEYSPACE": "cloudforge",
            "OPENBAO_ADDR": "http://localhost:8200",
            "OPENBAO_TOKEN": "dev-root-token",
            "CF_INTERNAL_SECRET": "dev-internal-secret",
            "HOST_KUBECONFIG": host_kubeconfig,
        },
        resource_deps=["db-migrate", "openbao-init"],
        labels=["cf-services"],
    )

    local_resource(
        "cf-router",
        serve_cmd="go run ./services/cf-router/...",
        deps=[
            "services/cf-router",
            "libs/cloudforge-core",
            "libs/scylladb",
            "libs/clients/cf-accounts",
        ],
        env={
            "HTTP_ADDR": ":8083",
            "SWAGGER_ADDR": ":8090",
            "SCYLLADB_HOSTS": "localhost:9042",
            "SCYLLADB_KEYSPACE": "cloudforge",
            "CF_ACCOUNTS_URL": "http://localhost:8081",
            "CF_PROVISIONER_URL": "http://localhost:8082",
            "CF_INTERNAL_SECRET": "dev-internal-secret",
            "KEYCLOAK_JWKS_URL": "http://localhost:8084/auth/realms/cloudforge/protocol/openid-connect/certs",
            "CORS_ALLOWED_ORIGINS": "http://localhost:8090,http://127.0.0.1:8090",
        },
        resource_deps=["cf-accounts"],
        labels=["cf-services"],
        readiness_probe=probe(
            http_get=http_get_action(port=8083, path="/health"),
            period_secs=3,
        ),
    )
else:
    for svc in ["cf-accounts", "cf-provisioner", "cf-router"]:
        port = {"cf-accounts": 8081, "cf-provisioner": 8082, "cf-router": 8083}[svc]
        port_forwards = [str(port) + ":" + str(port)]
        if svc == "cf-router":
            port_forwards.append("8090:8090")

        docker_build(
            "ghcr.io/jtomasevic/cloud-forge-2/" + svc,
            context=".",
            dockerfile="services/" + svc + "/Dockerfile",
            live_update=[
                sync("services/" + svc, "/app/services/" + svc),
                sync("libs", "/app/libs"),
            ],
        )
        k8s_yaml("dev/k8s/manifests/" + svc + ".yaml")
        k8s_resource(
            svc,
            port_forwards=port_forwards,
            labels=["cf-services"],
        )
