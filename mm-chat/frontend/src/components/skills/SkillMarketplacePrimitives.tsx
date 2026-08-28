"use client";

import type { ReactNode } from "react";
import { Folder, PackageOpen, ShieldCheck, Star, Tags } from "lucide-react";
import Image from "next/image";
import { useTranslations } from "next-intl";
import { useState } from "react";

import type {
  SkillCatalogSummaryDTO,
  SkillMarketplaceCategoryDTO,
  SkillMarketplaceSummaryDTO,
} from "@/services/api/client";

import { StatusPill } from "./SkillStorePrimitives";

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
          icon={<Tags size={14} aria-hidden="true" />}
          onClick={() => onSelect("")}
        />
        {categories.map((item) => (
          <MarketplaceCategoryButton
            key={item.category}
            active={activeCategory === item.category}
            count={item.count}
            disabled={disabled}
            label={item.category}
            icon={<Folder size={14} aria-hidden="true" />}
            onClick={() => onSelect(item.category)}
          />
        ))}
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
  icon: ReactNode;
  onClick: () => void;
}) {
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
      <span className="shrink-0">{icon}</span>
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
            <span>v{item.version}</span>
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

function SkillIcon({ icon, label }: { icon?: string; label: string }) {
  const [failedImageUrl, setFailedImageUrl] = useState("");
  const imageUrl = getSkillIconImageURL(icon);
  const shortText =
    icon && !imageUrl && Array.from(icon).length <= 4 ? icon : "";
  const fallback = shortText || label.trim().slice(0, 1).toUpperCase();

  return (
    <span
      aria-hidden="true"
      className="relative flex h-11 w-11 shrink-0 items-center justify-center overflow-hidden rounded-xl bg-cyan-50 text-sm font-bold text-cyan-700 dark:bg-cyan-950/30 dark:text-cyan-300"
    >
      <span className="absolute inset-0 flex items-center justify-center">
        {fallback || <PackageOpen size={18} aria-hidden="true" />}
      </span>
      {imageUrl && failedImageUrl !== imageUrl ? (
        <Image
          src={imageUrl}
          alt=""
          width={44}
          height={44}
          loading="lazy"
          referrerPolicy="no-referrer"
          onError={() => setFailedImageUrl(imageUrl)}
          className="relative z-10 h-full w-full object-cover"
        />
      ) : null}
    </span>
  );
}

export function getSkillIconImageURL(icon?: string): string {
  if (!icon) return "";
  try {
    const parsed = new URL(icon);
    if (
      parsed.protocol !== "https:" ||
      parsed.hostname !== "github.com" ||
      parsed.port ||
      parsed.username ||
      parsed.password ||
      parsed.search ||
      parsed.hash ||
      !/^\/[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})\.png$/.test(parsed.pathname)
    ) {
      return "";
    }
    return parsed.toString();
  } catch {
    return "";
  }
}

function formatCount(count: number): string {
  if (count < 1_000) return String(count);
  if (count < 1_000_000) return `${(count / 1_000).toFixed(1)}k`;
  return `${(count / 1_000_000).toFixed(1)}m`;
}
