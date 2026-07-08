import { unsupportedFeature } from "../errors";
import type {
  AppendUserMessageInput,
  ChatApi,
  ChatMessageDTO,
  ChatRunResult,
  CreateConversationInput,
  ConversationDTO,
  StreamAssistantMessageInput,
  ChatStreamHandlers,
} from "../types";

export function createLocalChatApiShell(): ChatApi {
  return {
    async createConversation(
      _input: CreateConversationInput,
    ): Promise<ConversationDTO> {
      throw unsupportedFeature("local chat adapter wiring");
    },
    async listConversations(): Promise<ConversationDTO[]> {
      throw unsupportedFeature("local chat adapter wiring");
    },
    async appendUserMessage(
      _input: AppendUserMessageInput,
    ): Promise<ChatMessageDTO> {
      throw unsupportedFeature("local chat adapter wiring");
    },
    async listMessages(_conversationId: string): Promise<ChatMessageDTO[]> {
      throw unsupportedFeature("local chat adapter wiring");
    },
    async streamAssistantMessage(
      _input: StreamAssistantMessageInput,
      _handlers?: ChatStreamHandlers,
    ): Promise<ChatRunResult> {
      return {
        status: "unsupported",
        error: unsupportedFeature("local stream adapter wiring").toEnvelope()
          .error,
      };
    },
    async cancelRun(_runId: string): Promise<ChatRunResult> {
      return {
        status: "unsupported",
        error: unsupportedFeature("local cancel adapter wiring").toEnvelope()
          .error,
      };
    },
  };
}
