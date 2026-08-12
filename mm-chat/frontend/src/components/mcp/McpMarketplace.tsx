"use client";

import {
  FormEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  AlertTriangle,
  BarChart3,
  Box,
  Briefcase,
  CheckCircle2,
  CloudSun,
  Code2,
  Coffee,
  ExternalLink,
  Gamepad2,
  GraduationCap,
  Hammer,
  HeartPulse,
  ImageIcon,
  ListChecks,
  Loader2,
  MessagesSquare,
  Newspaper,
  Plane,
  Search,
  ShieldCheck,
  Star,
  Wrench,
  X,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { useTranslations } from "next-intl";

import type {
  McpMarketplaceCompatibility,
  McpMarketplaceCategory,
  McpMarketplaceInstallResult,
  McpMarketplaceItem,
  McpMarketplaceItemDetail,
} from "@/lib/mcp/types";
import { ApiClientError, createNeoChatApiClient } from "@/services/api/client";

import McpServerIcon from "./McpServerIcon";

interface McpMarketplaceProps {
  conversationId?: string;
  enabled: boolean;
  onInstalled: (result: McpMarketplaceInstallResult) => void;
}

export default function McpMarketplace({
  conversationId,
  enabled,
  onInstalled,
}: McpMarketplaceProps) {
  const t = useTranslations("Mcp");
  const client = useMemo(() => createNeoChatApiClient(), []);
  const [query, setQuery] = useState("");
  const [items, setItems] = useState<McpMarketplaceItem[]>([]);
  const [categories, setCategories] = useState<McpMarketplaceCategory[]>([]);
  const [activeCategory, setActiveCategory] = useState<
    MarketplaceCategoryId | ""
  >("");
  const [totalCount, setTotalCount] = useState(0);
  const [sourceUrl, setSourceUrl] = useState("");
  const [detail, setDetail] = useState<McpMarketplaceItemDetail | null>(null);
  const [loading, setLoading] = useState(false);
  const [detailLoading, setDetailLoading] = useState(false);
  const [installing, setInstalling] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const searchRequestRef = useRef(0);

  const search = useCallback(
    async (
      searchQuery: string,
      category: MarketplaceCategoryId | "",
      signal?: AbortSignal,
    ) => {
      if (!enabled) return;
      const searchRequest = ++searchRequestRef.current;
      const isCurrentRequest = () =>
        searchRequestRef.current === searchRequest && !signal?.aborted;
      setLoading(true);
      setError("");
      try {
        const result = await client.mcp.searchMarketplace({
          query: searchQuery,
          category: category || undefined,
          page: 1,
          pageSize: 20,
          signal,
        });
        if (!isCurrentRequest()) return;
        setItems(result.items);
        setCategories(result.categories);
        setTotalCount(result.totalCount);
        setSourceUrl(result.sourceUrl);
        setError("");
      } catch (searchError) {
        if (!isCurrentRequest()) return;
        setItems([]);
        setTotalCount(0);
        setError(marketplaceError(searchError, t));
      } finally {
        if (isCurrentRequest()) setLoading(false);
      }
    },
    [client.mcp, enabled, t],
  );

  useEffect(() => {
    const controller = new AbortController();
    void search("", "", controller.signal);
    return () => controller.abort();
  }, [search]);

  const submitSearch = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setDetail(null);
    setNotice("");
    void search(query.trim(), activeCategory);
  };

  const selectCategory = useCallback(
    (category: MarketplaceCategoryId | "") => {
      setActiveCategory(category);
      setDetail(null);
      setNotice("");
      void search(query.trim(), category);
    },
    [query, search],
  );

  const categoryCounts = useMemo(
    () => new Map(categories.map((item) => [item.category, item.count])),
    [categories],
  );
  const allCategoryCount = useMemo(() => {
    const categorized = categories.reduce(
      (total, item) => total + item.count,
      0,
    );
    return Math.max(categorized, activeCategory ? 0 : totalCount);
  }, [activeCategory, categories, totalCount]);

  const openDetail = useCallback(
    async (item: McpMarketplaceItem) => {
      setDetailLoading(true);
      setError("");
      setNotice("");
      try {
        const next = await client.mcp.getMarketplaceItem({
          identifier: item.identifier,
        });
        setDetail(next);
      } catch (detailError) {
        setError(marketplaceError(detailError, t));
      } finally {
        setDetailLoading(false);
      }
    },
    [client.mcp, t],
  );

  const install = useCallback(async () => {
    if (!detail || installing) return;
    setInstalling(true);
    setError("");
    setNotice("");
    try {
      const selection = conversationId
        ? await client.mcp.getConversationSelection(conversationId)
        : null;
      const result = await client.mcp.installMarketplaceItem({
        identifier: detail.identifier,
        version: detail.version,
        ...(conversationId ? { conversationId } : {}),
        selectionRevision: selection?.revision ?? 0,
        enableForConversation: Boolean(conversationId),
      });
      setNotice(
        result.validationErrorCode
          ? t("marketplaceInstalledNeedsAttention")
          : result.enabledForConversation
            ? t("marketplaceInstalledAndEnabled")
            : t("marketplaceInstalled"),
      );
      onInstalled(result);
    } catch (installError) {
      setError(marketplaceError(installError, t));
    } finally {
      setInstalling(false);
    }
  }, [client.mcp, conversationId, detail, installing, onInstalled, t]);

  if (!enabled) {
    return (
      <div className="flex h-full items-center justify-center p-8 text-sm text-gray-500 dark:text-muted-foreground">
        {t("serverModeOnly")}
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <form
        onSubmit={submitSearch}
        className="flex shrink-0 gap-2 border-b border-gray-200 p-4 dark:border-border"
      >
        <label className="relative min-w-0 flex-1">
          <Search
            size={15}
            className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
            aria-hidden="true"
          />
          <span className="sr-only">{t("marketplaceSearch")}</span>
          <input
            type="search"
            value={query}
            maxLength={200}
            onChange={(event) => setQuery(event.target.value)}
            placeholder={t("marketplaceSearchPlaceholder")}
            className="h-10 w-full rounded-lg border border-gray-300 bg-white pl-9 pr-3 text-sm outline-none transition-colors focus:border-cyan-500 dark:border-border dark:bg-background"
          />
        </label>
        <button
          type="submit"
          disabled={loading}
          className="inline-flex h-10 items-center gap-2 rounded-lg bg-cyan-600 px-4 text-sm font-medium text-white disabled:opacity-50"
        >
          {loading ? <Loader2 size={15} className="animate-spin" /> : null}
          {t("marketplaceSearchAction")}
        </button>
      </form>

      {error ? (
        <div
          role="alert"
          className="mx-4 mt-3 flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-200"
        >
          <AlertTriangle size={14} className="mt-0.5 shrink-0" />
          <span>{error}</span>
        </div>
      ) : null}

      {notice ? (
        <div
          role="status"
          className="mx-4 mt-3 flex items-start gap-2 rounded-lg border border-emerald-200 bg-emerald-50 px-3 py-2 text-xs text-emerald-700 dark:border-emerald-900/60 dark:bg-emerald-950/30 dark:text-emerald-200"
        >
          <CheckCircle2 size={14} className="mt-0.5 shrink-0" />
          <span>{notice}</span>
        </div>
      ) : null}

      <div className="flex min-h-0 flex-1 flex-col md:flex-row">
        <MarketplaceCategoryNav
          activeCategory={activeCategory}
          counts={categoryCounts}
          totalCount={allCategoryCount}
          disabled={loading}
          onSelect={selectCategory}
        />
        <div className="min-h-0 flex-1 overflow-y-auto p-4 custom-scrollbar">
          {loading && items.length === 0 ? (
            <div className="flex items-center justify-center gap-2 py-16 text-sm text-gray-500">
              <Loader2 size={16} className="animate-spin" />
              {t("marketplaceLoading")}
            </div>
          ) : items.length === 0 && !error ? (
            <div className="py-16 text-center text-sm text-gray-500">
              {t("marketplaceEmpty")}
            </div>
          ) : (
            <>
              <div className="mb-3 flex items-center justify-between text-xs text-gray-500 dark:text-muted-foreground">
                <span>{t("marketplaceResults", { count: totalCount })}</span>
                {sourceUrl ? (
                  <a
                    href={sourceUrl}
                    target="_blank"
                    rel="noreferrer noopener"
                    className="inline-flex items-center gap-1 hover:text-cyan-600"
                  >
                    LobeHub <ExternalLink size={11} />
                  </a>
                ) : null}
              </div>
              <div className="grid gap-3 md:grid-cols-2">
                {items.map((item) => (
                  <button
                    key={item.identifier}
                    type="button"
                    disabled={detailLoading}
                    onClick={() => void openDetail(item)}
                    className="rounded-xl border border-gray-200 bg-white p-4 text-left transition-colors hover:border-cyan-300 hover:bg-cyan-50/30 disabled:opacity-60 dark:border-border dark:bg-card dark:hover:border-cyan-900 dark:hover:bg-cyan-950/10"
                  >
                    <div className="flex items-start gap-3">
                      <McpServerIcon icon={item.icon} />
                      <div className="min-w-0 flex-1">
                        <div className="flex flex-wrap items-center gap-1.5">
                          <span className="truncate text-sm font-semibold text-gray-800 dark:text-foreground">
                            {item.name}
                          </span>
                          {item.official ? (
                            <span className="inline-flex items-center gap-1 rounded-full bg-blue-50 px-1.5 py-0.5 text-[9px] text-blue-700 dark:bg-blue-950/40 dark:text-blue-200">
                              <ShieldCheck size={10} />{" "}
                              {t("marketplaceOfficial")}
                            </span>
                          ) : null}
                        </div>
                        <p className="mt-1 line-clamp-2 text-xs text-gray-500 dark:text-muted-foreground">
                          {item.description || t("marketplaceNoDescription")}
                        </p>
                        <div className="mt-3 flex flex-wrap gap-2 text-[10px] text-gray-500">
                          <span>
                            {t("marketplaceToolCount", {
                              count: item.toolCount,
                            })}
                          </span>
                          {item.connectionType ? (
                            <span>{item.connectionType}</span>
                          ) : null}
                          {item.stars > 0 ? (
                            <span className="inline-flex items-center gap-0.5">
                              <Star size={10} /> {item.stars}
                            </span>
                          ) : null}
                        </div>
                      </div>
                    </div>
                  </button>
                ))}
              </div>
            </>
          )}
        </div>
      </div>

      {detailLoading ? (
        <div className="absolute inset-0 flex items-center justify-center bg-white/70 dark:bg-background/70">
          <Loader2 size={22} className="animate-spin text-cyan-600" />
        </div>
      ) : null}

      {detail ? (
        <MarketplaceDetail
          detail={detail}
          installing={installing}
          onClose={() => setDetail(null)}
          onInstall={() => void install()}
        />
      ) : null}
    </div>
  );
}

const MARKETPLACE_CATEGORIES = [
  { id: "developer", icon: Code2 },
  { id: "productivity", icon: ListChecks },
  { id: "tools", icon: Hammer },
  { id: "web-search", icon: Search },
  { id: "media-generate", icon: ImageIcon },
  { id: "business", icon: Briefcase },
  { id: "education-science", icon: GraduationCap },
  { id: "finance-stocks", icon: BarChart3 },
  { id: "news", icon: Newspaper },
  { id: "social", icon: MessagesSquare },
  { id: "gaming-entertainment", icon: Gamepad2 },
  { id: "lifestyle", icon: Coffee },
  { id: "health-wellness", icon: HeartPulse },
  { id: "travel-transport", icon: Plane },
  { id: "weather", icon: CloudSun },
] as const satisfies ReadonlyArray<{ id: string; icon: LucideIcon }>;

type MarketplaceCategoryId = (typeof MARKETPLACE_CATEGORIES)[number]["id"];

function MarketplaceCategoryNav({
  activeCategory,
  counts,
  totalCount,
  disabled,
  onSelect,
}: {
  activeCategory: MarketplaceCategoryId | "";
  counts: ReadonlyMap<string, number>;
  totalCount: number;
  disabled: boolean;
  onSelect: (category: MarketplaceCategoryId | "") => void;
}) {
  const t = useTranslations("Mcp");
  return (
    <nav
      aria-label={t("marketplaceCategoriesLabel")}
      className="shrink-0 border-b border-gray-200 bg-gray-50/60 p-2 dark:border-border dark:bg-muted/10 md:w-52 md:border-b-0 md:border-r md:p-3"
    >
      <div className="flex gap-1 overflow-x-auto custom-scrollbar md:h-full md:flex-col md:overflow-y-auto">
        <MarketplaceCategoryButton
          active={activeCategory === ""}
          count={totalCount}
          disabled={disabled}
          icon={Box}
          label={t("marketplaceAllCategories")}
          onClick={() => onSelect("")}
        />
        {MARKETPLACE_CATEGORIES.map((category) => (
          <MarketplaceCategoryButton
            key={category.id}
            active={activeCategory === category.id}
            count={counts.get(category.id) ?? 0}
            disabled={disabled}
            icon={category.icon}
            label={t(`marketplaceCategories.${category.id}`)}
            onClick={() => onSelect(category.id)}
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
  icon: Icon,
  label,
  onClick,
}: {
  active: boolean;
  count: number;
  disabled: boolean;
  icon: LucideIcon;
  label: string;
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
      <Icon size={14} className="shrink-0" aria-hidden="true" />
      <span className="whitespace-nowrap md:min-w-0 md:flex-1 md:truncate">
        {label}
      </span>
      <span className="rounded-full bg-gray-100 px-1.5 py-0.5 text-[9px] font-normal tabular-nums text-gray-500 dark:bg-muted">
        {formatMarketplaceCount(count)}
      </span>
    </button>
  );
}

function formatMarketplaceCount(count: number): string {
  if (count < 1_000) return String(count);
  if (count < 1_000_000) return `${(count / 1_000).toFixed(1)}k`;
  return `${(count / 1_000_000).toFixed(1)}m`;
}

function MarketplaceDetail({
  detail,
  installing,
  onClose,
  onInstall,
}: {
  detail: McpMarketplaceItemDetail;
  installing: boolean;
  onClose: () => void;
  onInstall: () => void;
}) {
  const t = useTranslations("Mcp");
  const installable = detail.deployments.some(
    (deployment) => deployment.compatibility === "installable",
  );
  return (
    <div className="absolute inset-0 z-10 flex items-end justify-end bg-black/20 p-3 backdrop-blur-[1px] md:p-5">
      <section className="flex max-h-full w-full max-w-xl flex-col overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-2xl dark:border-border dark:bg-card">
        <header className="flex items-start justify-between gap-3 border-b border-gray-200 px-5 py-4 dark:border-border">
          <div className="flex min-w-0 items-center gap-3">
            <McpServerIcon icon={detail.icon} large />
            <div className="min-w-0">
              <h2 className="truncate text-base font-bold text-gray-800 dark:text-foreground">
                {detail.name}
              </h2>
              <p className="mt-1 truncate text-xs text-gray-500">
                {detail.identifier} · v{detail.version}
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label={t("marketplaceCloseDetail")}
            className="rounded-full p-2 text-gray-500 hover:bg-gray-100 dark:hover:bg-accent"
          >
            <X size={16} />
          </button>
        </header>
        <div className="min-h-0 flex-1 space-y-5 overflow-y-auto px-5 py-4 custom-scrollbar">
          <p className="text-sm leading-6 text-gray-600 dark:text-muted-foreground">
            {detail.summary ||
              detail.description ||
              t("marketplaceNoDescription")}
          </p>
          <div className="flex flex-wrap gap-2 text-[11px]">
            <span className="rounded-full bg-gray-100 px-2 py-1 dark:bg-muted">
              {t("marketplaceSource")}: LobeHub
            </span>
            {detail.author ? (
              <span className="rounded-full bg-gray-100 px-2 py-1 dark:bg-muted">
                {detail.author}
              </span>
            ) : null}
            {detail.validated ? (
              <span className="rounded-full bg-emerald-50 px-2 py-1 text-emerald-700 dark:bg-emerald-950/30 dark:text-emerald-200">
                {t("marketplaceValidated")}
              </span>
            ) : null}
          </div>

          <section>
            <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">
              {t("marketplaceConnections")}
            </h3>
            <div className="space-y-2">
              {detail.deployments.map((deployment, index) => (
                <div
                  key={`${deployment.connectionType}:${deployment.installationMethod}:${index}`}
                  className="rounded-lg border border-gray-200 px-3 py-2 dark:border-border"
                >
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <span className="text-xs font-medium text-gray-700 dark:text-foreground/85">
                      {deployment.connectionType.toUpperCase()} ·{" "}
                      {deployment.installationMethod}
                    </span>
                    <CompatibilityBadge
                      compatibility={deployment.compatibility}
                    />
                  </div>
                  <p className="mt-1 text-[11px] text-gray-500">
                    {deployment.compatibilityReason}
                  </p>
                </div>
              ))}
            </div>
          </section>

          <section>
            <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">
              {t("marketplaceToolsPreview", { count: detail.tools.length })}
            </h3>
            {detail.tools.length > 0 ? (
              <div className="space-y-1.5">
                {detail.tools.map((tool) => (
                  <div
                    key={tool.name}
                    className="rounded-lg bg-gray-50 px-3 py-2 dark:bg-muted/30"
                  >
                    <div className="flex items-center gap-2 text-xs font-medium text-gray-700 dark:text-foreground/85">
                      <Wrench size={12} /> {tool.name}
                    </div>
                    {tool.description ? (
                      <p className="mt-1 text-[11px] text-gray-500">
                        {tool.description}
                      </p>
                    ) : null}
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-xs text-gray-500">
                {t("marketplaceToolsDiscoveredAfterInstall")}
              </p>
            )}
          </section>

          <div className="flex flex-wrap gap-3 text-xs">
            {detail.homepage ? (
              <a
                href={detail.homepage}
                target="_blank"
                rel="noreferrer noopener"
                className="inline-flex items-center gap-1 text-cyan-700 hover:underline dark:text-cyan-300"
              >
                {t("marketplaceHomepage")} <ExternalLink size={11} />
              </a>
            ) : null}
            {detail.repositoryUrl ? (
              <a
                href={detail.repositoryUrl}
                target="_blank"
                rel="noreferrer noopener"
                className="inline-flex items-center gap-1 text-cyan-700 hover:underline dark:text-cyan-300"
              >
                {t("marketplaceRepository")} <ExternalLink size={11} />
              </a>
            ) : null}
          </div>
          <div className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-[11px] leading-5 text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-100">
            {t("marketplaceTrustNotice")}
          </div>
        </div>
        <footer className="flex items-center justify-between gap-3 border-t border-gray-200 px-5 py-4 dark:border-border">
          <span className="text-[11px] text-gray-500">
            {installable
              ? t("marketplaceBackendValidation")
              : t("marketplaceNotInstallable")}
          </span>
          <button
            type="button"
            disabled={!installable || installing}
            onClick={onInstall}
            className="inline-flex items-center gap-2 rounded-lg bg-cyan-600 px-4 py-2 text-sm font-medium text-white disabled:cursor-not-allowed disabled:opacity-45"
          >
            {installing ? (
              <Loader2 size={15} className="animate-spin" />
            ) : (
              <Box size={15} />
            )}
            {t("marketplaceInstallAndEnable")}
          </button>
        </footer>
      </section>
    </div>
  );
}

function CompatibilityBadge({
  compatibility,
}: {
  compatibility: McpMarketplaceCompatibility;
}) {
  const t = useTranslations("Mcp");
  const classes =
    compatibility === "installable"
      ? "bg-emerald-100 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-200"
      : compatibility === "requires_runner"
        ? "bg-blue-100 text-blue-700 dark:bg-blue-950/40 dark:text-blue-200"
        : compatibility === "needs_configuration"
          ? "bg-amber-100 text-amber-700 dark:bg-amber-950/40 dark:text-amber-200"
          : "bg-gray-100 text-gray-600 dark:bg-muted dark:text-muted-foreground";
  return (
    <span className={`rounded-full px-2 py-0.5 text-[9px] ${classes}`}>
      {t(`marketplaceCompatibility.${compatibility}`)}
    </span>
  );
}

function marketplaceError(
  error: unknown,
  t: (key: MarketplaceErrorKey) => string,
): string {
  if (error instanceof ApiClientError) {
    if (error.code === "MCP_MARKETPLACE_DISABLED")
      return t("marketplaceDisabled");
    if (error.code === "MCP_MARKETPLACE_UNAVAILABLE")
      return t("marketplaceUnavailable");
    if (error.code === "MCP_MARKETPLACE_CHANGED")
      return t("marketplaceChanged");
    if (error.code === "MCP_MARKETPLACE_INCOMPATIBLE")
      return t("marketplaceNotInstallable");
    return error.message || t("marketplaceFailed");
  }
  return error instanceof Error && error.message
    ? error.message
    : t("marketplaceFailed");
}

type MarketplaceErrorKey =
  | "marketplaceDisabled"
  | "marketplaceUnavailable"
  | "marketplaceChanged"
  | "marketplaceNotInstallable"
  | "marketplaceFailed";
