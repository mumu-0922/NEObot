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
  McpMarketplaceDeployment,
} from "@/lib/mcp/types";
import {
  appendUniqueMarketplaceItems,
  hasNextMarketplacePage,
} from "@/lib/mcp/marketplacePagination";
import { ApiClientError, createNeoChatApiClient } from "@/services/api/client";

import McpServerIcon from "./McpServerIcon";

interface McpMarketplaceProps {
  conversationId?: string;
  enabled: boolean;
  initialQuery?: string;
  onInstalled: (result: McpMarketplaceInstallResult) => void;
}

interface MarketplaceDetailFailure {
  item: McpMarketplaceItem;
  message: string;
}

interface CustomRemoteDraft {
  enabled: boolean;
  endpointUrl: string;
  authType: "none" | "header" | "oauth";
  headerName: string;
  credential: string;
  clientId: string;
}

const emptyCustomRemoteDraft: CustomRemoteDraft = {
  enabled: false,
  endpointUrl: "",
  authType: "header",
  headerName: "Authorization",
  credential: "",
  clientId: "",
};

const MARKETPLACE_PAGE_SIZE = 20;

export default function McpMarketplace({
  conversationId,
  enabled,
  initialQuery = "",
  onInstalled,
}: McpMarketplaceProps) {
  const t = useTranslations("Mcp");
  const client = useMemo(() => createNeoChatApiClient(), []);
  const [query, setQuery] = useState(initialQuery);
  const [items, setItems] = useState<McpMarketplaceItem[]>([]);
  const [categories, setCategories] = useState<McpMarketplaceCategory[]>([]);
  const [activeCategory, setActiveCategory] = useState<
    MarketplaceCategoryId | ""
  >("");
  const [totalCount, setTotalCount] = useState(0);
  const [page, setPage] = useState(0);
  const [totalPages, setTotalPages] = useState(0);
  const [sourceUrl, setSourceUrl] = useState("");
  const [detail, setDetail] = useState<McpMarketplaceItemDetail | null>(null);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [detailLoading, setDetailLoading] = useState(false);
  const [installing, setInstalling] = useState(false);
  const [searchError, setSearchError] = useState("");
  const [detailError, setDetailError] =
    useState<MarketplaceDetailFailure | null>(null);
  const [installError, setInstallError] = useState("");
  const [selectedDeploymentHash, setSelectedDeploymentHash] = useState("");
  const [installSecrets, setInstallSecrets] = useState<Record<string, string>>(
    {},
  );
  const [customRemote, setCustomRemote] = useState<CustomRemoteDraft>(
    emptyCustomRemoteDraft,
  );
  const [nextPageError, setNextPageError] = useState("");
  const [notice, setNotice] = useState("");
  const searchRequestRef = useRef(0);
  const detailRequestRef = useRef(0);
  const activeRequestControllerRef = useRef<AbortController | null>(null);
  const detailRequestControllerRef = useRef<AbortController | null>(null);
  const activeSearchRef = useRef<{
    query: string;
    category: MarketplaceCategoryId | "";
  }>({ query: initialQuery, category: "" });
  const initialLoadingRef = useRef(false);
  const loadingMoreRef = useRef(false);
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const loadMoreSentinelRef = useRef<HTMLDivElement>(null);

  const search = useCallback(
    async (searchQuery: string, category: MarketplaceCategoryId | "") => {
      if (!enabled) return;
      activeRequestControllerRef.current?.abort();
      detailRequestControllerRef.current?.abort();
      detailRequestControllerRef.current = null;
      detailRequestRef.current += 1;
      const controller = new AbortController();
      activeRequestControllerRef.current = controller;
      const searchRequest = ++searchRequestRef.current;
      const isCurrentRequest = () =>
        searchRequestRef.current === searchRequest &&
        !controller.signal.aborted;
      activeSearchRef.current = { query: searchQuery, category };
      initialLoadingRef.current = true;
      loadingMoreRef.current = false;
      scrollContainerRef.current?.scrollTo({ top: 0 });
      setLoading(true);
      setLoadingMore(false);
      setDetailLoading(false);
      setSearchError("");
      setDetailError(null);
      setInstallError("");
      setNextPageError("");
      setDetail(null);
      setItems([]);
      setCategories([]);
      setTotalCount(0);
      setPage(0);
      setTotalPages(0);
      setSourceUrl("");
      try {
        const result = await client.mcp.searchMarketplace({
          query: searchQuery,
          category: category || undefined,
          page: 1,
          pageSize: MARKETPLACE_PAGE_SIZE,
          signal: controller.signal,
        });
        if (!isCurrentRequest()) return;
        setItems(result.items);
        setCategories(result.categories);
        setTotalCount(result.totalCount);
        setPage(result.page);
        setTotalPages(result.totalPages);
        setSourceUrl(result.sourceUrl);
        setSearchError("");
      } catch (searchError) {
        if (!isCurrentRequest()) return;
        setItems([]);
        setCategories([]);
        setTotalCount(0);
        setPage(0);
        setTotalPages(0);
        setSourceUrl("");
        setSearchError(marketplaceError(searchError, t));
      } finally {
        if (isCurrentRequest()) {
          initialLoadingRef.current = false;
          setLoading(false);
          if (activeRequestControllerRef.current === controller) {
            activeRequestControllerRef.current = null;
          }
        }
      }
    },
    [client.mcp, enabled, t],
  );

  useEffect(() => {
    setQuery(initialQuery);
    void search(initialQuery.trim(), "");
    return () => {
      searchRequestRef.current += 1;
      detailRequestRef.current += 1;
      activeRequestControllerRef.current?.abort();
      activeRequestControllerRef.current = null;
      detailRequestControllerRef.current?.abort();
      detailRequestControllerRef.current = null;
      initialLoadingRef.current = false;
      loadingMoreRef.current = false;
    };
  }, [initialQuery, search]);

  const hasMore = hasNextMarketplacePage(page, totalPages);

  const loadMore = useCallback(async () => {
    if (
      !enabled ||
      initialLoadingRef.current ||
      loadingMoreRef.current ||
      !hasNextMarketplacePage(page, totalPages)
    ) {
      return;
    }
    loadingMoreRef.current = true;
    setLoadingMore(true);
    setNextPageError("");
    const searchRequest = searchRequestRef.current;
    const nextPage = page + 1;
    const controller = new AbortController();
    activeRequestControllerRef.current = controller;
    const isCurrentRequest = () =>
      searchRequestRef.current === searchRequest && !controller.signal.aborted;
    try {
      const result = await client.mcp.searchMarketplace({
        query: activeSearchRef.current.query,
        category: activeSearchRef.current.category || undefined,
        page: nextPage,
        pageSize: MARKETPLACE_PAGE_SIZE,
        signal: controller.signal,
      });
      if (!isCurrentRequest()) return;
      if (result.page !== nextPage) {
        throw new Error(t("marketplaceFailed"));
      }
      setItems((current) =>
        appendUniqueMarketplaceItems(current, result.items),
      );
      setTotalCount(result.totalCount);
      setPage(result.page);
      setTotalPages(result.totalPages);
      setSourceUrl(result.sourceUrl);
    } catch (nextPageFailure) {
      if (!isCurrentRequest()) return;
      setNextPageError(marketplaceError(nextPageFailure, t));
    } finally {
      if (isCurrentRequest()) {
        loadingMoreRef.current = false;
        setLoadingMore(false);
        if (activeRequestControllerRef.current === controller) {
          activeRequestControllerRef.current = null;
        }
      }
    }
  }, [client.mcp, enabled, page, t, totalPages]);

  useEffect(() => {
    const root = scrollContainerRef.current;
    const sentinel = loadMoreSentinelRef.current;
    if (
      !root ||
      !sentinel ||
      !hasMore ||
      nextPageError ||
      typeof IntersectionObserver === "undefined"
    ) {
      return;
    }
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) void loadMore();
      },
      { root, rootMargin: "0px 0px 300px 0px" },
    );
    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [hasMore, loadMore, nextPageError]);

  const submitSearch = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setNotice("");
    void search(query.trim(), activeCategory);
  };

  const selectCategory = useCallback(
    (category: MarketplaceCategoryId | "") => {
      setActiveCategory(category);
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
      detailRequestControllerRef.current?.abort();
      const controller = new AbortController();
      detailRequestControllerRef.current = controller;
      const detailRequest = ++detailRequestRef.current;
      const isCurrentRequest = () =>
        detailRequestRef.current === detailRequest &&
        !controller.signal.aborted;
      setDetailLoading(true);
      setDetailError(null);
      setInstallError("");
      setNotice("");
      try {
        const next = await client.mcp.getMarketplaceItem({
          identifier: item.identifier,
          signal: controller.signal,
        });
        if (!isCurrentRequest()) return;
        setDetail(next);
        const preferred = preferredMarketplaceDeployment(next.deployments);
        setSelectedDeploymentHash(preferred?.hash ?? "");
        setInstallSecrets(
          Object.fromEntries(
            (preferred?.secretFields ?? []).map((field) => [field, ""]),
          ),
        );
        setCustomRemote(emptyCustomRemoteDraft);
      } catch {
        if (!isCurrentRequest()) return;
        setDetailError({
          item,
          message: t("marketplaceDetailFailed", { name: item.name }),
        });
      } finally {
        if (isCurrentRequest()) {
          setDetailLoading(false);
          if (detailRequestControllerRef.current === controller) {
            detailRequestControllerRef.current = null;
          }
        }
      }
    },
    [client.mcp, t],
  );

  const install = useCallback(async () => {
    if (!detail || !detail.canInstall || installing) return;
    const deployment = detail.deployments.find(
      (candidate) => candidate.hash === selectedDeploymentHash,
    );
    if (
      !customRemote.enabled &&
      (!deployment || !canInstallMarketplaceDeployment(deployment))
    ) {
      return;
    }
    setInstalling(true);
    setInstallError("");
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
        ...(customRemote.enabled
          ? {
              customEndpointUrl: customRemote.endpointUrl.trim(),
              customAuthType: customRemote.authType,
              ...(customRemote.authType === "header"
                ? {
                    customHeaderName: customRemote.headerName.trim(),
                    customHeaderPrefix:
                      customRemote.headerName.trim().toLowerCase() ===
                      "authorization"
                        ? "Bearer "
                        : "",
                    customCredential: customRemote.credential,
                  }
                : {}),
              ...(customRemote.authType === "oauth" &&
              customRemote.clientId.trim()
                ? { customClientId: customRemote.clientId.trim() }
                : {}),
            }
          : {
              deploymentHash: deployment?.hash,
              ...(deployment && deployment.secretFields.length > 0
                ? { secrets: installSecrets }
                : {}),
            }),
      });
      setNotice(
        result.validationErrorCode
          ? t("marketplaceInstalledNeedsAttention")
          : result.enabledForConversation
            ? t("marketplaceInstalledAndEnabled")
            : t("marketplaceInstalled"),
      );
      onInstalled(result);
      setInstallSecrets({});
      setCustomRemote(emptyCustomRemoteDraft);
      if (result.server.authType === "oauth" && !result.server.hasCredential) {
        const oauth = await client.mcp.startOAuth({
          serverRef: result.server.ref,
          conversationId,
          returnUrl: `${window.location.pathname}${window.location.search}${window.location.hash}`,
        });
        const authorizationUrl = validMarketplaceOAuthURL(
          oauth.authorizationUrl,
        );
        if (!authorizationUrl) {
          throw new ApiClientError(
            "INVALID_SERVER_RESPONSE",
            t("marketplaceFailed"),
          );
        }
        window.location.assign(authorizationUrl);
      }
    } catch (installError) {
      setInstallError(marketplaceError(installError, t));
    } finally {
      setInstalling(false);
    }
  }, [
    client.mcp,
    conversationId,
    customRemote,
    detail,
    installSecrets,
    installing,
    onInstalled,
    selectedDeploymentHash,
    t,
  ]);

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

      {searchError ? (
        <div
          role="alert"
          className="mx-4 mt-3 flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-200"
        >
          <AlertTriangle size={14} className="mt-0.5 shrink-0" />
          <span>{searchError}</span>
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
        <div
          ref={scrollContainerRef}
          className="min-h-0 flex-1 overflow-y-auto p-4 custom-scrollbar"
        >
          {loading && items.length === 0 ? (
            <div className="flex items-center justify-center gap-2 py-16 text-sm text-gray-500">
              <Loader2 size={16} className="animate-spin" />
              {t("marketplaceLoading")}
            </div>
          ) : items.length === 0 && !searchError ? (
            <div className="py-16 text-center text-sm text-gray-500">
              {t("marketplaceEmpty")}
            </div>
          ) : (
            <>
              <div className="mb-3 flex items-center justify-between text-xs text-gray-500 dark:text-muted-foreground">
                <span>
                  {t("marketplaceResults", {
                    loaded: items.length,
                    count: totalCount,
                  })}
                </span>
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
              {detailError ? (
                <div
                  role="alert"
                  className="mb-3 flex items-center justify-between gap-3 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-100"
                >
                  <span className="flex min-w-0 items-start gap-2">
                    <AlertTriangle size={14} className="mt-0.5 shrink-0" />
                    <span>{detailError.message}</span>
                  </span>
                  <button
                    type="button"
                    onClick={() => void openDetail(detailError.item)}
                    className="shrink-0 rounded-md border border-amber-300 bg-white/70 px-2.5 py-1 font-medium hover:bg-white dark:border-amber-800 dark:bg-amber-950/40 dark:hover:bg-amber-950/70"
                  >
                    {t("marketplaceRetryDetail")}
                  </button>
                </div>
              ) : null}
              <div className="grid gap-3 md:grid-cols-2">
                {items.map((item) => (
                  <button
                    key={item.identifier}
                    type="button"
                    disabled={detailLoading}
                    onClick={() => void openDetail(item)}
                    className="rounded-xl border border-gray-200 bg-white p-4 text-left transition-colors hover:border-cyan-300 hover:bg-cyan-50/30 disabled:opacity-60 dark:border-border dark:bg-card dark:hover:border-cyan-900 dark:hover:bg-cyan-950/10"
                    style={{
                      contentVisibility: "auto",
                      containIntrinsicSize: "auto 148px",
                    }}
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
              <div
                ref={loadMoreSentinelRef}
                className="flex min-h-16 flex-col items-center justify-center gap-2 py-4 text-center"
              >
                {hasMore ? (
                  <button
                    type="button"
                    disabled={loadingMore}
                    onClick={() => void loadMore()}
                    className="inline-flex items-center gap-2 rounded-lg border border-gray-200 bg-white px-4 py-2 text-xs font-medium text-gray-600 hover:border-cyan-300 hover:text-cyan-700 disabled:cursor-wait disabled:opacity-60 dark:border-border dark:bg-card dark:text-muted-foreground"
                  >
                    {loadingMore ? (
                      <Loader2 size={14} className="animate-spin" />
                    ) : null}
                    {loadingMore
                      ? t("marketplaceLoadingMore")
                      : nextPageError
                        ? t("marketplaceRetry")
                        : t("marketplaceLoadMore")}
                  </button>
                ) : items.length > 0 ? (
                  <span
                    aria-live="polite"
                    className="text-[11px] text-gray-400"
                  >
                    {t("marketplaceAllLoaded", { count: items.length })}
                  </span>
                ) : null}
                {nextPageError ? (
                  <p role="alert" className="text-[11px] text-red-600">
                    {nextPageError}
                  </p>
                ) : null}
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
          error={installError}
          installing={installing}
          selectedDeploymentHash={selectedDeploymentHash}
          secrets={installSecrets}
          customRemote={customRemote}
          onSelectDeployment={(deployment) => {
            setSelectedDeploymentHash(deployment.hash ?? "");
            setInstallSecrets(
              Object.fromEntries(
                deployment.secretFields.map((field) => [field, ""]),
              ),
            );
            setInstallError("");
          }}
          onSecretChange={(field, value) =>
            setInstallSecrets((current) => ({ ...current, [field]: value }))
          }
          onCustomRemoteChange={setCustomRemote}
          onClose={() => {
            setDetail(null);
            setInstallError("");
            setInstallSecrets({});
            setCustomRemote(emptyCustomRemoteDraft);
          }}
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
  error,
  installing,
  selectedDeploymentHash,
  secrets,
  customRemote,
  onClose,
  onInstall,
  onSelectDeployment,
  onSecretChange,
  onCustomRemoteChange,
}: {
  detail: McpMarketplaceItemDetail;
  error: string;
  installing: boolean;
  selectedDeploymentHash: string;
  secrets: Record<string, string>;
  customRemote: CustomRemoteDraft;
  onClose: () => void;
  onInstall: () => void;
  onSelectDeployment: (deployment: McpMarketplaceDeployment) => void;
  onSecretChange: (field: string, value: string) => void;
  onCustomRemoteChange: (draft: CustomRemoteDraft) => void;
}) {
  const t = useTranslations("Mcp");
  const selectedDeployment = detail.deployments.find(
    (deployment) => deployment.hash === selectedDeploymentHash,
  );
  const installable = canInstallMarketplaceDeployment(selectedDeployment);
  const secretsComplete = (selectedDeployment?.secretFields ?? []).every(
    (field) => Boolean(secrets[field]),
  );
  const customComplete =
    customRemote.endpointUrl.trim().startsWith("https://") &&
    (customRemote.authType !== "header" ||
      (Boolean(customRemote.headerName.trim()) &&
        Boolean(customRemote.credential)));
  const canSubmit = customRemote.enabled
    ? customComplete
    : installable && secretsComplete;
  const supportsCustomRemote = detail.deployments.some(
    (deployment) =>
      deployment.connectionType === "http" ||
      deployment.installMode === "header" ||
      deployment.installMode === "oauth" ||
      deployment.secretFields.length > 0,
  );
  const visibleDeployments = marketplaceVisibleDeployments(detail.deployments);
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
              {visibleDeployments.map((deployment, index) => {
                const selected =
                  Boolean(selectedDeploymentHash) &&
                  deployment.hash === selectedDeploymentHash;
                return (
                  <div
                    key={`${deployment.connectionType}:${deployment.installationMethod}:${index}`}
                    className={`overflow-hidden rounded-lg border ${selected ? "border-cyan-500 bg-cyan-50/40 dark:border-cyan-700 dark:bg-cyan-950/20" : "border-gray-200 dark:border-border"}`}
                  >
                    <button
                      type="button"
                      disabled={
                        !detail.canInstall ||
                        !canInstallMarketplaceDeployment(deployment)
                      }
                      onClick={() => onSelectDeployment(deployment)}
                      className="w-full px-3 py-2 text-left disabled:cursor-not-allowed disabled:opacity-65"
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
                    </button>
                    {detail.canInstall &&
                    selected &&
                    deployment.secretFields.length > 0 ? (
                      <div className="space-y-2 border-t border-cyan-200/80 px-3 py-3 dark:border-cyan-900/70">
                        <h4 className="text-[11px] font-semibold uppercase tracking-wide text-gray-500">
                          {t("marketplaceConfiguration")}
                        </h4>
                        {deployment.secretFields.map((field, fieldIndex) => (
                          <label key={field} className="block">
                            <span className="mb-1 block text-[11px] font-medium text-gray-600 dark:text-muted-foreground">
                              {field}
                            </span>
                            <input
                              type="password"
                              autoComplete="new-password"
                              autoFocus={fieldIndex === 0}
                              value={secrets[field] ?? ""}
                              onChange={(event) =>
                                onSecretChange(field, event.target.value)
                              }
                              placeholder={t("marketplaceSecretPlaceholder")}
                              className="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm outline-none focus:border-cyan-500 dark:border-border dark:bg-background"
                            />
                          </label>
                        ))}
                        <p className="text-[10px] leading-4 text-gray-500">
                          {t("marketplaceSecretNotice")}
                        </p>
                      </div>
                    ) : null}
                  </div>
                );
              })}
            </div>
          </section>

          {detail.canInstall && supportsCustomRemote ? (
            <section className="rounded-xl border border-dashed border-cyan-300 bg-cyan-50/30 p-3 dark:border-cyan-900 dark:bg-cyan-950/10">
              <label className="flex cursor-pointer items-start gap-2">
                <input
                  type="checkbox"
                  checked={customRemote.enabled}
                  onChange={(event) =>
                    onCustomRemoteChange({
                      ...customRemote,
                      enabled: event.target.checked,
                    })
                  }
                  className="mt-0.5"
                />
                <span>
                  <span className="block text-xs font-semibold text-gray-700 dark:text-foreground/85">
                    {t("marketplaceUseCustomEndpoint")}
                  </span>
                  <span className="mt-1 block text-[11px] leading-4 text-gray-500">
                    {t("marketplaceCustomEndpointNotice")}
                  </span>
                </span>
              </label>
              {customRemote.enabled ? (
                <div className="mt-3 space-y-2 border-t border-cyan-200/80 pt-3 dark:border-cyan-900/70">
                  <input
                    type="url"
                    autoFocus
                    value={customRemote.endpointUrl}
                    onChange={(event) =>
                      onCustomRemoteChange({
                        ...customRemote,
                        endpointUrl: event.target.value,
                      })
                    }
                    placeholder="https://your-relay.example.com/mcp"
                    className="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm outline-none focus:border-cyan-500 dark:border-border dark:bg-background"
                  />
                  <select
                    value={customRemote.authType}
                    onChange={(event) =>
                      onCustomRemoteChange({
                        ...customRemote,
                        authType: event.target
                          .value as CustomRemoteDraft["authType"],
                      })
                    }
                    className="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm dark:border-border dark:bg-background"
                  >
                    <option value="none">{t("authNone")}</option>
                    <option value="header">{t("authHeader")}</option>
                    <option value="oauth">OAuth</option>
                  </select>
                  {customRemote.authType === "header" ? (
                    <>
                      <input
                        value={customRemote.headerName}
                        onChange={(event) =>
                          onCustomRemoteChange({
                            ...customRemote,
                            headerName: event.target.value,
                          })
                        }
                        placeholder="Authorization"
                        className="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm dark:border-border dark:bg-background"
                      />
                      <input
                        type="password"
                        autoComplete="new-password"
                        value={customRemote.credential}
                        onChange={(event) =>
                          onCustomRemoteChange({
                            ...customRemote,
                            credential: event.target.value,
                          })
                        }
                        placeholder={t("credentialPlaceholder")}
                        className="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm dark:border-border dark:bg-background"
                      />
                    </>
                  ) : null}
                  {customRemote.authType === "oauth" ? (
                    <input
                      value={customRemote.clientId}
                      onChange={(event) =>
                        onCustomRemoteChange({
                          ...customRemote,
                          clientId: event.target.value,
                        })
                      }
                      placeholder={t("clientId")}
                      className="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm dark:border-border dark:bg-background"
                    />
                  ) : null}
                </div>
              ) : null}
            </section>
          ) : null}

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
          {error ? (
            <div
              role="alert"
              className="flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-200"
            >
              <AlertTriangle size={14} className="mt-0.5 shrink-0" />
              <span>{error}</span>
            </div>
          ) : null}
        </div>
        <footer className="flex items-center justify-between gap-3 border-t border-gray-200 px-5 py-4 dark:border-border">
          <span className="text-[11px] text-gray-500">
            {detail.installed
              ? t("marketplaceInstalledManageNotice")
              : customRemote.enabled && !customComplete
                ? t("marketplaceCompleteCustomEndpoint")
                : customRemote.enabled
                  ? t("marketplaceCustomEndpointValidation")
                  : installable && !secretsComplete
                    ? t("marketplaceCompleteConfiguration")
                    : installable
                      ? t("marketplaceBackendValidation")
                      : t("marketplaceNotInstallable")}
          </span>
          {detail.canInstall ? (
            <button
              type="button"
              disabled={detail.installed || !canSubmit || installing}
              onClick={onInstall}
              className="inline-flex items-center gap-2 rounded-lg bg-cyan-600 px-4 py-2 text-sm font-medium text-white disabled:cursor-not-allowed disabled:opacity-45"
            >
              {installing ? (
                <Loader2 size={15} className="animate-spin" />
              ) : (
                <Box size={15} />
              )}
              {detail.installed
                ? t("marketplaceAlreadyInstalled")
                : t("marketplaceInstallAndEnable")}
            </button>
          ) : null}
        </footer>
      </section>
    </div>
  );
}

function preferredMarketplaceDeployment(
  deployments: McpMarketplaceDeployment[],
): McpMarketplaceDeployment | undefined {
  return (
    deployments.find(
      (deployment) =>
        canInstallMarketplaceDeployment(deployment) && deployment.recommended,
    ) ?? deployments.find(canInstallMarketplaceDeployment)
  );
}

function marketplaceVisibleDeployments(
  deployments: McpMarketplaceDeployment[],
): McpMarketplaceDeployment[] {
  const installable = deployments.filter(canInstallMarketplaceDeployment);
  return installable.length > 0 ? installable : deployments;
}

function canInstallMarketplaceDeployment(
  deployment: McpMarketplaceDeployment | undefined,
): boolean {
  if (!deployment) return false;
  return (
    deployment.compatibility === "installable" ||
    (deployment.compatibility === "needs_configuration" &&
      (deployment.installMode === "header" ||
        deployment.installMode === "runner_env"))
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

function validMarketplaceOAuthURL(value: string): string | null {
  try {
    const parsed = new URL(value);
    if (
      parsed.protocol !== "https:" ||
      !parsed.hostname ||
      parsed.username ||
      parsed.password
    ) {
      return null;
    }
    return parsed.toString();
  } catch {
    return null;
  }
}

type MarketplaceErrorKey =
  | "marketplaceDisabled"
  | "marketplaceUnavailable"
  | "marketplaceChanged"
  | "marketplaceNotInstallable"
  | "marketplaceFailed";
