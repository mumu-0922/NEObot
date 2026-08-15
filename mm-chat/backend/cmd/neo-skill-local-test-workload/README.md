# `neo-skill-local-test-workload`

This static binary is the only payload in the disposable local-test OCI image.
It validates its own sandbox view and publishes one fixed JSON result through
the Runner's Artifact socket.

The workload checks:

- UID/GID `10001`;
- empty effective capabilities and `NoNewPrivs: 1`;
- loopback-only network namespace with no routes;
- read-only root filesystem and synthetic Workspace;
- writable bounded Scratch;
- no Secret path or sensitive environment names.

It contains no prompt, model, Tool, network client, credential, user data, or
arbitrary command interface. Build and execution are owned by the local-test
host manager; direct use is unsupported.

See [`DESIGN.md`](./DESIGN.md).
