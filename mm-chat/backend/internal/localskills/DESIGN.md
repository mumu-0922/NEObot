# Local Skill Runtime Design

## Boundary

The local runtime is intentionally direct. A command has the filesystem and
network authority available to the non-root Backend process and the configured
workspace mount. The hard blocklist and smart denial policy reduce accidental
damage; they do not contain deliberately adversarial code.

## Execution flow

```text
enabled local_direct runtime
  -> optional Host/WSL alias normalized to one workspace-relative path
  -> File Tools with versioned atomic writes
  -> optional skill exact immutable instructions
  -> terminal validated foreground/background command + optional Skill binding
  -> explicit environment + non-login shell + ordinary Backend UID/GID
  -> bounded stdout/stderr/exit or process-local Job framing
  -> same-model Tool continuation
```

The command uses a new process group. Timeout and explicit Run cancellation
send `SIGKILL` to the whole group and wait for the direct child before returning.
Combined stdout/stderr storage is capped while both streams continue draining,
so a verbose command cannot deadlock on a full pipe or allocate without bound.

`CallTimeout` is the foreground execution authority and the maximum explicit
`terminal.timeoutSeconds` advertised by Chat. `RunTimeout` remains the whole
Turn bound and the default for a background Job when the shared Tool schema
receives `runInBackground=true, timeoutSeconds=null`. This asymmetry is
intentional: one strict Tool schema must not advertise an explicit value that
the foreground executor rejects. A returned process is classified separately
from transport completion; exit code zero succeeds, while nonzero exit and
timeout remain bounded failed results so Chat can preserve diagnostics without
granting completion evidence.

File reads and searches enter through an `os.Root` anchored to the workspace.
An optional configured Host path exists only as an input alias and is reduced
to a relative name before this boundary. Terminal command text is never
rewritten; only its structured working directory uses the same resolver.
File writes and edits serialize in-process, compare the caller's complete-file
SHA-256 version twice, write and sync a same-directory temporary file, then
atomically rename it. This is optimistic conflict protection, not a replacement
for a filesystem Sandbox or a multi-process transactional filesystem.

Background Jobs hold the same global process-concurrency slot as foreground
commands. Their state, completion notice, and bounded captured output exist
only in Backend memory and are authorized by exact user plus conversation.
`job_output(wait=true)` waits on the Job completion channel rather than polling.
Shutdown cancels the shared lifecycle context and waits for all process groups
to be reaped before the executor closes.

## Rollback

Set `AGENT_LOCAL_RUNTIME_ENABLED=false` and recreate the Backend. Installed
Skill authority and workspace files remain intact; Chat exposes no local
workspace, Job, terminal, or Skill Tools. The
historical OCI Runner code is not invoked as an automatic fallback.
