import type { Message } from "@/types";

export const CHAT_NAVIGATION_READING_LINE_RATIO = 0.4;
export const CHAT_NAVIGATION_MAX_LABEL_CHARS = 160;

export interface ChatMessageNavigationItem {
  id: string;
  label: string;
  messageIndex: number;
}

export interface ChatMessageNavigationPosition {
  id: string;
  top: number;
  height: number;
}

interface ChatMessageNavigationLabelOptions {
  emptyLabel: string;
  attachmentPrefix: string;
  maxLabelChars?: number;
}

const normalizeNavigationText = (value: string) =>
  value.replace(/\s+/g, " ").trim();

const boundNavigationLabel = (value: string, maxLabelChars: number) =>
  value.length > maxLabelChars
    ? `${value.slice(0, Math.max(1, maxLabelChars - 1)).trimEnd()}…`
    : value;

export function getChatMessageElementId(messageId: string): string {
  return `chat-message-${messageId}`;
}

export function buildChatMessageNavigationItems(
  messages: readonly Message[],
  options: ChatMessageNavigationLabelOptions,
): ChatMessageNavigationItem[] {
  const maxLabelChars = Math.max(
    1,
    options.maxLabelChars ?? CHAT_NAVIGATION_MAX_LABEL_CHARS,
  );

  return messages.flatMap((message, messageIndex) => {
    if (message.role !== "user") return [];

    const contentLabel = normalizeNavigationText(message.content);
    const attachmentName = normalizeNavigationText(
      message.attachments?.[0]?.fileName ?? "",
    );
    const label = contentLabel
      ? contentLabel
      : attachmentName
        ? `${options.attachmentPrefix}${attachmentName}`
        : options.emptyLabel;

    return [
      {
        id: message.id,
        label: boundNavigationLabel(label, maxLabelChars),
        messageIndex,
      },
    ];
  });
}

export function resolveActiveChatNavigationMessageId(
  positions: readonly ChatMessageNavigationPosition[],
  readingLine: number,
): string | null {
  let activeId: string | null = null;
  let nearestDistance = Number.POSITIVE_INFINITY;

  for (const position of positions) {
    const center = position.top + position.height / 2;
    const distance = Math.abs(center - readingLine);
    if (distance < nearestDistance) {
      nearestDistance = distance;
      activeId = position.id;
    }
  }

  return activeId;
}

export function getChatNavigationRevealStart(
  currentStart: number,
  targetMessageIndex: number,
): number {
  return Math.min(Math.max(0, currentStart), Math.max(0, targetMessageIndex));
}

export function getChatNavigationScrollTop(input: {
  scrollTop: number;
  scrollHeight: number;
  clientHeight: number;
  containerTop: number;
  messageTop: number;
  messageHeight: number;
  readingLineRatio?: number;
}): number {
  const readingLineRatio =
    input.readingLineRatio ?? CHAT_NAVIGATION_READING_LINE_RATIO;
  const target =
    input.scrollTop +
    input.messageTop -
    input.containerTop +
    input.messageHeight / 2 -
    input.clientHeight * readingLineRatio;
  const maxScrollTop = Math.max(0, input.scrollHeight - input.clientHeight);

  return Math.min(maxScrollTop, Math.max(0, target));
}
