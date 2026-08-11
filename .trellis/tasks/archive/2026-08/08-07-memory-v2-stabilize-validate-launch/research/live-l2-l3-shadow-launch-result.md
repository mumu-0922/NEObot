# Live L2 Scene / L3 Persona shadow launch result

## Outcome

The sole-user deployment now generates L2 Scene and L3 Persona artifacts
automatically while both active readers remain fail-closed:

```text
MEMORY_L2_SCENE_SHADOW_ENABLED=true
MEMORY_L2_SCENE_READER_ENABLED=false
MEMORY_L3_PERSONA_SHADOW_ENABLED=true
MEMORY_L3_PERSONA_READER_ENABLED=false
```

Only `memory-worker` and `backend` were force-recreated with `--no-build` and
`--no-deps`. Both services are healthy on the existing schema-071 candidate
image. No migration, promotion, application Chat POST, or manual database
mutation was performed.

## Generated artifacts

The pre-existing pending refresh jobs were consumed by the live Worker. Final
aggregate-only verification proved:

```text
schema=71
L1 current=2
L2 profile=shadow, scenes=1, members=2, refresh attempt=1/completed
L2 projection=ready, promotion events=0
L3 profile=shadow, personas=1, members=2, refresh attempt=6/completed
L3 Persona token count=226
L3 projection=ready, promotion events=0
```

The L3 provider path had five bounded transient failures
(`L3_PERSONA_OUTPUT_INVALID` or `L3_PERSONA_PROVIDER_FAILED`) before succeeding
on attempt six. The Worker then completed the L3 embedding without operator
intervention. No dead letter remains.

## User-visible verification

A transient `GET /v1/memory-governance` read emitted only counts and bounded
metadata, not Scene/Persona plaintext. It returned HTTP `200` with one current
Scene and one current Persona. Both profiles and artifacts remain `shadow`, and
both profile `active` fields remain false because no benchmark/canary promotion
has occurred. Refreshing the Memory governance UI can therefore show the
generated artifacts without injecting them into normal answers.

The L1 health endpoint remained HTTP `200`, `ready/memory_ready`, with
`readyCount=2`. Unrelated container IDs were unchanged.

## Evidence and rollback

The first attempt stopped before environment mutation because its inspection
harness assumed every container had `.State.Health`. That retained harness
failure is separate from the successful retry.

Successful private evidence is retained with directory/file modes `0700/0600`:

```text
/var/tmp/neo-chat-memory-l2-l3-shadow-launch-retry-20260810T055921Z
```

Behavior rollback is to set both shadow flags and both reader flags to `false`,
validate Compose configuration, and recreate only `memory-worker` and
`backend` on the same pinned image. The generated shadow artifacts may remain
persisted but are not consumed when the gates are off.
