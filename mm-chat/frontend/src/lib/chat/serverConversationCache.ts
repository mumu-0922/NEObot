import type { Message, SessionMessageTree } from "@/types";
import {
  getActiveMessagePath,
  getAllMessagesFromTree,
} from "@/lib/chat/messageTree";

export const SERVER_CONVERSATION_MEMORY_CACHE_LIMIT = 6;

export interface ServerConversationMemorySnapshot {
  activeMessageTree: SessionMessageTree;
  activeMessages: Message[];
  messageCount: number;
}

const snapshots = new Map<string, ServerConversationMemorySnapshot>();

export const getServerConversationMemorySnapshot = (
  conversationId: string,
): ServerConversationMemorySnapshot | null => {
  const snapshot = snapshots.get(conversationId);
  if (!snapshot) return null;

  snapshots.delete(conversationId);
  snapshots.set(conversationId, snapshot);
  return snapshot;
};

export const setServerConversationMemorySnapshot = (
  conversationId: string,
  activeMessageTree: SessionMessageTree,
): ServerConversationMemorySnapshot => {
  const snapshot = {
    activeMessageTree,
    activeMessages: getActiveMessagePath(activeMessageTree),
    messageCount: getAllMessagesFromTree(activeMessageTree).length,
  };

  snapshots.delete(conversationId);
  snapshots.set(conversationId, snapshot);
  while (snapshots.size > SERVER_CONVERSATION_MEMORY_CACHE_LIMIT) {
    const oldestConversationId = snapshots.keys().next().value;
    if (typeof oldestConversationId !== "string") break;
    snapshots.delete(oldestConversationId);
  }
  return snapshot;
};

export const deleteServerConversationMemorySnapshot = (
  conversationId: string,
): void => {
  snapshots.delete(conversationId);
};

export const isServerConversationMemorySnapshotCurrent = (
  snapshot: ServerConversationMemorySnapshot,
  activeMessageTree: SessionMessageTree,
): boolean =>
  JSON.stringify(snapshot.activeMessageTree) ===
  JSON.stringify(activeMessageTree);

export const clearServerConversationMemoryCache = (): void => {
  snapshots.clear();
};
