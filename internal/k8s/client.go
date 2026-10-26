package k8s

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

var ErrNotFound = errors.New("kubernetes: not found")

// Client is a minimal Kubernetes REST client: server-side apply, get, delete.
// It deliberately avoids client-go to keep the control plane dependency-free;
// swap in controller-runtime once the platform needs watches/informers.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

const saDir = "/var/run/secrets/kubernetes.io/serviceaccount"

// InCluster builds a client from the pod's service account.
func InCluster() (*Client, error) {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" {
		return nil, errors.New("not running in a Kubernetes pod")
	}
	tok, err := os.ReadFile(saDir + "/token")
	if err != nil {
		return nil, err
	}
	ca, err := os.ReadFile(saDir + "/ca.crt")
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(ca)
	return &Client{
		BaseURL: "https://" + host + ":" + port,
		Token:   strings.TrimSpace(string(tok)),
		HTTP: &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
		}},
	}, nil
}

// resource maps apiVersion/kind to its REST collection.
var restResources = map[string]struct {
	plural     string
	namespaced bool
}{
	"v1/Namespace":                             {"namespaces", false},
	"v1/ServiceAccount":                        {"serviceaccounts", true},
	"v1/Service":                               {"services", true},
	"v1/ResourceQuota":                         {"resourcequotas", true},
	"apps/v1/Deployment":                       {"deployments", true},
	"rbac.authorization.k8s.io/v1/Role":        {"roles", true},
	"rbac.authorization.k8s.io/v1/RoleBinding": {"rolebindings", true},
	"networking.k8s.io/v1/NetworkPolicy":       {"networkpolicies", true},
	"ray.io/v1/RayCluster":                     {"rayclusters", true},
}

// Path returns the REST path of an object (or of its collection if name is "").
func Path(apiVersion, kind, namespace, name string) (string, error) {
	r, ok := restResources[apiVersion+"/"+kind]
	if !ok {
		return "", fmt.Errorf("kubernetes: unsupported kind %s/%s", apiVersion, kind)
	}
	p := "/apis/" + apiVersion
	if apiVersion == "v1" {
		p = "/api/v1"
	}
	if r.namespaced {
		p += "/namespaces/" + namespace
	}
	p += "/" + r.plural
	if name != "" {
		p += "/" + name
	}
	return p, nil
}

// Apply performs a server-side apply owned by the "lp" field manager.
func (c *Client) Apply(ctx context.Context, obj Obj) error {
	meta, _ := obj["metadata"].(Obj)
	name, _ := meta["name"].(string)
	ns, _ := meta["namespace"].(string)
	path, err := Path(obj["apiVersion"].(string), obj["kind"].(string), ns, name)
	if err != nil {
		return err
	}
	body, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	_, err = c.do(ctx, http.MethodPatch, path+"?fieldManager=lp&force=true", "application/apply-patch+yaml", body)
	return err
}

func (c *Client) Get(ctx context.Context, apiVersion, kind, ns, name string) (Obj, error) {
	path, err := Path(apiVersion, kind, ns, name)
	if err != nil {
		return nil, err
	}
	b, err := c.do(ctx, http.MethodGet, path, "", nil)
	if err != nil {
		return nil, err
	}
	var out Obj
	return out, json.Unmarshal(b, &out)
}

// Delete removes an object with foreground propagation; NotFound is success.
func (c *Client) Delete(ctx context.Context, apiVersion, kind, ns, name string) error {
	path, err := Path(apiVersion, kind, ns, name)
	if err != nil {
		return err
	}
	_, err = c.do(ctx, http.MethodDelete, path, "application/json", []byte(`{"propagationPolicy":"Foreground"}`))
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

func (c *Client) do(ctx context.Context, method, path, contentType string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, ErrNotFound
	case resp.StatusCode >= 300:
		return nil, fmt.Errorf("kubernetes: %s %s: %d: %s", method, path, resp.StatusCode, b)
	}
	return b, nil
}
