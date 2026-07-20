import { describe, expect, it } from "vitest";

import {
  hasCurrentPublicProgressIntent,
  inferPendingChatProgressStage,
} from "@/lib/chat/generationProgress";

describe("chat generation progress", () => {
  it.each([
    "西安天气预报",
    "今天 Kimi 有什么新闻？",
    "latest OpenAI news",
    "look up the current exchange rate",
    "今日の天気を検索",
  ])("recognizes current public intent in %s", (question) => {
    expect(hasCurrentPublicProgressIntent(question)).toBe(true);
  });

  it("prioritizes web progress for current public questions", () => {
    expect(
      inferPendingChatProgressStage({
        question: "西安天气预报",
        searchEnabled: true,
        knowledgeCollectionIds: ["collection-1"],
      }),
    ).toBe("search");
  });

  it("shows knowledge progress for a stable private question", () => {
    expect(
      inferPendingChatProgressStage({
        question: "公司的报销规则是什么？",
        searchEnabled: true,
        knowledgeCollectionIds: ["collection-1"],
      }),
    ).toBe("knowledge");
  });

  it("falls back to web or model progress from enabled sources", () => {
    expect(
      inferPendingChatProgressStage({
        question: "解释量子纠缠",
        searchEnabled: true,
      }),
    ).toBe("search");
    expect(
      inferPendingChatProgressStage({
        question: "解释量子纠缠",
        searchEnabled: false,
      }),
    ).toBe("model");
  });
});
