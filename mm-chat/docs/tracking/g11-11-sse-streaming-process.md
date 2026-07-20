# G11.11 Browser SSE Streaming Repair

## 2026-07-20 — Proxy compression buffering closure

### Outcome

Chat and image-generation SSE responses now carry
`Cache-Control: no-cache, no-transform`. Next.js therefore does not gzip the
rewritten `/mm-api` stream, and browser clients receive `message.delta` frames
while the provider is generating instead of receiving the full answer at the
terminal event.

### Root cause

The provider and Go handler were already correctly streaming:

```text
provider data frame -> ProviderEventDelta -> message.delta -> Flush()
```

The production frontend uses the same-origin Next.js rewrite
`/mm-api/* -> backend:8080/*`. Browsers advertise compressed encodings. Next's
compression middleware treated `text/event-stream` as compressible, buffered
small writes inside gzip, and released them together when the stream ended.
`X-Accel-Buffering: no` does not control the Next compression middleware.

### Evidence

| Probe                                                 | Result                                                                    |
| ----------------------------------------------------- | ------------------------------------------------------------------------- |
| direct Go endpoint                                    | 118 deltas across 3.176 s                                                 |
| Next rewrite without compressed request               | 79 deltas across 4.770 s                                                  |
| Next rewrite with browser-like compression before fix | `Content-Encoding: gzip`; 85 deltas across 0.001 s                        |
| Next rewrite with browser-like compression after fix  | no `Content-Encoding`; deltas delivered progressively                     |
| browser DOM before fix                                | assistant content jumped from 0 to 319 characters at completion           |
| browser DOM after fix                                 | assistant content advanced from 4 to 318 characters over multiple renders |

The regression assertion now requires `no-transform` on every successful chat
stream response, including the image-generation SSE path. Full backend tests,
race tests, vet, source image rebuild, and backend/frontend health passed. All
temporary browser and API smoke conversations were deleted.

### Boundary and rollback

The fix is intentionally targeted to SSE responses; global Next compression
remains enabled for ordinary pages and assets. Provider chunk size and cadence
remain provider-controlled, so a provider may still emit short bursts, but the
proxy no longer merges the entire answer.

Rollback reverts the two response-header changes, regression assertion, and
documentation. No schema, stored provider configuration, Key, conversation, or
frontend state contract changed.
