package k8s

import (
	"fmt"
	"strconv"

	"github.com/lakehouse-platform/lp/internal/store"
)

type Obj = map[string]any

// RuntimeConfig carries image and data-plane endpoints baked into clusters.
type RuntimeConfig struct {
	SparkImage        string
	RayImage          string
	IcebergCatalogURI string
	S3Endpoint        string
}

const sparkConnectPort = 15002

func labels(ws *store.Workspace, extra map[string]string) map[string]any {
	l := map[string]any{
		"app.kubernetes.io/managed-by": "lp",
		"lp.io/tenant":                 ws.TenantID,
		"lp.io/workspace":              ws.ID,
	}
	for k, v := range extra {
		l[k] = v
	}
	return l
}

// WorkspaceManifests returns the per-workspace baseline: namespaces with
// restricted Pod Security, a Spark service account allowed to manage executor
// pods, default-deny ingress, and a resource quota.
func WorkspaceManifests(ws *store.Workspace) []Obj {
	compute, notebooks := ComputeNamespace(ws), NotebookNamespace(ws)
	var out []Obj
	for _, ns := range []string{compute, notebooks} {
		out = append(out, Obj{
			"apiVersion": "v1", "kind": "Namespace",
			"metadata": Obj{"name": ns, "labels": labels(ws, map[string]string{
				"pod-security.kubernetes.io/enforce": "baseline",
			})},
		})
	}
	out = append(out,
		Obj{"apiVersion": "v1", "kind": "ServiceAccount",
			"metadata": Obj{"name": "lp-spark", "namespace": compute, "labels": labels(ws, nil)}},
		Obj{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role",
			"metadata": Obj{"name": "lp-spark", "namespace": compute, "labels": labels(ws, nil)},
			"rules": []any{
				Obj{"apiGroups": []any{""}, "resources": []any{"pods", "services", "configmaps", "persistentvolumeclaims"},
					"verbs": []any{"create", "get", "list", "watch", "delete", "deletecollection", "patch", "update"}},
			}},
		Obj{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "RoleBinding",
			"metadata": Obj{"name": "lp-spark", "namespace": compute, "labels": labels(ws, nil)},
			"roleRef":  Obj{"apiGroup": "rbac.authorization.k8s.io", "kind": "Role", "name": "lp-spark"},
			"subjects": []any{Obj{"kind": "ServiceAccount", "name": "lp-spark", "namespace": compute}}},
		// Only same-workspace notebooks and the platform namespace may reach compute.
		Obj{"apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy",
			"metadata": Obj{"name": "lp-workspace-isolation", "namespace": compute, "labels": labels(ws, nil)},
			"spec": Obj{
				"podSelector": Obj{},
				"policyTypes": []any{"Ingress"},
				"ingress": []any{Obj{"from": []any{
					Obj{"podSelector": Obj{}},
					Obj{"namespaceSelector": Obj{"matchLabels": Obj{"kubernetes.io/metadata.name": notebooks}}},
					Obj{"namespaceSelector": Obj{"matchLabels": Obj{"kubernetes.io/metadata.name": "lp-system"}}},
				}}},
			}},
		Obj{"apiVersion": "v1", "kind": "ResourceQuota",
			"metadata": Obj{"name": "lp-compute-quota", "namespace": compute, "labels": labels(ws, nil)},
			"spec":     Obj{"hard": Obj{"requests.cpu": "64", "requests.memory": "256Gi", "pods": "200"}}},
	)
	return out
}

