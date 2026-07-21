import { describe, expect, it } from "vitest";

import {
  CHAT_MESSAGE_RENDER_BATCH_SIZE,
  getInitialChatMessageRenderStart,
  getNextChatMessageRenderStart,
  INITIAL_CHAT_MESSAGE_RENDER_COUNT,
} from "@/lib/chat/progressiveMessageRendering";

describe("progressive chat message rendering", () => {
  it("renders short conversations in one pass", () => {
    expect(
      getInitialChatMessageRenderStart(INITIAL_CHAT_MESSAGE_RENDER_COUNT),
    ).toBe(0);
  });

  it("starts from the recent tail and reveals bounded older batches", () => {
    const start = getInitialChatMessageRenderStart(51);
    expect(start).toBe(51 - INITIAL_CHAT_MESSAGE_RENDER_COUNT);
    expect(getNextChatMessageRenderStart(start)).toBe(
      start - CHAT_MESSAGE_RENDER_BATCH_SIZE,
    );
    expect(getNextChatMessageRenderStart(3)).toBe(0);
  });
});
