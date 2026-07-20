# G11.10 In-thread Chat Generation Progress

## 2026-07-20 — Empty-wait progress closure

### Outcome

Server chat no longer leaves an unexplained blank area between the persisted
user message and the first assistant stream event. The message stream now shows
one compact, accessible status row:

- `正在检索知识库` for stable questions with bound Knowledge;
- `正在搜索网页` when Search will handle a current/public question, or when
  Search is the only enabled external source;
- `正在生成回答` when no source capability is active;
- `正在生成回复` after an empty assistant draft exists and the provider is
  starting to stream.

Image generation keeps its dedicated elapsed-time progress component. The
removed composer-top pipeline bar remains removed.

### Root cause and boundary

The Go stream handler performs Knowledge retrieval, source routing, and
external Search before it creates the assistant message and writes
`message.started`. The frontend therefore had no server event to render during
the slowest pre-stream interval, even though the user message was already
visible.

This slice does not reorder the Go transaction/error boundary. While the server
generation is still `pending`, the frontend derives a truthful coarse status
from the persisted conversation configuration and the same current-public
intent vocabulary used by the Go source-fusion router. It does not invent a
percentage or claim document/result counts. Once `message.started` creates the
assistant draft, normal stream state takes authority.

### Implementation

- `src/lib/chat/generationProgress.ts` owns deterministic stage inference and
  multilingual current-public markers.
- `src/components/chat/ChatGenerationProgress.tsx` renders the compact icon,
  label, motion, `role=status`, `aria-live=polite`, and `aria-busy=true`.
- `ChatApp.tsx` renders the pre-stream row only for the exact pending server
  user message, excluding image generation.
- `MessageItem.tsx` replaces the unlabelled bubble animation for an empty
  assistant draft with `正在生成回复`.
- Chinese, English, and Japanese labels are present without explanatory UI
  copy.

### Verification

| Gate                                        | Result                                            |
| ------------------------------------------- | ------------------------------------------------- |
| focused progress and removed-pipeline tests | 9 passed                                          |
| frontend format / lint / typecheck          | passed                                            |
| full frontend tests                         | 179 files, 862 tests passed                       |
| frontend production build                   | passed                                            |
| Compose source rebuild                      | backend and frontend healthy                      |
| real browser, Search on, `西安天气预报`     | web-search status appeared about 0.3 s after send |
| terminal answer                             | real provider answer and Web sources rendered     |
| smoke cleanup                               | temporary two-message conversation deleted        |

### Risk and rollback

The pending label is coarse because the current SSE contract starts after
Knowledge/Search preparation; it is not a server-emitted fine-grained trace.
The marker set must remain aligned with `backend/internal/chat/source_fusion.go`.
A future protocol slice may emit authenticated phase events if exact
Knowledge-to-Web transitions are required.

Rollback is frontend-only: revert the progress component/helper, the two render
sites, locale keys, tests, and documentation. No schema, persisted message, API,
provider configuration, or secret changed.
