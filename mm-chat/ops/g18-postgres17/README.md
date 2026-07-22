# G18.2 PostgreSQL 17 restore drill

This disposable harness proves the database major-version boundary before any
shadow retrieval DDL is added.

```text
fresh PG16.13 + current migrations + synthetic authority/projection fixture
  -> custom logical backup (owners and ACLs preserved)
  -> fresh PG17.10 + bootstrapped capability roles
  -> full restore + extension activation + migration no-op proof
  -> same backup -> fresh PG16 rollback database + migration no-op proof
```

Run from anywhere:

```bash
mm-chat/scripts/run-g18-postgres17-restore-drill.sh
```

The harness has no published ports and never reads a project env file. It uses
a unique Compose project and project-scoped volumes. Its trap removes all
containers, networks, and database volumes. The successful report directory in
`/tmp/mm-chat-g18-postgres17.*` contains only synthetic logs, a synthetic dump,
and its checksum.

The PG17 image guard is also exercised against a fake PG16 `PG_VERSION` file.
Success requires exit `78`, the explicit logical-restore instruction, and an
unchanged fake version file.

This drill does **not** migrate the running mm-chat database. Production
cutover remains G18.5 and must start from a separately verified production
backup. Never pass `mm-chat/data/postgres` to this harness or the PG17 image.
