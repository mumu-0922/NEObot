# Local Skill Runtime Contract

## Scope

`local_direct` is the current single-server Agent Skill execution backend. It
follows the Hermes Agent model: an installed Skill is an instruction directory,
and the normal Chat Tool Loop loads its files and runs commands when needed.

```text
owner installation -> admitted canonical package -> immutable local materialization
  -> revisioned catalog replacement -> optional skill
enabled local_direct -> File/Job/terminal Tools -> same-model answer
```

An ordinary Skill needs `SKILL.md`. `scripts/`, `references/`, and `assets/`
are optional. `neo.runtime.json`, OCI images, Podman, WSL systemd, `sudo`,
Runner mTLS, and production isolation evidence are not prerequisites.

## Package authority

- Only the authenticated owner's current installation is eligible.
- The server resolves the admitted package and canonical object key; callers
  cannot supply an object key or materialization path.
- Canonical ZIP size, package name, installation fingerprint, archive
  validation, and content fingerprint are rechecked before publication.
- Publication uses a same-filesystem temporary directory and atomic rename.
- Reads accept only exact inventory paths and reject absolute paths, traversal,
  missing files, symlinks, oversized files, and content drift.
- A `terminal.skill` binding rehashes the complete exact inventory again before
  exposing the selected root to the child command.
- `skill` reads only `SKILL.md`. A loaded Skill may access exact files below
  `scripts/`, `references/`, and `assets/` through a `terminal.skill` binding
  and `$NEO_CHAT_ACTIVE_SKILL_ROOT`; callers never receive the server path.

## Native Tools

| Tool | Contract |
| --- | --- |
| `skill({name})` | Loads UTF-8 `SKILL.md` for one exact current installation. A duplicate load under the same catalog revision returns `alreadyLoaded` without repeating the body. |
| `file_read({path,offset?,limit?})` | Reads one bounded UTF-8 window and returns the version of the complete workspace-relative regular file. |
| `file_write({path,content,expectedVersion})` | Atomically creates or replaces one bounded UTF-8 file. `expectedVersion="absent"` is valid only for creation. |
| `file_edit({path,oldText,newText,replaceAll,expectedVersion})` | Performs exact text replacement over the version-pinned file; the default requires exactly one match. |
| `file_search({path?,query,glob?,maxResults?})` | Searches bounded regular UTF-8 workspace files for literal text while skipping symlinks and generated dependency directories. |
| `publish_file({path,displayName,contentType})` | Snapshots one final binary or text workspace file into actor-owned `purpose=export` storage and projects it onto the assistant Message as an authenticated `purpose=output` attachment. Nullable display name and content type values use safe filename/MIME inference. |
| `terminal({command,skill?,workingDir?,timeoutSeconds?,runInBackground})` | Runs one bounded shell command as the Backend user in the configured workspace. `skill` exposes that installed package only through `NEO_CHAT_ACTIVE_SKILL_ROOT`; background mode returns a process-local Job ID. |
| `job_list({})` | Lists process-local Jobs for the exact user and Conversation without command text or output. |
| `job_output({jobId,wait,timeoutSeconds?})` | Reads one owned Job and optionally waits at most ten seconds without busy-polling. Output appears only after a terminal state. |
| `job_kill({jobId})` | Cancels one owned Job and kills/reaps its complete process group. |

`skills_list` and `skill_view` remain accepted by the Backend for a bounded
continuation migration period, but their definitions are absent from every new
model request.

When `local_direct` is enabled with no installed Skills, the catalog is still
an explicit empty tombstone and only `skill` is omitted. File, Job, and
`terminal` Tools remain available. Disabling `local_direct` removes all of
these Tools without deleting installations or workspace data.

The system prompt contains one bounded complete replacement of the installed
catalog, identified by a SHA-256 revision. Removing every installation emits
an empty tombstone, so no earlier catalog name remains authoritative. Full
instructions enter model context only through `skill`, except for a current
user's deterministic `/skill-name` invocation. Every Tool result and injected
Skill block is marked as untrusted user-authorized guidance and cannot override
system/developer instructions.

An exact installed name or one unique strong lexical description match queues
a required Skill prelude. That Provider round exposes only `skill`, constrains
the `name` schema to the selected value, and completes before the normal first
task round. A `/skill-name` token bounded by whitespace loads the same
`SKILL.md` on the server before the first Provider round; unknown names and
path-like tokens stay ordinary user text.

## Direct execution and limits

`terminal` is direct local execution, not a Sandbox. A permitted command has
the filesystem and network authority of the Backend process and configured
workspace. Agent Center and deployment docs must state this explicitly.

The executor:

- supplies an explicit child environment rather than inheriting database,
  Provider, storage, or secret environment variables;
- uses a non-login shell, so writable workspace profile files are not executed
  implicitly, and exposes only the selected Skill root rather than a global
  package-cache variable;
- confines the command working directory to the configured workspace,
  including symlink resolution;
