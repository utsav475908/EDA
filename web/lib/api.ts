// Typed client for the lp-server REST API (mirrors proto/lp/**).
// Replace with an OpenAPI-generated client once `buf generate` runs in CI.

export type ClusterState =
  | "CLUSTER_STATE_PENDING"
  | "CLUSTER_STATE_RUNNING"
  | "CLUSTER_STATE_TERMINATING"
  | "CLUSTER_STATE_TERMINATED"
  | "CLUSTER_STATE_ERROR";

export interface Tenant { id: string; name: string; isolationTier: string; createTime: string }
export interface Workspace { id: string; tenantId: string; name: string; namespacePrefix: string; createTime: string }
export interface NodeSpec { cpu: string; memory: string; gpus?: number }
export interface Cluster {
  id: string;
  workspaceId: string;
  name: string;
  kind: "CLUSTER_KIND_SPARK" | "CLUSTER_KIND_RAY";
  runtimeVersion: string;
  driver: NodeSpec;
  worker: NodeSpec;
  autoscale: { minWorkers: number; maxWorkers: number };
  autoterminateMinutes: number;
  state: ClusterState;
  stateMessage?: string;
  connectUrl?: string;
  creator: string;
  createTime: string;
}

export class ApiError extends Error {
  constructor(public status: number, message: string) { super(message); }
}

function token(): string {
  // Phase 1 dev token; Phase 2 uses the OIDC session from NextAuth/Keycloak.
  return (typeof window !== "undefined" && localStorage.getItem("lp_token")) || "dev-admin";
}

async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(`/api/v1${path}`, {
    method,
    headers: { Authorization: `Bearer ${token()}`, "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
    cache: "no-store",
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new ApiError(res.status, data.message ?? res.statusText);
  return data as T;
}

export const api = {
  listTenants: () => call<{ tenants: Tenant[] }>("GET", "/tenants").then((r) => r.tenants ?? []),
  listWorkspaces: (tenantId: string) =>
    call<{ workspaces: Workspace[] }>("GET", `/tenants/${tenantId}/workspaces`).then((r) => r.workspaces ?? []),
  listClusters: (ws: string) =>
    call<{ clusters: Cluster[] }>("GET", `/workspaces/${ws}/clusters`).then((r) => r.clusters ?? []),
  createCluster: (ws: string, cluster: Partial<Cluster>) => call<Cluster>("POST", `/workspaces/${ws}/clusters`, { cluster }),
  terminateCluster: (ws: string, id: string) => call<object>("DELETE", `/workspaces/${ws}/clusters/${id}`),
};
