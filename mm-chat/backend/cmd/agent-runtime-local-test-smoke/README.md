# `agent-runtime-local-test-smoke`

This command runs one synthetic, explicitly non-production Skill through the
real `agentrunner` Workspace, signed request, rootless Podman, artifact intake,
result, cancel/reap, and cleanup lifecycle.

It is installed only by `scripts/agent-runner-local-test.sh install-toolchain`.
Operators normally invoke it through:

```bash
bash mm-chat/scripts/agent-runner-local-test.sh smoke
```

The direct command requires absolute paths for the pinned Podman launcher,
static workload, reviewed seccomp profile, and disposable state root. Its only
stdout value is one `neo.agent-runner-local-test-report/v1` JSON document. All
four paths must use their fixed names below the same private local-test root;
symlinked or group/world-accessible roots are rejected.

This command uses no database, Provider, MCP, user Project, Egress, Secret,
production release manifest, activation record, or promotion authority.

See [`DESIGN.md`](./DESIGN.md) and
[`../../../docs/deployment/agent-runner-local-test.md`](../../../docs/deployment/agent-runner-local-test.md).
