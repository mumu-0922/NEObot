# Pi resources and XLSX artifact forensics

## XLSX failure evidence

- The failing artifact is `/mnt/d/AI/myDOA/近7天国际金价_XAUUSD.xlsx`.
- ZIP integrity passes and cell data is present in `xl/worksheets/sheet1.xml`.
- The browser preview succeeds because
  `mm-chat/backend/internal/hostworkspace/file_preview.go` reads bounded
  worksheet XML and cell values only. It does not validate DrawingML, ChartML,
  relationships, formula caches, or native Microsoft Excel compatibility.
- The Agent generated the file through
  `/mnt/d/AI/myDOA/create_gold_xlsx.py`, which manually assembles eleven OOXML
  parts with Python `zipfile`. It did not use a spreadsheet library or a native
  office compatibility check.
- The hand-written drawing contains incomplete anchors and the chart is a
  minimal manually authored ChartML tree. Microsoft Excel therefore asks to
  repair workbook content. The exact repair record is unavailable until Excel
  accepts repair, so the precise rejected element remains `[unverified]`; the
  product defect is already proven at the generation/validation boundary.
- `unzip -t` proves only ZIP integrity. It is not a valid XLSX acceptance gate.

## What Pi actually does

Primary local source: `/home/mumu/projects/pi-web` at release `v0.8.9`.

- Pi Web constructs Agent sessions through one SDK service graph in
  `lib/rpc-manager.ts`; runtime resources are resolved before session creation.
- `DefaultResourceLoader` is used both by the Agent runtime and the Skills UI,
  preventing a separate UI-only interpretation of loaded Skills.
- `DefaultPackageManager` resolves package resources and exposes package status,
  scope and diagnostics. One package may contribute Extensions, Skills, Prompt
  Templates and Themes.
- Skills use progressive disclosure: metadata is always visible; full
  `SKILL.md` loads on demand. A Skill directory may include `scripts/`,
  `references/` and `assets/`.
- Pi uses `/skill:<name> [arguments]` as the deterministic user invocation.
- Extensions register reliable custom Tools, slash commands and lifecycle
  hooks, and can intercept Tool calls/results. This is the correct layer for
  deterministic file-format generation or validation behavior.
- Pi Web provides separate Skills and Plugins configuration views, but both
  read the same resolved runtime authority. Plugin UI groups contributed
  Extension/Skill/Prompt/Theme resources by global/project package.

Official documentation:

- <https://badlogic-pi-mono.mintlify.app/coding-agent/skills>
- <https://badlogic-pi-mono.mintlify.app/coding-agent/extensions>
- <https://badlogic-pi-mono.mintlify.app/coding-agent/pi-packages>
- <https://badlogic-pi-mono.mintlify.app/concepts/extensibility>

## Neo Chat current state

- The immutable Agent Runtime Resource Registry now unifies backend Run/Step
  assembly for built-ins, installed Skills and MCP Tools.
- The registry is internal only: there is no authenticated read-only Resources
  API, composer resource inspector, or Run snapshot diagnostic surface.
- Skill management lives under Sidebar `技能商店`; the current runtime cache
  contains one installed package, `workspace-smoke`.
- MCP management lives under Sidebar `工具`; manifest-available Servers include
  Browser (Playwright), Context7 and Tavily, while actual enablement remains
  user/conversation/workspace authority.
- The backend currently documents a Neo-specific `/skill-name` deterministic
  invocation rather than Pi/Agent-Skills-style `/skill:<name>`, and the composer
  has no discoverable slash-command picker.
- Workspace file publication can expose any created `.xlsx`, but no format-
  specific generator or validator proves native Excel compatibility before the
  file card is shown.

## Feasible approaches

### A. Delivery-first Pi seam (recommended)

1. Add a read-only Resources projection API/UI sourced from the existing
   immutable registry; show loaded Built-ins, Skills and MCP, scope, status,
   revision and redacted diagnostics.
2. Add composer slash discovery and standard `/skill:<name> [args]`, retaining
   legacy parsing only for compatibility.
3. Add a first-party Office/XLSX resource package: a Skill for workflow plus a
   deterministic server/Host-Runner Tool or pinned helper for generation and
   validation. File publication must fail closed when the validator fails.
4. Keep existing Skill Store, MCP grants/selections, Workspace trust and
   approval systems as the only mutation authorities.

This fixes the observed failure and creates the seam needed for future package
resources without executing arbitrary Pi extensions inside Neo Chat.

### B. Resource UX only

Expose installed resources and slash invocation, but leave Office files to
uncontrolled Bash scripts. This improves discoverability but does not prevent
corrupt XLSX delivery.

### C. Full Pi-compatible package/extension runtime

Implement npm/git/local package installation, arbitrary Extension code,
lifecycle hooks, Prompts and Themes now. This has the closest feature parity but
introduces a new code-execution and trust boundary and is too broad for the
current repair.

## Recommendation

Choose Approach A. Treat MCP as a resource contribution in the unified read
view while preserving its stricter server-owned installation, credentials and
selection authority. Do not copy Pi's full-system-permission extension model.
