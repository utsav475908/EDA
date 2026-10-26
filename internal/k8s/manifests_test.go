package k8s

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/lakehouse-platform/lp/internal/store"
)

var (
	testWS = &store.Workspace{ID: "w-abc", TenantID: "t-1", NamespacePrefix: "ws-abc"}
	testRC = RuntimeConfig{SparkImage: "lp/spark:test", RayImage: "ray:test",
		IcebergCatalogURI: "http://catalog:8181/catalog", S3Endpoint: "http://minio:9000"}
)

func TestSparkConnectManifests(t *testing.T) {
	c := &store.Cluster{ID: "c-1", Kind: store.KindSpark,
		Driver: store.NodeSpec{CPU: "2", Memory: "8Gi"}, Worker: store.NodeSpec{CPU: "4", Memory: "16Gi"},
		AutoScale: store.AutoScale{MinWorkers: 1, MaxWorkers: 5}}
	objs := SparkConnectManifests(testWS, c, testRC)
	if len(objs) != 2 {
		t.Fatalf("want service+deployment, got %d objects", len(objs))
	}
	b, _ := json.Marshal(objs)
	s := string(b)
	for _, want := range []string{
		`"namespace":"ws-abc-compute"`,
		"spark.dynamicAllocation.maxExecutors=5",
		"spark.executor.memory=14745m",
		"spark.sql.catalog.lakehouse.uri=http://catalog:8181/catalog",
		"X-Iceberg-Access-Delegation=vended-credentials",
		"spark.driver.host=lp-c-1-driver.ws-abc-compute.svc.cluster.local",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("manifest missing %q", want)
		}
	}
	for _, o := range objs {
		meta := o["metadata"].(Obj)
		if _, err := Path(o["apiVersion"].(string), o["kind"].(string), meta["namespace"].(string), meta["name"].(string)); err != nil {
			t.Error(err)
		}
	}
}

func TestWorkspaceManifestsAreRoutable(t *testing.T) {
	for _, o := range WorkspaceManifests(testWS) {
		meta := o["metadata"].(Obj)
		ns, _ := meta["namespace"].(string)
		p, err := Path(o["apiVersion"].(string), o["kind"].(string), ns, meta["name"].(string))
		if err != nil {
			t.Fatal(err)
		}
		if o["kind"] == "Namespace" && strings.Contains(p, "/namespaces//") {
			t.Fatalf("bad namespace path %s", p)
		}
	}
}

func TestRayClusterGPU(t *testing.T) {
	c := &store.Cluster{ID: "c-2", Kind: store.KindRay,
		Driver: store.NodeSpec{CPU: "4", Memory: "16Gi"}, Worker: store.NodeSpec{CPU: "8", Memory: "64Gi", GPUs: 2},
		AutoScale: store.AutoScale{MinWorkers: 0, MaxWorkers: 4}}
	b, _ := json.Marshal(RayClusterManifest(testWS, c, testRC))
	if !strings.Contains(string(b), `"nvidia.com/gpu":2`) || !strings.Contains(string(b), `"maxReplicas":4`) {
		t.Fatalf("unexpected RayCluster: %s", b)
	}
}

func TestSparkMem(t *testing.T) {
	for in, want := range map[string]string{"8Gi": "7372m", "1024Mi": "921m", "bogus": "1g"} {
		if got := sparkMem(in); got != want {
			t.Errorf("sparkMem(%q) = %q, want %q", in, got, want)
		}
	}
}
