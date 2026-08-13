#!/usr/bin/env bash
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MANIFEST="${1:-$PROJECT_DIR/config/agent-runner/release-manifest.example.json}"

cd "$PROJECT_DIR/backend"
go run ./cmd/neo-runner-probe --manifest "$MANIFEST"
