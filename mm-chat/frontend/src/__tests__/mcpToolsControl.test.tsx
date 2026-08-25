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
const chatApp = readFileSync(
  new URL("../components/app/ChatApp.tsx", import.meta.url),
  "utf8",
);
const processTracePanel = readFileSync(
  new URL("../components/content/ProcessTracePanel.tsx", import.meta.url),
  "utf8",
);

describe("MCP Tools management control", () => {
  it("has no composer trigger or pre-send recovery surface", () => {
    expect(control).not.toContain("<Tooltip content={statusLabel}");
    expect(control).not.toContain('variant?: "composer"');
    expect(control).not.toContain("attentionMessage");
    expect(control).not.toContain("disableAllAndContinue");
    expect(control).not.toContain("enabledServers.slice(0, 2)");
    expect(control).not.toContain(
      'className="hidden max-w-28 truncate rounded-full',
    );
  });

  it("keeps server tools folded until requested and hides unknown labels", () => {
    expect(control).toContain("const [expandedServerKeys");
    expect(control).toContain("new Set(),");
    expect(control).toContain("aria-expanded={toolsExpanded}");
    expect(control).toContain("aria-controls={toolListId}");
    expect(control).toContain('t("toolCount", { count: server.tools.length })');
    expect(control).toContain("toolsExpanded &&");
    expect(control).toContain('tool.classification !== "unknown"');
    expect(control).toContain("onClick={() => toggleTool(server, tool.name)}");
  });

  it("keeps MCP out of the composer", () => {
    expect(control).toContain('saveSelection("custom", [])');
    expect(input).not.toContain("McpToolsControl");
    expect(input).not.toContain("mcpAdmissionAttention");
  });

  it("allows OAuth dynamic registration and validates HTTPS redirects", () => {
    expect(control).toContain(
      'draft.authType === "oauth" && draft.clientId.trim()',
    );
    expect(control).toContain('parsed.protocol !== "https:"');
    expect(control).toContain("validHttpsURL(result.authorizationUrl)");
    expect(control).toContain(
      "`${window.location.pathname}${window.location.search}${window.location.hash}`",
    );
    expect(control).not.toContain("returnUrl: window.location.href");
  });

  it("collects custom URL and Header credential separately before validation", () => {
    expect(control).toContain("headerValue: string");
    expect(control).toContain("customRemoteUrlNotice");
    expect(control).toContain("customRemoteSecretNotice");
    expect(control).toContain("client.mcp.setCredential");
    expect(control).toContain("value: draft.headerValue");
    expect(control).toContain("headerPrefix:");
    expect(control).toContain(
      "await client.mcp.validatePrivateServer(server.ref.id)",
    );
  });

  it("supports a first-class management page without browser-owned authority", () => {
    expect(page).toContain('variant="embedded"');
    expect(control).toContain('variant?: "page" | "embedded"');
    expect(control).toContain("client.mcp.listServers");
    expect(control).toContain('aria-label={t("serverList")}');
    expect(page).toContain("conversationId={conversationId}");
    expect(control).toContain("<McpServerIcon icon={server.icon} />");
    expect(marketplace).toContain("<McpServerIcon icon={item.icon} />");
    expect(serverIcon).toContain('referrerPolicy="no-referrer"');
    expect(serverIcon).toContain(
      'className="absolute inset-0 flex items-center justify-center"',
    );
    expect(serverIcon).toContain('className="relative z-10 h-full w-full');
    expect(serverIcon).toContain("shortText || <Box");
  });

  it("opens an agent-created MCP draft in the installed configuration surface", () => {
    expect(processTracePanel).toContain("resourceId: configuration.resourceId");
    expect(chatApp).toContain('setResourcePanelQuery("")');
    expect(chatApp).toContain("setResourcePanelMcpRef(");
    expect(chatApp).toContain("initialServerRef={resourcePanelMcpRef}");
    expect(page).toContain("initialServerRef={initialServerRef}");
    expect(control).toContain("initialServerRef?: string");
    expect(control).toContain("configurationTargetHandled");
    expect(control).toContain("mcpServerElementID(server.ref)");
    expect(control).toContain("void authorize(server)");
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
    expect(marketplace).toContain("page: nextPage");
    expect(marketplace).toContain("appendUniqueMarketplaceItems");
    expect(marketplace).toContain("initialLoadingRef.current");
    expect(marketplace).toContain("loadingMoreRef.current");
    expect(marketplace).toContain(
      "activeRequestControllerRef.current?.abort()",
    );
    expect(marketplace).toContain("new IntersectionObserver");
    expect(marketplace).toContain(
      'typeof IntersectionObserver === "undefined"',
    );
    expect(marketplace).toContain('rootMargin: "0px 0px 300px 0px"');
    expect(marketplace).toContain('contentVisibility: "auto"');
    expect(marketplace).toContain('t("marketplaceRetry")');
    expect(marketplace).toContain("const [searchError, setSearchError]");
    expect(marketplace).toContain("const [detailError, setDetailError]");
    expect(marketplace).toContain("const [installError, setInstallError]");
    expect(marketplace).toContain("detailRequestRef.current === detailRequest");
    expect(marketplace).toContain(
      't("marketplaceDetailFailed", { name: item.name })',
    );
    expect(marketplace).toContain(
      "onClick={() => void openDetail(detailError.item)}",
    );
    expect(marketplace).toContain("{searchError ? (");
    const detailFlow = marketplace.slice(
      marketplace.indexOf("const openDetail = useCallback"),
      marketplace.indexOf("const install = useCallback"),
    );
    expect(detailFlow).toContain("setDetailError({");
    expect(detailFlow).not.toContain("setSearchError(");
    const installFlowStart = marketplace.indexOf("const install = useCallback");
    const installFlow = marketplace.slice(
      installFlowStart,
      marketplace.indexOf("if (!enabled)", installFlowStart),
    );
    expect(installFlow).toContain("setInstallError(");
    expect(installFlow).not.toContain("setSearchError(");
    expect(marketplace).not.toContain("setError(");
    expect(marketplace).not.toContain("fetch(");
    expect(marketplace).not.toContain("npx ");
    expect(marketplace).not.toContain("docker run");
    expect(marketplace).toContain("canInstallMarketplaceDeployment");
    expect(marketplace).toContain('deployment.installMode === "header"');
    expect(marketplace).toContain('deployment.installMode === "runner_env"');
    expect(marketplace).toContain("detail.canInstall &&");
    expect(marketplace).toContain("deployment.secretFields.length > 0");
    expect(marketplace).toContain('autoComplete="new-password"');
    expect(marketplace).toContain('t("marketplaceCompleteConfiguration")');
    expect(marketplace).toContain('t("marketplaceUseCustomEndpoint")');
    expect(marketplace).toContain("const supportsCustomRemote =");
    expect(marketplace).toContain('deployment.connectionType === "http"');
    expect(marketplace).toContain("deployment.secretFields.length > 0");
    expect(marketplace).toContain(
      "{detail.canInstall && supportsCustomRemote ? (",
    );
    expect(marketplace).toContain("marketplaceVisibleDeployments");
    expect(marketplace).toContain("{detail.canInstall ? (");
    expect(marketplace).toContain("disabled={detail.installed ||");
    expect(control).toContain("needsAuth && server.canManage");
    expect(control).toContain("{canManage ? (");
    expect(marketplace).toContain('t("marketplaceAlreadyInstalled")');
    expect(marketplace).toContain("customEndpointUrl:");
    expect(marketplace).toContain("customCredential:");
    expect(marketplace).toContain(
      'customRemote.endpointUrl.trim().startsWith("https://")',
    );
  });

  it("reloads authoritative drafts after a create or validation failure", () => {
    expect(control).toContain("const message = formatError(createError");
    expect(control).toContain("await load();\n      setError(message);");
  });
});
