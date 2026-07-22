import {
  ApiClientError,
  createNeoChatApiClient,
  joinUrl,
  type ApiClientConfig,
  type AppendUserMessageInput,
  type ChatMessageDTO,
  type ConversationDTO,
  type CreateConversationInput,
  type DeleteMessageInput,
  type DuplicateConversationInput,
  type GenerateConversationTitleInput,
  type ModelRef,
  type UpdateConversationInput,
  type UpdateMessageInput,
  type NeoChatApiClient,
  type ServerAttachmentDTO,
} from "./client";
import { normalizeSessionTitle } from "../../lib/chat/entities";
import {
  isReasoningEffort,
  normalizeReasoningEffort,
} from "../../lib/chat/reasoning";
import type { ReasoningEffort, SearchMode } from "../../lib/chat/types";
import {
  isSearchMode,
  normalizeSearchMode,
  searchModeEnabled,
} from "../../lib/chat/searchMode";
import { IMAGE_CONTENT_POLICY_VIOLATION_CODE } from "../../lib/chat/types";
import type { ProcessStep } from "../../lib/chat/types";
import {
  processTraceFromMessageMetadata,
  reasoningFromMessageMetadata,
} from "../../lib/chat/processTrace";
import { SERVER_DEFAULT_PROVIDER_ID } from "../../lib/defaultConfig/shared";
import {
  normalizeMessageKnowledgeMetadata,
  reconcileMessageKnowledgeContent,
} from "../../lib/knowledge/citations";
import type { MessageKnowledgeMetadata } from "../../lib/knowledge/types";
import {
  MAX_CONVERSATION_KNOWLEDGE_COLLECTIONS,
  normalizeKnowledgeCollectionIds,
} from "../../lib/utils/knowledgeAttachments";

const SERVER_DEFAULT_BACKEND_PROVIDER_ID = "openai_compatible";

export interface ChatCrudSessionConfig {
  searchMode?: SearchMode;
  useSearch?: boolean;
  useReasoning?: boolean;
  reasoningEffort?: ReasoningEffort;
  activePlugins?: string[];
  activeSkills?: string[];
  selectedKnowledgeCollectionIds?: string[];
}

export interface ChatCrudSession {
  id: string;
  title: string;
  messageCount: number;
  updatedAt: number;
  model: string;
  pinned: boolean;
  systemInstruction?: string;
  config?: ChatCrudSessionConfig;
}

export interface ChatCrudAttachment {
  id: string;
  source: "server";
  fileId: string;
  fileName: string;
  mimeType: string;
  size: number;
  sha256: string;
  purpose: string;
  url: string;
}

export interface ChatCrudMessage {
  id: string;
  role: "user" | "model";
  content: string;
  timestamp: number;
  metadata?: Record<string, unknown>;
  reasoning?: string;
  processTrace?: ProcessStep[];
  attachments?: ChatCrudAttachment[];
  model?: string;
  generationError?: {
    message: string;
    recoverable?: boolean;
    code?: string;
  };
  outputBlocks?: unknown[];
  knowledge?: MessageKnowledgeMetadata;
  parentMessageId?: string;
  treeParentMessageId?: string | null;
}

export interface ChatCrudServiceOptions {
  config?: ApiClientConfig;
  client?: NeoChatApiClient;
}

export interface ChatCrudService {
  mode: NeoChatApiClient["mode"];
  serverEnabled: boolean;
  createConversation(input: CreateConversationInput): Promise<ChatCrudSession>;
  listConversations(): Promise<ChatCrudSession[]>;
  updateConversation(input: UpdateConversationInput): Promise<ChatCrudSession>;
  deleteConversation(conversationId: string): Promise<void>;
  duplicateConversation(
    input: DuplicateConversationInput,
  ): Promise<ChatCrudSession>;
  generateConversationTitle(
    input: GenerateConversationTitleInput,
  ): Promise<string>;
  updateMessage(input: UpdateMessageInput): Promise<ChatCrudMessage>;
  deleteMessage(input: DeleteMessageInput): Promise<void>;
  appendUserMessage(input: AppendUserMessageInput): Promise<ChatCrudMessage>;
  listMessages(conversationId: string): Promise<ChatCrudMessage[]>;
}

