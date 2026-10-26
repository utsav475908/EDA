package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lakehouse-platform/lp/internal/store"
)

func TestLakekeeperCreateWarehouse(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/management/v1/warehouse" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	l := &Lakekeeper{BaseURL: srv.URL, S3Endpoint: "http://minio:9000", S3Region: "local"}
	ws := &store.Workspace{ID: "w-1", Bucket: "lp-warehouse", StoragePrefix: "w-1"}
	if err := l.CreateWarehouse(context.Background(), ws); err != nil {
		t.Fatal(err)
	}
	sp := got["storage-profile"].(map[string]any)
	if got["warehouse-name"] != "w-1" || sp["bucket"] != "lp-warehouse" || sp["key-prefix"] != "w-1" || sp["sts-enabled"] != true {
		t.Fatalf("unexpected request body: %v", got)
	}
}
