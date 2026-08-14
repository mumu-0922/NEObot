# G20.8 product surface boundaries

## Comparable patterns

- GitHub Actions separates marketplace/install state from workflow Run detail,
  logs, approvals and cancellation rather than presenting them as one editor.
- Kubernetes-style control planes keep desired configuration, immutable
  revisions, observed execution and rollout/canary status in separate views.
- Package stores separate discovery/admission/install from the runtime process
  page; administrative review is not exposed as an end-user install shortcut.

## Repository facts

- `skillsupply` already exposes server-authoritative candidate, Store, install,
  library and uninstall routes under `/v1/skills/*`.
- The current frontend `SkillMarket` and `settingsStore.installedSkills` are the
  legacy pure-text Skill path and remain authoritative through G20.8.
- G20.2-G20.7 service packages have durable control-plane methods but no public
  Run, approval, Child, Cron or Draft HTTP handlers and no startup workers.
- `ChatApp` already has top-level panels for legacy Skills and Assistants;
  Assistant administration must stay separate from package Skills.

## Recommended mapping

- Add a distinct top-level **Agent Center** rather than another Settings tab.
- Inside Agent Center use four task-oriented views:
  1. **Package Skills** — Store, admission status and installed immutable
     fingerprints, backed by existing `/v1/skills/*` routes.
  2. **Runs** — root/Child process tree, step/attempt status, approvals,
     Artifacts and cancel/kill state.
  3. **Schedules** — immutable Cron revisions, approval, pause/resume/delete and
     next-trigger state.
  4. **Learning Review** — administrator-only Draft checks, bounded diff,
     Reject/Promote and cleanup status.
- Keep the existing pure-text Skill panel visibly labelled **Legacy Skills**
  until G20.9 deletes its browser state. Do not silently translate it to
  package Skills.
- Every mutable action carries expected revision/fingerprint and reloads on
  conflict. UI never infers authority from cached display state.

## UX requirements

- URL-addressable tabs and selected records survive reload/back/forward.
- Desktop uses list/detail; mobile uses drill-in pages with a persistent back
  path and no horizontal process graph dependency.
- State is conveyed by text plus icon/color, live updates have an accessible
  status region, and every approval/cancel/Promote action has keyboard and
  screen-reader labels.
- Disabled Runtime/Shadow states remain honest, actionable status rather than
  a fake success or hidden control.
