# Agent Broker

`agentbroker` is the held G20.4 control foundation for deterministic Agent Tool
registries and durable side effects. It has no HTTP, Chat or startup wiring.

The package owns:

- exact Capability Grant intersection and depth-1 Tool removal;
- PostgreSQL-backed Prepare, approval, Cancel/Commit race, Grant revocation,
  receipt and recovery flow;
- shared-policy Egress and single-use Secret mediation with in-memory
  zeroization on terminal/revocation paths;
- Project patch/CAS and Artifact publication interfaces;
- the mutable MCP executor adapter, which never retries after a possible send.

Production Agent execution remains unavailable. The Project CAS implementation
in this package is explicitly a deterministic test fake.
