import type { AgentMarketLocale } from "@/lib/market/agentLocale";

const categoryLabels: Record<AgentMarketLocale, Record<string, string>> = {
  en: {
    academic: "Academic",
    career: "Career",
    copywriting: "Copywriting",
    design: "Design",
    education: "Education",
    emotions: "Emotions",
    engineering: "Engineering",
    entertainment: "Entertainment",
    games: "Games",
    general: "General",
    life: "Life",
    marketing: "Marketing",
    office: "Office",
    programming: "Programming",
    translation: "Translation",
    uncategorized: "Uncategorized",
  },
  zh: {
    academic: "学术",
    career: "职业",
    copywriting: "文案",
    design: "设计",
    education: "教育",
    emotions: "情感",
    engineering: "工程",
    entertainment: "娱乐",
    games: "游戏",
    general: "通用",
    life: "生活",
    marketing: "商业",
    office: "办公",
    programming: "编程",
    translation: "翻译",
    uncategorized: "未分类",
  },
  ja: {
    academic: "学術",
    career: "キャリア",
    copywriting: "コピーライティング",
    design: "デザイン",
    education: "教育",
    emotions: "感情",
    engineering: "エンジニアリング",
    entertainment: "エンターテインメント",
    games: "ゲーム",
    general: "一般",
    life: "ライフ",
    marketing: "マーケティング",
    office: "オフィス",
    programming: "プログラミング",
    translation: "翻訳",
    uncategorized: "未分類",
  },
};

export function formatAgentMarketCategory(
  value: string,
  locale: AgentMarketLocale,
): string {
  const normalized = value.trim().toLowerCase();
  const translated = categoryLabels[locale][normalized];
  if (translated) return translated;

  return (normalized || "general")
    .replace(/[_-]+/g, " ")
    .replace(/\b\w/g, (character) => character.toUpperCase());
}

export function agentMarketCountKey(query: string, category: string): string {
  return `${query.trim()}\u0000${category.trim().toLowerCase()}`;
}

export function missingAgentMarketCountCategories(
  query: string,
  categories: string[],
  counts: Readonly<Record<string, number>>,
): string[] {
  return categories.filter(
    (category) => counts[agentMarketCountKey(query, category)] === undefined,
  );
}
