# Memory v2 schema foundation

Memory v2 keeps Go and PostgreSQL authoritative. Migration
`053_memory_project_scope_settings` adds the Project/scope/settings foundation
without enabling a new reader, worker, Provider call, or public Project API.

## Authority model

```text
authenticated user
  -> Global Memory
  -> Project -> Project Memory
  -> Conversation -> Conversation Memory
```

`projects` is a first-class user-owned entity. Conversation membership and
Memory scope use composite `(resource_id, user_id)` foreign keys so an ID from
another user cannot become authority. Scope foreign keys use `ON DELETE
RESTRICT`; future Project/Conversation deletion must execute its reviewed
impact, tombstone, and purge flow before deleting the parent.

`source_conversation_id` remains provenance only. It does not make a Global
Memory Conversation-scoped.

## Additive defaults and backfill

- Existing settings are unchanged. Sensitive Memory defaults off; L2 and L3
  modes default to `inherit`.
- Existing Memories become Global, with null scope foreign keys and scope
  generation one.
- Existing Conversations have no Project, generation one, and independent
  Use/Learn modes set to `inherit`.
- The three active-content unique indexes admit the same normalized content in
  different scopes but reject duplicates inside one exact scope.

## Runtime boundary

PR2 keeps `/v1/memory-settings` and `/v1/memories` on the v1 Global view. The
Go repository explicitly creates and filters `scope_type='global'`; it cannot
list, edit, delete, or mark a Project/Conversation Memory as used. The new
`projects` table receives no `go_api_runtime` grant in PR2.

## Rollback

Down is allowed only before v2 authority is used. It fails atomically when any
Project exists, any Memory is non-Global, Conversation policy/generation has
changed, or Sensitive/L2/L3 settings differ from their migration defaults.

Migration `053` replaces the unique index used by the pre-`053` backend's
Memory create statement. Therefore an old and new backend must not run
together after `053`. Forward deployment has an explicit short outage:

```text
stop every pre-053 backend
  -> up migration 053
  -> deploy the post-053 backend
```

If a pre-traffic schema rollback is approved:

```text
stop every post-053 backend
  -> verify down guards
  -> down migration 053
  -> deploy the pre-053 backend
```

After v2 data exists, keep the additive schema and roll back only later feature
flags/readers. Never delete Projects or scoped Memory merely to make down pass.
