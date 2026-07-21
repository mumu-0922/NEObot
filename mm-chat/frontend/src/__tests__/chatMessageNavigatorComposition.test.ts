import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

describe("ChatMessageNavigator composition", () => {
  it("wires message IDs, progressive reveal, and edge controls into ChatApp", () => {
    const chatApp = readFileSync(
      resolve(process.cwd(), "src/components/app/ChatApp.tsx"),
      "utf8",
    );
    const messageItem = readFileSync(
      resolve(process.cwd(), "src/components/chat/MessageItem.tsx"),
      "utf8",
    );
    const navigator = readFileSync(
      resolve(process.cwd(), "src/components/chat/ChatMessageNavigator.tsx"),
      "utf8",
    );

    expect(chatApp).toContain("<ChatMessageNavigator");
    expect(chatApp).toContain("pendingChatNavigationRef");
    expect(chatApp).toContain("handleNavigateToUserMessage");
    expect(chatApp).toContain("handleJumpToChatTop");
    expect(chatApp).toContain("handleJumpToChatBottom");
    expect(chatApp).toContain('scrollTo({ top: 0, behavior: "auto" })');
    expect(chatApp).toContain(
      'scrollTo({ top: container.scrollHeight, behavior: "auto" })',
    );
    expect(chatApp).toContain("getChatNavigationRevealStart");
    expect(messageItem).toContain("id={getChatMessageElementId(message.id)}");
    expect(navigator).toContain('aria-current={isActive ? "location"');
    expect(navigator).toContain("data-message-id={item.id}");
    expect(navigator).toContain("window.matchMedia(DESKTOP_NAVIGATION_QUERY)");
    expect(navigator).toContain('t("jumpToConversationTop")');
    expect(navigator).toContain('t("jumpToConversationBottom")');
    expect(navigator).toContain("ResizeObserver");
  });
});
