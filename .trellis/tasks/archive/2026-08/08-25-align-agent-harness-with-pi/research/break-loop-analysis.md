## Bug Analysis: Agent harness drifted from the Pi execution model

### 1. Root Cause Category

- **Category**: B/C/E - Cross-layer contract, change propagation failure, and
  implicit assumptions.
- **Specific Cause**: Tool naming, completion policy, generated-file identity,
  and UI behavior evolved as independent mechanisms. The implementation assumed
  every mutation needed a server-owned verification ceremony and every usable
  file needed an object-store copy, rather than treating the model Tool loop and
  the bound workspace as the primary authorities.

### 2. Why Fixes Failed

1. Earlier timeout changes addressed elapsed time but retained the forced
   completion gate, so longer Runs still spent rounds proving completion.
2. Artifact publication fixed download visibility but copied files away from
   the project and did not make Open the normal action.
3. Isolated Backend fixes did not close the durable Message -> frontend parser
   -> viewer -> owner-scoped Host read chain, so reload and file opening could
   still diverge.

### 3. Prevention Mechanisms

| Priority | Mechanism | Specific Action | Status |
| --- | --- | --- | --- |
| P0 | Architecture | Canonical `read/write/edit/grep/bash` registry with hidden legacy aliases | DONE |
| P0 | Runtime | Natural no-Tool completion; keep loop detection, cancellation, and scoped timeouts | DONE |
| P0 | Contract | Strict `workspace_file` block and pinned owner-scoped content/preview routes | DONE |
| P0 | Test | Cross-layer output-block persistence, traversal, authority, XLSX, and UI tests | DONE |
| P1 | Documentation | Update Trellis specs, product contracts, module design, and thinking guide | DONE |

### 4. Systematic Expansion

- **Similar Issues**: MCP aliases, Browser Tool presentation, and future plugin
  Tools can drift when definition, dispatch, trace, and UI are reviewed alone.
- **Design Improvement**: Treat Tool registry plus durable output-block schema
  as the canonical bridge between execution and presentation.
- **Process Improvement**: Every Agent Tool change must trace advertised schema,
  legacy replay, execution, durability, reload, and negative authorization in
  one task before deployment.

### 5. Knowledge Capture

- [x] Updated backend Agent/Tool loop specs.
- [x] Updated frontend Host Workspace spec.
- [x] Updated product contracts and module README/DESIGN.
- [x] Added the Agent Tool, Completion, and Workspace Output checklist to the
      cross-layer thinking guide.
- [x] No local `src/templates/markdown/spec/` tree exists in this initialized
      project, so there is no generated template copy to synchronize.
