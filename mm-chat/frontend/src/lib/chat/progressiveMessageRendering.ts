export const INITIAL_CHAT_MESSAGE_RENDER_COUNT = 8;
export const CHAT_MESSAGE_RENDER_BATCH_SIZE = 6;

export const getInitialChatMessageRenderStart = (
  messageCount: number,
): number => Math.max(0, messageCount - INITIAL_CHAT_MESSAGE_RENDER_COUNT);

export const getNextChatMessageRenderStart = (currentStart: number): number =>
  Math.max(0, currentStart - CHAT_MESSAGE_RENDER_BATCH_SIZE);
