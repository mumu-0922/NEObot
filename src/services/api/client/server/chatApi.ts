import { ApiClientError, unsupportedFeature } from "../errors";
import type {
  AppendUserMessageInput,
  ApiPage,
  ChatApi,
  ChatMessageDTO,
  ChatRunResult,
  ChatStreamHandlers,
  ConversationDTO,
  CreateConversationInput,
  StreamAssistantMessageInput,
} from "../types";
import type { HttpClient } from "./httpClient";

const conversationsPath = "/v1/chat/conversations";

type CreateConversationRequestBody = {
  title?: string;
  modelRef?: CreateConversationInput["modelRef"];
  systemInstruction?: string;
  config?: Record<string, unknown>;
  idempotencyKey?: string;
};

type AppendUserMessageRequestBody = {
  content: string;
  parentMessageId?: string;
  attachments?: AppendUserMessageInput["attachments"];
  metadata?: Record<string, unknown>;
  idempotencyKey?: string;
};

export function createServerChatApiShell(httpClient: HttpClient): ChatApi {
  return {
    async createConversation(
      input: CreateConversationInput,
    ): Promise<ConversationDTO> {
      return httpClient.requestJson<ConversationDTO>(conversationsPath, {
        method: "POST",
        body: createConversationBody(input),
      });
    },
    async listConversations(): Promise<ConversationDTO[]> {
      const page =
        await httpClient.requestJson<ApiPage<ConversationDTO>>(
          conversationsPath,
        );
      return getPageItems(page, "conversation list");
    },
    async appendUserMessage(
      input: AppendUserMessageInput,
    ): Promise<ChatMessageDTO> {
      return httpClient.requestJson<ChatMessageDTO>(
        `${conversationPath(input.conversationId)}/messages`,
        {
          method: "POST",
          body: appendUserMessageBody(input),
        },
      );
    },
    async listMessages(conversationId: string): Promise<ChatMessageDTO[]> {
      const page = await httpClient.requestJson<ApiPage<ChatMessageDTO>>(
        `${conversationPath(conversationId)}/messages`,
      );
      return getPageItems(page, "message list");
    },
    async streamAssistantMessage(
      _input: StreamAssistantMessageInput,
      _handlers?: ChatStreamHandlers,
    ): Promise<ChatRunResult> {
      return {
        status: "unsupported",
        error: unsupportedFeature("server streamAssistantMessage").toEnvelope()
          .error,
      };
    },
    async cancelRun(_runId: string): Promise<ChatRunResult> {
      return {
        status: "unsupported",
        error: unsupportedFeature("server cancelRun").toEnvelope().error,
      };
    },
  };
}

function createConversationBody(
  input: CreateConversationInput,
): CreateConversationRequestBody {
  return removeUndefined({
    title: input.title,
    modelRef: input.modelRef,
    systemInstruction: input.systemInstruction,
    config: input.config,
    idempotencyKey: input.idempotencyKey,
  });
}

function appendUserMessageBody(
  input: AppendUserMessageInput,
): AppendUserMessageRequestBody {
  if (!input.content.trim()) {
    throw new ApiClientError("EMPTY_CONTENT", "message content is required");
  }

  return removeUndefined({
    content: input.content,
    parentMessageId: input.parentMessageId,
    attachments: normalizeServerAttachments(input.attachments),
    metadata: input.metadata,
    idempotencyKey: input.idempotencyKey,
  });
}

function normalizeServerAttachments(
  attachments: AppendUserMessageInput["attachments"],
): AppendUserMessageInput["attachments"] {
  if (!attachments?.length) return undefined;

  return attachments.map((attachment) => {
    if (attachment.source && attachment.source !== "server") {
      throw new ApiClientError(
        "UNSUPPORTED_ATTACHMENT_SOURCE",
        "server mode only accepts server file attachments.",
      );
    }
    return removeUndefined({
      source: "server" as const,
      fileId: attachment.fileId,
      purpose: attachment.purpose,
    });
  });
}

function conversationPath(conversationId: string): string {
  return `${conversationsPath}/${encodeURIComponent(conversationId)}`;
}

function getPageItems<T>(page: ApiPage<T>, label: string): T[] {
  if (!page || !Array.isArray(page.items)) {
    throw new ApiClientError(
      "INVALID_SERVER_RESPONSE",
      `Server returned invalid ${label} response.`,
    );
  }
  return page.items;
}

function removeUndefined<T extends Record<string, unknown>>(value: T): T {
  return Object.fromEntries(
    Object.entries(value).filter(([, item]) => item !== undefined),
  ) as T;
}
