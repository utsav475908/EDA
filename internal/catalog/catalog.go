// Package catalog provisions Iceberg warehouses in the REST catalog.
// Each workspace gets one warehouse rooted at s3://<bucket>/<workspace-id>/,
// and the catalog vends credentials scoped to that prefix.
package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/lakehouse-platform/lp/internal/store"
)

type Catalog interface {
	CreateWarehouse(ctx context.Context, ws *store.Workspace) error
}

// Noop is used in dev mode when no catalog is deployed.
type Noop struct{}

func (Noop) CreateWarehouse(context.Context, *store.Workspace) error { return nil }

// Lakekeeper talks to the Lakekeeper management API.
type Lakekeeper struct {
	BaseURL      string // e.g. http://lp-lakekeeper:8181
	S3Endpoint   string
	S3Region     string
	S3AccessKey  string
	S3SecretKey  string
	HTTP         *http.Client
	ManagementTk string // bearer token when Lakekeeper authz is enabled
}

func (l *Lakekeeper) CreateWarehouse(ctx context.Context, ws *store.Workspace) error {
	body := map[string]any{
		"warehouse-name": ws.ID,
		"storage-profile": map[string]any{
			"type":              "s3",
			"bucket":            ws.Bucket,
			"key-prefix":        ws.StoragePrefix,
			"endpoint":          l.S3Endpoint,
			"region":            l.S3Region,
			"path-style-access": true,
			"flavor":            "minio",
			"sts-enabled":       true,
		},
		"storage-credential": map[string]any{
			"type":                  "s3",
			"credential-type":       "access-key",
			"aws-access-key-id":     l.S3AccessKey,
			"aws-secret-access-key": l.S3SecretKey,
		},
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.BaseURL+"/management/v1/warehouse", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if l.ManagementTk != "" {
		req.Header.Set("Authorization", "Bearer "+l.ManagementTk)
	}
	client := l.HTTP
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// 409: warehouse already exists (retry after a partial failure) is fine.
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusConflict {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("lakekeeper: create warehouse %s: %d: %s", ws.ID, resp.StatusCode, msg)
	}
	return nil
}
