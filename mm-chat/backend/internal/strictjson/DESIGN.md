# strictjson Design

## Goals

- Make Provider JSON deterministic before it can influence durable state.
- Keep duplicate, unknown, missing, and trailing-value handling identical
  across Memory planners.
- Remain dependency-free beyond the Go standard library.

## Non-goals

- Defining domain schemas or accepting partially compatible versions.
- Sanitizing text, authorizing IDs, or deciding whether model output may be
  executed.
- Providing a general-purpose streaming JSON parser.

## Decode pipeline

```text
bounded bytes
  -> token walk rejects recursive duplicate keys and unbalanced delimiters
  -> encoding/json with DisallowUnknownFields
  -> caller custom UnmarshalJSON may require exact keys
  -> second decode must return EOF
  -> domain validation and authority rebinding remain with the caller
```

Two passes are intentional. `encoding/json` otherwise accepts duplicate object
keys using last-value-wins behavior, which is unsafe for action, target, and
revision fields. The second pass retains normal Go decoding and custom
unmarshal behavior without implementing a separate schema engine.

## Decisions

| Decision | Reason | Consequence |
| --- | --- | --- |
| Recursive token walk before struct decode | Duplicate keys can appear inside target arrays, not only at the root | The body is parsed twice, bounded to small Provider outputs |
| Exact-key helper is separate | Nullable fields must still be distinguishable from missing fields | Callers opt in through custom `UnmarshalJSON` |
| Domain-neutral errors | Provider output is untrusted and must not be echoed | Callers map failures to bounded result codes |
| No permissive compatibility mode | Versioned planners require fail-closed rollout | Schema changes require an explicit new version |

## Security considerations

- Never log the original body from a decode error; it may contain private
  Memory or credentials.
- `RequireExactKeys` alone does not own recursive duplicate detection. Use it
  only beneath `Decode` at an external-data boundary.
- A successful decode is still proposal data. User, scope, target, revision,
  and persistence authority must be rebound independently.

## Known limits

- The entire bounded body is held in memory and parsed twice.
- JSON numeric semantics remain those of the destination Go type.
- Semantic cross-field validation belongs to each caller.

## Change history

- 2026-07-28: extracted the Memory worker strict decoder for reuse by the
  direct-user Memory action planner.
