# Runner cold-start retrospective

## Root cause category

- **Category D — test coverage gap**: unit tests exercised process lifecycle
  with an image-local helper binary, but did not execute a freshly downloaded
  npm package under the actual Compose tmpfs and HTTP timeouts.
- **Category E — implicit assumptions**: the implementation assumed Docker
  tmpfs honored an omitted executable flag and that extending the child connect
  context alone was enough for a slow cold start.

## Failed paths

1. The first production-equivalent smoke downloaded the package but failed with
   `Permission denied`; Docker had mounted `/work` with its default `noexec`.
2. Adding `exec` let npm launch the package, but the internal HTTP server still
   closed the response at 35 seconds while the child had a two-minute connect
   budget.

## Prevention now in place

- Compose explicitly renders `/work:exec,nosuid,nodev` and keeps `/tmp:noexec`.
- Runner client, child initialization, and internal HTTP timeouts are ordered:
  two-minute child cold start, two-minute response-header bound, and a
  two-minute-plus-15-second HTTP envelope.
- The backend MCP spec records both invariants.
- A production-topology smoke proved sealed artifact -> live npm download ->
  MCP initialize -> `tools/list` with Context7 `2.2.0`.

## Systematic expansion

Every future Runner isolation or timeout change must be tested with a cache-cold
exact npm artifact in the real container topology. Image build success and
image-local helper tests are insufficient evidence for dynamic npm execution.
