import { beforeEach, describe, expect, it, vi } from "vitest";
import { API_INPUT_LIMITS } from "../config/limits";
import { processMessageForSending } from "../lib/chat/messageProcessor";
import { CHAT_INPUT_TRUNCATION_NOTICE } from "../lib/utils/chatInput";
import { createKnowledgeCollectionAttachment } from "../lib/utils/knowledgeAttachments";

const mocks = vi.hoisted(() => ({
  resolveOPFSUrl: vi.fn(),
}));

vi.mock("../utils/opfs", () => ({
  resolveOPFSUrl: mocks.resolveOPFSUrl,
}));

const encodeText = (value: string) =>
  btoa(String.fromCharCode(...new TextEncoder().encode(value)));

describe("message preprocessing", () => {
  beforeEach(() => {
    mocks.resolveOPFSUrl.mockReset();
  });

  it("keeps final model text within the chat request limit", async () => {
    const result = await processMessageForSending({
      text: "u".repeat(API_INPUT_LIMITS.maxChatTextChars - 200),
      attachments: [
        {
          id: "att_1",
          mimeType: "text/plain",
          fileName: "large.txt",
          data: encodeText("c".repeat(10_000)),
        },
      ],
      selectedModel: "provider:model",
      modelMetadata: { model: { attachment: false } },
      customModelMetadata: {},
    });

    expect(result.finalText.length).toBeLessThanOrEqual(
      API_INPUT_LIMITS.maxChatTextChars,
    );
    expect(result.finalText.endsWith(CHAT_INPUT_TRUNCATION_NOTICE)).toBe(true);
  });

  it("converts text attachments into prompt context", async () => {
    const result = await processMessageForSending({
      text: "Read this",
      attachments: [
        {
          id: "att_text",
          mimeType: "text/markdown",
          fileName: "brief.md",
          data: encodeText("Project notes"),
        },
      ],
      selectedModel: "provider:model",
      modelMetadata: { model: { attachment: true } },
      customModelMetadata: {},
    });

    expect(result.finalAttachments).toEqual([]);
    expect(result.finalText).toContain('name="brief.md"');
    expect(result.finalText).toContain("Project notes");
    expect(result.userMessage.attachments).toHaveLength(1);
  });

  it("does not query or inject browser-local Knowledge attachments", async () => {
    const knowledgeAttachment = createKnowledgeCollectionAttachment({
      collectionId: "collection_1",
      collectionName: "Legacy KB",
    });
    const result = await processMessageForSending({
      text: "Use the selected server collection",
      attachments: [knowledgeAttachment],
      selectedModel: "provider:model",
      modelMetadata: { model: { attachment: false } },
      customModelMetadata: {},
    });

    expect(result.finalText).toBe("Use the selected server collection");
    expect(result.finalAttachments).toEqual([]);
    expect(result.ragSources).toEqual([]);
    expect(result.userMessage.attachments).toEqual([knowledgeAttachment]);
  });
});
