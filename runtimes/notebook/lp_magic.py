"""Notebook helpers: attach to an LP cluster and keep it alive.

    %lp_attach <cluster_id>   # creates `spark` bound to the cluster
"""
import json
import os
import threading
import urllib.request

API = os.environ.get("LP_API_URL", "http://lp-server.lp-system.svc:8080")
WORKSPACE = os.environ.get("LP_WORKSPACE_ID", "")


def _token():
    # JupyterHub injects the user's OIDC access token via auth_state.
    return os.environ.get("LP_ACCESS_TOKEN", "")


def _call(method, path):
    req = urllib.request.Request(f"{API}{path}", method=method,
                                 headers={"Authorization": f"Bearer {_token()}"})
    with urllib.request.urlopen(req, timeout=10) as r:
        body = r.read()
        return json.loads(body) if body else {}


def _heartbeat(cluster_id, stop):
    while not stop.wait(60):
        try:
            _call("POST", f"/api/v1/workspaces/{WORKSPACE}/clusters/{cluster_id}/heartbeat")
        except Exception:
            pass


def attach(cluster_id):
    from pyspark.sql import SparkSession

    c = _call("GET", f"/api/v1/workspaces/{WORKSPACE}/clusters/{cluster_id}")
    if c.get("state") != "CLUSTER_STATE_RUNNING":
        raise RuntimeError(f"cluster {cluster_id} is {c.get('state')}: {c.get('stateMessage', '')}")
    spark = SparkSession.builder.remote(c["connectUrl"]).getOrCreate()
    threading.Thread(target=_heartbeat, args=(cluster_id, threading.Event()), daemon=True).start()
    return spark


def load():
    try:
        ip = get_ipython()  # noqa: F821
    except NameError:
        return

    def lp_attach(line):
        ip.user_ns["spark"] = attach(line.strip())
        print("spark session attached to", line.strip())

    ip.register_magic_function(lp_attach, "line", "lp_attach")
