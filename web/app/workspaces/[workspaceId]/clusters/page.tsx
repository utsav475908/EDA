"use client";

import { use, useCallback, useEffect, useState } from "react";
import { api, type Cluster } from "@/lib/api";

export default function ClustersPage({ params }: { params: Promise<{ workspaceId: string }> }) {
  const { workspaceId } = use(params);
  const [clusters, setClusters] = useState<Cluster[]>([]);
  const [name, setName] = useState("");
  const [maxWorkers, setMaxWorkers] = useState(2);
  const [error, setError] = useState("");

  const refresh = useCallback(() => api.listClusters(workspaceId).then(setClusters).catch((e) => setError(String(e))), [workspaceId]);

  useEffect(() => {
    refresh();
    const t = setInterval(refresh, 5000);
    return () => clearInterval(t);
  }, [refresh]);

  async function create(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    try {
      await api.createCluster(workspaceId, { name, autoscale: { minWorkers: 0, maxWorkers } });
      setName("");
      refresh();
    } catch (err) {
      setError(String(err));
    }
  }

  return (
    <>
      <h1>Clusters</h1>
      <form onSubmit={create} style={{ display: "flex", gap: 8, marginBottom: 16 }}>
        <input placeholder="cluster name" value={name} onChange={(e) => setName(e.target.value)} required />
        <input type="number" min={1} max={20} value={maxWorkers} onChange={(e) => setMaxWorkers(+e.target.value)} title="max workers" />
        <button className="primary" type="submit">Create Spark cluster</button>
      </form>
      {error && <p className="state-CLUSTER_STATE_ERROR">{error}</p>}
      <table>
        <thead><tr><th>Name</th><th>State</th><th>Workers</th><th>Runtime</th><th>Connect</th><th /></tr></thead>
        <tbody>
          {clusters.map((c) => (
            <tr key={c.id}>
              <td>{c.name}<div className="muted">{c.id} · {c.creator}</div></td>
              <td className={`state-${c.state}`}>{c.state.replace("CLUSTER_STATE_", "")}<div className="muted">{c.stateMessage}</div></td>
              <td>{c.autoscale.minWorkers}–{c.autoscale.maxWorkers}</td>
              <td>{c.runtimeVersion}</td>
              <td>{c.connectUrl ? <code>%lp_attach {c.id}</code> : "—"}</td>
              <td>
                {(c.state === "CLUSTER_STATE_RUNNING" || c.state === "CLUSTER_STATE_PENDING") && (
                  <button onClick={() => api.terminateCluster(workspaceId, c.id).then(refresh)}>Terminate</button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  );
}
