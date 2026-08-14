# `neo-runnerd` host template

This directory is a reviewed installation contract, not an installer. G20.3
does not create the `neo-runner` account, write subordinate-ID ranges, install
Podman, write certificates, start systemd, or modify the live host.

G21.0 packages this contract with static Runner binaries into a deterministic
derived artifact without installing it:

```bash
bash scripts/build-agent-runner-bundle.sh \
  --output /secure/release/neo-runner-g21.0 \
  --release-commit "$RELEASE_COMMIT"
python3 scripts/verify-agent-runner-bundle.py \
  --bundle /secure/release/neo-runner-g21.0 \
  --expected-class template
```

The builder refuses an existing output, writes only the explicit derived path
and disables module network resolution. Its manifest binds the exact seven
payload paths, modes, sizes and SHA-256 values. Production additionally
requires a clean exact Git commit and a non-placeholder approved release
manifest. The checked-in manifest is deliberately unapproved and can produce
only a template bundle.

Before copying this template:

1. install the exact approved release tuple from the release manifest;
2. create a non-login `neo-runner` identity with unique subuid/subgid ranges;
3. provision a delegated cgroup v2 subtree and mode-0700 state directories;
4. provision mode-0600 mTLS/private-key material and the authority public key;
5. replace every zero fingerprint, set `approved=true`, and review/sign the
   complete manifest as one release artifact;
6. run `scripts/verify-agent-runner-host.sh` as the exact service user;
7. store its complete approved, <=24-hour Isolation Acceptance report at the
   manifest-bound path/hash; a probe-only report is insufficient;
8. keep global Agent Runtime disabled until that evidence is reviewed.

Before promotion, hash-bind the accepted release to
`config/agent-runner/production-policy.json` and evaluate the separately
captured content-free record:

```bash
bash scripts/verify-agent-production-closure.sh \
  --record /secure/operator-evidence/agent-production-closure.json
```

Only `PROMOTION_READY` from `production` evidence is eligible for a separate
activation decision. The checked-in template and this development host remain
`PROMOTION_HELD` / `ISOLATION_UNAVAILABLE`; the evaluator never starts or
reconfigures this service.

The first activation stage is narrower than final promotion. Capture a fresh
`neo.agent-production-activation/v1` `control_plane` record and run
`scripts/evaluate-agent-production-activation.py` with the exact manifest and
Runner binary, private HTTPS endpoint, client certificate, server CA,
identities and release commit. Only `ACTIVATION_READY` may start the separate
`agent-runtime-control` Compose profile, whose sole RPC authority is
`probe`/`list`/`reconcile`. Root Run and every Broker/Child/Cron/Learning path
remain disabled.

`Delegate=yes` requires the service to write only its delegated cgroup subtree,
so the unit deliberately sets `ProtectControlGroups=no`. Rootless Podman also
needs the explicit user/mount/PID/IPC/UTS/cgroup namespace allowlist; a blanket
`RestrictNamespaces=yes` would make every launch fail after probe.

The pinned `newuidmap`/`newgidmap` helpers need their reviewed setuid transition.
Therefore service-level `NoNewPrivileges=yes` and an empty capability bounding
set are forbidden: the unit leaves Ambient capabilities empty and permits only
`CAP_SETUID`/`CAP_SETGID` in the bounding set for those helpers. Every created
Sandbox is independently inspected for empty capabilities and
`no-new-privileges` before start.

The unit intentionally does not use Docker, `sudo`, `--privileged`, host
network, an application Compose service, or a database/object-store credential.
