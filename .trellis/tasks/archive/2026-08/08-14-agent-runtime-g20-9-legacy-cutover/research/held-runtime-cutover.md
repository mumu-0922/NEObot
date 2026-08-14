# G20.9 held Runtime cutover boundary

## Runtime truth

G20.8 delivered separate package Skill Store/library and Agent Center Run
controls, but the exact host still returns `ISOLATION_UNAVAILABLE`. Package code
is not executed by the browser/API process, no executable Shadow job is
scheduled, and no Shadow output enters Chat. G20.9 must preserve that truth.

## One-way source switch

The correct source cutover is not to wire a hidden browser fallback. It is:

```text
legacy browser text-Skill executor removed
        +
package Skill/Run control plane retained
        +
exact-host Runner unavailable => stable held response
```

Thus the Neo Runtime is the only *eligible* execution domain after G20.9, while
production execution remains disabled until isolation acceptance. Local Chat
continues as ordinary model Chat with Search/Reasoning/Knowledge/MCP boundaries;
it does not execute package Skills implicitly.

## Forbidden compatibility paths

- no `resolveSkillsForMessage`, text prompt concatenation or model-based
  auto-selection;
- no old URL/editor/selection path;
- no ID/title/name match from old data to package admission/install;
- no dual write or dual execute;
- no API/browser/rootful host executor when Runner is unavailable;
- no interpretation of historical retirement facts as runnable identity.

## Promotion evidence still missing

Source/tests cannot turn `ISOLATION_UNAVAILABLE` into production acceptance.
Promotion still requires exact host/runtime/Runner/bundle fingerprints,
Isolation Acceptance Suite, backup/restore, clean restart, Kill Switch/reap,
bounded canary observations and all-path rollback rehearsal. Until then, UI and
API must honestly expose held status with no fallback.
