import { v7 as uuidv7 } from "uuid";
import type {
  ImageSource,
  Message,
  MessageOutputBlock,
  Source,
  ToolCall,
} from "../../types";

export interface MessageOutputBlockBuilderOptions {
  createId?: () => string;
  initialBlocks?: MessageOutputBlock[];
}

interface SearchBlockUpdate {
  isSearching?: boolean;
  error?: string;
  results?: {
    sources?: Source[];
    images?: ImageSource[];
  };
}

const cloneToolCall = (toolCall: ToolCall): ToolCall => ({ ...toolCall });

const cloneBlock = (block: MessageOutputBlock): MessageOutputBlock => {
  switch (block.type) {
    case "text":
      return { ...block };
    case "reasoning":
      return { ...block };
    case "search":
      return {
        ...block,
        error: block.error,
        sources: [...block.sources],
        images: [...block.images],
      };
    case "tool_group":
      return {
        ...block,
        toolCalls: block.toolCalls.map(cloneToolCall),
      };
    case "workspace_file":
      return { ...block };
  }
};

export function normalizeServerMessageOutputBlocks(
  values: unknown[],
): MessageOutputBlock[] {
  const normalized: MessageOutputBlock[] = [];
  for (const value of values) {
    if (!value || typeof value !== "object" || Array.isArray(value)) continue;
    const block = value as Record<string, unknown>;
    if (typeof block.id !== "string" || block.id.length < 1) continue;
    switch (block.type) {
      case "text":
      case "reasoning":
        if (typeof block.content === "string") {
          normalized.push(block as unknown as MessageOutputBlock);
        }
        break;
      case "search":
        if (Array.isArray(block.sources) && Array.isArray(block.images)) {
          normalized.push(block as unknown as MessageOutputBlock);
        }
        break;
      case "tool_group":
        if (Array.isArray(block.toolCalls)) {
          normalized.push(block as unknown as MessageOutputBlock);
        }
        break;
      case "workspace_file": {
        const size = Number(block.size);
        if (
          typeof block.workspaceId !== "string" ||
          typeof block.path !== "string" ||
          typeof block.fileName !== "string" ||
          typeof block.mimeType !== "string" ||
          typeof block.version !== "string" ||
          !Number.isSafeInteger(size) ||
          size < 0 ||
          size > 50 * 1024 * 1024 ||
          !/^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(
            block.workspaceId,
          ) ||
          block.path.length < 1 ||
          block.path.length > 4096 ||
          block.path.startsWith("/") ||
          block.path.includes("\\") ||
          block.path.split("/").includes("..") ||
          block.fileName.length < 1 ||
          block.fileName.length > 1024 ||
          block.mimeType.length < 1 ||
          block.mimeType.length > 256 ||
          !/^sha256:[0-9a-f]{64}$/.test(block.version)
        ) {
          break;
        }
        normalized.push({
          id: block.id,
          type: "workspace_file",
          workspaceId: block.workspaceId,
          path: block.path,
          fileName: block.fileName,
          mimeType: block.mimeType,
          size,
          version: block.version,
        });
        break;
      }
    }
  }
  return normalized;
}

