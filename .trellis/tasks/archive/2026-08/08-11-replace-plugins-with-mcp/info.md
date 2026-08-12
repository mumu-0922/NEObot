# Technical Design Index

The authoritative product requirements are in [`prd.md`](prd.md).

Implementation must preserve these ownership boundaries:

1. **Frontend** owns display state and local-mode selection persistence only.
2. **Go backend** owns users, Workspaces, grants, server definitions, secrets,
   OAuth, protocol sessions, tool snapshots, scheduling, calls, results,
   budgets, audit, and the provider-native continuation loop.
3. **Runner** owns approved child-process lifecycle only. It receives a bounded
   internal request and never decides user authorization.
4. **PostgreSQL/MinIO** own durable state; Redis may only accelerate or signal.
5. **Catalog/manifest** remain versioned declarative authorities for shipped
   and deployment-managed definitions.

Build outward from the existing chat ToolRoundProvider/process-step seams and
provider secret vault. Do not create parallel provider continuation, secret,
HTTP error, storage, or frontend persistence mechanisms.

