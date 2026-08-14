# G21.1 activation and identity research

## Boundary

G21.0 activation is intentionally fixed to `control_plane`, authorizes only
`probe/list/reconcile`, and binds
`spiffe://neo-chat/agent-runtime-control`. Reusing that record or certificate
for launch would silently convert maintenance approval into execution
approval.

## Recommended design

- Add an independent strict activation document with stage
  `root_run_canary`, caller
  `spiffe://neo-chat/agent-runtime-root-canary`, Root Runs true and every
  Broker/Child/Cron/Learning capability false.
- Bind the exact canary plan and authority public key fingerprints in addition
  to the G21.0 release/target/mTLS tuple.
- Require a separately mounted canary client certificate/private key,
  authority private key and activation record. None is shared with the control
  worker or API.
- Keep the broad `AGENT_RUNTIME_ENABLED` false. Use a narrower default-off
  `AGENT_ROOT_RUN_CANARY_ENABLED` switch and a separate Compose profile.
- Enforce method policy at the Runner HTTP boundary: control identity receives
  only `probe/list/reconcile`; canary identity receives only
  `probe/list/reconcile/launch/heartbeat/cancel`. Broker methods remain denied.
- Require the canary database LOGIN to inherit exactly
  `agent_orchestrator_runtime` and `agent_runner_control`, with no additional
  recursive memberships or elevated attributes.

## Evidence rule

Checked-in records remain held templates with `ISOLATION_UNAVAILABLE`. A real
activation record is short-lived, content-free, exact-release bound and kept
outside Git. This development machine must continue to fail the exact-host
gate without any installation or credential provisioning.