export function createMessageOutputBlockBuilder(
  options: MessageOutputBlockBuilderOptions = {},
) {
  const createId = options.createId ?? (() => uuidv7());
  const blocks = (options.initialBlocks || []).map(cloneBlock);
  let activeSearchBlockId: string | undefined;
  for (let index = blocks.length - 1; index >= 0; index -= 1) {
    const block = blocks[index];
    if (block.type === "search" && block.isSearching) {
      activeSearchBlockId = block.id;
      break;
    }
  }

  const getLastBlock = () => blocks[blocks.length - 1];

  const findToolCallLocation = (toolCallId: string) => {
    for (const block of blocks) {
      if (block.type !== "tool_group") continue;
      const index = block.toolCalls.findIndex((tc) => tc.id === toolCallId);
      if (index !== -1) return { block, index };
    }
    return null;
  };

  const updateToolCallInGroup = (
    block: Extract<MessageOutputBlock, { type: "tool_group" }>,
    toolCall: ToolCall,
  ) => {
    const index = block.toolCalls.findIndex((tc) => tc.id === toolCall.id);
    if (index === -1) {
      block.toolCalls.push(cloneToolCall(toolCall));
      return;
    }
    block.toolCalls[index] = {
      ...block.toolCalls[index],
      ...toolCall,
    };
  };

  return {
    appendText(content: string) {
      if (!content) return;
      const last = getLastBlock();
      if (last?.type === "text") {
        last.content += content;
        return;
      }
      blocks.push({
        id: createId(),
        type: "text",
        content,
      });
    },

    appendReasoning(content: string) {
      if (!content) return;
      const last = getLastBlock();
      if (last?.type === "reasoning") {
        last.content += content;
        return;
      }
      blocks.push({
        id: createId(),
        type: "reasoning",
        content,
      });
    },

    upsertSearch(update: SearchBlockUpdate) {
      const activeTarget = activeSearchBlockId
        ? blocks.find(
            (block) =>
              block.type === "search" && block.id === activeSearchBlockId,
          )
        : undefined;
      const lastBlock = getLastBlock();
      const target:
        Extract<MessageOutputBlock, { type: "search" }> | undefined =
        activeTarget?.type === "search"
          ? activeTarget
          : lastBlock?.type === "search"
            ? lastBlock
            : undefined;

      const sources = update.results?.sources || [];
      const images = update.results?.images || [];
      const isSearching = update.isSearching ?? target?.isSearching ?? false;
      const error = update.error;

      if (target?.type === "search") {
        target.isSearching = isSearching;
        target.error = error;
        if (update.results) {
          target.sources = sources;
          target.images = images;
        }
        activeSearchBlockId = isSearching ? target.id : undefined;
        return;
      }

      const block: MessageOutputBlock = {
        id: createId(),
        type: "search",
        isSearching,
        ...(error ? { error } : {}),
        sources,
        images,
      };
      blocks.push(block);
      activeSearchBlockId = isSearching ? block.id : undefined;
    },

    appendToolCall(toolCall: ToolCall) {
      const last = getLastBlock();
      if (last?.type === "tool_group") {
        updateToolCallInGroup(last, toolCall);
        return;
      }

      blocks.push({
        id: createId(),
        type: "tool_group",
        toolCalls: [cloneToolCall(toolCall)],
      });
    },

    updateToolCall(toolCall: ToolCall) {
      const location = findToolCallLocation(toolCall.id);
      if (location) {
        updateToolCallInGroup(location.block, toolCall);
        return;
      }
      const last = getLastBlock();
      if (last?.type === "tool_group") {
        updateToolCallInGroup(last, toolCall);
        return;
      }
      blocks.push({
        id: createId(),
        type: "tool_group",
        toolCalls: [cloneToolCall(toolCall)],
      });
    },

    getBlocks(): MessageOutputBlock[] {
      return blocks.map(cloneBlock);
    },
  };
}

export function getMessageOutputBlocks(message: Message): MessageOutputBlock[] {
  if (message.outputBlocks?.length) {
    const blocks = message.outputBlocks.map(cloneBlock);
    const hasTextBlock = blocks.some((block) => block.type === "text");
    if (!hasTextBlock && message.content) {
      blocks.push({
        id: `${message.id}-server-text`,
        type: "text",
        content: message.content,
      });
    }
    return blocks;
  }

  const blocks: MessageOutputBlock[] = [];
  const sources = message.searchSources || [];
  const images = message.searchImages || [];

  if (message.isSearching || sources.length > 0 || images.length > 0) {
    blocks.push({
      id: `${message.id}-legacy-search`,
      type: "search",
      isSearching: message.isSearching,
      sources,
      images,
    });
  }

  if (message.toolCalls?.length) {
    blocks.push({
      id: `${message.id}-legacy-tools`,
      type: "tool_group",
      toolCalls: message.toolCalls.map(cloneToolCall),
    });
  }

  if (message.reasoning) {
    blocks.push({
      id: `${message.id}-legacy-reasoning`,
      type: "reasoning",
      content: message.reasoning,
    });
  }

  if (message.content) {
    blocks.push({
      id: `${message.id}-legacy-text`,
      type: "text",
      content: message.content,
    });
  }

  return blocks;
}
