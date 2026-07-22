# G19 Provider-Native Tool Loop and Process Trace Process

## 2026-07-22 — G19.1 contract and owner lock

The owner reported that enabling the current globe causes external Search on
nearly every Knowledge-unavailable turn. Runtime tracing confirmed the cause:
`useSearch=true` reaches `planSourceFusion`, which sets `SearchRequested=true`
for non-Knowledge questions before the final model request. The recent
conversation-aware Query repair corrected what was searched but did not decide
whether Search was needed.

Kelivo was audited at commit
`545f7d67de250283232c9487aa5f4f42e85a7643`. Its external mode exposes one
`search_web(query)` function beside the retained conversation, sends
`tool_choice:auto`, accumulates streamed Tool Call arguments, executes the
selected provider, appends the provider-native Tool Result, and continues the
same model. When the model emits no Tool Call, no Search request occurs.

The audit also found that Kelivo generally uses `while (true)` until a provider
stops returning Tool Calls. OpenAI Chat Completions, Anthropic, Gemini, and
Vertex paths have no shared total-round or per-tool count. Only its OpenAI
Responses path blocks an identical Tool signature repeated for three
consecutive rounds. Multiple client Tool Calls are executed sequentially.

The owner completed a seventeen-question behavior lock:

- the globe is `off | model_builtin | external`, persisted per conversation,
  inherited by new conversations, and strictly mutually exclusive;
- off means no network planning or Search I/O; enabled modes are Auto, while
  explicit/current Search intent must Search;
- current/public questions may fuse Knowledge with Web evidence;
- Search failure degrades truthfully without fabricated `[W#]` citations;
- a provider-native generic Tool Loop is required, with current-model
  compatibility planning only when function calling is unsupported;
- the single-user deployment deliberately has no product-level Tool Round or
  Tool Call count limit;
- real provider reasoning and factual process steps are displayed separately,
  persisted in sanitized form, live-expanded, and collapsed on completion;
- Reasoning, Search, and Knowledge controls remain independent;
- read-only tools run automatically, while side effects require approval;
- model built-in and external Search never execute together;
- official built-in capabilities are explicit, while custom compatible models
  require administrator opt-in plus a real test;
- unused retrieval sources remain in the process view, while only final-answer,
  current-turn backend-minted markers render Citation cards;
- `search_web` is implemented before the existing RAG chain is promoted to
  `search_knowledge`.

G19.1 adds only research, contract, plan, process, and tracking documentation.
It does not change source code, database schema, Docker state, provider
configuration, credentials, network behavior, or live conversations.

Verification:

```text
Kelivo audit commit recorded                  passed
current Go forced-Search branch traced        passed
17 owner decisions captured                   passed
G19.1-G19.7 scope/rollback boundaries         passed
production code/schema/runtime changes        none
```

Next: implement G19.2 durable provider-reasoning/process-step events and the
reloadable process panel while leaving current Search and RAG execution
unchanged. Test, record, and commit G19.2 before beginning the Tool Loop.
