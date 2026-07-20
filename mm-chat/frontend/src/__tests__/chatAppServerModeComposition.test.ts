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

    expect(chatApp).toContain("serverReadState.sessions");
    expect(chatApp).toContain("serverReadState.activeMessages");
    expect(chatApp).toContain("sendServerMessageAndStream");
    expect(chatApp).toContain(
      "getActiveMessagePath(latestServerState.activeMessageTree).at(-1)?.id",
    );
    expect(chatApp).toContain("parentMessageId,");
    expect(chatApp).toContain("activeImageGeneration");
    expect(chatApp).toContain("ImageGenerationProgress");
    expect(chatApp).toContain("startedAt: Date.now()");
    expect(chatApp).toContain("uploadMessageAttachmentsForServer");
    expect(chatApp).toContain("persistConversationKnowledgeSelection");
    expect(chatApp).toContain("updateServerSessionConfig");
    expect(chatApp).toContain("selectedKnowledgeCollectionIds");
    expect(chatApp).not.toContain("buildServerKnowledgeStreamConfig");
    expect(chatApp).not.toContain("buildServerKnowledgeMessageMetadata");
    expect(chatApp).toContain("chatConfig: composerChatConfig");
    expect(chatApp).toContain("installedPlugins,");
    expect(chatApp).toContain("activePlugins,");
    expect(chatApp).toContain("if (serverModeEnabled) return;");
    expect(chatApp).toContain("abortActiveGeneration");
    expect(chatApp).toContain("localSessionToolsDisabled={serverModeEnabled}");
    expect(chatApp).toContain(
      "allowReasoningWhenSessionToolsDisabled={serverModeEnabled}",
    );
    expect(chatApp).toContain(
      "allowSkillsWhenSessionToolsDisabled={serverModeEnabled}",
    );
    expect(chatApp).toContain(
      "allowPluginsWhenSessionToolsDisabled={serverModeEnabled}",
    );
    expect(chatApp).toContain("activeSkillIdsOverride={");
    expect(chatApp).toContain("onActiveSkillIdsChange={");
    expect(chatApp).toContain(
      "const skillResolution = await resolveSkillsForMessage",
    );
    expect(chatApp).toContain("autoSelect: false");
    expect(chatApp).toContain("skillResolution.context");
    expect(chatApp).toContain(
      "const pluginResolution = await orchestrateServerPlugins",
    );
    expect(chatApp).toContain("pluginResolution.context");
    expect(chatApp).toContain(
      "onLocalSessionToolUnavailable={showServerUnsupportedAction}",
    );
    expect(chatApp).toContain("isSearchEnabled={composerChatConfig.useSearch}");
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
    expect(chatApp).not.toContain("installedPlugins={serverModeEnabled");
    expect(chatApp).toContain(
      "activeSkillIds: serverModeEnabled ? activeSkillIds : []",
    );
    expect(chatApp).toContain(
      "activePluginIdsOverride: serverModeEnabled ? activePlugins : undefined",
    );
    expect(chatApp).toContain(
      "activeSkillIdsOverride: serverModeEnabled ? activeSkillIds : undefined",
    );
    expect(chatApp).not.toContain("activePlugins: serverModeEnabled ? []");

    expect(generationController).toContain("abortActiveGeneration");
    expect(generationController).toContain("await state.syncActiveSession");
  });
});
