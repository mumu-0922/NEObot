# Permission rollout evidence

## Release identity

```text
Source commit: 504d3938
Backend:  mm-chat/backend:host-permissions-504d3938-20260821T073624Z
Frontend: mm-chat/frontend:host-permissions-504d3938-20260821T073624Z
Schema:   103:chat_agent_permission_modes
```

Both running container image IDs matched the locally inspected immutable
candidates. Backend, Frontend, and PostgreSQL reported healthy after targeted
recreation; unrelated services were not rebuilt or recreated.

## Rollback state

The production-overlay backup wrapper could not run because this retained
deployment does not define its required `POSTGRES_DATA_DIR` preflight key.
Instead, the existing component backup tools created and checksum-verified one
matched PostgreSQL/MinIO pair with id:

```text
20260821T073553Z-permission-504d3938
```

The exact pre-release environment was retained mode `0600` under
`backup/config/`. Protected runtime data, secrets, project directories, and
the stable Host identity were preserved.

## Verification

- Backend focused `go test -race` and `go vet` passed for Agent Host,
  Host Workspace, Chat, migration, HTTP wiring, and commands.
- Frontend format, lint, type-check, 59 focused tests, and production build
  passed.
- Host lifecycle smoke passed with exact permission capability health.
- Security scan reported no Critical/High findings.
- Live capabilities advertised exactly `read-only`, `workspace-write`, and
  `danger-full-access`.
- Direct authenticated Host protocol smokes ran once on WSL storage and once on
  `/mnt/d`: Read Only denied an inside write, Workspace Write allowed the
  inside write and denied the outside write, and Full access wrote outside
  using ordinary-user authority. All fixtures were removed.
- The live Frontend asset contains the new permission selector and localized
  Full access confirmation; existing 229 Conversations migrated to the
  recommended durable `workspace-write` default.
