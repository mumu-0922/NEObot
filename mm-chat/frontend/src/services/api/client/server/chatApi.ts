import { ApiClientError } from "../errors";
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
  ServerStreamEvent,
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

type StreamAssistantMessageRequestBody = {
  userMessageId: string;
  modelRef: StreamAssistantMessageInput["modelRef"];
  config?: Record<string, unknown>;
  systemInstruction?: string;
  systemPrompt?: string;
  metadata?: Record<string, unknown>;
  idempotencyKey: string;
};

type CancelRunResponse = {
  runId: string;
  status: "cancelled";
  message?: ChatMessageDTO;
};

type StreamDispatchState = {
  startedRunId?: string;
  lastSequenceByRunId: Map<string, number>;
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
      input: StreamAssistantMessageInput,
      handlers?: ChatStreamHandlers,
    ): Promise<ChatRunResult> {
      let result: ChatRunResult | null = null;
      const dispatchState: StreamDispatchState = {
        lastSequenceByRunId: new Map(),
      };

      try {
        await httpClient.requestSse(
          `${conversationPath(input.conversationId)}/stream`,
          {
            method: "POST",
            body: streamAssistantMessageBody(input),
            signal: input.signal,
            onFrame: ({ data }) => {
              if (result) return;
              if (input.signal?.aborted && dispatchState.startedRunId) {
                throw streamAbortedAfterStartError();
              }
              result = dispatchStreamEvent(data, handlers, dispatchState);
              if (
                !result &&
                input.signal?.aborted &&
                dispatchState.startedRunId
              ) {
                throw streamAbortedAfterStartError();
              }
            },
          },
        );
      } catch (error) {
        if (
          isStreamInterrupted(error) &&
          input.signal?.aborted &&
          dispatchState.startedRunId
        ) {
          return cancelRunById(httpClient, dispatchState.startedRunId);
        }

        return runResultFromError(error, {
          streamInterruptedStatus: input.signal?.aborted
            ? "cancelled"
            : "failed",
        });
      }

      if (!result && input.signal?.aborted && dispatchState.startedRunId) {
        return cancelRunById(httpClient, dispatchState.startedRunId);
      }

      return (
        result ?? {
          status: "failed",
          error: new ApiClientError(
            "STREAM_INTERRUPTED",
            "Stream ended without a terminal event.",
            { recoverable: true },
          ).toEnvelope().error,
        }
      );
    },
    async cancelRun(runId: string): Promise<ChatRunResult> {
      return cancelRunById(httpClient, runId);
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

function streamAssistantMessageBody(
  input: StreamAssistantMessageInput,
): StreamAssistantMessageRequestBody {
  if (!input.idempotencyKey.trim()) {
    throw new ApiClientError(
      "IDEMPOTENCY_KEY_REQUIRED",
      "stream idempotencyKey is required",
    );
  }

  return removeUndefined({
    userMessageId: input.userMessageId,
    modelRef: input.modelRef,
    config: input.config,
    systemInstruction: input.systemInstruction,
    systemPrompt: input.systemPrompt,
    metadata: input.metadata,
    idempotencyKey: input.idempotencyKey,
  });
}

function dispatchStreamEvent(
  event: ServerStreamEvent,
  handlers?: ChatStreamHandlers,
  state?: StreamDispatchState,
): ChatRunResult | null {
  if (state && !shouldDispatchSequencedEvent(event, state)) {
    return null;
  }

  switch (event.type) {
    case "message.started":
      if (state && event.runId) {
        state.startedRunId = event.runId;
      }
      handlers?.onStarted?.(event);
      return null;
    case "message.delta":
      handlers?.onDelta?.(event);
      return null;
    case "usage.updated":
      handlers?.onUsage?.(event);
      return null;
    case "message.completed":
      handlers?.onCompleted?.(event);
      return {
        status: "completed",
        ...(event.message ? { message: event.message } : {}),
      };
    case "message.error": {
      handlers?.onError?.(event);
      return {
        status: "failed",
        error:
          event.error ??
          new ApiClientError("STREAM_FAILED", "Server stream failed.", {
            recoverable: true,
          }).toEnvelope().error,
      };
    }
    case "message.cancelled":
      handlers?.onCancelled?.(event);
      return {
        status: "cancelled",
        ...(event.message ? { message: event.message } : {}),
      };
    default:
      return null;
  }
}

async function cancelRunById(
  httpClient: HttpClient,
  runId: string,
): Promise<ChatRunResult> {
  try {
    const response = await httpClient.requestJson<CancelRunResponse>(
      `/v1/chat/runs/${encodeURIComponent(runId)}/cancel`,
      { method: "POST" },
    );
    return {
      status: "cancelled",
      ...(response.message ? { message: response.message } : {}),
    };
  } catch (error) {
    return runResultFromError(error);
  }
}

function shouldDispatchSequencedEvent(
  event: ServerStreamEvent,
  state: StreamDispatchState,
): boolean {
  if (typeof event.sequence !== "number") return true;
  if (!Number.isInteger(event.sequence) || event.sequence < 1) {
    throw new ApiClientError(
      "STREAM_PROTOCOL_ERROR",
      "SSE event sequence must be a positive integer.",
      { recoverable: true },
    );
  }

  const runId = event.runId ?? state.startedRunId;
  if (!runId) return true;

  const previous = state.lastSequenceByRunId.get(runId);
  if (previous === undefined) {
    if (event.sequence !== 1) {
      throw streamInterruptedError(runId, event.sequence, 1);
    }
    state.lastSequenceByRunId.set(runId, event.sequence);
    return true;
  }

  if (event.sequence <= previous) return false;
  const expected = previous + 1;
  if (event.sequence !== expected) {
    throw streamInterruptedError(runId, event.sequence, expected);
  }

  state.lastSequenceByRunId.set(runId, event.sequence);
  return true;
}

function streamInterruptedError(
  runId: string,
  received: number,
  expected: number,
): ApiClientError {
  return new ApiClientError(
    "STREAM_INTERRUPTED",
    `Stream sequence gap for run "${runId}": expected ${expected}, received ${received}.`,
    { recoverable: true },
  );
}

function isStreamInterrupted(error: unknown): boolean {
  return error instanceof ApiClientError && error.code === "STREAM_INTERRUPTED";
}

function streamAbortedAfterStartError(): ApiClientError {
  return new ApiClientError("STREAM_INTERRUPTED", "Stream was aborted.", {
    recoverable: true,
  });
}

function runResultFromError(
  error: unknown,
  options: { streamInterruptedStatus?: ChatRunResult["status"] } = {},
): ChatRunResult {
  const clientError =
    error instanceof ApiClientError
      ? error
      : new ApiClientError(
          "NETWORK_ERROR",
          error instanceof Error ? error.message : "Stream request failed.",
          { recoverable: true },
        );

  return {
    status:
      clientError.code === "STREAM_INTERRUPTED"
        ? (options.streamInterruptedStatus ?? "failed")
        : "failed",
    error: clientError.toEnvelope().error,
  };
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
