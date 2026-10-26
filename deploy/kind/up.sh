#!/usr/bin/env bash
# Bring up a full Phase 1 Lakehouse Platform on a local kind cluster.
# Requires: docker, kind, kubectl, helm. Takes ~10 minutes on first run.
set -euo pipefail
cd "$(dirname "$0")/../.."

CLUSTER=lp
NS=lp-system

step() { printf '\n\033[1;34m==> %s\033[0m\n' "$*"; }

step "kind cluster"
kind get clusters | grep -qx "$CLUSTER" || kind create cluster --config deploy/kind/kind-config.yaml

step "build and load images"
docker build -t lp/lp-server:dev .
docker build -t lp/spark-runtime:3.5.3-lp1 runtimes/spark
docker build -t lp/notebook:dev runtimes/notebook
docker build -t lp/mlflow:2.18.0-lp1 runtimes/mlflow
for img in lp/lp-server:dev lp/spark-runtime:3.5.3-lp1 lp/notebook:dev lp/mlflow:2.18.0-lp1; do
  kind load docker-image "$img" --name "$CLUSTER"
done

step "helm repos"
helm repo add cnpg https://cloudnative-pg.github.io/charts >/dev/null
helm repo add minio https://charts.min.io/ >/dev/null
helm repo add kuberay https://ray-project.github.io/kuberay-helm/ >/dev/null
helm repo add jupyterhub https://hub.jupyter.org/helm-chart/ >/dev/null
helm repo update >/dev/null

kubectl create namespace "$NS" --dry-run=client -o yaml | kubectl apply -f -

step "operators: CloudNativePG, KubeRay"
helm upgrade --install cnpg cnpg/cloudnative-pg --version 0.22.1 -n cnpg-system --create-namespace --wait
helm upgrade --install kuberay-operator kuberay/kuberay-operator --version 1.2.2 -n "$NS" --wait

step "MinIO (object storage)"
helm upgrade --install minio minio/minio --version 5.3.0 -n "$NS" -f deploy/kind/minio-values.yaml --wait

step "Lakehouse Platform (PostgreSQL, Iceberg catalog, MLflow, lp-server)"
helm upgrade --install lp deploy/helm/lp -n "$NS" -f deploy/kind/lp-values.yaml --wait --timeout 10m

step "JupyterHub"
helm upgrade --install jupyterhub jupyterhub/jupyterhub --version 4.0.0 -n "$NS" -f jupyter/values.yaml --wait --timeout 10m

cat <<MSG

Lakehouse Platform is up.
  API         http://localhost:8080   (token: dev-admin)
  JupyterHub  http://localhost:8000
  MLflow      http://localhost:5000
  MinIO       http://localhost:9001

Next: ./deploy/kind/smoke.sh
MSG
