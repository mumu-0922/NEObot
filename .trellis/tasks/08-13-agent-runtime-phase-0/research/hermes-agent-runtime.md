# Hermes Agent Runtime Research

## Evidence Pin

```text
repository: NousResearch/hermes-agent
commit: ae56c97c6063a87250e74eccfb5697dd303bf930
version: 0.20.0
license: MIT
captured: 2026-08-13
```

The source snapshot was inspected locally at that exact commit. Source and docs
were treated as design evidence, not Neo runtime authority.

## Reusable Patterns

- Skill progressive disclosure: metadata is discoverable before full `SKILL.md`
  or supporting files are loaded into context.
- Skill packages can carry instruction, scripts and references rather than only
  a browser-stored prompt string.
- Delegation has explicit lifecycle/context, narrower leaf tools and cancellation.
- Cron executes isolated sessions and persists jobs/execution facts.
- Write approval, safety guardrails and ESTOP are separate controls rather than
  one prompt instruction.
- Background review/self-improvement mechanisms make learning observable and
  reviewable.

Representative implementation surfaces:

```text
agent/skill_preprocessing.py
agent/skill_bundles.py
tools/skills_tool.py
tools/skill_manager_tool.py
tools/delegate_tool.py
tools/async_delegation.py
agent/delegation_context.py
cron/jobs.py
cron/scheduler.py
tools/write_approval.py
agent/estop.py
agent/background_review.py
agent/learning_mutations.py
```

Hermes also carries a leaf-agent blocklist including `delegate_task` and
`cronjob`. This confirms tool narrowing is necessary, but Neo requires a stronger
construction-time guarantee.

## Rejected Inheritance

- Hermes supports configurable/deeper delegation. Neo permanently caps depth at
  one and physically excludes `delegate_task` from every Child registry.
- Hermes environments include deployment modes that may start as root or add
  capabilities. Neo requires a non-root host daemon plus rootless OCI, empty
  capabilities and fail-closed probe evidence.
- Hermes can mutate/improve Skills through runtime learning paths. Neo learning
  may only emit quarantined Draft packages; validation plus human Promote creates
  a new immutable fingerprint.
- Local file/SQLite/job authority is insufficient for Neo's restart, lease,
  multi-process and audit requirements. PostgreSQL remains durable authority.
- Hermes behavior and package formats are not copied wholesale; Neo adopts the
  capability concepts under its own contracts.

## Neo Mapping

| Hermes concept | Neo decision |
| --- | --- |
| Skill discovery/load | Agent Skills package + progressive metadata/full-content load |
| Toolsets/leaf blocklist | server-issued Capability Grant + construction-time child registry filtering |
| Delegation | durable Child Run, fixed depth 1, subset snapshots |
| Cron | frozen scheduled Run template with current revoke/kill recheck |
| Approval | Prepare/Commit plus explicit approval class |
| ESTOP | hierarchical durable Kill Switch |
| Learning mutation | Draft-only candidate, validation evidence, human Promote |

## Conclusion

Hermes is a capability benchmark and pattern source, not an embeddable security
boundary for Neo. The recommended path is a clean-room Neo runtime that borrows
progressive Skills, lifecycle and control concepts while hardening durability,
supply chain, delegation and isolation.
