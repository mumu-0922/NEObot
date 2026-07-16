#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
usage: plan-former-root-deletion.sh [--root <former-root>]

Dry-run only. Prints the legacy top-level application paths that are candidates
for an owner-confirmed former-root cleanup after mm-chat standalone gates pass.
It never deletes files.
USAGE
}

former_root=""
while (( $# > 0 )); do
  case "$1" in
    --root)
      if (( $# < 2 )); then
        echo "plan-former-root-deletion: --root requires a path" >&2
        exit 2
      fi
      former_root="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      exit 2
      ;;
  esac
done

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
mm_chat_dir="$(cd "${script_dir}/.." && pwd -P)"
if [[ -z "${former_root}" ]]; then
  former_root="$(cd "${mm_chat_dir}/.." && pwd -P)"
else
  former_root="$(cd "${former_root}" && pwd -P)"
fi

if [[ ! -d "${former_root}/mm-chat" ]]; then
  echo "plan-former-root-deletion: ${former_root} does not contain mm-chat/" >&2
  exit 1
fi
if [[ ! -x "${former_root}/mm-chat/scripts/verify-standalone.sh" ]]; then
  echo "plan-former-root-deletion: mm-chat/scripts/verify-standalone.sh is missing or not executable" >&2
  exit 1
fi

legacy_candidates=(
  .dockerignore
  .env.example
  .github
  .gitignore
  .mypy_cache
  .next
  .prettierignore
  .ruff_cache
  .wrangler
  AGENTS.md
  CHANGELOG.md
  CODE_OF_CONDUCT.md
  CONTRIBUTING.md
  Dockerfile
  LICENSE
  README.md
  README.zh-CN.md
  ROADMAP.md
  SECURITY.md
  docker-compose.yml
  docs
  eslint.config.mjs
  next-env.d.ts
  next.config.ts
  node_modules
  open-next.config.ts
  package.json
  pnpm-lock.yaml
  pnpm-workspace.yaml
  postcss.config.mjs
  prettier.config.mjs
  public
  scripts
  src
  tailwind.config.ts
  tsconfig.json
  tsconfig.tsbuildinfo
  wrangler.jsonc
)
protected_paths=(
  .agents
  .codex
  .git
  .trellis
  .vscode
  mm-chat
)

contains() {
  local needle="$1"
  shift
  local item
  for item in "$@"; do
    [[ "${item}" == "${needle}" ]] && return 0
  done
  return 1
}

path_size() {
  local path="$1"
  local size
  size="$(du -sh -- "${path}" 2>/dev/null | awk 'NR == 1 {print $1}' || true)"
  printf '%s' "${size:-?}"
}

candidate_exists=()
for rel in "${legacy_candidates[@]}"; do
  if [[ -e "${former_root}/${rel}" || -L "${former_root}/${rel}" ]]; then
    candidate_exists+=("${rel}")
  fi
done

manual_review=()
unclassified=()
while IFS= read -r -d '' top_path; do
  rel="${top_path##*/}"
  if contains "${rel}" "${protected_paths[@]}"; then
    continue
  fi
  if [[ "${rel}" == .env && "${rel}" != .env.example ]] || [[ "${rel}" == .env.* && "${rel}" != .env.example ]]; then
    manual_review+=("${rel}")
    continue
  fi
  if contains "${rel}" "${legacy_candidates[@]}"; then
    continue
  fi
  unclassified+=("${rel}")
done < <(find "${former_root}" -mindepth 1 -maxdepth 1 -print0 | sort -z)

cat <<EOF_HEADER
Former-root deletion dry-run
root: ${former_root}

This script is non-destructive. It prints candidates only.
Do not run deletion until backups, restore drills, visual smoke, and owner
confirmation are recorded in docs/deployment/former-root-delete-plan.md.
EOF_HEADER

printf '\nProtected paths (not deletion candidates):\n'
for rel in "${protected_paths[@]}"; do
  if [[ -e "${former_root}/${rel}" || -L "${former_root}/${rel}" ]]; then
    printf '  %-8s %s\n' "$(path_size "${former_root}/${rel}")" "${rel}"
  fi
done

printf '\nLegacy application deletion candidates:\n'
if (( ${#candidate_exists[@]} == 0 )); then
  printf '  none\n'
else
  for rel in "${candidate_exists[@]}"; do
    printf '  %-8s %s\n' "$(path_size "${former_root}/${rel}")" "${rel}"
  done
fi

printf '\nManual-review top-level env/secret-like paths (not in rm plan):\n'
if (( ${#manual_review[@]} == 0 )); then
  printf '  none\n'
else
  for rel in "${manual_review[@]}"; do
    printf '  %-8s %s\n' "$(path_size "${former_root}/${rel}")" "${rel}"
  done
fi

printf '\nUnclassified top-level paths (not in rm plan):\n'
if (( ${#unclassified[@]} == 0 )); then
  printf '  none\n'
else
  for rel in "${unclassified[@]}"; do
    printf '  %-8s %s\n' "$(path_size "${former_root}/${rel}")" "${rel}"
  done
fi

printf '\nOwner-confirmed deletion command block (dry-run output only):\n'
if (( ${#candidate_exists[@]} == 0 )); then
  printf '  # no legacy candidates found\n'
else
  for rel in "${candidate_exists[@]}"; do
    printf '  rm -rf -- %q\n' "${former_root}/${rel}"
  done
fi

printf '\nNo deletion was performed.\n'