export function createChatCrudService(
  options: ChatCrudServiceOptions = {},
): ChatCrudService {
  const client = options.client ?? createNeoChatApiClient(options.config);
  const baseUrl = client.config.baseUrl;
  const serverEnabled =
    client.mode === "server" && client.capabilities.chatCrud === true;

  function requireServerCrud(): void {
    if (!serverEnabled) {
      throw new ApiClientError(
        "SERVER_CHAT_CRUD_DISABLED",
        "Server chat CRUD is not enabled for the current API mode.",
        { recoverable: true },
      );
    }
  }

  return {
    mode: client.mode,
    serverEnabled,

    async createConversation(input) {
      requireServerCrud();
      return mapConversationDtoToSession(
        await client.chat.createConversation(input),
      );
    },

    async listConversations() {
      requireServerCrud();
      const conversations = await client.chat.listConversations();
      return conversations.map(mapConversationDtoToSession);
    },

    async updateConversation(input) {
      requireServerCrud();
      return mapConversationDtoToSession(
        await client.chat.updateConversation(input),
      );
    },

    async deleteConversation(conversationId) {
      requireServerCrud();
      await client.chat.deleteConversation(conversationId);
    },

    async duplicateConversation(input) {
      requireServerCrud();
      return mapConversationDtoToSession(
        await client.chat.duplicateConversation(input),
      );
    },

    async generateConversationTitle(input) {
      requireServerCrud();
      const response = await client.chat.generateConversationTitle(input);
      return normalizeSessionTitle(response.title);
    },

    async updateMessage(input) {
      requireServerCrud();
      return mapChatMessageDtoToMessage(
        await client.chat.updateMessage(input),
        {
          baseUrl,
        },
      );
    },

    async deleteMessage(input) {
      requireServerCrud();
      await client.chat.deleteMessage(input);
    },

    async appendUserMessage(input) {
      requireServerCrud();
      return mapChatMessageDtoToMessage(
        await client.chat.appendUserMessage(input),
        { baseUrl },
      );
    },

    async listMessages(conversationId) {
      requireServerCrud();
      const messages = await client.chat.listMessages(conversationId);
      return messages.map((message) =>
        mapChatMessageDtoToMessage(message, { baseUrl }),
      );
    },
  };
}

export function mapConversationDtoToSession(
  conversation: ConversationDTO,
): ChatCrudSession {
  return {
    id: conversation.id,
    title: conversation.title.trim() || "New Chat",
    messageCount: Math.max(
      0,
      Math.floor(Number(conversation.messageCount) || 0),
    ),
    updatedAt: parseServerTimestamp(
      conversation.updatedAt,
      "conversation.updatedAt",
    ),
    model: modelRefToModelString(conversation.modelRef),
    pinned: conversation.pinned === true || conversation.config.pinned === true,
    systemInstruction:
      typeof conversation.systemInstruction === "string"
        ? conversation.systemInstruction
        : undefined,
    config: normalizeConversationConfig(conversation.config),
  };
}

export function mapChatMessageDtoToMessage(
  message: ChatMessageDTO,
  options: { baseUrl?: string } = {},
): ChatCrudMessage {
  const timestamp = parseServerTimestamp(
    message.createdAt,
    "message.createdAt",
  );
  const role = mapServerRoleToLegacyRole(message.role);
  const model = modelRefToModelString(message.modelRef);
  const knowledge = normalizeMessageKnowledgeMetadata(
    message.metadata,
    message.content,
  );
  const content = reconcileMessageKnowledgeContent(
    normalizeImageGenerationContent(message),
    knowledge,
  );
  const generationError = normalizeServerGenerationError(message);
  const reasoning = reasoningFromMessageMetadata(message.metadata);
  const processTrace = processTraceFromMessageMetadata(message.metadata);

  return {
    id: message.id,
    role,
    content,
    timestamp,
    ...(message.metadata ? { metadata: message.metadata } : {}),
    ...(reasoning ? { reasoning } : {}),
    ...(processTrace ? { processTrace } : {}),
    ...(knowledge ? { knowledge } : {}),
    ...(role === "model" && model ? { model } : {}),
    ...(generationError ? { generationError } : {}),
    ...(message.attachments.length > 0
      ? {
          attachments: message.attachments.map((attachment) =>
            mapServerAttachmentToAttachment(attachment, options),
          ),
        }
      : {}),
    ...(message.outputBlocks.length > 0
      ? { outputBlocks: message.outputBlocks }
      : {}),
    ...(message.parentMessageId
      ? { parentMessageId: message.parentMessageId }
      : {}),
    ...treeParentFromMetadata(message.metadata),
  };
}

function normalizeServerGenerationError(
  message: ChatMessageDTO,
): ChatCrudMessage["generationError"] {
  if (message.role !== "assistant" || message.status !== "failed") {
    return undefined;
  }

  const errorCode =
    typeof message.metadata.errorCode === "string"
      ? message.metadata.errorCode.trim()
      : "";
  const isImageGeneration = message.metadata.kind === "image_generation";
  return {
    message:
      message.content.trim() ||
      (isImageGeneration
        ? "Image generation failed."
        : "Server generation failed."),
    recoverable: errorCode !== IMAGE_CONTENT_POLICY_VIOLATION_CODE,
    ...(errorCode ? { code: errorCode } : {}),
  };
}

