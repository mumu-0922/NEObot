# Frontend dependency security patch result

## Outcome

- Upgraded `next` and `eslint-config-next` from `16.2.9` to `16.2.12`.
- Upgraded `@opennextjs/cloudflare` from `1.20.1` to `1.20.2`, which resolves
  `@opennextjs/aws@4.1.0` with the patched Next peer boundary.
- Updated security overrides to resolve:
  - `sharp@0.35.0`
  - `postcss@8.5.18`
  - `protobufjs@7.6.5`
  - `dompurify@3.4.12`
  - `js-yaml@4.3.0`
  - compatible `brace-expansion@1.1.16`, `2.1.2`, and `5.0.8` lines
- Official-registry production audit changed from `15 findings / 8 high` to
  `No known vulnerabilities found`.
- Added the generated `frontend/.open-next` tree to the clean-copy exclusion
  list without weakening source symlink validation.

## Compatibility evidence

- A rejected attempt to force every `brace-expansion` consumer to v5 made
  ESLint fail with `TypeError: expand is not a function`; it was reverted.
- The final compatible backports pass ESLint, while the OpenNext Worker build
  proves the `@node-minify` build path.
- `sharp@0.35.0` loaded through Next's dependency path and produced a 91-byte
  one-pixel PNG under Node `22.22.1`.
- The local OpenNext build completed with Next `16.2.12`, Cloudflare `1.20.2`,
  and AWS `4.1.0` and wrote `.open-next/worker.js`.

## Audit evidence

Production command:

```bash
npm_config_registry=https://registry.npmjs.org \
  corepack pnpm audit --prod --audit-level=high
```

Result: `No known vulnerabilities found`.

The unfiltered development audit retains two high reports for
`GHSA-mh99-v99m-4gvg`:

1. ESLint -> Minimatch 3 -> `brace-expansion@1.1.16`.
2. OpenNext/AWS -> node-minify -> Glob 9 -> Minimatch 8 ->
   `brace-expansion@2.1.2`.

These do not enter the production audit. Both receive repository-controlled
glob patterns only; the compatible advisory backports are installed; lint and
the real Worker build pass. Forcing v5 is ABI-incompatible. The upstream exit
is to adopt ESLint 10 after TypeScript ESLint supports it and an OpenNext/AWS
release that removes node-minify 8. This is an explicit bounded build-tool
exception, not an ignored production result.

## Verification

- `corepack pnpm install --frozen-lockfile`: passed with pnpm `10.30.3`.
- Frontend Prettier, ESLint, TypeScript, and Vitest: passed; `193` files and
  `936` tests passed.
- Next production build: passed on `16.2.12`.
- OpenNext Cloudflare Worker build: passed.
- `bash mm-chat/scripts/verify-standalone.sh`: passed after a Worker build.
- `bash mm-chat/scripts/verify-standalone.sh --full`: passed.
  - Frontend: format/lint/typecheck, 936 tests, and build passed.
  - Backend: vet and all Go tests passed.
  - RAG: Ruff/mypy passed; `1,906 passed / 7 skipped`.
- `bash -n mm-chat/scripts/verify-standalone.sh`: passed.
- Scoped `git diff --check`: passed.
- Secret-like diff scan: no match.

## Security scanner triage

The generic source scanner reported only pre-existing pattern false positives:

- the literal private-key delimiter is a parser regular expression in
  `src/lib/byok/pem.ts`, not key material;
- three `innerHTML = ""` calls clear a detached Mermaid render host rather than
  inject attacker-controlled HTML;
- existing `dangerouslySetInnerHTML` sites were not changed by this task and
  remain covered by their established sanitization/tests.

No application source, API, provider, storage, secret, or runtime configuration
changed in this task.

## Rollback

Revert the focused dependency/security commit. No schema, data, secret, or
runtime-state rollback is required.
