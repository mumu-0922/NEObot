# Playwright MCP 0.0.79 review

## Reviewed authority

- Package: `@playwright/mcp@0.0.79`
- npm integrity:
  `sha512-VpqD4a3vFyGQMY9sh3UJiO6wjcurggkljKfAyCHL0QWGY5m6Ehr3MNsAAHPDHO//n13g0PCjpHatAOiulrqdZQ==`
- Runner base image:
  `mcr.microsoft.com/playwright/mcp:v0.0.79@sha256:18c0a9c934004fe9580cc79f1e8e6e6cde7c667348b215335e8a23fd3e509804`
- The complete npm package, generated README, CLI help, and bundled Tool
  implementations were inspected rather than relying on release notes.

## Actual upstream Tool surface

An MCP `initialize` plus `tools/list` returned 24 Tools:

```text
browser_close browser_resize browser_console_messages browser_handle_dialog
browser_evaluate browser_file_upload browser_drop browser_find
browser_fill_form browser_press_key browser_type browser_navigate
browser_navigate_back browser_network_requests browser_network_request
browser_run_code_unsafe browser_take_screenshot browser_snapshot
browser_click browser_drag browser_hover browser_select_option browser_tabs
browser_wait_for
```

The package itself describes `browser_run_code_unsafe` as arbitrary JavaScript
in the Playwright server process and RCE-equivalent. Package review therefore
cannot be treated as authority to expose every current or future upstream Tool.

Neo Chat admits only these 17 exact names:

```text
browser_click browser_close browser_console_messages browser_drag
browser_fill_form browser_find browser_handle_dialog browser_hover
browser_navigate browser_navigate_back browser_press_key browser_resize
browser_select_option browser_snapshot browser_tabs browser_type
browser_wait_for
```

The manifest allowlist is enforced while normalizing `tools/list`, while
filtering any preloaded snapshot, and again immediately before `tools/call`.
Only the upstream read-only `browser_console_messages`, `browser_find`, and
`browser_snapshot` Tools receive Neo Chat's `read` scheduling/retry policy.
`browser_wait_for` remains an ordered `write` barrier because upstream marks
it non-read-only, even though its visible purpose is waiting.

## Runtime proof

A local HTTP fixture served a heading and button. The exact production CLI
flags plus a development-only local Chromium executable path completed:

```text
initialize protocolVersion=2025-06-18
tools/list toolCount=24 hasUnsafe=true
browser_navigate error=false
browser_snapshot error=false
snapshotContainsFixture=true
```

The focused Runner connector test separately proves that an unsafe direct call
returns `ErrToolNotFound` without reaching the Runner HTTP route.

## Decisions

- Use `instanceScope="run"` so Browser page, Cookie, and in-memory process state
  cannot cross Chat Runs.
- Reclaim the least-recent idle Runner child at capacity; never evict an active
  call. This prevents distinct run-scoped instances from exhausting all four
  slots until the periodic reaper runs.
- Do not add general Code Mode in this Gate. Native Tools remain the fallback
  and correctness boundary; upstream unsafe Playwright code is not Code Mode.
- Docker image verification remains environment-blocked on the current host;
  protocol/browser smoke and Go/lockfile gates do not substitute for the
  release image build.
