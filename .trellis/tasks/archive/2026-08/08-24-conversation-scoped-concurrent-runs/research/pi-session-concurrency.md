# Pi Web Session Concurrency Pattern

## Sources

- [Pi Web README](https://github.com/agegr/pi-web/blob/main/README.md)
- [Pi Web RPC manager](https://github.com/agegr/pi-web/blob/main/lib/rpc-manager.ts)
- [Pi Web session sidebar](https://github.com/agegr/pi-web/blob/main/components/SessionSidebar.tsx)
- [Pi Web agent-session hook](https://github.com/agegr/pi-web/blob/main/hooks/useAgentSession.ts)

## What Pi owns

- The workspace/current working directory groups sessions and supplies their
  filesystem context. It is not the generation lock.
- Each session ID owns an independent `AgentSessionWrapper`. Pi Web keeps those
  wrappers in a registry keyed by session ID, so two sessions under the same
  project can continue independently.
- Running state is calculated per wrapper from prompt, stream, compaction, and
  shell activity rather than from the currently selected sidebar row.
- Prompt admission is serialized within one session. While that session is
  active, new input follows Pi's `steer` or `followUp` queue behavior instead of
  starting a second conflicting agent loop.

## Navigation and sidebar behavior

- Selecting another session changes the UI subscription; it does not destroy
  the previous server-side wrapper.
- The sidebar maintains a server-derived set of running session IDs and renders
  an accessible animated indicator for every matching row.
- Visible tabs poll the running-session set, and the active-session hook also
  reconciles on visibility and online changes. This repairs missed terminal
  events and prevents permanently stuck indicators.
- When a background session changes from running to stopped, Pi marks it unread
  so completion remains visible after the spinner disappears.

## Mapping to Neo Chat

```text
Host Workspace / filesystem context
  |-- Conversation A -> Run A (running)
  |-- Conversation B -> Run B (running)
  `-- Conversation C -> idle
```

- Adopt Pi's ownership boundary: one active Run per conversation, concurrency
  across different conversations, even when they share a Host Workspace.
- Key Frontend controllers, request tokens, stop actions, and running indicators
  by conversation ID.
- Derive the sidebar active set from Backend-owned durable message state and
  reconcile it while active. Neo Chat can be stronger than Pi Web here because
  PostgreSQL already persists assistant generation status and `runId`.
- Keep same-conversation queuing/steering outside the first implementation;
  block a second send until that conversation's active Run finishes. This
  preserves message ordering while leaving a future queue feature possible.
- The background-completion unread marker is useful Pi parity but is separable
  from the requested running spinner.
