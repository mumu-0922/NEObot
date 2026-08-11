import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const control = readFileSync(
  new URL("../components/mcp/McpToolsControl.tsx", import.meta.url),
  "utf8",
);
const input = readFileSync(
  new URL("../components/chat/MessageInput.tsx", import.meta.url),
  "utf8",
);

describe("MCP Tools composer control", () => {
  it("opens on preflight attention and offers disable-all-and-continue", () => {
    expect(control).toContain("setOpen(true)");
    expect(control).toContain('saveSelection("custom", [])');
    expect(control).toContain("disableAllAndContinue");
    expect(input).toContain("attention={mcpAdmissionAttention}");
    expect(input).toContain("void handleSend()");
  });

  it("requires OAuth client identity and validates HTTPS redirects", () => {
    expect(control).toContain(
      'draft.authType === "oauth" && !draft.clientId.trim()',
    );
    expect(control).toContain('parsed.protocol !== "https:"');
    expect(control).toContain("validHttpsURL(result.authorizationUrl)");
  });
});
