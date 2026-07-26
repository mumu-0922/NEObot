## Summary

-

## Test plan

- [ ] `bash mm-chat/scripts/verify-standalone.sh --full`
- [ ] Frontend: format, lint, typecheck, test, and build
- [ ] Backend: `go vet ./...` and `go test ./...`
- [ ] RAG: Ruff format/lint, mypy, and pytest
- [ ] Compose config rendered with `mm-chat/.env.single-server.example`

Check only the components affected by this change and list any intentionally
skipped gate below.

## Notes

- [ ] I updated `mm-chat/docs/` or examples when behavior changed.
- [ ] I added or updated tests for user-facing or security-sensitive changes.
- [ ] I did not include secrets, private logs, database/object backups, or user
      files.
- [ ] I preserved `mm-chat/data/`, `mm-chat/secrets/`, `mm-chat/backup/`, and
      the live `mm-chat/.env.single-server`.
