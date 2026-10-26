# ADR 0003: Enforce Ranger policies in Spark via Kyuubi AuthZ (Phase 2)

Status: proposed

## Context
Apache Ranger has native plugins for Trino and Hive, but no first-class Spark
plugin. Spark users with file access could bypass table policies.

## Decision
1. Storage layer (Phase 1): only vended, prefix-scoped credentials reach user
   compute (ADR 0002). This blocks cross-workspace reads regardless of engine.
2. Engine layer (Phase 2): bundle the Apache Kyuubi Spark AuthZ plugin in the
   Spark runtime, enforcing Ranger table/column/row-filter/masking policies in
   Spark SQL. Trino uses Ranger's native plugin.
3. `authz-svc` is the only writer of Ranger policies; users manage grants with
   `GRANT ... ON TABLE ... TO group` through the LP API and UI.

## Consequences
- Column masking and row filters work in Spark SQL and Trino. Raw DataFrame
  reads of files remain bounded by the vended credentials.
- Spark Connect sessions must run as the end user (proxy user = OIDC subject)
  so that Ranger sees the correct principal; interactive clusters therefore
  become single-user or "shared with user isolation" in Phase 2.
