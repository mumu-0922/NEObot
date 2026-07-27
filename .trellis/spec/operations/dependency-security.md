# Dependency Security

## Scenario: Patch an audited dependency graph

### 1. Scope / Trigger

Apply this contract when a frontend, Go, Python, container, or GitHub Actions
dependency advisory requires a manifest, override, or lockfile change. Treat
production and development-tool reachability separately, but never hide an
unresolved finding without a written compatibility and input-boundary analysis.

### 2. Signatures

Frontend baseline and audit commands:

```bash
cd mm-chat/frontend
corepack pnpm install --frozen-lockfile
npm_config_registry=https://registry.npmjs.org \
  corepack pnpm audit --prod --audit-level=high
npm_config_registry=https://registry.npmjs.org \
  corepack pnpm audit --audit-level=high
corepack pnpm why <affected-package> --depth 8
```

Release verification:

```bash
corepack pnpm format:check
corepack pnpm lint
corepack pnpm typecheck
corepack pnpm test
corepack pnpm build
bash mm-chat/scripts/verify-standalone.sh --full
```

When OpenNext or its transitive build graph changes, also run
`corepack pnpm build:worker` in an environment where the child `pnpm build`
command can resolve a `pnpm` executable.

### 3. Contracts

- The committed `package.json`, `pnpm-workspace.yaml`, and `pnpm-lock.yaml`
  must agree and install with pnpm 10.30.3 using `--frozen-lockfile`.
- Audit evidence must use the official npm registry. A mirror without the npm
  audit endpoint is an environment failure, not a clean result.
- Update direct framework/tool pairs together when their compatibility is
  coupled, for example `next` with `eslint-config-next`.
- Prefer the smallest compatible parent upgrade. Use a pnpm override only when
  the parent cannot select a patched release and the resolved package contract
  is proven by its actual callers.
- Do not force a patched major solely to make audit output green. Package
  export shape, Node engines, peer ranges, and release builds remain part of
  the security contract.
- Production high/critical findings block delivery. A development-only finding
  may remain only when compatible backports are installed, its input is trusted
  and repository-controlled, the incompatible fix is reproduced, and the
  exact parent chains plus upstream exit are recorded.
- Never combine a bounded security patch with unrelated major upgrades.

### 4. Validation and Error Matrix

| Condition                                                | Required result                                                                           |
| -------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| Official audit endpoint is unavailable                   | Retry against `https://registry.npmjs.org`; do not claim pass.                            |
| Production audit reports high/critical                   | Upgrade or override safely; block commit until clear.                                     |
| Override changes CommonJS/ESM export shape               | Revert the override and patch/assess the parent chain.                                    |
| Peer dependency excludes the proposed parent major       | Do not force it into the graph without an upstream-compatible upgrade.                    |
| Lint, typecheck, unit, standalone, or Worker build fails | Treat the remediation as broken and return to the dependency graph.                       |
| Full audit retains a development-only advisory           | Record exact paths, trusted inputs, compatible backports, and upstream remediation owner. |

### 5. Good / Base / Bad Cases

- **Good**: patch the vulnerable direct framework, update narrow overrides,
  inspect `pnpm why`, clear the production audit, document one unreachable
  build-tool exception, and pass both standalone and Worker builds.
- **Base**: a compatible patch release removes the advisory with only manifest
  and lockfile changes; component and frozen-install gates pass.
- **Bad**: force every old CommonJS consumer onto a new ESM/exports major,
  observe an empty audit, skip lint/build, and publish a toolchain that crashes.

### 6. Tests Required

- Assert the frozen install resolves the intended patched direct versions.
- Run production and full official-registry audits and retain the decisive
  finding counts/paths.
- Run `pnpm why` for every overridden package and confirm no vulnerable version
  remains on production paths.
- Run frontend format, lint, typecheck, unit tests, and Next production build.
- Run OpenNext Worker build when Next/OpenNext/Sharp/PostCSS or build-only glob
  dependencies change.
- Run `verify-standalone.sh --full`; generated `.next` and `.open-next` trees
  must not enter the isolated source copy.

### 7. Wrong vs Correct

#### Wrong

```yaml
overrides:
  "brace-expansion@<5.0.8": 5.0.8
```

This crosses multiple majors. `minimatch@3` expects the legacy CommonJS module
to be callable and fails with `TypeError: expand is not a function`.

#### Correct

```text
audit -> inspect parent paths -> install compatible backports -> run callers
      -> clear production findings -> document bounded dev-only exception
      -> frozen install + component build + Worker build + full clean-copy gate
```
