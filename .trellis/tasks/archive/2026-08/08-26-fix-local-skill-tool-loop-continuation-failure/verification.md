# Verification

## Code gates

- `go test ./internal/resourceorchestrator ./internal/chat`
- `go vet ./...`
- `go test ./...`
- `go test -race ./internal/resourceorchestrator ./internal/chat`
- `bash mm-chat/scripts/verify-agent-local-runtime.sh`
- `bash mm-chat/scripts/verify-standalone.sh --full`
- `git diff --check`

All passed on 2026-08-26. The standalone gate included 996 Frontend tests and
1910 passed / 7 skipped RAG tests.

## Security and quality gates

- Trellis spec/data-flow review: passed.
- `verify-change`: passed.
- `verify-quality`: passed with only pre-existing file-length warnings.
- `verify-security` on `internal/resourceorchestrator`: zero findings.
- Full `internal/chat` security scan reported only two pre-existing fake-key
  fixtures in untouched `provider_openai_test.go`.

## Deployment and live smoke

- Backend image: `mm-chat/backend:provider-resource-fix-20260826T072408Z`
- Image digest: `sha256:e718ce4d414a45309f43bfef19519f9a7451e7160c6005fdf760526103858b8e`
- Backend container became healthy; database, Redis, and storage readiness were
  all `ready`.
- A disposable authenticated Workspace conversation submitted
  `https://www.aihero.dev/skills-grill-me帮我安装这个skill` against the live API.
- Result: `message.completed=true`, Resource search trace present, truthful
  no-admitted-candidate/no-install answer present, and no Provider/Local Skill
  masked failure.
- Disposable conversation and session cleanup both left zero live rows.
