# Security Policy

MM Chat is a self-hosted application with a browser frontend, Go API,
PostgreSQL, Redis, private MinIO storage, and a private Python RAG worker. It is
not a turnkey public multi-tenant SaaS boundary.

## Supported versions

Security fixes are handled on the default branch. If maintained release
branches are introduced, this policy will list them explicitly.

## Reporting a vulnerability

Report vulnerabilities privately through
[GitHub Security Advisories](https://github.com/mumu-0922/NEObot/security/advisories/new).
Do not include secrets, private chat logs, database dumps, object archives, or
user files in a public issue.

A useful report includes:

- affected commit or release;
- affected component (`frontend`, `backend`, `rag`, `postgres`, or deployment);
- the shortest safe reproduction path and expected impact;
- deployment topology and relevant non-secret configuration;
- sanitized logs or proof-of-concept artifacts.

## Security boundaries

- The Go API owns durable identity, chat, Knowledge, consent, and file metadata.
- PostgreSQL, Redis, MinIO, and the RAG worker remain private Compose services.
- Browser uploads and Provider credentials cross explicit authenticated or BYOK
  boundaries; deployments must protect logs, environment files, and upstreams.
- `mm-chat/data/`, `mm-chat/secrets/`, `mm-chat/backup/`, and
  `mm-chat/.env.single-server` are runtime state and must never be committed.
- Production changes must follow the backup, restore-drill, immutable-image,
  migration, and rollback procedures under `mm-chat/docs/deployment/`.

Before public multi-user exposure, review authentication, tenant isolation,
quotas, auditability, abuse controls, outbound URL policy, and Provider spend
limits for the intended threat model.
