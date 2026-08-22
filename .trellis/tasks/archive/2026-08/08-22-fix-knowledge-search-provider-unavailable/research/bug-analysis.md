# Bug Analysis: required-auth Knowledge answer consent was never provisioned

## 1. Root Cause Category

- **Category**: B/E — Cross-Layer Contract plus Implicit Assumption
- **Specific Cause**: `cmd/api` treated `AUTH_MODE=development` as the boundary
  for fixed-owner Knowledge answer-consent bootstrap. In the single-server
  product, required auth changes request admission, not the identity of the
  bootstrap owner. Runtime chat resolved the selected server-stored Provider
  as `pjrsvy/server-stored/<model>`, but no matching query or collection answer
  consent existed.

## 2. Why Earlier Diagnosis Did Not Close the Bug

1. Upload and document-state inspection proved parser recovery but did not
   exercise answer egress governance.
2. Service health and a successful Provider chat response excluded neither
   missing Knowledge consent nor endpoint-identity mismatch.
3. A later diagnostic briefly reported Provider unavailability because its
   disposable container mounted the default stale keyring rather than the
   active Compose keyring. Resolving the live mount produced the correct
   1,024-dimension Embedding, 11 candidates, applied Rerank, and authorized
   answer gate.

## 3. Prevention Mechanisms

| Priority | Mechanism | Specific Action | Status |
| --- | --- | --- | --- |
| P0 | Architecture | Bind automatic answer consent to the fixed owner, not auth mode | DONE |
| P0 | Security | Preserve Provider ID, endpoint class, and model in consent identity | DONE |
| P0 | Test coverage | Prove owner receives future collection consent and invited user does not | DONE |
| P1 | Diagnostics | Persist only fixed Knowledge failure-stage categories | DONE |
| P1 | Operations | Verify the deployed image/version and actual keyring mount before live probes | DONE |
| P1 | Documentation | Record the required-auth bootstrap contract in RAG spec and cross-layer guide | DONE |

## 4. Systematic Expansion

- **Similar Issues**: Other startup automation guarded by development auth may
  incorrectly skip fixed-owner initialization in required-auth deployments.
  SiliconFlow retrieval consent remains a separate contract and must retain
  its exact Provider/Profile fences.
- **Design Improvement**: Keep authentication, owner scope, Provider identity,
  and processing consent as four explicit dimensions rather than one mode
  switch.
- **Process Improvement**: A Provider-switch acceptance must exercise chat
  resolution and Knowledge answer governance, and must compare the source
  build version with `/v1/version` before interpreting historical failures.

## 5. Knowledge Capture

- [x] Updated `.trellis/spec/backend/rag-retrieval-storage.md`.
- [x] Updated `.trellis/spec/guides/cross-layer-thinking-guide.md`.
- [x] Updated G18 plan/process maintenance evidence.
- [x] Added owner-isolation and sanitized-stage regression coverage.
- [x] Template sync checked; this repository has no
      `src/templates/markdown/spec/` mirror.
