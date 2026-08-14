# G20.9 historical invocation retirement projection

## Current leak

`Message.skillInvocations` exposes a list of legacy IDs, titles, descriptions,
categories and manual/auto modes. `MessageItem` renders one titled badge per
entry and may show the old description. This makes deleted definitions appear
current and leaves identity/details available to future callers.

## Narrow projection

Normalize every non-empty, valid legacy invocation array into:

```ts
{ legacySkillRetired: true }
```

Discard the array and every item field immediately. Render exactly one neutral,
non-interactive history label using the Message locale:

```text
旧版技能已退役
```

English/Japanese locales communicate the same retirement fact. There is no
Tooltip with old detail, install/edit/open action, package name match, prompt
injection or Tool authority.

## Entry points

- API/history schema: accept only the bounded historical shape needed for old
  records, then transform it to the boolean fact.
- Browser message normalization: perform the same collapse for IndexedDB arrays
  and already-normalized facts; this path is the persistence migration guard.
- Runtime `Message` type: expose only `legacySkillRetired?: true`.
- UI: one `role=status`-compatible visual fact, no per-invocation iteration.

Server Chat messages do not have a typed legacy invocation field; do not add one.
If an old local message is sent back as model history, the provider receives
message content only and no retired metadata authority.

## Verification

Tests must prove multiple old invocation entries yield one label; serialized
normalized output contains none of the old ID/title/description/mode values;
ordinary message content, Tool calls, RAG evidence and attachments remain
unchanged; new messages cannot create the retirement fact through a Skill
resolver because that resolver no longer exists.
