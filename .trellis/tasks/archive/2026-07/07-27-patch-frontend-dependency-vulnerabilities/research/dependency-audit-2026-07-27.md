# Frontend dependency audit — 2026-07-27

## Baseline

Command:

```bash
npm_config_registry=https://registry.npmjs.org \
  corepack pnpm audit --prod --audit-level=high
```

Result: 15 production dependency findings: 1 low, 6 moderate, and 8 high.
The unfiltered audit reports 21 findings: 1 low, 6 moderate, and 14 high.

## Decisive high findings

- `next@16.2.9`: four high advisories covering App Router
  Middleware/Proxy bypass, Server Actions denial of service, custom-server
  Server Actions SSRF, and rewrite destination SSRF. Each advisory identifies
  `>=16.2.11` as patched. The current stable release is `16.2.12`.
- `sharp@0.34.5`: inherited libvips findings; the advisory identifies
  `>=0.35.0` as patched. Next `16.2.12` still declares `sharp@^0.34.5`, so a
  narrow override and build verification are required if the audit is to
  close before Next changes its optional range.
- `postcss@8.5.10` / `8.5.15`: source-map arbitrary read/path traversal;
  patched at `>=8.5.18` for the broadest advisory.
- `brace-expansion@1.1.13`, `2.0.3`, and `5.0.7`: denial-of-service/OOM
  findings. One advisory recognizes backports `1.1.16` and `2.1.2`, but the
  newer OOM advisory marks every version `<=5.0.7` vulnerable and recognizes
  only `5.0.8` as patched. A validation attempt that forced all copies to
  `5.0.8` was rejected: `minimatch@3` expects `require("brace-expansion")` to
  return a function, while v5 returns an exports object, causing ESLint to fail
  with `TypeError: expand is not a function`. The compatible backports must
  remain while the old parent chains are upgraded or explicitly assessed.
- `js-yaml@4.2.0`: quadratic CPU consumption; patched at `>=4.3.0`.

## Repository constraints

- `pnpm-workspace.yaml` already carries narrow security overrides, including
  now-stale pins for brace expansion, PostCSS, and js-yaml. Updating the owned
  override policy is more explicit than adding ad-hoc direct dependencies.
- The frontend targets Node.js 22. `sharp@0.35.0` requires Node `>=20.9.0` and
  `brace-expansion@5.0.8` supports Node 20 or 22+, so their declared engines fit
  the project runtime.
- Source inspection found no `next/image`, `<Image>`, `ImageResponse`, or direct
  `sharp` use. Nevertheless the Next standalone artifact installs the optional
  dependency, so audit/build closure is preferable to an undocumented ignore.
- `next@16.2.12` remains compatible with React 19 and the current compiler
  plugin. `@opennextjs/cloudflare@1.20.2` is the compatible patch available for
  the existing 1.20 line.
- After the first high-severity remediation pass, the production audit exposed
  one moderate `protobufjs@7.6.3` parser DoS fixed in `7.6.5` and one low
  `dompurify@3.4.11` custom-element hook inconsistency fixed in `3.4.12`.
  Existing code does not parse user-supplied `.proto` schemas or configure
  DOMPurify custom elements, but patched overrides are available without a
  product change and are included rather than recorded as exceptions.

## Chosen approach

1. Update Next and its ESLint config to `16.2.12`.
2. Refresh only compatible build parents needed for the patched graph.
3. Update existing narrow overrides for all audited affected version ranges.
4. Reinstall, re-audit, inspect resolved paths, and run the full standalone
   gate.

The OpenNext build passed after exposing a pre-existing gate-order issue:
`build:worker` creates a gitignored `.open-next` bundle containing symlinks,
while `verify-standalone.sh` originally excluded `.next` but copied
`.open-next`. The standalone gate therefore rejected generated output as if it
were source. Excluding `.open-next` at the same tar boundary restores a
repeatable `Worker build -> clean-copy verification` sequence without relaxing
the symlink check for source files.

Do not use an unrestricted `--latest` refresh because current latest versions
include unrelated major upgrades such as TypeScript 7, ESLint 10, Google GenAI
2, and KaTeX 0.18.
