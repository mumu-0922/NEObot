import { unsupportedFeature } from "../errors";
import type {
  ChatApi,
  ChatMessageDTO,
  ChatRunResult,
  ConversationDTO,
  ServerPlannedToolCall,
} from "../types";

export function createLocalChatApiShell(): ChatApi {
  return {
    async createConversation(): Promise<ConversationDTO> {
      throw unsupportedFeature("local chat adapter wiring");
    },
    async listConversations(): Promise<ConversationDTO[]> {
      throw unsupportedFeature("local chat adapter wiring");
    },
    async updateConversation(): Promise<ConversationDTO> {
      throw unsupportedFeature("local chat adapter wiring");
    },
    async deleteConversation(): Promise<void> {
      throw unsupportedFeature("local chat adapter wiring");
    },
    async duplicateConversation(): Promise<ConversationDTO> {
      throw unsupportedFeature("local chat adapter wiring");
    },
    async generateConversationTitle() {
      throw unsupportedFeature("local title generation adapter wiring");
    },
    async generateRelatedQuestions() {
      throw unsupportedFeature("local related questions adapter wiring");
    },
    async generateText() {
      throw unsupportedFeature("local text generation adapter wiring");
    },
    async updateMessage(): Promise<ChatMessageDTO> {
      throw unsupportedFeature("local chat adapter wiring");
    },
    async deleteMessage(): Promise<void> {
      throw unsupportedFeature("local chat adapter wiring");
    },
    async appendUserMessage(): Promise<ChatMessageDTO> {
      throw unsupportedFeature("local chat adapter wiring");
    },
    async listMessages(): Promise<ChatMessageDTO[]> {
      throw unsupportedFeature("local chat adapter wiring");
    },
    async streamAssistantMessage(): Promise<ChatRunResult> {
      return {
        status: "unsupported",
        error: unsupportedFeature("local stream adapter wiring").toEnvelope()
          .error,
      };
    },
    async planTools(): Promise<ServerPlannedToolCall[]> {
      throw unsupportedFeature("local tool planning adapter wiring");
    },
    async cancelRun(): Promise<ChatRunResult> {
      return {
        status: "unsupported",
        error: unsupportedFeature("local cancel adapter wiring").toEnvelope()
          .error,
      };
    },
  };
}
