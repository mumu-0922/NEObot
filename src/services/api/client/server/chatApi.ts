import { unsupportedFeature } from "../errors";
import type {
  AppendUserMessageInput,
  ChatApi,
  ChatMessageDTO,
  ChatRunResult,
  ChatStreamHandlers,
  ConversationDTO,
  CreateConversationInput,
  StreamAssistantMessageInput,
} from "../types";
import type { HttpClient } from "./httpClient";

export function createServerChatApiShell(_httpClient: HttpClient): ChatApi {
  return {
    async createConversation(
      _input: CreateConversationInput,
    ): Promise<ConversationDTO> {
      throw unsupportedFeature("server createConversation");
    },
    async listConversations(): Promise<ConversationDTO[]> {
      throw unsupportedFeature("server listConversations");
    },
    async appendUserMessage(
      _input: AppendUserMessageInput,
    ): Promise<ChatMessageDTO> {
      throw unsupportedFeature("server appendUserMessage");
    },
    async listMessages(_conversationId: string): Promise<ChatMessageDTO[]> {
      throw unsupportedFeature("server listMessages");
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
