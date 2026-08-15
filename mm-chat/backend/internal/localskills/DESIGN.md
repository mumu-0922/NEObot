# Local Skill Runtime Design

## Boundary

The local runtime is intentionally direct. A command has the filesystem and
network authority available to the non-root Backend process and the configured
workspace mount. The hard blocklist and smart denial policy reduce accidental
damage; they do not contain deliberately adversarial code.

## Execution flow

```text
installed Skill index
  -> skill_view exact immutable file
  -> terminal validated command + optional active Skill binding
  -> explicit environment + non-login shell + ordinary Backend UID/GID
  -> bounded stdout/stderr/exit framing
  -> same-model Tool continuation
```

The command uses a new process group. Timeout and explicit Run cancellation
send `SIGKILL` to the whole group and wait for the direct child before returning.
Combined stdout/stderr storage is capped while both streams continue draining,
so a verbose command cannot deadlock on a full pipe or allocate without bound.

## Rollback

Set `AGENT_LOCAL_RUNTIME_ENABLED=false` and recreate the Backend. Installed
Skill authority remains intact; Chat simply exposes no local Skill Tools. The
historical OCI Runner code is not invoked as an automatic fallback.
