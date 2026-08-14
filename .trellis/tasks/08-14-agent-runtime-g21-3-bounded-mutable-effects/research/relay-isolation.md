# G21.3 relay and identity isolation research

## Question

Should the mutable canary reuse the G21.2 Broker identity/profile/relay?

## Existing repo facts

- G21.0 control, G21.1 Root and G21.2 Broker callers have distinct Runner method
  policies. G21.2 also owns object-store and MCP credentials that a Project-only
  canary does not need.
- Runner currently relays Broker Prepare/Commit through one private mTLS tuple.
  Reusing the G21.2 process would widen its strict five-action plan and make its
  existing evidence ambiguous.

## Decision

Use a fourth caller, controller/profile, relay server identity and ninth
PostgreSQL LOGIN. Extend Runner relay configuration to an exact caller-to-relay
map with two independently complete tuples:

- G21.2 Broker/Artifact caller -> existing Broker relay;
- G21.3 Project-mutation caller -> new Project relay.

The handler selects a relay only after mTLS caller and signed ticket validation.
Missing, partial or cross-wired tuples fail closed. Control/Root callers cannot
reach either relay; Broker and Project callers cannot cross-route.

The Project canary service receives only its database URL, Runner/relay TLS,
activation/plan/approval public material and no S3, MCP, Provider, vault or
generic Egress credential. Sandbox remains `networkMode=none` and receives none
of those controller inputs.