- enforces per-call timeout, per-Run timeout, combined stdout/stderr bytes,
  calls, rounds, and global concurrency;
- starts a process group and kills the complete group on timeout or Chat Run
  cancellation;
- always blocks catastrophic command patterns and, in default `smart` mode,
  returns `approval_required` for destructive patterns;
- records only Tool name, round, `local_direct`, classification, optional
  timeout, duration, and failure category in process events. Command text and
  output are never process-trace or SSE metadata.

These guards reduce accidental damage. They are not protection against an
adversarial allowed process.

## Workspace files

- Every caller path is workspace-relative. Go `os.Root` operations reject
  absolute paths, traversal, symlinks, non-regular files, and escapes.
- A complete file is at most 2 MiB. One read window and one write/edit payload
  are at most 24 KiB and must be valid UTF-8 without NUL bytes.
- Versions are `sha256:<hex>` over the complete bytes or `absent`. Writes and
  edits require the exact expected version and return `version_conflict` after
  any observed external change instead of overwriting it.
- Writes serialize in the Backend process, recheck the version after syncing a
  same-directory temporary file, atomically rename, then sync the parent
  directory.
- Search is literal and bounded to 2,000 files, 32 MiB scanned bytes, and 200
  results. It skips symlinks and hidden/generated dependency directories.
- `file_write` and `file_edit` are mutations, not completion evidence. The
  Agent must subsequently read/search/execute and verify the resulting state.

## Published chat artifacts

- `publish_file` is model-visible only in Agent mode when both `local_direct`
  and the server File service are available. Chat mode physically omits the
  complete local Runtime, including publication.
- Publication accepts only a workspace-relative regular file, rejects every
  symlink component and directory, supports binary bytes, rejects empty files,
  and applies `MAX_UPLOAD_BYTES` to both one file and the Turn total.
- One Turn may publish at most eight unique `(path, sha256 version)` snapshots.
  Repeating an unchanged path returns the existing File ID and does not upload
  or attach a duplicate.
- Published objects use the authenticated actor, `purpose=export`, and current
  Conversation metadata. The assistant-only link purpose is `output`; user
  message input rejects that purpose.
- Every terminal assistant outcome keeps already published artifacts. If
  assistant finalization fails, the Backend rereads current Message authority:
  unlinked files are deleted, while files found linked after an ambiguous
  commit acknowledgement are preserved. If authority cannot be reread during
  an outage, cleanup fails safe by retaining the private actor-owned File
  rather than deleting a potentially committed attachment.
- The UI downloads through authenticated `GET /v1/files/{id}/content` with
  `disposition=attachment`, creates a short-lived Blob URL, and reports missing
  or deleted files as an error. It never treats a workspace path or naked
  object-store URL as a download. The File endpoint also forces every
  `purpose=export` response to attachment disposition even if a caller asks
  for inline rendering.

## Background Jobs

- A Job is memory-only and scoped to the exact authenticated user plus
  Conversation. Cross-scope absence and denial both return `job_not_found`.
- Status is `running|stopping|completed|killed|failed`. A foreground command
  and a Job share the same process-concurrency slots and output cap.
- A Job uses the local Run timeout. `job_output(wait=true)` waits at most ten
  seconds on completion; sleeps and busy polling are forbidden.
- Completion notices are injected into the next Agent Step or the next request
  in that Conversation, but contain only Job ID/status and never imply output.
- A running start/list/kill result cannot verify completion. Only a successful
  `job_output` with `status=completed` may serve as evidence.
- Process trace marks these Tools with `durability=process_local`; the UI warns
  that service restart loses them. Shutdown cancels and reaps every Job.

## Failure and rollback

- A model without native Tool support fails before assistant creation with
  `SKILL_MODEL_UNSUPPORTED`.
- Package preparation failure returns `SKILL_RUNTIME_UNAVAILABLE` without
  exposing storage or filesystem details.
- Run deadline returns `LOCAL_SKILL_BUDGET_EXHAUSTED`; cancellation terminates
  the process group and finalizes the Chat Run as cancelled.
- Workspace conflicts and bounds return typed `version_conflict`,
  `file_not_found`, `file_too_large`, `invalid_utf8`, `edit_conflict`,
  `path_invalid`, or `arguments_invalid` Tool Results.
- Background Job failures are bounded Tool Results. Backend restart does not
  recover or resume a prior process-local Job.
- Set `AGENT_LOCAL_RUNTIME_ENABLED=false` and recreate the Backend to roll back
  immediately. Installed packages remain stored and no OCI Runtime is enabled.

Verification includes `bash scripts/verify-chat-artifacts-postgres17.sh`, the
local Tool unit suite, Backend vet/tests, Frontend artifact tests and production
build. The PostgreSQL drill uses an ephemeral PostgreSQL 17 container and proves
assistant output round-trip, two-user isolation, and deleted-file rejection.
