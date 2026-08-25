# Content Components

Content components render model output and tool output in reusable formats.

## Files

- `Artifact.tsx` renders editable generated artifacts and preview controls.
- `MarkdownRenderer.tsx` renders Markdown, safe inline HTML visual blocks, code highlighting, math, media references, Mermaid diagrams, mind maps, and rich inline content.
- `ReasoningBlock.tsx` renders model reasoning summaries or reasoning traces when available.
- `SourceBlock.tsx` renders web-search sources, image results, citation context, and visible search failure states.
- `SourceFusionNotice.tsx` renders separate compact notices for allowlisted
  partial Web retrieval and total Web degradation, and stays hidden for
  disabled, skipped, or empty-result lanes.
- `ToolCallBlock.tsx` renders tool-call arguments, execution status, and results.
- `WorkspaceFileCard.tsx` opens bound Agent outputs from their current project
  path, previews text/DOCX/XLSX and authenticated media, warns on version drift,
  and keeps download as a secondary action.

## Guidelines

- Keep formatting helpers in `src/lib/utils`.
- Keep rendering resilient to missing or partially streamed data.
- Treat tool results as untrusted display data and preserve safe formatting.
- Treat inline HTML, generated SVG, tool output, and artifact preview data as untrusted display data.
- Prefer shared primitives for copy, tooltip, and preview interactions.
- Never construct a Host path or naked file URL in the browser. Workspace file
  blocks must pass strict UUID/path/version/size validation before rendering,
  and bytes must come through the authenticated Workspace API.
