# Thinking Guides

> **Purpose**: Expand your thinking to catch things you might not have considered.

---

## Why Thinking Guides?

**Most bugs and tech debt come from "didn't think of that"**, not from lack of
skill. Agent allocation adds another failure mode: parallel work can silently
trade delivery speed for shared-state conflicts, review gaps, or context drift.

This guide helps you **ask the right questions before coding**.

---

## Available Guides

| Guide                                                 | Purpose                                                           | When to Use                                                  |
| ----------------------------------------------------- | ----------------------------------------------------------------- | ------------------------------------------------------------ |
| [Agent Orchestration Guide](./agent-orchestration.md) | Allocate Sub-agents and Review Agents without sacrificing quality | When work may benefit from parallelism or independent review |

---

## Quick Reference: Thinking Triggers

### When to Think About Agent Orchestration

- [ ] Two or more bounded workstreams may proceed independently
- [ ] File ownership can remain disjoint or read-only
- [ ] A security-sensitive or complex diff may need independent review
- [ ] Coordination cost may be lower than the time or quality benefit

→ Read [Agent Orchestration Guide](./agent-orchestration.md)

---

## Pre-Modification Rule (CRITICAL)

> **Before changing ANY value, ALWAYS search first!**

```bash
# Search for the value you're about to change
grep -r "value_to_change" .
```

This single habit prevents most "forgot to update X" bugs.

---

## How to Use This Directory

1. **Before coding**: Skim the orchestration guide when parallel work may help
2. **During coding**: Recheck it when coordination cost or shared state appears
3. **After bugs**: Add new insights to the relevant guide (learn from mistakes)

---

## Contributing

Found a new "didn't think of that" moment? Add it to the relevant guide.

---

**Core Principle**: 30 minutes of thinking saves 3 hours of debugging.
