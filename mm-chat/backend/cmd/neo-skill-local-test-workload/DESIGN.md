# `neo-skill-local-test-workload` design

## Goal

Provide a deterministic, content-free payload that can prove the Sandbox view
from inside one real rootless container.

## Design

The binary has no flags and accepts no input. It performs a fixed sequence of
filesystem, process-status, network-namespace, Scratch, and environment checks.
Only after every check passes does it send
`draft-local-test-smoke.json` through `/run/neo-broker/artifact.sock` using the
Runner's bounded length-framed Artifact protocol. It validates the returned
name, media type, size, and SHA-256 receipt, then remains alive briefly so the
Runner can prove and reap a running Sandbox.

## Security decisions

- No shell, subprocess, dynamic command, external network, or durable secret.
- Fixed maximum result size and strict response fields.
- Root/Workspace negative write probes remove any unexpected file before
  failing.
- The result contains only schema, outcome, and fixed check identifiers.
- Artifact connection/retry is bounded; inability to prove current Attempt
  authority fails closed.

## Known limit

This payload is a local isolation smoke, not a general Agent Skill runtime or
production canary.

## Change history

- 2026-08-15: initial fixed local-test workload.
