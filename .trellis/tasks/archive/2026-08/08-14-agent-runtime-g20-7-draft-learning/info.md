# G20.7 technical design index

- Product scope: `prd.md`
- Provenance research: `research/immutable-draft-provenance.md`
- Check safety research: `research/evaluation-and-gaming-boundaries.md`
- Promotion/cleanup research: `research/human-promotion-and-cleanup.md`
- Owning source: `mm-chat/backend/internal/agentlearning/`
- Owning persistence: migration `089_agent_draft_learning`
- Verification: `mm-chat/scripts/verify-agent-learning{,-postgres17}.sh`

The slice is held: no HTTP/frontend/Chat/startup/Compose wiring and no production
Runtime, Scheduler or evaluator activation.
