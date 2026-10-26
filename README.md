# Lakehouse Platform (LP)

A self-hosted, Databricks-style data and AI platform. LP owns the control plane
(Go) and integrates open-source engines in the data plane on Kubernetes.

```
Next.js UI ──REST──▶ lp-server (Go) ──▶ Kubernetes: Spark Connect · KubeRay · JupyterHub
                          │                         MLflow · Lakekeeper (Iceberg REST)
                          ▼                                   │
                     PostgreSQL                         MinIO (S3)
```

See `docs/adr/` for the key decisions.

## Repository layout

| Path | What |
|---|---|
| `proto/` | API source of truth (Protobuf + HTTP annotations, `buf generate`) |
| `cmd/lp-server/` | Control-plane binary + end-to-end API tests |
| `internal/auth` | Dev tokens, OIDC/JWKS JWT verification (Keycloak), tenant checks |
| `internal/iam` | Tenants and workspaces; provisions namespaces and Iceberg warehouses |
| `internal/compute` | Cluster API, policy validation, reconciler, idle auto-termination |
| `internal/k8s` | Manifests (Spark Connect, RayCluster, workspace isolation), REST client |
| `internal/catalog` | Lakekeeper warehouse provisioning |
| `migrations/` | PostgreSQL schema (with row-level security per tenant) |
| `runtimes/` | Spark runtime, notebook, MLflow images |
| `jupyter/` | JupyterHub (z2jh) values: Spark Connect kernels, `%lp_attach`, RTC |
| `deploy/helm/lp` | Chart: lp-server, CloudNativePG, Lakekeeper, MLflow |
| `deploy/kind/` | One-command local platform + smoke test |
| `web/` | Next.js UI (workspaces, clusters) |

## Quick start

Control plane only (simulated compute, no Kubernetes):

```bash
make test
make run        # :8080, token "dev-admin"

H='Authorization: Bearer dev-admin'
curl -H "$H" -d '{"name":"analytics"}' localhost:8080/api/v1/tenants
curl -H "$H" -d '{"name":"ml"}' localhost:8080/api/v1/tenants/<tenant-id>/workspaces
curl -H "$H" -d '{"cluster":{"name":"etl","autoscale":{"maxWorkers":4}}}' \
     localhost:8080/api/v1/workspaces/<workspace-id>/clusters
```

Full platform on kind (docker, kind, kubectl, helm):

```bash
make kind-up
./deploy/kind/smoke.sh
make web-dev    # http://localhost:3000
```

In a notebook (JupyterHub → choose workspace ID):

```python
%lp_attach c-0123456789ab
spark.sql("CREATE NAMESPACE IF NOT EXISTS demo")
spark.range(1000).writeTo("demo.numbers").createOrReplace()
```

## Multi-tenancy model

- **Tenant**: team or business unit. Access is via the `tenant:<id>` group from Keycloak; `lp-admins` sees everything.
- **Workspace**: gets `ws-<id>-compute` and `ws-<id>-notebooks` namespaces, a
  default-deny NetworkPolicy, a ResourceQuota, a Spark service account, and an Iceberg
  warehouse at `s3://lp-warehouse/<workspace-id>/`.
- Engines get **vended, prefix-scoped S3 credentials** from the catalog. No
  static keys reach user pods.
- Objects outside a caller's tenants return `404`, so their existence isn't leaked.

## Phase 1 status

Done and tested (`go test ./...`):
- [x] REST API: tenants, workspaces, clusters (create/get/list/terminate/heartbeat)
- [x] OIDC JWT verification (RS256, JWKS rotation, iss/aud/exp checks) + dev tokens
- [x] Tenant isolation checks, cluster policy validation (workers, CPU, GPU, runtimes)
- [x] Reconciler: PENDING → RUNNING → idle auto-terminate → TERMINATED; error cleanup
- [x] Spark Connect + Iceberg REST + dynamic allocation manifests; KubeRay manifests
- [x] Workspace namespace/RBAC/NetworkPolicy/quota provisioning
- [x] Lakekeeper warehouse per workspace

Written but not yet validated in a live cluster:
- [ ] Helm chart, kind bootstrap, Dockerfiles, JupyterHub values, Next.js UI
      (verify the pinned image and chart versions when you first run them)

Next for Phase 1:
- [ ] PostgreSQL `Store` implementation (pgx + sqlc) against `migrations/`
- [ ] `buf generate` in CI; replace hand-written handlers with grpc-gateway
- [ ] Leader election for the reconciler (multi-replica lp-server)
- [ ] JupyterHub: spawn notebooks into the workspace's notebooks namespace
- [ ] Keycloak in the kind stack; NextAuth OIDC session in the UI

## Roadmap

- **Phase 2**: catalog-svc (Unity-style grants), Apache Ranger + Kyuubi AuthZ, Trino,
  notebook sharing/versioning service, Argo jobs, OpenMetadata, audit service
- **Phase 3**: Feast feature store, MLflow → KServe serving, inference tables,
  Prometheus/Loki/Tempo/OpenCost observability
- **Phase 4**: GPU operator + Kueue, Ray Train LLM fine-tuning, vLLM serving, AI
  gateway, ACL-aware RAG (pgvector), MCP-based agents
