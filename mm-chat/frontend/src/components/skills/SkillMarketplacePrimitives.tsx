"use client";

import type { LucideIcon } from "lucide-react";
import {
  BarChart3,
  BrainCircuit,
  CloudCog,
  Code2,
  FileText,
  Gamepad2,
  GitBranch,
  ImageIcon,
  Landmark,
  ListChecks,
  Megaphone,
  MessagesSquare,
  MousePointerClick,
  Network,
  NotebookTabs,
  PanelsTopLeft,
  Search,
  ShieldCheck,
  Smartphone,
  Star,
  Tags,
  Terminal,
  Workflow,
} from "lucide-react";
import { useTranslations } from "next-intl";

import type {
  SkillCatalogSummaryDTO,
  SkillMarketplaceCategoryDTO,
  SkillMarketplaceSummaryDTO,
} from "@/services/api/client";

import { SkillIcon } from "./SkillIcon";
import { StatusPill } from "./SkillStorePrimitives";

export const MARKETPLACE_CATEGORIES = [
  { id: "coding-agents-ides", icon: Code2 },
  { id: "devops-cloud", icon: CloudCog },
  { id: "web-frontend-development", icon: PanelsTopLeft },
  { id: "cli-utilities", icon: Terminal },
  { id: "productivity-tasks", icon: ListChecks },
  { id: "ai-llms", icon: BrainCircuit },
  { id: "git-github", icon: GitBranch },
  { id: "data-analytics", icon: BarChart3 },
  { id: "marketing-sales", icon: Megaphone },
  { id: "search-research", icon: Search },
  { id: "self-hosted-automation", icon: Workflow },
  { id: "agent-to-agent-protocols", icon: Network },
  { id: "finance", icon: Landmark },
  { id: "communication", icon: MessagesSquare },
  { id: "notes-pkm", icon: NotebookTabs },
  { id: "image-video-generation", icon: ImageIcon },
  { id: "browser-automation", icon: MousePointerClick },
  { id: "ios-macos-development", icon: Smartphone },
  { id: "security-passwords", icon: ShieldCheck },
  { id: "gaming", icon: Gamepad2 },
  { id: "pdf-documents", icon: FileText },
] as const satisfies ReadonlyArray<{ id: string; icon: LucideIcon }>;

export function MarketplaceCategoryNav({
  categories,
  activeCategory,
  totalCount,
  disabled,
  onSelect,
}: {
  categories: SkillMarketplaceCategoryDTO[];
  activeCategory: string;
  totalCount: number;
  disabled: boolean;
  onSelect: (category: string) => void;
}) {
  const t = useTranslations("SkillStore");
  const categoryCounts = new Map(
    categories.map((category) => [category.category, category.count]),
  );
  return (
    <nav
      aria-label={t("marketplaceCategoriesLabel")}
      className="shrink-0 border-b border-gray-200 bg-gray-50/60 p-2 dark:border-border dark:bg-muted/10 md:w-52 md:border-r md:border-b-0 md:p-3"
    >
      <div className="flex gap-1 overflow-x-auto custom-scrollbar md:h-full md:flex-col md:overflow-y-auto">
        <MarketplaceCategoryButton
          active={!activeCategory}
          count={totalCount}
          disabled={disabled}
          label={t("allCategories")}
          icon={Tags}
          onClick={() => onSelect("")}
        />
        {MARKETPLACE_CATEGORIES.map((category) => {
          const count = categoryCounts.get(category.id);
          if (count === undefined) return null;
          return (
            <MarketplaceCategoryButton
              key={category.id}
              active={activeCategory === category.id}
              count={count}
              disabled={disabled}
              label={t(`marketplaceCategories.${category.id}`)}
              icon={category.icon}
              onClick={() => onSelect(category.id)}
            />
          );
        })}
      </div>
    </nav>
  );
}

