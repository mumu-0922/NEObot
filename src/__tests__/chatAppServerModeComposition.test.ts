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
    expect(chatApp).toContain("uploadMessageAttachmentsForServer");
    expect(chatApp).toContain("config: serverSessionChatConfig");
    expect(chatApp).toContain("chatConfig: composerChatConfig");
    expect(chatApp).toContain("installedPlugins: serverModeEnabled ? []");
    expect(chatApp).toContain("activePlugins: serverModeEnabled ? []");
    expect(chatApp).toContain("if (serverModeEnabled) return;");
    expect(chatApp).toContain("abortActiveGeneration");
    expect(chatApp).toContain("localSessionToolsDisabled={serverModeEnabled}");
    expect(chatApp).toContain(
      "onLocalSessionToolUnavailable={showServerUnsupportedAction}",
    );
    expect(chatApp).toContain("isSearchEnabled={composerChatConfig.useSearch}");
    expect(chatApp).toContain(
      "isReasoningEnabled={composerChatConfig.useReasoning}",
    );
    expect(chatApp).not.toContain("installedPlugins={serverModeEnabled");
    expect(chatApp).not.toContain("activeSkillIds={serverModeEnabled");

    expect(generationController).toContain("abortActiveGeneration");
    expect(generationController).toContain("await state.syncActiveSession");
  });
});
