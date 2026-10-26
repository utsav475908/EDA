# ADR 0001: Own the control plane in Go, integrate OSS engines in the data plane

Status: accepted

## Context
We want a self-hosted Databricks-like platform. Databricks' value is mostly its
control plane (workspaces, identity, catalog, permissions, cluster lifecycle,
jobs, UI) layered over open engines.

## Decision
- We write the control plane (`lp-server`, later split into services) in Go.
- Spark, Ray, JupyterHub, MLflow, MinIO, Lakekeeper, Argo and KServe run
  unmodified; we integrate through their APIs and Kubernetes resources.
- APIs are defined once in Protobuf (`proto/`). The public surface is REST via
  grpc-gateway; services talk gRPC internally.
- The control plane records desired state; reconcilers converge Kubernetes.
- Phase 1 ships as a single binary using only the Go standard library
  (hand-written REST handlers that match the grpc-gateway JSON shape, raw
  Kubernetes REST with server-side apply). Generated stubs replace the
  handlers when the services split.

## Consequences
- Upgrading engines is a version bump plus conformance tests, not a merge.
- We must maintain a curated runtime image (`runtimes/spark`) per release.
- The control plane never reads table data, which keeps it small and keeps the
  permission boundary at the catalog and storage layers.
