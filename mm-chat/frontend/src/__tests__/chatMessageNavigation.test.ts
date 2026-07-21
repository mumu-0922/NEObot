import { describe, expect, it } from "vitest";

import type { Message } from "@/types";
import {
  buildChatMessageNavigationItems,
  getChatMessageElementId,
  getChatNavigationRevealStart,
  getChatNavigationScrollTop,
  resolveActiveChatNavigationMessageId,
} from "@/lib/chat/messageNavigation";

const message = (input: Partial<Message> & Pick<Message, "id" | "role">) =>
  ({
    content: "",
    timestamp: 1,
    ...input,
  }) as Message;

describe("chat message navigation", () => {
  it("builds bounded labels for active-path user messages only", () => {
    const items = buildChatMessageNavigationItems(
      [
        message({ id: "u1", role: "user", content: "  First\n question  " }),
        message({ id: "a1", role: "model", content: "Answer" }),
        message({
          id: "u2",
          role: "user",
          attachments: [
            { id: "f1", fileName: "brief.pdf", mimeType: "application/pdf" },
          ],
        }),
        message({ id: "u3", role: "user", content: "123456789" }),
      ],
      {
        emptyLabel: "User message",
        attachmentPrefix: "Attachment: ",
        maxLabelChars: 8,
      },
    );

    expect(items).toEqual([
      { id: "u1", label: "First q…", messageIndex: 0 },
      { id: "u2", label: "Attachm…", messageIndex: 2 },
      { id: "u3", label: "1234567…", messageIndex: 3 },
    ]);
    expect(getChatMessageElementId("u1")).toBe("chat-message-u1");
  });

  it("selects the user message nearest the reading line", () => {
    expect(
      resolveActiveChatNavigationMessageId(
        [
          { id: "u1", top: 100, height: 40 },
          { id: "u2", top: 320, height: 80 },
        ],
        300,
      ),
    ).toBe("u2");
    expect(resolveActiveChatNavigationMessageId([], 300)).toBeNull();
  });

  it("calculates and clamps message scroll targets", () => {
    expect(getChatNavigationRevealStart(43, 12)).toBe(12);
    expect(getChatNavigationRevealStart(43, 48)).toBe(43);
    expect(
      getChatNavigationScrollTop({
        scrollTop: 1_000,
        scrollHeight: 5_000,
        clientHeight: 800,
        containerTop: 50,
        messageTop: 450,
        messageHeight: 80,
      }),
    ).toBe(1_120);
    expect(
      getChatNavigationScrollTop({
        scrollTop: 0,
        scrollHeight: 500,
        clientHeight: 800,
        containerTop: 50,
        messageTop: 50,
        messageHeight: 40,
      }),
    ).toBe(0);
  });
});