// SparkConnectManifests renders a Spark Connect server (driver) plus the
// headless service that executors and notebooks use to reach it.
func SparkConnectManifests(ws *store.Workspace, c *store.Cluster, rc RuntimeConfig) []Obj {
	ns, name := ComputeNamespace(ws), ResourceName(c)
	sel := map[string]string{"lp.io/cluster": c.ID, "lp.io/role": "driver"}
	conf := [][2]string{
		{"spark.master", "k8s://https://kubernetes.default.svc"},
		{"spark.submit.deployMode", "client"},
		{"spark.kubernetes.namespace", ns},
		{"spark.kubernetes.container.image", rc.SparkImage},
		{"spark.kubernetes.authenticate.driver.serviceAccountName", "lp-spark"},
		{"spark.kubernetes.executor.podNamePrefix", name},
		{"spark.kubernetes.executor.label.lp.io/cluster", c.ID},
		{"spark.kubernetes.executor.label.lp.io/workspace", ws.ID},
		{"spark.driver.host", connectHost(ws, c)},
		{"spark.driver.bindAddress", "0.0.0.0"},
		{"spark.driver.port", "7078"},
		{"spark.blockManager.port", "7079"},
		{"spark.executor.cores", c.Worker.CPU},
		{"spark.executor.memory", sparkMem(c.Worker.Memory)},
		{"spark.dynamicAllocation.enabled", "true"},
		{"spark.dynamicAllocation.shuffleTracking.enabled", "true"},
		{"spark.dynamicAllocation.minExecutors", strconv.Itoa(int(c.AutoScale.MinWorkers))},
		{"spark.dynamicAllocation.maxExecutors", strconv.Itoa(int(c.AutoScale.MaxWorkers))},
		{"spark.connect.grpc.binding.port", strconv.Itoa(sparkConnectPort)},
		// Iceberg REST catalog; S3 credentials are vended per table by the catalog.
		{"spark.sql.extensions", "org.apache.iceberg.spark.extensions.IcebergSparkSessionExtensions"},
		{"spark.sql.catalog.lakehouse", "org.apache.iceberg.spark.SparkCatalog"},
		{"spark.sql.catalog.lakehouse.type", "rest"},
		{"spark.sql.catalog.lakehouse.uri", rc.IcebergCatalogURI},
		{"spark.sql.catalog.lakehouse.warehouse", ws.ID},
		{"spark.sql.catalog.lakehouse.header.X-Iceberg-Access-Delegation", "vended-credentials"},
		{"spark.sql.catalog.lakehouse.io-impl", "org.apache.iceberg.aws.s3.S3FileIO"},
		{"spark.sql.catalog.lakehouse.s3.endpoint", rc.S3Endpoint},
		{"spark.sql.catalog.lakehouse.s3.path-style-access", "true"},
		{"spark.sql.defaultCatalog", "lakehouse"},
	}
	args := []any{}
	for _, kv := range conf {
		args = append(args, "--conf", kv[0]+"="+kv[1])
	}
	lbl := labels(ws, map[string]string{"lp.io/cluster": c.ID, "lp.io/role": "driver"})
	return []Obj{
		{"apiVersion": "v1", "kind": "Service",
			"metadata": Obj{"name": name + "-driver", "namespace": ns, "labels": lbl},
			"spec": Obj{
				"clusterIP": "None",
				"selector":  toAny(sel),
				"ports": []any{
					Obj{"name": "connect", "port": sparkConnectPort},
					Obj{"name": "driver", "port": 7078},
					Obj{"name": "blockmanager", "port": 7079},
					Obj{"name": "ui", "port": 4040},
				},
			}},
		{"apiVersion": "apps/v1", "kind": "Deployment",
			"metadata": Obj{"name": name + "-driver", "namespace": ns, "labels": lbl},
			"spec": Obj{
				"replicas": 1,
				"strategy": Obj{"type": "Recreate"},
				"selector": Obj{"matchLabels": toAny(sel)},
				"template": Obj{
					"metadata": Obj{"labels": lbl},
					"spec": Obj{
						"serviceAccountName": "lp-spark",
						"securityContext":    Obj{"runAsNonRoot": true, "runAsUser": 185, "fsGroup": 185},
						"containers": []any{Obj{
							"name":    "spark-connect",
							"image":   rc.SparkImage,
							"command": []any{"/opt/spark/sbin/start-connect-server.sh"},
							"args":    args,
							"env":     []any{Obj{"name": "SPARK_NO_DAEMONIZE", "value": "true"}},
							"ports": []any{
								Obj{"name": "connect", "containerPort": sparkConnectPort},
								Obj{"name": "driver", "containerPort": 7078},
								Obj{"name": "blockmanager", "containerPort": 7079},
								Obj{"name": "ui", "containerPort": 4040},
							},
							"readinessProbe": Obj{"tcpSocket": Obj{"port": sparkConnectPort}, "periodSeconds": 5},
							"resources":      resources(c.Driver),
						}},
					},
				},
			}},
	}
}

// RayClusterManifest renders a KubeRay RayCluster with in-tree autoscaling.
func RayClusterManifest(ws *store.Workspace, c *store.Cluster, rc RuntimeConfig) Obj {
	lbl := labels(ws, map[string]string{"lp.io/cluster": c.ID})
	container := func(name string, n store.NodeSpec) Obj {
		return Obj{"name": name, "image": rc.RayImage, "resources": resources(n)}
	}
	return Obj{
		"apiVersion": "ray.io/v1", "kind": "RayCluster",
		"metadata": Obj{"name": ResourceName(c), "namespace": ComputeNamespace(ws), "labels": lbl},
		"spec": Obj{
			"enableInTreeAutoscaling": true,
			"headGroupSpec": Obj{
				"rayStartParams": Obj{"dashboard-host": "0.0.0.0", "num-cpus": "0"},
				"template":       Obj{"metadata": Obj{"labels": lbl}, "spec": Obj{"containers": []any{container("ray-head", c.Driver)}}},
			},
			"workerGroupSpecs": []any{Obj{
				"groupName":      "workers",
				"replicas":       c.AutoScale.MinWorkers,
				"minReplicas":    c.AutoScale.MinWorkers,
				"maxReplicas":    c.AutoScale.MaxWorkers,
				"rayStartParams": Obj{},
				"template":       Obj{"metadata": Obj{"labels": lbl}, "spec": Obj{"containers": []any{container("ray-worker", c.Worker)}}},
			}},
		},
	}
}

func resources(n store.NodeSpec) Obj {
	req := Obj{"cpu": n.CPU, "memory": n.Memory}
	lim := Obj{"memory": n.Memory}
	if n.GPUs > 0 {
		req["nvidia.com/gpu"] = n.GPUs
		lim["nvidia.com/gpu"] = n.GPUs
	}
	return Obj{"requests": req, "limits": lim}
}

// sparkMem converts a Kubernetes quantity ("8Gi") into a Spark heap size,
// leaving ~10% for Spark's memoryOverhead so the pod fits its request.
func sparkMem(q string) string {
	var n int
	var unit string
	if _, err := fmt.Sscanf(q, "%d%s", &n, &unit); err != nil || n <= 0 {
		return "1g"
	}
	mb := n
	switch unit {
	case "Gi", "G":
		mb = n * 1024
	case "Mi", "M":
	default:
		return "1g"
	}
	return strconv.Itoa(mb*9/10) + "m"
}

func toAny(m map[string]string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
