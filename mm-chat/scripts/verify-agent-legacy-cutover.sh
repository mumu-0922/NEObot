#!/usr/bin/env bash
set -euo pipefail

project_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"

(
  cd "${project_dir}/backend"
  go test ./internal/chat
  go vet ./internal/chat
)

(
  cd "${project_dir}/frontend"
  corepack pnpm exec vitest run \
    src/__tests__/legacySkillRetirement.test.ts \
    src/__tests__/schemas.test.ts \
    src/__tests__/chatEntities.test.ts \
    src/__tests__/effectiveChatContext.test.ts \
    src/__tests__/chatPanelUrlState.test.ts \
    src/__tests__/messageInputComposition.test.ts \
    src/__tests__/messageItemComposition.test.ts \
    src/__tests__/chatAppServerModeComposition.test.ts \
    src/__tests__/agentCenterComposition.test.ts
  corepack pnpm typecheck
)

python3 - "${project_dir}" <<'PY'
from __future__ import annotations

import pathlib
import sys

root = pathlib.Path(sys.argv[1])

required = (
    "backend/internal/chat/legacy_skill_retirement.go",
    "frontend/src/store/storage/legacySkillRetirement.ts",
    "scripts/cutover-legacy-skills.sql",
    "scripts/verify-agent-legacy-cutover-postgres17.sh",
)
for relative in required:
    path = root / relative
    assert path.is_file() and path.stat().st_size > 0, relative

forbidden = (
    "frontend/src/components/agent/LegacySkillCutoverCard.tsx",
    "frontend/src/components/skill",
    "frontend/src/components/skill/SkillMarket.tsx",
    "frontend/src/services/api/skillService.ts",
    "frontend/src/lib/skills",
    "frontend/src/lib/skills/index.ts",
    "frontend/src/lib/skills/legacyCutover.ts",
    "frontend/src/lib/skills/types.ts",
    "frontend/public/data/skills.json",
    "frontend/public/data/skills/index.json",
)
for relative in forbidden:
    assert not (root / relative).exists(), relative

retired_fields = (
    "installedSkills",
    "customSkills",
    "activeSkillIds",
    "skillAutoSelect",
    "skillCatalogs",
    "skillCatalogTimestamps",
    "skillDefinitions",
    "skillDefinitionTimestamps",
)
retirement = (root / "frontend/src/store/storage/legacySkillRetirement.ts").read_text()
for field in retired_fields:
    assert field in retirement, field
assert "LEGACY_SKILL_RETIREMENT_MARKER" in retirement
assert "localStorageRef.setItem(\n      LEGACY_SKILL_RETIREMENT_MARKER" in retirement

allowed_retired_authority_guards = {
    root / "frontend/src/store/storage/legacySkillRetirement.ts",
    root / "frontend/src/lib/chat/entities.ts",
    root / "frontend/src/lib/api/schemas.ts",
    root / "frontend/src/store/storage/migrations.ts",
}
authority_tokens = retired_fields + ("activeSkills", "skillInvocations")
violations: list[str] = []
for path in (root / "frontend/src").rglob("*"):
    if not path.is_file() or path.suffix not in {".ts", ".tsx"}:
        continue
    if "__tests__" in path.parts or path in allowed_retired_authority_guards:
        continue
    text = path.read_text(encoding="utf-8")
    hits = [token for token in authority_tokens if token in text]
    if hits:
        violations.append(f"{path.relative_to(root)}: {', '.join(hits)}")
assert not violations, "retired frontend authority references:\n" + "\n".join(violations)

chat_app = (root / "frontend/src/components/app/ChatApp.tsx").read_text()
chat_service = (root / "frontend/src/services/api/chatService.ts").read_text()
for token in (
    "resolveSkillsForMessage",
    "skillsContext",
    "autoSelectSkills",
    "activeSkillIdsOverride",
):
    assert token not in chat_app, token
    assert token not in chat_service, token

message_item = (root / "frontend/src/components/chat/MessageItem.tsx").read_text()
assert 't("legacySkillRetired")' in message_item
assert "message.skillInvocations" not in message_item

cutover = (root / "scripts/cutover-legacy-skills.sql").read_text()
for signature in (
    "\\set ON_ERROR_STOP on",
    "LOCK TABLE conversations IN SHARE ROW EXCLUSIVE MODE",
    "metadata = metadata - 'activeSkills'",
    "LEGACY_SKILL_EXPECTED_COUNT_MISMATCH",
    "LEGACY_SKILL_BACKUP_FINGERPRINT_REQUIRED",
    "ROLLBACK;",
):
    assert signature in cutover, signature
assert "UPDATE messages" not in cutover
assert "UPDATE conversations\n  SET metadata =" in cutover
assert not list((root / "backend/migrations").glob("091*legacy*"))

runtime_service = (root / "backend/internal/agentcontrol/service.go").read_text()
runtime_migration = (root / "backend/migrations/090_agent_product_shadow.up.sql").read_text()
assert "return ErrIsolationUnavailable" in runtime_service
assert "os/exec" not in runtime_service and "exec.Command" not in runtime_service
assert "'ISOLATION_UNAVAILABLE'" in runtime_migration

print(
    "Agent legacy cutover source verification: hard deletion, bounded history guard, "
    "backup-gated SQL, and held Runtime boundary passed"
)
PY

echo "Agent legacy cutover source verification: passed"