function normalizeImageGenerationContent(message: ChatMessageDTO): string {
  const isLegacyImagePlaceholder =
    message.role === "assistant" &&
    message.metadata.kind === "image_generation" &&
    message.content.trim() === "Image generated." &&
    message.attachments.some((attachment) =>
      attachment.mimeType.startsWith("image/"),
    );
  return isLegacyImagePlaceholder ? "" : message.content;
}

function treeParentFromMetadata(
  metadata: Record<string, unknown>,
): Pick<ChatCrudMessage, "treeParentMessageId"> {
  if (!Object.prototype.hasOwnProperty.call(metadata, "treeParentMessageId")) {
    return {};
  }
  const value = metadata.treeParentMessageId;
  if (typeof value === "string" && value.trim()) {
    return { treeParentMessageId: value.trim() };
  }
  if (value === null) {
    return { treeParentMessageId: null };
  }
  return {};
}

export function modelRefToModelString(modelRef?: ModelRef): string {
  const providerId = modelRef?.providerId?.trim() ?? "";
  const modelId = modelRef?.modelId?.trim() ?? "";
  if (providerId && modelId) return `${providerId}:${modelId}`;
  return modelId || providerId;
}

export function modelStringToModelRef(model: string): ModelRef | undefined {
  const trimmed = model.trim();
  if (!trimmed) return undefined;

  const parsed = parseLegacyModelString(trimmed);
  const providerId =
    parsed.providerId === SERVER_DEFAULT_PROVIDER_ID
      ? SERVER_DEFAULT_BACKEND_PROVIDER_ID
      : (parsed.providerId ?? "");

  return {
    providerId,
    modelId: parsed.modelName,
  };
}

export function parseServerTimestamp(value: string, fieldName: string): number {
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) {
    throw new ApiClientError(
      "INVALID_SERVER_RESPONSE",
      `Server returned invalid ${fieldName}.`,
    );
  }
  return timestamp;
}

function normalizeConversationConfig(
  config: Record<string, unknown>,
): ChatCrudSessionConfig | undefined {
  const normalized: ChatCrudSessionConfig = {};

  if (
    isSearchMode(config.searchMode) ||
    typeof config.useSearch === "boolean"
  ) {
    const searchMode = normalizeSearchMode(config.searchMode, config.useSearch);
    normalized.searchMode = searchMode;
    normalized.useSearch = searchModeEnabled(searchMode);
  }
  if (typeof config.useReasoning === "boolean") {
    normalized.useReasoning = config.useReasoning;
  }
  if (isReasoningEffort(config.reasoningEffort)) {
    normalized.reasoningEffort = normalizeReasoningEffort(
      config.reasoningEffort,
    );
  }
  if (Array.isArray(config.activePlugins)) {
    const activePlugins = config.activePlugins.filter(
      (value): value is string => typeof value === "string" && value !== "",
    );
    if (activePlugins.length > 0) normalized.activePlugins = activePlugins;
  }
  if (Array.isArray(config.activeSkills)) {
    const activeSkills = config.activeSkills.filter(
      (value): value is string => typeof value === "string" && value !== "",
    );
    if (activeSkills.length > 0) normalized.activeSkills = activeSkills;
  }
  if (Array.isArray(config.selectedKnowledgeCollectionIds)) {
    normalized.selectedKnowledgeCollectionIds = normalizeKnowledgeCollectionIds(
      config.selectedKnowledgeCollectionIds.filter(
        (value): value is string => typeof value === "string",
      ),
    ).slice(0, MAX_CONVERSATION_KNOWLEDGE_COLLECTIONS);
  }

  return Object.keys(normalized).length > 0 ? normalized : undefined;
}

function parseLegacyModelString(model: string): {
  providerId?: string;
  modelName: string;
} {
  const separatorIndex = model.indexOf(":");
  if (separatorIndex > 0) {
    const providerId = model.slice(0, separatorIndex);
    const modelName = model.slice(separatorIndex + 1);
    if (modelName) {
      return { providerId, modelName };
    }
  }

  return { modelName: model };
}

function mapServerRoleToLegacyRole(
  role: ChatMessageDTO["role"],
): ChatCrudMessage["role"] {
  if (role === "user") return "user";
  if (role === "assistant") return "model";

  throw new ApiClientError(
    "UNSUPPORTED_MESSAGE_ROLE",
    `Server message role "${role}" cannot be rendered by the legacy chat UI.`,
  );
}

function mapServerAttachmentToAttachment(
  attachment: ServerAttachmentDTO,
  options: { baseUrl?: string },
): ChatCrudAttachment {
  return {
    id: attachment.id || attachment.fileId,
    source: "server",
    fileId: attachment.fileId,
    fileName: attachment.fileName || "download",
    mimeType: attachment.mimeType || "application/octet-stream",
    size: attachment.size,
    sha256: attachment.sha256,
    purpose: attachment.purpose || "input",
    url: joinUrl(
      options.baseUrl ?? "",
      `/v1/files/${encodeURIComponent(attachment.fileId)}/content`,
    ),
  };
}
