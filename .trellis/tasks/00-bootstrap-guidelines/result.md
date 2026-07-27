# Bootstrap guidelines and session commit isolation result

## Outcome

- Replaced all placeholder frontend Trellis specs with English, codebase-backed
  conventions for structure, components, hooks, state, type safety, and quality.
- Added concrete examples and file references from the current Next.js 16,
  React 19, Zustand, Zod, and Vitest implementation.
- Changed `add_session.py` so the session auto-commit receives only the journal
  written by the invocation and the developer workspace `index.md`.
- Changed the Git commit to `git commit --only ... -- <journal> <index>`, so
  unrelated pre-staged work remains staged and cannot enter the journal commit.
- Removed the unused broad path-discovery helper that included every active task
  and the entire task archive.
- Captured the executable session auto-commit contract in
  `.trellis/spec/operations/session-auto-commit.md`.

## Regression evidence

The isolated Python `unittest` suite creates a fresh temporary Git repository
per test and proves:

- normal append commits exactly `journal-1.md` and workspace `index.md`;
- an unrelated pre-staged file is excluded and remains staged;
- a dirty archive file and untracked active task remain untouched;
- rotation commits exactly the newly-created journal and the index;
- `session_auto_commit: false` writes journal/index without invoking Git.

`safe_trellis_paths_to_add` no longer exists under `.trellis/scripts/`, and the
session code never uses force-add or a `.trellis/` directory pathspec.

## Verification

- Python `py_compile`: passed for `add_session.py`, `safe_commit.py`, and the
  regression test.
- Python isolated Git regressions: `3 passed`.
- Trellis/frontend/task Markdown Prettier check: passed.
- Frontend frozen install, Prettier, ESLint, and strict TypeScript: passed.
- Frontend Vitest: `193` files and `936` tests passed.
- Next.js 16.2.12 production build: passed with 17 routes generated.

The existing Next.js warning that the `middleware` filename convention is
deprecated remains unchanged; no product source was modified by this task.

## Rollback

Revert the focused session-auto-commit implementation/test/spec changes and the
frontend bootstrap specs. No application schema, runtime data, secret, or
deployment rollback is required.
