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
const page = readFileSync(
  new URL("../components/mcp/McpToolsPage.tsx", import.meta.url),
  "utf8",
);
const marketplace = readFileSync(
  new URL("../components/mcp/McpMarketplace.tsx", import.meta.url),
  "utf8",
);
const serverIcon = readFileSync(
  new URL("../components/mcp/McpServerIcon.tsx", import.meta.url),
  "utf8",
);

describe("MCP Tools composer control", () => {
  it("keeps the composer trigger icon-only while retaining its status tooltip", () => {
    expect(control).toContain("<Tooltip content={statusLabel}");
    expect(control).toContain('<Wrench size={16} aria-hidden="true" />');
    expect(control).not.toContain("enabledServers.slice(0, 2)");
    expect(control).not.toContain(
      'className="hidden max-w-28 truncate rounded-full',
    );
  });

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
    expect(control).toContain(
      "`${window.location.pathname}${window.location.search}${window.location.hash}`",
    );
    expect(control).not.toContain("returnUrl: window.location.href");
  });

  it("supports a first-class management page without browser-owned authority", () => {
    expect(page).toContain('variant="embedded"');
    expect(control).toContain('variant?: "composer" | "page" | "embedded"');
    expect(control).toContain("client.mcp.listServers");
    expect(control).toContain('aria-label={t("serverList")}');
    expect(page).toContain("conversationId={conversationId}");
    expect(control).toContain("<McpServerIcon icon={server.icon} />");
    expect(marketplace).toContain("<McpServerIcon icon={item.icon} />");
    expect(serverIcon).toContain('referrerPolicy="no-referrer"');
    expect(serverIcon).toContain("shortText || <Box");
  });

  it("keeps Marketplace discovery server-authoritative and never executes install commands", () => {
    expect(page).toContain('"installed" | "marketplace"');
    expect(page).toContain("<McpMarketplace");
    expect(marketplace).toContain("client.mcp.searchMarketplace");
    expect(marketplace).toContain("client.mcp.getMarketplaceItem");
    expect(marketplace).toContain("client.mcp.installMarketplaceItem");
    expect(marketplace).toContain("MARKETPLACE_CATEGORIES");
    expect(marketplace).toContain("category: category || undefined");
    expect(marketplace).toContain(
      "selectionRevision: selection?.revision ?? 0",
    );
    expect(marketplace).toContain(
      "const searchRequest = ++searchRequestRef.current",
    );
    expect(marketplace).toContain("searchRequestRef.current === searchRequest");
    expect(marketplace).not.toContain("fetch(");
    expect(marketplace).not.toContain("npx ");
    expect(marketplace).not.toContain("docker run");
  });

  it("reloads authoritative drafts after a create or validation failure", () => {
    expect(control).toContain("const message = formatError(createError");
    expect(control).toContain("await load();\n      setError(message);");
  });
});
