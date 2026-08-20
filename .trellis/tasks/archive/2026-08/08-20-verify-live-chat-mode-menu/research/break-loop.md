# Bug Analysis: Source Fix Was Not Propagated to the Live Frontend

## 1. Root Cause Category

- **Category**: C — Change Propagation Failure
- **Specific cause**: The menu source and local production bundle were fixed,
  but `127.0.0.1:18080` continued running a Frontend image built before the fix
  commit. The first completion claim treated local build output as evidence for
  the live container.

## 2. Why the First Fix Failed

1. **Incomplete scope**: The code defect was fixed correctly, but the active
   release artifact was never traced or replaced.
2. **Mental model**: Verification stopped at source/tests/build instead of
   following endpoint -> container -> image -> served asset.
3. **Acceptance gap**: No post-change screenshot or actively served CSS proof
   was required before declaring the visual issue resolved.

## 3. Prevention Mechanisms

| Priority | Mechanism            | Specific action                                                                                         | Status |
| -------- | -------------------- | ------------------------------------------------------------------------------------------------------- | ------ |
| P0       | Runtime verification | Compare live Frontend image creation/ID with the fix commit and fetch the decisive asset from the edge  | DONE   |
| P0       | Rollback             | Retain the old image and a mode-`0600` environment backup before frontend-only recreation               | DONE   |
| P1       | Documentation        | Add live UI release-propagation checks to the runtime image-pinning spec                                | DONE   |
| P1       | User acceptance      | Tell the user to reload an already-open production tab because it cannot hot-swap a new immutable chunk | DONE   |

## 4. Systematic Expansion

- **Similar issues**: Any Frontend-only fix can appear ineffective when Docker
  still serves a pre-fix image, even if Vitest and `next build` are green.
- **Design improvement**: Release metadata should expose source revision in the
  Frontend image or health response for cheaper provenance checks.
- **Process improvement**: For live-page bug reports, completion evidence must
  include active image identity and a served-asset/rendered-behavior check.

## 5. Knowledge Capture

- [x] Updated
      `.trellis/spec/operations/runtime-recreate-image-pinning.md`.
- [x] Persisted exact rollout and rollback evidence in `live-runtime.md`.
- [x] No template mirror exists in this repository, so there is no generated
      spec template to synchronize.
