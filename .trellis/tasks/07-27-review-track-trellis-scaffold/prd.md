# Review and Track Shared Trellis Scaffold

## Goal

Eliminate the repository's partially tracked Trellis setup by committing the
project-level scaffold required by a fresh clone while keeping developer
identity, session runtime, caches, backups, secrets, and personal editor state
local.

## What I Already Know

- The repository already tracks Trellis specs, tasks, archives, and the
  developer journal.
- `add_session.py`, `safe_commit.py`, and their regression test are tracked,
  but most imported `.trellis/scripts/common/*.py` modules are still untracked.
- The approved direction is: shared scaffold in Git, machine/runtime state
  ignored, and every candidate reviewed before staging.
- Product source remains exclusively under `mm-chat/`; this task is limited to
  repository tooling and metadata.

## Assumptions to Validate

- `.agents/`, project `.codex/` hooks/configuration, Trellis scripts, workflow,
  configuration, version/hash metadata, and the shared workspace index contain
  no personal secrets or machine-specific absolute paths and are intended to
  travel with the repository.
- `.vscode/settings.json` may contain pre-existing personal configuration and
  will be excluded unless every changed setting is demonstrably project-wide.

## Requirements

- Inventory and classify every currently dirty Trellis/Codex/agent/editor path.
- Commit only repository-shared Trellis/Codex/agent scaffold using explicit
  pathspecs; never use `git add .` or broad force-add.
- Preserve local-only ignores for developer identity, runtime sessions,
  temporary state, Python caches, backups, and worktrees.
- Scan candidate files for secrets, personal absolute paths, generated caches,
  broken links, and configuration that would not work in a fresh clone.
- Resolve the partial-script state so every tracked Trellis entrypoint has its
  required Python modules available after clone.
- Leave unrelated `mm-chat/` product source and runtime state unchanged.
- Document the final tracked/local boundary and a rollback path.

## Acceptance Criteria

- [ ] Every pre-task dirty path is classified as shared, local-only, or
      unrelated/personal.
- [ ] Shared Trellis Python entrypoints import and compile from the resulting
      tracked file set.
- [ ] Trellis workflow/context/task commands pass focused smoke tests.
- [ ] Project Codex/agent hooks reference repository-relative, existing files.
- [ ] Candidate secret and machine-specific path scans have no unresolved hit.
- [ ] Local developer/runtime/cache/backup paths remain ignored and untracked.
- [ ] `.vscode/settings.json` is either excluded or justified setting by
      setting; no personal preference is silently committed.
- [ ] `git status` after the work commits contains only explicitly retained
      local/unrelated paths.
- [ ] A result document records inventory, verification, compatibility, and
      rollback evidence.

## Definition of Done

- Focused syntax, unit/contract, hook, and clean-copy checks pass.
- The shared/local boundary is recorded in operations documentation.
- Changes are split into coherent, explicitly approved commits without amend
  or push.
- The task is archived and the session journal is recorded.

## Out of Scope

- Product application changes under `mm-chat/`.
- Redesigning Trellis workflow semantics or Codex behavior.
- Committing developer identity, secrets, live session state, caches, backups,
  worktrees, or unrelated personal editor configuration.
- Updating Trellis from an external release unless the local scaffold is proven
  internally inconsistent and the user separately approves an update.

## Technical Notes

- Primary operations specs:
  `.trellis/spec/operations/repository-root-boundary.md` and
  `.trellis/spec/operations/session-auto-commit.md`.
- The reviewed shared allowlist is `.agents/skills/**`, project `.codex/`
  config/hooks, `.trellis/config.yaml`, `.trellis/.version`,
  `.trellis/workflow.md`, `.trellis/scripts/**`, `.trellis/.gitignore`, the
  shared workspace index, and the code-reuse guide. Existing tracked Codex
  agent definitions remain shared but need no new staging.
- Local-only exclusions are `.trellis/.developer`, `.current-task`,
  `.runtime/`, `.template-hashes.json`, `.cache/`, `worktrees/`, backups,
  temporary files, and Python caches.
- `.vscode/settings.json` has a line-ending-only pre-task diff and is excluded
  without modification or staging.
- Inventory and redacted audit evidence are recorded in
  `research/scaffold-inventory.md`.
- The current dirty state predates this task and must be treated as untrusted
  candidate content until inspected.
- Rollback is a focused revert of the scaffold commit; it must not delete local
  runtime state.
