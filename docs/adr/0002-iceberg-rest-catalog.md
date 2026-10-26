# ADR 0002: Lakekeeper as the Iceberg REST catalog; OpenMetadata for discovery

Status: accepted (revisit Apache Polaris at Phase 2 review)

## Context
Iceberg needs a transactional catalog to commit table metadata. OpenMetadata is
a discovery and lineage catalog; it cannot serve Iceberg commits.

## Options
| | Lakekeeper | Apache Polaris | Nessie |
|---|---|---|---|
| Iceberg REST spec | yes | yes | yes |
| Credential vending (S3 STS) | yes, incl. MinIO | yes | partial |
| Backing store | PostgreSQL | PostgreSQL (JDBC) | PostgreSQL / others |
| Multi-tenant warehouses/projects | yes | catalogs | branches |
| Runtime | Rust, small footprint | JVM | JVM |

## Decision
Lakekeeper, one warehouse per workspace (`warehouse-name = workspace id`),
rooted at `s3://lp-warehouse/<workspace-id>/`. Engines request
`X-Iceberg-Access-Delegation: vended-credentials`, so they never hold static
S3 keys. OpenMetadata ingests from the catalog for search and lineage.

## Consequences
- Storage-level isolation between workspaces comes from prefix-scoped STS
  credentials, independent of engine-level authorization.
- Lakekeeper's admin key (`storage-credential`) must be a MinIO user that can
  assume roles but is not exposed to user pods.
