import {
  ApiClientError,
  createNeoChatApiClient,
  type ApiClientConfig,
  type ChatStreamHandlers,
  type NeoChatApiClient,
  type StreamAssistantMessageInput,
} from "./client";
import {
  mapChatMessageDtoToMessage,
  type ChatCrudMessage,
} from "./chatCrudService";

export interface ChatStreamRunResult {
  status: "completed" | "failed" | "cancelled" | "unsupported";
  message?: ChatCrudMessage;
  error?: {
    code: string;
    message: string;
    recoverable?: boolean;
    requestId?: string;
  };
}

export interface ChatStreamServiceOptions {
  config?: ApiClientConfig;
  client?: NeoChatApiClient;
}

export interface ChatStreamService {
  mode: NeoChatApiClient["mode"];
  streamEnabled: boolean;
  streamAssistantMessage(
    input: StreamAssistantMessageInput,
    handlers?: ChatStreamHandlers,
  ): Promise<ChatStreamRunResult>;
  cancelRun(runId: string): Promise<ChatStreamRunResult>;
}

export function createChatStreamService(
  options: ChatStreamServiceOptions = {},
): ChatStreamService {
  const client = options.client ?? createNeoChatApiClient(options.config);
  const baseUrl = client.config.baseUrl;
  const streamEnabled =
    client.mode === "server" && client.capabilities.chatStream === true;

  function requireServerStream(): void {
    if (!streamEnabled) {
      throw new ApiClientError(
        "SERVER_CHAT_STREAM_DISABLED",
        "Server chat streaming is not enabled for the current API mode.",
        { recoverable: true },
      );
    }
  }

  return {
    mode: client.mode,
    streamEnabled,

    async streamAssistantMessage(input, handlers) {
      requireServerStream();
      const result = await client.chat.streamAssistantMessage(input, handlers);
      return mapChatRunResultToStreamResult(result, { baseUrl });
    },

    async cancelRun(runId) {
      requireServerStream();
      const result = await client.chat.cancelRun(runId);
      return mapChatRunResultToStreamResult(result, { baseUrl });
    },
  };
}

function mapChatRunResultToStreamResult(
  result: Awaited<
    ReturnType<NeoChatApiClient["chat"]["streamAssistantMessage"]>
  >,
  options: { baseUrl?: string },
): ChatStreamRunResult {
  return {
    status: result.status,
    ...(result.message
      ? { message: mapChatMessageDtoToMessage(result.message, options) }
      : {}),
    ...(result.error ? { error: result.error } : {}),
  };
}
