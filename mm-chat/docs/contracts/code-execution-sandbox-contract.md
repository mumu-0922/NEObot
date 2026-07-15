# Code Execution Sandbox Contract

Status: G6.5d contract gate complete; runtime executor remains disabled.

This document defines the minimum contract that must exist before
`POST /v1/code/executions` can move beyond the current fail-closed admission
state. It is intentionally strict because code execution is the highest-risk G6
job class.

## 1. Scope / Trigger

Trigger: G6 introduced a Go-owned code-execution admission route. Before any real
executor is enabled, the project needs an explicit sandbox, storage, audit,
rate-limit, and rollback contract.

In scope:

- Python code execution submitted through `POST /v1/code/executions`;
- deterministic sandbox resource bounds;
- output capture and storage boundary;
- audit metadata and cancellation behavior;
- provider/model-assisted execution only if it still obeys the sandbox boundary.

Out of scope until separately approved:

- browser-visible `codeExecution=true` capability;
- JavaScript, shell, notebook, Dockerfile, or arbitrary binary execution;
- network-enabled code execution;
- mounting user files or server secrets into the sandbox;
- using LLM “simulated execution” as a substitute for sandboxed execution.

## 2. Signatures

Current fail-closed route:

```http
POST /v1/code/executions
Content-Type: application/json
```

Request body:

```json
{
  "modelRef": { "providerId": "gemini", "modelId": "gemini-code" },
  "language": "python",
  "code": "print('hello')"
}
```

Future successful response shape:

```json
{
  "jobId": "job_01...",
  "status": "completed",
  "stdout": "hello\n",
  "stderr": "",
  "exitCode": 0,
  "durationMs": 120,
  "outputFileId": "optional-server-file-id"
}
```

Cancellation route:

```http
POST /v1/jobs/{jobId}/cancel
```

## 3. Contracts

### Request constraints

| Field | Contract |
| --- | --- |
| `modelRef.providerId` | Required non-empty identifier. Not a secret. |
| `modelRef.modelId` | Required non-empty model/runtime selector. Not a secret. |
| `language` | Defaults to `python`; no other language until explicitly added. |
| `code` | Required, max 100,000 Unicode code points. Preserve exact submitted text after validation. |

### Sandbox constraints

| Boundary | Required contract |
| --- | --- |
| Process user | Non-root, no privilege escalation. |
| Filesystem | Fresh per-run temp workspace; no host project mount; read-only runtime image. |
| Network | Deny all egress by default. No metadata-service access. |
| Secrets | No provider keys, DB URLs, object-store keys, BYOK keys, or env secrets in sandbox. |
| Time | Hard wall-clock timeout and process kill. |
| CPU / memory | Explicit CPU and memory caps. OOM must terminate the run. |
| Output | Bounded stdout/stderr bytes; truncate with metadata when exceeded. |
| Persistence | Only sanitized output artifacts may be written through backend storage APIs. |
| Cleanup | Workspace removed after terminal state or cancellation. |

### Audit contract

Audit events must never include submitted source code, stdout/stderr content,
file contents, or secrets. Allowed fields:

```text
kind, status, userId, providerId, modelId, language, reason, jobId,
durationMs, exitCode, truncatedOutput flags, outputFileId
```

## 4. Validation & Error Matrix

| Condition | HTTP | Code |
| --- | ---: | --- |
| Invalid JSON / unknown field | 400 | `INVALID_REQUEST` |
| Missing `modelRef.providerId` or `modelRef.modelId` | 400 | `MODEL_REF_REQUIRED` |
| Empty code | 400 | `CODE_REQUIRED` |
| Code over max length | 400 | `CODE_TOO_LARGE` |
| Unsupported language | 400 | `UNSUPPORTED_CODE_LANGUAGE` |
| Audit sink unavailable | 503 | `JOB_AUDIT_UNAVAILABLE` |
| Executor disabled | 501 | `CODE_EXECUTION_UNAVAILABLE` |
| Sandbox startup failed | 503 | `SANDBOX_UNAVAILABLE` |
| Timeout | 408 | `CODE_EXECUTION_TIMEOUT` |
| Cancellation accepted | 202 | terminal event or status says `cancelled` |
| Cancellation unavailable | 501 | `JOB_CANCELLATION_UNAVAILABLE` |

## 5. Good / Base / Bad Cases

Good:

- Python code runs in an isolated, network-denied sandbox.
- Output is bounded, stored only through backend-owned file/object storage, and
  linked by server file id.
- Audit records job metadata without source code or output payloads.

Base:

- Current implementation validates request shape, records sanitized admission
  audit metadata, and returns `CODE_EXECUTION_UNAVAILABLE`.

Bad:

- Running submitted code inside the Go API process.
- Mounting the repository, Docker socket, `.env`, cloud credentials, or user
  upload directories into the executor.
- Sending source code to a provider and labeling the response as sandboxed
  execution.
- Returning unbounded stdout/stderr directly into chat state.

## 6. Tests Required

Before enabling `codeExecution=true`, add and pass:

- unit tests for sandbox command construction and forbidden mount/env/network
  settings;
- handler tests for all validation errors and executor-disabled behavior;
- cancellation tests that terminate a running sandbox and clean workspace files;
- output-limit tests for stdout/stderr truncation and metadata flags;
- audit tests proving source code and output payloads are not recorded;
- storage tests proving artifacts are written through server file/object APIs;
- clean-copy smoke with no host project tree dependency.

## 7. Wrong vs Correct

### Wrong

```text
Go API receives code → runs `python` locally → returns raw stdout
```

Why wrong: the API process, host filesystem, env, and network become part of the
attack surface.

### Correct

```text
Go API validates request → writes sanitized audit admission → starts isolated
sandbox with no secrets/no network/resource limits → captures bounded output →
stores optional artifact through backend storage → returns terminal metadata
```

This contract is a hard gate: any future executor that cannot satisfy it must
remain behind `CODE_EXECUTION_UNAVAILABLE`.
