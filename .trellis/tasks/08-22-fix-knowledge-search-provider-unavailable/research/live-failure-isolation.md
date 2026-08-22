# Live Knowledge failure isolation

## Evidence

- The failed 19:54 turn persisted `dependency_unavailable` for Provider
  `PJRSVY:gpt-5.6-terra` and selected collection `test`.
- All Compose services are healthy; the RAG Worker is not on the query-time
  retrieval path.
- The active retrieval generation is the frozen SiliconFlow BGE-M3 profile.
- A bounded live embedding probe returned the expected model and 1,024-vector
  shape.
- The exact native tool query returned 11 fenced hybrid candidates.
- Four lexical candidates reauthorized and hydrated against the failed turn's
  active user/session/conversation.
- `PJRSVY` is enabled and its model-provider connection fingerprint is valid.
- No `pjrsvy/server-stored/gpt-5.6-terra` query or collection answer consent
  exists. Only the independent `openai_compatible/server-default` consent
  exists for that model.
- The live deployment uses required authentication, while startup answer
  consent provisioning is development-only.

## Constraints

- Never reuse a different endpoint's answer consent just because the providers
  speak the same wire protocol.
- Failure diagnostics may record a fixed stage/category only; they must not
  record query text, Knowledge plaintext, credentials, URLs, or raw provider
  response bodies.
- Query-time retrieval must remain fail closed and may fall back to the normal
  model answer only without Knowledge evidence or citation markers.

## Implementation direction

1. Preserve server-stored Provider IDs as answer-governance processor
   identities.
2. Make the missing-consent path explicit and repair the required-auth
   provisioning/lifecycle for the fixed deployment owner instead of aliasing it
   to `server-default`.
3. Add sanitized stage-level coverage so future dependency failures can be
   distinguished without exposing private inputs.
