"use client";

import { useEffect, useState } from "react";
import { api, type Tenant, type Workspace } from "@/lib/api";

export default function Home() {
  const [rows, setRows] = useState<{ tenant: Tenant; workspaces: Workspace[] }[]>([]);
  const [error, setError] = useState("");

  useEffect(() => {
    (async () => {
      try {
        const tenants = await api.listTenants();
        setRows(await Promise.all(tenants.map(async (t) => ({ tenant: t, workspaces: await api.listWorkspaces(t.id) }))));
      } catch (e) {
        setError(String(e));
      }
    })();
  }, []);

  return (
    <>
      <h1>Workspaces</h1>
      {error && <p className="state-CLUSTER_STATE_ERROR">{error}</p>}
      {rows.map(({ tenant, workspaces }) => (
        <section key={tenant.id}>
          <h2>{tenant.name} <span className="muted">({tenant.id})</span></h2>
          {workspaces.length === 0 ? <p className="muted">No workspaces yet.</p> : (
            <ul>
              {workspaces.map((w) => (
                <li key={w.id}><a href={`/workspaces/${w.id}/clusters`}>{w.name}</a> <span className="muted">{w.id}</span></li>
              ))}
            </ul>
          )}
        </section>
      ))}
    </>
  );
}
