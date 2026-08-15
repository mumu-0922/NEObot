# Agent Runtime Child Canary Command Design

## Purpose

Provide a separately deployable, fail-closed controller for one reviewed
synthetic Parent/Child lineage without enabling generic Agent Runtime.

## Startup chain

1. Parse exact boolean, duration, identity and private-file configuration.
2. Validate the strict canary plan and migration-093 activation evidence.
3. Prove the Ed25519 authority key pair matches.
4. Open the dedicated PostgreSQL LOGIN and verify its recursive membership is
   exactly `agent_orchestrator_runtime`, `agent_runner_control` and
   `agent_delegation_control`.
5. Create the mTLS Runner client, signed authority service, durable delegation
   service and exact Runner Reaper.
6. Run the monotonic canary lifecycle or read-only health proof.

## Security boundaries

- The caller policy has only `probe`, `list`, `reconcile`, `launch`,
  `heartbeat` and `cancel`; Prepare/Commit has no route.
- Runner and Sandboxes receive no database, controller, Provider, object-store,
  vault or MCP credential.
- Logs emit bounded state/error codes only; plan content and credentials are
  never logged.
- Activation stays held when exact-host isolation evidence is absent or stale.

## Known limits

The command intentionally supports one fixed synthetic plan, one Parent and one
Child. It is not a product worker or public API.

## Change history

- 2026-08-15: Initial G21.4 standalone controller.
