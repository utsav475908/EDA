// Package store holds platform metadata (tenants, workspaces, clusters).
// JSON field names match the grpc-gateway rendering of proto/lp/**.
package store

import "time"

type IsolationTier string

const (
	IsolationSoft IsolationTier = "ISOLATION_TIER_SOFT"
	IsolationHard IsolationTier = "ISOLATION_TIER_HARD"
)

type Tenant struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	IsolationTier IsolationTier `json:"isolationTier"`
	CreateTime    time.Time     `json:"createTime"`
}

type Workspace struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenantId"`
	Name            string    `json:"name"`
	NamespacePrefix string    `json:"namespacePrefix"`
	Bucket          string    `json:"bucket"`
	StoragePrefix   string    `json:"storagePrefix"`
	CreateTime      time.Time `json:"createTime"`
}

type ClusterKind string

const (
	KindSpark ClusterKind = "CLUSTER_KIND_SPARK"
	KindRay   ClusterKind = "CLUSTER_KIND_RAY"
)

type ClusterState string

const (
	StatePending     ClusterState = "CLUSTER_STATE_PENDING"
	StateRunning     ClusterState = "CLUSTER_STATE_RUNNING"
	StateTerminating ClusterState = "CLUSTER_STATE_TERMINATING"
	StateTerminated  ClusterState = "CLUSTER_STATE_TERMINATED"
	StateError       ClusterState = "CLUSTER_STATE_ERROR"
)

type NodeSpec struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
	GPUs   int32  `json:"gpus,omitempty"`
}

type AutoScale struct {
	MinWorkers int32 `json:"minWorkers"`
	MaxWorkers int32 `json:"maxWorkers"`
}

type Cluster struct {
	ID                   string       `json:"id"`
	WorkspaceID          string       `json:"workspaceId"`
	Name                 string       `json:"name"`
	Kind                 ClusterKind  `json:"kind"`
	RuntimeVersion       string       `json:"runtimeVersion"`
	Driver               NodeSpec     `json:"driver"`
	Worker               NodeSpec     `json:"worker"`
	AutoScale            AutoScale    `json:"autoscale"`
	AutoterminateMinutes int32        `json:"autoterminateMinutes"`
	State                ClusterState `json:"state"`
	StateMessage         string       `json:"stateMessage,omitempty"`
	ConnectURL           string       `json:"connectUrl,omitempty"`
	Creator              string       `json:"creator"`
	CreateTime           time.Time    `json:"createTime"`
	LastActivityTime     time.Time    `json:"lastActivityTime"`
}

// Active reports whether the cluster still holds (or is acquiring) resources.
func (c *Cluster) Active() bool {
	return c.State == StatePending || c.State == StateRunning || c.State == StateTerminating
}
