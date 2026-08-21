import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

import { createServerChatApiShell } from "../services/api/client/server/chatApi";
import type {
  HttpClient,
  SseRequestOptions,
} from "../services/api/client/server/httpClient";

describe("safe answer continuation", () => {
  it("sends the durable source message id through the existing stream contract", async () => {
    const requests: Array<{ path: string; body: unknown }> = [];
    const chat = createServerChatApiShell(
      createHttpClient(async (path, options) => {
        requests.push({ path, body: options.body });
        options.onFrame({
          event: "message.started",
          data: {
            type: "message.started",
            runId: "run-continue",
            messageId: "answer-continued",
            sequence: 1,
          },
        });
        options.onFrame({
          event: "message.completed",
          data: {
            type: "message.completed",
            runId: "run-continue",
            messageId: "answer-continued",
            sequence: 2,
          },
        });
      }),
    );

    await expect(
      chat.streamAssistantMessage({
        conversationId: "conversation-1",
        userMessageId: "question-1",
        continuationOfMessageId: "answer-interrupted",
        modelRef: { providerId: "openai", modelId: "gpt-5.5" },
        idempotencyKey: "continue-1",
      }),
    ).resolves.toMatchObject({ status: "completed" });
    expect(requests).toEqual([
      {
        path: "/v1/chat/conversations/conversation-1/stream",
        body: expect.objectContaining({
          userMessageId: "question-1",
          continuationOfMessageId: "answer-interrupted",
          idempotencyKey: "continue-1",
        }),
      },
    ]);
  });

  it("shows Continue only for an exact interrupted partial answer and keeps Regenerate", () => {
    const messageItem = readFileSync(
      resolve(process.cwd(), "src/components/chat/MessageItem.tsx"),
      "utf8",
    );
    const chatApp = readFileSync(
      resolve(process.cwd(), "src/components/app/ChatApp.tsx"),
      "utf8",
    );

    expect(messageItem).toContain(
      "generationError?.code === PROVIDER_STREAM_INTERRUPTED_CODE",
    );
    expect(messageItem).toContain("message.content.trim().length > 0");
    expect(messageItem).toContain('t("continueAnswer")');
    expect(messageItem).toContain('tooltip={t("regenerate")}');
    expect(chatApp).toContain("continuationOfMessageId:");
    expect(chatApp).toContain('mode === "continue" ? messageId : undefined');

    for (const locale of ["zh", "en", "ja"]) {
      const messages = JSON.parse(
        readFileSync(
          resolve(process.cwd(), `src/i18n/locales/${locale}/Message.json`),
          "utf8",
        ),
      ) as Record<string, string>;
      const chatMessages = JSON.parse(
        readFileSync(
          resolve(process.cwd(), `src/i18n/locales/${locale}/ChatApp.json`),
          "utf8",
        ),
      ) as Record<string, string>;
      expect(messages.continueAnswer).toBeTruthy();
      expect(messages.continueAnswerHint).toBeTruthy();
      expect(chatMessages.errContinue).toBeTruthy();
    }
  });
});

function createHttpClient(
  requestSse: (path: string, options: SseRequestOptions) => Promise<void>,
): HttpClient {
  return {
    buildUrl: (path) => path,
    async requestJson() {
      throw new Error("requestJson not mocked");
    },
    async requestMultipartJson() {
      throw new Error("requestMultipartJson not mocked");
    },
    async requestBinary() {
      throw new Error("requestBinary not mocked");
    },
    requestSse,
  };
}
