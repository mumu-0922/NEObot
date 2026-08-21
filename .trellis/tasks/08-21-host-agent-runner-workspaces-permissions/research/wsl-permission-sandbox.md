# WSL permission sandbox validation

## Target evidence

Validated on the deployment WSL2 kernel
`6.18.33.2-microsoft-standard-WSL2`. Landlock ABI 7 is available.

Landlock correctly allowed Workspace writes and denied outside writes on the
WSL ext4 filesystem. The same allow rule on `/mnt/d` DrvFS/9p denied both
inside and outside writes. It therefore cannot truthfully implement the stated
cross-filesystem `workspace-write` preset on this target.

Bubblewrap with the exact shape below succeeded on both WSL storage and
`/mnt/d`:

```text
--ro-bind / /
--bind <canonical-workspace> <canonical-workspace>
```

The Workspace write succeeded and the outside write failed with
`Read-only file system`. Read Only omits the writable bind. This is a write
boundary, not confidentiality or network isolation; the ordinary Host user's
read authority remains visible.

## Packaging decision

The no-sudo Host wrapper extracts, rather than installs, Ubuntu package
`bubblewrap_0.6.1-1ubuntu0.1_amd64.deb` from the Ubuntu security archive.

```text
Package SHA256: f75c835d6871d1b36370e12ee82940334b2a9f94efc7b959b5b236447e89743d
Binary SHA256:  d78807229d616606e339c5988392b9e0ab4a6a6998fa51e4590837f426a12fca
```

The extracted executable lives under `.runtime/agent-host/bwrap`, mode `0700`,
and must be owned by root or the effective Host user with no group/world write.
Startup executes the same argument builder used for real Terminal calls and
advertises all three modes only when the WSL and available `/mnt/d` probes
pass. A configured external binary is accepted only through the same ownership,
canonical-path, regular-file, execute-bit, and write-bit validation.

## Rejected alternatives

- Do not advertise Landlock-backed Workspace Write on DrvFS after only an ext4
  probe.
- Do not use `apt install`, `sudo`, setuid Bubblewrap, or a privileged Runner.
- Do not infer permission from command text; enforce at process/filesystem and
  structured File Tool boundaries.
