import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

describe("ChatApp server mode composition", () => {
  it("routes server chat UI through server read/send state without local tool writes", () => {
    const chatApp = readFileSync(
      resolve(process.cwd(), "src/components/app/ChatApp.tsx"),
      "utf8",
    );
    const generationController = readFileSync(
      resolve(
        process.cwd(),
        "src/features/chat/hooks/useChatGenerationController.ts",
      ),
      "utf8",
    );
    const generationReconciliation = readFileSync(
      resolve(
        process.cwd(),
        "src/features/chat/hooks/useServerGenerationReconciliation.ts",
      ),
      "utf8",
    );

    expect(chatApp).toContain("serverReadState.sessions");
    expect(chatApp).toContain("serverReadState.activeMessages");
    expect(chatApp).toContain("sendServerMessageAndStream");
    expect(chatApp).toMatch(
      /getActiveMessagePath\(\s*latestServerState\.activeMessageTree,?\s*\)\.at\(-1\)\s*\?\.id/,
    );
    expect(chatApp).toContain("parentMessageId,");
    expect(chatApp).toContain("activeImageGeneration");
    expect(chatApp).toContain("findRecentImageGenerationModel");
    expect(chatApp).toContain(
      "recentImageGenerationModel: findRecentImageGenerationModel(",
    );
    expect(chatApp).toContain("ImageGenerationProgress");
    expect(chatApp).toContain("startedAt: Date.now()");
    expect(chatApp).toContain("uploadMessageAttachmentsForServer");
    expect(chatApp).toContain("onUserMessageAccepted: () =>");
    expect(chatApp).toContain("const acceptance = new Promise<boolean>");
    expect(chatApp).toContain("settleAcceptance(true);");
    expect(chatApp).toContain("settleAcceptance(messageAccepted);");
    expect(chatApp).toContain("return acceptance;");
    expect(chatApp).not.toContain("return messageAccepted;");
    expect(chatApp).toContain("if (!messageAccepted) {");
    expect(chatApp).not.toContain(
      "Server mode requires message text with attachments.",
    );
    expect(chatApp).toContain("persistConversationKnowledgeSelection");
    expect(chatApp).toContain("updateServerSessionConfig");
    expect(chatApp).toContain("updateServerSessionModel");
    expect(chatApp).toContain(
      "currentSession?.model || selectedChatModel || selectedModel",
    );
    expect(chatApp).toContain("selectedKnowledgeCollectionIds");
    expect(chatApp).not.toContain("buildServerKnowledgeStreamConfig");
    expect(chatApp).not.toContain("buildServerKnowledgeMessageMetadata");
    expect(chatApp).toContain("chatConfig: composerChatConfig");
    expect(chatApp).not.toContain("orchestrateServerPlugins");
    expect(chatApp).not.toContain("installedPlugins,");
    expect(chatApp).not.toContain("activePlugins,");
    expect(chatApp).toContain("if (serverModeEnabled) return;");
    expect(chatApp).toContain("abortActiveGeneration");
    expect(chatApp).toContain(
      "if (isGenerating && !serverModeEnabled && currentSessionId)",
    );
    expect(chatApp).not.toMatch(
      /if \(serverModeEnabled\) \{\s+if \(isGenerating\) \{\s+abortActiveGeneration\(\);/,
    );
    expect(chatApp).toContain("cancelServerGeneration(sessionId)");
    expect(chatApp).toContain(
      "const cancellation = cancelServerGeneration(sessionId);",
    );
    expect(chatApp.indexOf("cancelServerGeneration(sessionId)")).toBeLessThan(
      chatApp.indexOf("abortActiveGeneration(sessionId)"),
    );
    expect(chatApp).toContain("runningSessionIds={runningSessionIds}");
    expect(chatApp).toContain("unreadSessionIds={unreadSessionIds}");
    expect(chatApp).toContain("useServerGenerationReconciliation({");
    expect(chatApp).not.toContain(
      "shouldAbortActiveGenerationForSessionDelete({",
    );
    expect(chatApp).toContain("localSessionToolsDisabled={serverModeEnabled}");
    expect(chatApp).toContain(
      "allowReasoningWhenSessionToolsDisabled={serverModeEnabled}",
    );
    expect(chatApp).not.toContain("allowSkillsWhenSessionToolsDisabled");
    expect(chatApp).toContain("toolMode={composerChatConfig.toolMode}");
    expect(chatApp).toContain("effectiveToolMode={effectiveToolMode}");
    expect(chatApp).toContain("canSelectAgentMode={");
    expect(chatApp).toContain("serverConfig?.mcp.enabled === true");
    expect(chatApp).not.toContain("preflightMcp({");
    expect(chatApp).not.toContain("mcpConversationId={");
    expect(chatApp).toContain("persistToolMode");
    expect(chatApp).toContain("persistAgentPermissionMode");
    expect(chatApp).toContain("updateServerSessionPermission");
    expect(chatApp).toContain(
      "availablePermissionModes={availablePermissionModes}",
    );
    expect(chatApp).toContain("permissionLocked={isGenerating}");
    expect(chatApp).toContain("resolveEffectiveChatToolMode");
    expect(chatApp).toContain('import("@/components/mcp/McpToolsPage")');
    expect(chatApp).toContain('onOpenTools={() => navigateToPanel("tools")}');
    expect(chatApp).toContain('isToolsOpen={viewMode === "tools"}');
    expect(chatApp).toContain('viewMode === "tools"');
    expect(chatApp).toContain(
      "conversationId={visibleCurrentSessionId ?? undefined}",
    );
    expect(chatApp).not.toContain("activeSkillIdsOverride");
    expect(chatApp).not.toContain("onActiveSkillIdsChange");
    expect(chatApp).not.toContain("resolveSkillsForMessage");
    expect(chatApp).not.toContain("skillResolution.context");
    expect(chatApp).toContain(
      "onLocalSessionToolUnavailable={showServerUnsupportedAction}",
    );
    expect(chatApp).toContain("searchMode={composerChatConfig.searchMode}");
    expect(chatApp).toContain("onSearchModeChange={(searchMode) =>");
    expect(chatApp).toContain("persistServerSearchMode(searchMode)");
    expect(chatApp).toContain("searchMode: currentSearchMode");
    expect(chatApp).toContain(
      "const processingSearchMode = normalizeSearchMode(",
    );
    expect(chatApp).toContain("config: processingChatConfig");
    expect(chatApp).toContain(
      "isReasoningEnabled={composerChatConfig.useReasoning}",
    );
    expect(chatApp).toContain(
      "reasoningEffort={composerChatConfig.reasoningEffort}",
    );
    expect(chatApp).toContain("onReasoningChange={(enabled, effort) =>");
    expect(chatApp).toContain(
      "currentSessionConfig?.useReasoning ?? chatConfig.useReasoning",
    );
    expect(chatApp).toContain(
      "currentSessionConfig?.reasoningEffort ?? chatConfig.reasoningEffort",
    );
    expect(chatApp).toContain("persistServerReasoningSelection");
    expect(chatApp).toContain("config: { useReasoning, reasoningEffort }");
    expect(chatApp).not.toContain(
      'showServerUnsupportedAction("chat deletion")',
    );
    expect(chatApp).not.toContain(
      'showServerUnsupportedAction("chat renaming")',
    );
    expect(chatApp).not.toContain('showServerUnsupportedAction("pinning")');
    expect(chatApp).not.toContain(
      'showServerUnsupportedAction("system instruction editing")',
    );
    expect(chatApp).not.toContain(
      'showServerUnsupportedAction("message deletion")',
    );
    expect(chatApp).not.toContain(
      'showServerUnsupportedAction("message retraction")',
    );
    expect(chatApp).not.toContain(
      'showServerUnsupportedAction("regeneration")',
    );
    expect(chatApp).not.toContain(
      'showServerUnsupportedAction("message version switching")',
    );
    expect(chatApp).not.toContain(
      'showServerUnsupportedAction("message editing")',
    );
    expect(chatApp).not.toContain(
      'showServerUnsupportedAction("message edit branches")',
    );
    expect(chatApp).not.toContain(
      'showServerUnsupportedAction("reasoning toggle")',
    );
    expect(chatApp).not.toContain(
      'showServerUnsupportedAction("assistant presets")',
    );
    expect(chatApp).not.toContain(
      'showServerUnsupportedAction("chat duplication")',
    );
    expect(chatApp).not.toContain(
      'showServerUnsupportedAction("smart rename")',
    );
    expect(chatApp).toContain("updateServerMessageContent");
    expect(chatApp).toContain("treeParentMessageId: sourceParentId");
    expect(chatApp).toContain("regenerateServerAssistantMessage");
    expect(chatApp).toContain("switchServerMessageVersion");
    expect(chatApp).toContain("duplicateServerSession");
    expect(chatApp).toContain("updateServerSessionInstruction");
    expect(chatApp).toContain("generateServerConversationTitle");
    expect(chatApp).toContain("await moveSessionToWorkspace(sessionId");
    expect(chatApp).toContain("await deleteServerSession(sessionId)");
    expect(chatApp).toContain("await selectServerSession(previousSessionId)");
    expect(chatApp).toContain("Failed to roll back ungrouped server chat");
    expect(chatApp).not.toContain("activeSkillIds");
    expect(chatApp).not.toContain("activePluginIdsOverride");

    expect(generationController).toContain("abortActiveGeneration");
    expect(generationController).toContain(
      "new Map<string, ActiveGenerationRun>()",
    );
    expect(generationController).toContain("await state.syncActiveSession");
    expect(generationReconciliation).toContain("let reconciling = false");
    expect(generationReconciliation).toContain(
      'document.visibilityState !== "visible"',
    );
    expect(generationReconciliation).toContain(
      'window.addEventListener("online", reconcileNow)',
    );
  });
});
