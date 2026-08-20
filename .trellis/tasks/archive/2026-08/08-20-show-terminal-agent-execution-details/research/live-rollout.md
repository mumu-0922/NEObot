# Terminal presentation live rollout

## Candidate and rollback

- Source commit: `64838937` (`feat: show Terminal agent execution details`)
- Backend candidate:
  `mm-chat/backend:terminal-presentation-64838937-20260820T085115Z`
- Frontend candidate:
  `mm-chat/frontend:terminal-presentation-64838937-20260820T085115Z`
- Protected rollback directory:
  `mm-chat/backup/terminal-presentation-64838937-20260820T085115Z`
- The directory retains the mode-`0600` prior environment, old Backend and
  Frontend image identities, retained image tags, before/after container IDs,
  health responses, image inspections, rollout script, live SSE and compact
  acceptance result. It is runtime state and is not committed.

The first rollout attempt correctly recreated only Backend/Frontend and passed
health, but its compiled-artifact assertion incorrectly required the runtime
`$NEO_CHAT_WORKSPACE` value to exist as a literal in the Frontend bundle. The
rollback trap restored the prior environment and recreated both old images.
The assertion was narrowed to the static localized Terminal card markers and
the same candidates were redeployed successfully.

## Live identities

```text
Backend container: c2a851f29800
Backend image:     sha256:a6c4eb54d58c6c462a9312e920e0b0f620cfaf54a438ebfe2f0ab67edf3f738d
Frontend container: 32f0ef85453b
Frontend image:     sha256:1d046460936e3db413f5624e20f5af880df851726740f8f047b4c46658b1cf82
Backend health: healthy
Frontend health: healthy
```

Only `mm-chat-backend-1` and `mm-chat-frontend-1` changed identity. The final
container set and all unrelated container IDs match the pre-rollout capture.
Direct Backend readiness, Frontend-proxied readiness, Frontend root, compiled
Terminal card locale markers, and recent fatal/panic log checks passed.

## Live Agent acceptance

At `2026-08-20T08:54:54Z`, an exact temporary current-user session sent one
Agent-mode Turn asking Terminal to execute a read-only `pwd` plus marker
command. The same `gpt-5.6-sol` Provider completed normally.

Decisive assertions:

```text
requestedToolModeAgent=true
effectiveToolModeAgent=true
successfulTerminalObserved=true
liveTerminalPresentation=true
durableTerminalPresentation=true
liveReloadParity=true
stableWorkspaceAlias=true
rawOutputAbsent=true
verifyCompletionAbsent=true
streamHasNoError=true
```

The live stream contained Terminal running and completed ProcessSteps. Their
presentation command was identical, cwd began with `$NEO_CHAT_WORKSPACE`, and
the completed card retained `exitCode=0`. Conversation reload reconstructed the
same presentation from durable Agent events. Neither presentation contained
stdout nor stderr. The two temporary acceptance Conversations (the first was
created by an assertion-only failed proof) were deleted by exact title/time
scope after evidence capture; temporary sessions were removed.

## Rollback

Restore
`backup/terminal-presentation-64838937-20260820T085115Z/.env.single-server.before`
atomically to `.env.single-server`, then recreate only `backend frontend` with
`compose.single-server.yml + compose.production.yml`, `--no-build --no-deps
--pull never --force-recreate`. The retained pre-rollout image tags and exact
image IDs are recorded inside the protected rollback directory.
