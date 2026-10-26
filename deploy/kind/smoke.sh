#!/usr/bin/env bash
# Phase 1 exit criterion: tenant -> workspace -> Spark cluster -> Iceberg table.
set -euo pipefail
API=${API:-http://localhost:8080/api/v1}
AUTH="Authorization: Bearer ${LP_TOKEN:-dev-admin}"
j() { python3 -c "import sys,json; print(json.load(sys.stdin)$1)"; }

T=$(curl -fsS -H "$AUTH" -d '{"name":"demo"}' "$API/tenants" | j '["id"]')
W=$(curl -fsS -H "$AUTH" -d '{"name":"sandbox"}' "$API/tenants/$T/workspaces" | j '["id"]')
C=$(curl -fsS -H "$AUTH" -d '{"cluster":{"name":"smoke","autoscale":{"minWorkers":1,"maxWorkers":2}}}' \
      "$API/workspaces/$W/clusters" | j '["id"]')
echo "tenant=$T workspace=$W cluster=$C"

for _ in $(seq 1 60); do
  STATE=$(curl -fsS -H "$AUTH" "$API/workspaces/$W/clusters/$C" | j '["state"]')
  echo "state: $STATE"
  [[ "$STATE" == CLUSTER_STATE_RUNNING ]] && break
  [[ "$STATE" == CLUSTER_STATE_ERROR ]] && exit 1
  sleep 5
done

NS="ws-${W#w-}-compute"
kubectl -n "$NS" get pods
echo "Run a Spark Connect query from a pod in the workspace:"
echo "  spark = SparkSession.builder.remote('sc://lp-$C-driver.$NS.svc.cluster.local:15002').getOrCreate()"
echo "  spark.sql('CREATE NAMESPACE IF NOT EXISTS demo'); spark.range(10).writeTo('demo.t').create()"