function MarketplaceCategoryButton({
  active,
  count,
  disabled,
  label,
  icon,
  onClick,
}: {
  active: boolean;
  count: number;
  disabled: boolean;
  label: string;
  icon: LucideIcon;
  onClick: () => void;
}) {
  const Icon = icon;
  return (
    <button
      type="button"
      aria-pressed={active}
      disabled={disabled}
      onClick={onClick}
      className={`flex shrink-0 items-center gap-2 rounded-lg px-3 py-2 text-left text-xs transition-colors disabled:opacity-60 md:w-full ${
        active
          ? "bg-white font-semibold text-cyan-700 shadow-sm ring-1 ring-gray-200 dark:bg-card dark:text-cyan-300 dark:ring-border"
          : "text-gray-600 hover:bg-white/80 hover:text-gray-900 dark:text-muted-foreground dark:hover:bg-card/70 dark:hover:text-foreground"
      }`}
    >
      <Icon size={14} aria-hidden="true" className="shrink-0" />
      <span className="whitespace-nowrap md:min-w-0 md:flex-1 md:truncate">
        {label}
      </span>
      <span className="rounded-full bg-gray-100 px-1.5 py-0.5 text-[9px] font-normal tabular-nums text-gray-500 dark:bg-muted">
        {formatCount(count)}
      </span>
    </button>
  );
}

export function MarketplaceSkillCard({
  item,
  active,
  onOpen,
}: {
  item: SkillMarketplaceSummaryDTO;
  active: boolean;
  onOpen: (button: HTMLButtonElement) => void;
}) {
  const t = useTranslations("SkillStore");
  return (
    <button
      type="button"
      aria-current={active ? "true" : undefined}
      onClick={(event) => onOpen(event.currentTarget)}
      className={`rounded-xl border bg-white p-4 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500 dark:bg-card ${
        active
          ? "border-cyan-500 bg-cyan-50/30 dark:border-cyan-700 dark:bg-cyan-950/10"
          : "border-gray-200 hover:border-cyan-300 hover:bg-cyan-50/30 dark:border-border dark:hover:border-cyan-900 dark:hover:bg-cyan-950/10"
      }`}
      style={{ contentVisibility: "auto", containIntrinsicSize: "auto 148px" }}
    >
      <div className="flex items-start gap-3">
        <SkillIcon icon={item.icon} label={item.name} />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-1.5">
            <span className="truncate text-sm font-semibold">{item.name}</span>
            {item.official ? (
              <span className="inline-flex items-center gap-1 rounded-full bg-blue-50 px-1.5 py-0.5 text-[9px] text-blue-700 dark:bg-blue-950/40 dark:text-blue-200">
                <ShieldCheck size={10} aria-hidden="true" />
                {t("official")}
              </span>
            ) : null}
          </div>
          <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">
            {item.description || item.identifier}
          </p>
          <div className="mt-3 flex flex-wrap gap-2 text-[10px] text-muted-foreground">
            <span>{t("installs", { count: item.installCount })}</span>
            {item.rating > 0 ? (
              <span className="inline-flex items-center gap-0.5">
                <Star size={10} aria-hidden="true" /> {item.rating.toFixed(1)}
              </span>
            ) : null}
          </div>
        </div>
      </div>
    </button>
  );
}

export function CuratedSkillCard({
  item,
  active,
  onOpen,
}: {
  item: SkillCatalogSummaryDTO;
  active: boolean;
  onOpen: (button: HTMLButtonElement) => void;
}) {
  const t = useTranslations("SkillStore");
  return (
    <button
      type="button"
      aria-current={active ? "true" : undefined}
      onClick={(event) => onOpen(event.currentTarget)}
      className={`rounded-xl border bg-white p-4 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500 dark:bg-card ${
        active
          ? "border-cyan-500 bg-cyan-50/30 dark:border-cyan-700 dark:bg-cyan-950/10"
          : "border-gray-200 hover:border-cyan-300 hover:bg-cyan-50/30 dark:border-border dark:hover:border-cyan-900 dark:hover:bg-cyan-950/10"
      }`}
    >
      <div className="flex items-start gap-3">
        <SkillIcon label={item.name} />
        <div className="min-w-0 flex-1">
          <div className="flex items-center justify-between gap-2">
            <span className="truncate text-sm font-semibold">{item.name}</span>
            <StatusPill value={t("curated")} />
          </div>
          <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">
            {item.repository}/{item.path}
          </p>
        </div>
      </div>
    </button>
  );
}

function formatCount(count: number): string {
  if (count < 1_000) return String(count);
  if (count < 1_000_000) return `${(count / 1_000).toFixed(1)}k`;
  return `${(count / 1_000_000).toFixed(1)}m`;
}
