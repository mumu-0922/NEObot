# Agent Runner local test host

This guide prepares the current Ubuntu 22.04 WSL2 machine to run one harmless
Agent Skill inside the real rootless Runner isolation path. It is a local test
profile, not a production deployment.

## What this does

1. enables systemd in the existing WSL distro;
2. installs Ubuntu's build and rootless-user helpers;
3. downloads and verifies pinned Go, Podman source, crun, and conmon inputs;
4. builds Podman as the normal desktop user in a dedicated local-test root;
5. runs one no-network/no-secret synthetic Skill and removes it completely.

It does not use Docker as an executor, enable an API/Chat Agent path, inspect
real user data, create production evidence, or change the production Runner
manifest and gates.

## The five commands

Run from the repository root.

### 1. Check

```bash
bash mm-chat/scripts/agent-runner-local-test.sh status
```

The initial expected result is `SYSTEM_BOOTSTRAP_REQUIRED`.

### 2. Install the system prerequisites

```bash
sudo bash mm-chat/scripts/agent-runner-local-test.sh bootstrap-system
```

`sudo` prompts in the operator's own terminal. The script never reads a
password from stdin, an environment variable, a file, or a command argument.
It preserves the existing `/etc/wsl.conf` sections, adds `systemd=true`, and
installs only the package allowlist in
`config/agent-runner/local-test-toolchain.lock.json`.
If package installation is interrupted, rerun the same command. It resumes
only while the recorded WSL file and fixed package set still match; otherwise
it stops without guessing.

### 3. Restart WSL once

From Windows PowerShell:

```powershell
wsl --shutdown
```

This stops the current Linux session. Reopen Ubuntu and the repository, then
run the status command again. Do not run the shutdown from an active build.

### 4. Install the pinned local toolchain

```bash
bash mm-chat/scripts/agent-runner-local-test.sh install-toolchain
```

The default root is
`~/.local/share/neo-chat/agent-runner-local-test/`. Downloads are size/hash
verified before extraction. Podman is built from the exact pinned v6.1.0
source archive and its vendored Go dependencies with Go 1.25.9; crun and conmon
are the exact pinned upstream binaries.
The wrapper clears proxy variables and uses dedicated configuration, storage,
and runtime directories.

### 5. Run the real bounded Skill smoke

```bash
bash mm-chat/scripts/agent-runner-local-test.sh smoke
```

Success prints `LOCAL_SKILL_SMOKE_PASSED`. The report is written below the
local-test root with mode `0600`. It contains only fixed check names and cleanup
counts, is marked `local_test`, sets `productionEligible=false`, and is rejected
by the production closure evaluator.

## Status meanings

| Code | Plain meaning | Next action |
| --- | --- | --- |
| `SYSTEM_BOOTSTRAP_REQUIRED` | system packages or WSL setting are missing | run step 2 |
| `RESTART_REQUIRED` | the setting is written but this WSL session is old | run step 3 |
| `CGROUP_DELEGATION_REQUIRED` | systemd is running without a usable user cgroup | close/reopen WSL; inspect systemd if it persists |
| `TOOLCHAIN_INSTALL_REQUIRED` | the safe host is ready but Podman is not installed | run step 4 |
| `LOCAL_SMOKE_READY` | the local execution box is ready | run step 5 |

The checked-in production command remains expected to fail on this profile:

```bash
bash mm-chat/scripts/verify-agent-runner-host.sh
# ISOLATION_UNAVAILABLE
```

That is intentional. A desktop-user local test is not an approved dedicated
production Runner account or production Isolation Acceptance.

## Rollback

Remove the isolated user toolchain first:

```bash
bash mm-chat/scripts/agent-runner-local-test.sh rollback-user
```

Then restore the recorded WSL setting and remove only packages that were absent
before this bootstrap:

```bash
sudo bash mm-chat/scripts/agent-runner-local-test.sh rollback-system
```

Run `wsl --shutdown` once more for the restored WSL setting to take effect.
Rollback refuses a changed `/etc/wsl.conf`, a managed Sandbox still present,
binary/receipt drift, an unsafe path, or a modified package name. It never runs
`apt autoremove` and never reads or changes `.env.single-server`, `data/`,
`secrets/`, or `backup/`.

## Next boundary

A passing smoke proves the local execution box only. Connecting an installed
Package Skill to the visible Agent page is a separate slice and must reuse this
proven path rather than enable production flags or bypass server authority.
