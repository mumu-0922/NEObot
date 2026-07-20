export type ChatGenerationProgressStage = "knowledge" | "search" | "model";

const CURRENT_PUBLIC_ENGLISH_MARKERS = [
  "today",
  "latest",
  "current",
  "currently",
  "now",
  "recent",
  "this week",
  "this month",
  "this year",
  "live",
  "news",
  "weather",
  "price",
  "exchange rate",
  "stock",
  "official site",
  "online",
  "web",
  "internet",
  "search for",
  "look up",
] as const;

const CURRENT_PUBLIC_CHINESE_MARKERS = [
  "今天",
  "今日",
  "最新",
  "当前",
  "现在",
  "近期",
  "最近",
  "今年",
  "本周",
  "本月",
  "实时",
  "新闻",
  "天气",
  "价格",
  "汇率",
  "股价",
  "官网",
  "网上",
  "网络",
  "互联网",
  "搜索",
  "搜一下",
  "查一下",
] as const;

function normalizeEnglishQuestion(question: string): string {
  return ` ${question
    .toLocaleLowerCase("en")
    .split(/[^\p{L}\p{N}]+/u)
    .filter(Boolean)
    .join(" ")} `;
}

export function hasCurrentPublicProgressIntent(question: string): boolean {
  const normalized = question.trim().toLocaleLowerCase();
  if (!normalized) return false;

  const englishWords = normalizeEnglishQuestion(normalized);
  if (
    CURRENT_PUBLIC_ENGLISH_MARKERS.some((marker) =>
      englishWords.includes(` ${marker} `),
    )
  ) {
    return true;
  }

  return CURRENT_PUBLIC_CHINESE_MARKERS.some((marker) =>
    normalized.includes(marker),
  );
}

export function inferPendingChatProgressStage({
  question,
  searchEnabled,
  knowledgeCollectionIds,
}: {
  question: string;
  searchEnabled: boolean;
  knowledgeCollectionIds?: readonly string[];
}): ChatGenerationProgressStage {
  if (searchEnabled && hasCurrentPublicProgressIntent(question)) {
    return "search";
  }
  if (knowledgeCollectionIds?.length) {
    return "knowledge";
  }
  if (searchEnabled) {
    return "search";
  }
  return "model";
}
