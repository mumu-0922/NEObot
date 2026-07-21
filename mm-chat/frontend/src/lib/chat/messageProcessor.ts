import { v7 as uuidv7 } from "uuid";
import type { Attachment, Message, Source } from "../../types";
import type { ModelInfo } from "@/services/api/chatService";
import {
  separateKBAttachments,
  processAttachmentsForModel,
} from "../utils/attachments";
import { parseModelString } from "../utils/model";
import { resolveOPFSUrl } from "../../utils/opfs";
import { appendContextToChatInput } from "../utils/chatInput";

export interface ProcessMessageOptions {
  text: string;
  attachments: Attachment[];
  selectedModel: string;
  modelMetadata: Record<string, any>;
  customModelMetadata: Record<string, any>;
}

export interface ProcessedMessageData {
  finalText: string;
  finalAttachments: Attachment[];
  ragSources: Source[];
  userMessage: Message;
}

/**
 * Process message and attachments before sending to LLM
 */
export async function processMessageForSending(
  options: ProcessMessageOptions,
): Promise<ProcessedMessageData> {
  const {
    text,
    attachments,
    selectedModel,
    modelMetadata,
    customModelMetadata,
  } = options;

  let finalText = text;
  let convertedContent = "";
  const finalAttachments: Attachment[] = [];

  const { otherAttachments } = separateKBAttachments(attachments);

  // Check model capability
  const { modelName: modelId } = parseModelString(selectedModel);
  const meta = customModelMetadata[modelId] || modelMetadata[modelId];
  const supportAttachment = meta ? (meta.attachment ?? false) : true;

  // Process other attachments
  const attachmentResult = await processAttachmentsForModel(
    otherAttachments,
    supportAttachment,
    resolveOPFSUrl,
  );
  finalAttachments.push(...attachmentResult.finalAttachments);
  convertedContent += attachmentResult.convertedContent;

  // Combine text with converted content
  finalText = appendContextToChatInput(finalText, convertedContent);

  // Create user message
  const userMessage: Message = {
    id: uuidv7(),
    role: "user",
    content: text,
    timestamp: Date.now(),
    attachments: attachments,
  };

  return {
    finalText,
    finalAttachments,
    ragSources: [],
    userMessage,
  };
}

/**
 * Create initial bot message placeholder
 */
export function createBotMessagePlaceholder(
  modelDisplayName: string,
  ragSources: Source[],
): Message {
  const botMsgId = uuidv7();
  const startTime = Date.now();

  return {
    id: botMsgId,
    role: "model",
    content: "",
    reasoning: "",
    timestamp: startTime,
    model: modelDisplayName,
    ragSources: ragSources.length > 0 ? ragSources : undefined,
    isSearching: false,
  };
}

/**
 * Get model display name from available models
 */
export function getModelDisplayName(
  selectedModel: string,
  availableModels: ModelInfo[],
): string {
  const currentModelInfo = availableModels.find(
    (m) => m.name === selectedModel,
  );
  return currentModelInfo?.displayName || selectedModel;
}
