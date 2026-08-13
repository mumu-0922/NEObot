"use client";

import {
  FormEvent,
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import {
  AlertTriangle,
  BotMessageSquare,
  Check,
  Copy,
  Loader2,
  MessageSquarePlus,
  MoreHorizontal,
  PenLine,
  Plus,
  RefreshCw,
  Search,
  ShieldCheck,
  Store,
  Trash2,
  X,
} from "lucide-react";
import { useLocale, useTranslations } from "next-intl";

import SafeImage from "@/components/ui/SafeImage";
import { Dialog } from "@/components/ui/primitives";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { MARKET_LIMITS } from "@/config/limits";
import type {
  AssistantLibraryEntry,
  AssistantMarketCategory,
  LobeAgent,
} from "@/lib/assistant/types";
import {
  agentMarketCountKey,
  formatAgentMarketCategory,
  missingAgentMarketCountCategories,
} from "@/lib/market/agentCategory";
import { normalizeAgentMarketLocale } from "@/lib/market/agentLocale";
import { ApiClientError, createNeoChatApiClient } from "@/services/api/client";

interface AssistantHubProps {
  onClose: () => void;
  onSelect: (agent: LobeAgent) => void;
  onOpenTools: () => void;
}

interface AssistantDraft {
  avatar: string;
  title: string;
  description: string;
  category: string;
  tags: string[];
  systemPrompt: string;
}

const PAGE_SIZE = 20;

const emptyDraft: AssistantDraft = {
  avatar: "🤖",
  title: "",
  description: "",
  category: "general",
  tags: [],
  systemPrompt: "",
};

export default function AssistantHub({
  onClose,
  onSelect,
  onOpenTools,
}: AssistantHubProps) {
  const t = useTranslations("Assistant");
  const locale = normalizeAgentMarketLocale(useLocale());
  const client = useMemo(() => createNeoChatApiClient(), []);
  const [tab, setTab] = useState<"library" | "market">("library");
  const [library, setLibrary] = useState<AssistantLibraryEntry[]>([]);
  const [libraryLoading, setLibraryLoading] = useState(true);
  const [libraryError, setLibraryError] = useState("");
  const [libraryActionId, setLibraryActionId] = useState("");
  const [editing, setEditing] = useState<
    AssistantLibraryEntry | null | undefined
  >(undefined);
  const defaultTabResolvedRef = useRef(false);

  const loadLibrary = useCallback(async () => {
    setLibraryLoading(true);
    setLibraryError("");
    try {
      const entries = await client.agents.listLibrary!();
      setLibrary(entries);
      if (!defaultTabResolvedRef.current) {
        defaultTabResolvedRef.current = true;
        setTab(entries.length > 0 ? "library" : "market");
      }
    } catch (error) {
      setLibraryError(errorMessage(error, t("libraryLoadFailed")));
    } finally {
      setLibraryLoading(false);
    }
  }, [client.agents, t]);

  useEffect(() => {
    void loadLibrary();
  }, [loadLibrary]);

  const startChat = useCallback(
    (entry: AssistantLibraryEntry) => {
      onSelect({
        identifier: entry.sourceIdentifier || entry.id,
        meta: {
          avatar: entry.avatar,
          title: entry.title,
          description: entry.description,
          category: entry.category,
          tags: entry.tags,
          systemRole: entry.systemPrompt,
        },
        author: entry.author,
        homepage: entry.homepage,
        createdAt: entry.createdAt,
        updatedAt: entry.updatedAt,
        isCustom: entry.source === "custom",
        fingerprint: entry.contentFingerprint,
        requiredTools: entry.requiredTools,
        libraryId: entry.id,
        installed: true,
      });
    },
    [onSelect],
  );

  const deleteEntry = useCallback(
    async (entry: AssistantLibraryEntry) => {
      setLibraryActionId(entry.id);
      try {
        await client.agents.deleteLibraryEntry!({
          assistantId: entry.id,
          revision: entry.revision,
        });
        setLibrary((current) => current.filter((item) => item.id !== entry.id));
      } catch (error) {
        setLibraryError(errorMessage(error, t("deleteFailed")));
      } finally {
        setLibraryActionId("");
      }
    },
    [client.agents, t],
  );

  const copyEntry = useCallback(
    async (entry: AssistantLibraryEntry) => {
      setLibraryActionId(entry.id);
      try {
        const copy = await client.agents.copyToCustom!({
          assistantId: entry.id,
          expectedRevision: entry.revision,
        });
        setLibrary((current) => [copy, ...current]);
        setEditing(copy);
      } catch (error) {
        setLibraryError(errorMessage(error, t("copyFailed")));
      } finally {
        setLibraryActionId("");
      }
    },
    [client.agents, t],
  );

  const updateInstalled = useCallback(
    async (entry: AssistantLibraryEntry) => {
      setLibraryActionId(entry.id);
      try {
        const updated = await client.agents.updateInstalled!({
          assistantId: entry.id,
          expectedRevision: entry.revision,
        });
        setLibrary((current) =>
          current.map((item) => (item.id === updated.id ? updated : item)),
        );
      } catch (error) {
        setLibraryError(errorMessage(error, t("updateFailed")));
      } finally {
        setLibraryActionId("");
      }
    },
    [client.agents, t],
  );

  return (
    <div className="flex h-full w-full flex-col overflow-hidden bg-gray-50/50 dark:bg-background">
      <header className="shrink-0 border-b border-gray-200/60 bg-white/95 px-5 pt-4 dark:border-border dark:bg-card/95 md:px-6">
        <div className="flex items-center justify-between gap-3 pb-4">
          <div className="flex min-w-0 items-center gap-3">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-linear-to-tr from-rose-500 to-orange-500 text-white shadow-lg shadow-rose-500/20">
              <BotMessageSquare size={20} aria-hidden="true" />
            </div>
            <div className="min-w-0">
              <h1 className="truncate text-lg font-bold text-gray-800 dark:text-foreground">
                {t("hubTitle")}
              </h1>
              <p className="mt-0.5 truncate text-xs text-gray-500 dark:text-muted-foreground">
                {t("hubSubtitleNew")}
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label={t("closeHubAria")}
            className="shrink-0 rounded-full p-2 text-gray-500 hover:bg-gray-200/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-rose-500/60 dark:text-muted-foreground dark:hover:bg-accent"
          >
            <X size={20} aria-hidden="true" />
          </button>
        </div>
        <div role="tablist" aria-label={t("pageTabs")} className="flex gap-1">
          <PageTab
            active={tab === "library"}
            label={t("myAssistants")}
            icon={<BotMessageSquare size={14} />}
            onClick={() => setTab("library")}
          />
          <PageTab
            active={tab === "market"}
            label={t("assistantStore")}
            icon={<Store size={14} />}
            onClick={() => setTab("market")}
          />
        </div>
      </header>

      <main className="mx-auto min-h-0 w-full max-w-7xl flex-1 p-4 md:p-6">
        <section className="relative h-full overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-border dark:bg-card">
          {tab === "library" ? (
            <LibraryView
              entries={library}
              loading={libraryLoading}
              error={libraryError}
              actionId={libraryActionId}
              onRetry={loadLibrary}
              onCreate={() => setEditing(null)}
              onEdit={(entry) =>
                entry.source === "custom"
                  ? setEditing(entry)
                  : void copyEntry(entry)
              }
              onDelete={deleteEntry}
              onCopy={copyEntry}
              onUpdate={updateInstalled}
              onStart={startChat}
              onOpenTools={onOpenTools}
              onBrowse={() => setTab("market")}
            />
          ) : (
            <MarketView
              locale={locale}
              installed={library}
              onInstalled={(entry) => {
                setLibrary((current) => [
                  entry,
                  ...current.filter((item) => item.id !== entry.id),
                ]);
              }}
              onOpenLibrary={() => {
                setTab("library");
                void loadLibrary();
              }}
              onOpenTools={onOpenTools}
            />
          )}
        </section>
      </main>

      {editing !== undefined && (
        <AssistantEditor
          entry={editing}
          onClose={() => setEditing(undefined)}
          onSaved={(saved) => {
            setLibrary((current) => [
              saved,
              ...current.filter((item) => item.id !== saved.id),
            ]);
            setEditing(undefined);
          }}
          onDeleted={(id) => {
            setLibrary((current) => current.filter((item) => item.id !== id));
            setEditing(undefined);
          }}
        />
      )}
    </div>
  );
}

function LibraryView({
  entries,
  loading,
  error,
  actionId,
  onRetry,
  onCreate,
  onEdit,
  onDelete,
  onCopy,
  onUpdate,
  onStart,
  onOpenTools,
  onBrowse,
}: {
  entries: AssistantLibraryEntry[];
  loading: boolean;
  error: string;
  actionId: string;
  onRetry: () => void;
  onCreate: () => void;
  onEdit: (entry: AssistantLibraryEntry) => void;
  onDelete: (entry: AssistantLibraryEntry) => void;
  onCopy: (entry: AssistantLibraryEntry) => void;
  onUpdate: (entry: AssistantLibraryEntry) => void;
  onStart: (entry: AssistantLibraryEntry) => void;
  onOpenTools: () => void;
  onBrowse: () => void;
}) {
  const t = useTranslations("Assistant");
  return (
    <div className="flex h-full flex-col overflow-hidden">
      <div className="flex shrink-0 items-center justify-between gap-3 border-b border-gray-100 px-5 py-4 dark:border-border">
        <div>
          <h2 className="text-sm font-semibold text-gray-800 dark:text-foreground">
            {t("myAssistants")}
          </h2>
          <p className="mt-0.5 text-xs text-gray-500 dark:text-muted-foreground">
            {t("librarySubtitle")}
          </p>
        </div>
        <button
          type="button"
          onClick={onCreate}
          className="inline-flex items-center gap-1.5 rounded-xl bg-rose-600 px-3 py-2 text-sm font-medium text-white hover:bg-rose-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-rose-500/60"
        >
          <Plus size={15} aria-hidden="true" /> {t("createAssistant")}
        </button>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-5 custom-scrollbar [scrollbar-gutter:stable]">
        {error && (
          <ErrorNotice message={error} action={t("retry")} onAction={onRetry} />
        )}
        {loading ? (
          <LoadingState label={t("loadingLibrary")} />
        ) : entries.length === 0 ? (
          <div className="flex min-h-80 flex-col items-center justify-center px-6 text-center">
            <div className="mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-rose-50 text-rose-500 dark:bg-rose-950/30">
              <BotMessageSquare size={26} aria-hidden="true" />
            </div>
            <h3 className="font-semibold text-gray-800 dark:text-foreground">
              {t("emptyLibraryTitle")}
            </h3>
            <p className="mt-2 max-w-md text-sm text-gray-500 dark:text-muted-foreground">
              {t("emptyLibraryBody")}
            </p>
            <button
              type="button"
              onClick={onBrowse}
              className="mt-5 inline-flex items-center gap-2 rounded-xl border border-rose-200 bg-rose-50 px-4 py-2 text-sm font-medium text-rose-700 hover:bg-rose-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-rose-500/60 dark:border-rose-900/60 dark:bg-rose-950/20 dark:text-rose-300"
            >
              <Store size={15} aria-hidden="true" /> {t("browseStore")}
            </button>
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
            {entries.map((entry) => (
              <LibraryCard
                key={entry.id}
                entry={entry}
                busy={actionId === entry.id}
                onEdit={() => onEdit(entry)}
                onDelete={() => onDelete(entry)}
                onCopy={() => onCopy(entry)}
                onUpdate={() => onUpdate(entry)}
                onStart={() => onStart(entry)}
                onOpenTools={onOpenTools}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

function LibraryCard({
  entry,
  busy,
  onEdit,
  onDelete,
  onCopy,
  onUpdate,
  onStart,
  onOpenTools,
}: {
  entry: AssistantLibraryEntry;
  busy: boolean;
  onEdit: () => void;
  onDelete: () => void;
  onCopy: () => void;
  onUpdate: () => void;
  onStart: () => void;
  onOpenTools: () => void;
}) {
  const t = useTranslations("Assistant");
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [confirmUpdate, setConfirmUpdate] = useState(false);
  return (
    <article className="flex min-h-56 flex-col rounded-2xl border border-gray-200 bg-white p-4 [contain:paint] transition-[border-color,box-shadow] hover:border-rose-300 hover:shadow-md dark:border-border dark:bg-muted dark:hover:border-rose-800">
      <div className="flex items-start gap-3">
        <AssistantAvatar avatar={entry.avatar} title={entry.title} />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-1.5">
            <h3 className="min-w-0 truncate font-semibold text-gray-900 dark:text-foreground">
              {entry.title}
            </h3>
            <SourceBadge source={entry.source} />
            {entry.updateAvailable && (
              <span className="rounded-full bg-amber-100 px-2 py-0.5 text-[10px] font-medium text-amber-700 dark:bg-amber-950/40 dark:text-amber-300">
                {t("updateAvailable")}
              </span>
            )}
          </div>
          <p className="mt-1 text-xs text-gray-500 dark:text-muted-foreground">
            {entry.category || "general"}
          </p>
        </div>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              aria-label={t("assistantActions", { title: entry.title })}
              disabled={busy}
              className="rounded-lg p-1.5 text-gray-500 hover:bg-gray-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-rose-500/60 disabled:opacity-50 dark:hover:bg-accent"
            >
              {busy ? (
                <Loader2 size={16} className="animate-spin" />
              ) : (
                <MoreHorizontal size={16} />
              )}
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            {entry.source === "custom" ? (
              <DropdownMenuItem onSelect={onEdit}>
                <PenLine size={14} /> {t("editAssistant")}
              </DropdownMenuItem>
            ) : (
              <DropdownMenuItem onSelect={onCopy}>
                <Copy size={14} /> {t("copyAndEdit")}
              </DropdownMenuItem>
            )}
            {entry.updateAvailable && (
              <DropdownMenuItem onSelect={() => setConfirmUpdate(true)}>
                <RefreshCw size={14} /> {t("updateAssistant")}
              </DropdownMenuItem>
            )}
            <DropdownMenuItem onSelect={() => setConfirmDelete(true)}>
              <Trash2 size={14} />
              {entry.source === "custom" ? t("delete") : t("uninstall")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      <p className="mt-4 line-clamp-3 flex-1 text-sm leading-6 text-gray-600 dark:text-muted-foreground">
        {entry.description || t("noDescription")}
      </p>
      <DeclaredTools
        requiredTools={entry.requiredTools}
        onOpenTools={onOpenTools}
      />
      <div className="mt-4 flex flex-wrap gap-1.5">
        {entry.tags.slice(0, 4).map((tag) => (
          <span
            key={tag}
            className="rounded-md bg-gray-100 px-2 py-1 text-[10px] text-gray-600 dark:bg-accent dark:text-muted-foreground"
          >
            #{tag}
          </span>
        ))}
      </div>
      <button
        type="button"
        onClick={onStart}
        className="mt-4 inline-flex w-full items-center justify-center gap-2 rounded-xl bg-gray-900 px-3 py-2 text-sm font-medium text-white hover:bg-black focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-rose-500/60 dark:bg-white dark:text-gray-900"
      >
        <MessageSquarePlus size={15} aria-hidden="true" /> {t("startChat")}
      </button>
      {confirmDelete && (
        <ConfirmDialog
          title={
            entry.source === "custom"
              ? t("deleteAssistantTitle")
              : t("uninstallAssistantTitle")
          }
          body={t("historyPreserved")}
          confirmLabel={
            entry.source === "custom"
              ? t("confirmDelete")
              : t("confirmUninstall")
          }
          onCancel={() => setConfirmDelete(false)}
          onConfirm={() => {
            setConfirmDelete(false);
            onDelete();
          }}
        />
      )}
      {confirmUpdate && (
        <ConfirmDialog
          title={t("updateAssistantTitle")}
          body={t("updateAssistantBody")}
          confirmLabel={t("confirmUpdate")}
          busy={busy}
          destructive={false}
          onCancel={() => setConfirmUpdate(false)}
          onConfirm={() => {
            setConfirmUpdate(false);
            onUpdate();
          }}
        />
      )}
    </article>
  );
}

function MarketView({
  locale,
  installed,
  onInstalled,
  onOpenLibrary,
  onOpenTools,
}: {
  locale: "en" | "zh" | "ja";
  installed: AssistantLibraryEntry[];
  onInstalled: (entry: AssistantLibraryEntry) => void;
  onOpenLibrary: () => void;
  onOpenTools: () => void;
}) {
  const t = useTranslations("Assistant");
  const client = useMemo(() => createNeoChatApiClient(), []);
  const [query, setQuery] = useState("");
  const [activeQuery, setActiveQuery] = useState("");
  const [category, setCategory] = useState("");
  const [agents, setAgents] = useState<LobeAgent[]>([]);
  const [categories, setCategories] = useState<AssistantMarketCategory[]>([]);
  const [page, setPage] = useState(0);
  const [totalPages, setTotalPages] = useState(0);
  const [totalCount, setTotalCount] = useState(0);
  const [availableCounts, setAvailableCounts] = useState<
    Record<string, number>
  >({});
  const availableCountsRef = useRef<Record<string, number>>({});
  const [source, setSource] = useState("");
  const [canReview, setCanReview] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState("");
  const [nextError, setNextError] = useState("");
  const [detail, setDetail] = useState<LobeAgent | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const searchGenerationRef = useRef(0);
  const loadingMoreRef = useRef(false);
  const searchControllerRef = useRef<AbortController | null>(null);
  const detailControllerRef = useRef<AbortController | null>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const searchInputId = useId();
  const installedIdentifiers = useMemo(
    () =>
      new Set(installed.map((entry) => entry.sourceIdentifier).filter(Boolean)),
    [installed],
  );
  const search = useCallback(
    async (nextQuery: string, nextCategory: string) => {
      searchControllerRef.current?.abort();
      detailControllerRef.current?.abort();
      const controller = new AbortController();
      searchControllerRef.current = controller;
      const generation = ++searchGenerationRef.current;
      setLoading(true);
      setError("");
      setNextError("");
      setAgents([]);
      setPage(0);
      setDetail(null);
      scrollRef.current?.scrollTo({ top: 0 });
      try {
        const result = await client.agents.searchMarket!({
          query: nextQuery,
          category: nextCategory || undefined,
          locale,
          page: 1,
          pageSize: PAGE_SIZE,
          signal: controller.signal,
        });
        if (
          generation !== searchGenerationRef.current ||
          controller.signal.aborted
        )
          return;
        setAgents(dedupeAgents(result.agents));
        setCategories(result.categories);
        setPage(result.page);
        setTotalPages(result.totalPages);
        setTotalCount(result.totalCount);
        availableCountsRef.current = {
          ...availableCountsRef.current,
          [agentMarketCountKey(nextQuery, nextCategory)]: result.totalCount,
        };
        setAvailableCounts((current) => ({
          ...current,
          [agentMarketCountKey(nextQuery, nextCategory)]: result.totalCount,
        }));
        setSource(result.source);
        setCanReview(result.canReview === true);
        if (result.unavailable) setError(t("marketUnavailable"));
      } catch (searchError) {
        if (
          generation !== searchGenerationRef.current ||
          controller.signal.aborted
        )
          return;
        setError(errorMessage(searchError, t("marketLoadFailed")));
      } finally {
        if (
          generation === searchGenerationRef.current &&
          !controller.signal.aborted
        ) {
          setLoading(false);
        }
      }
    },
    [client.agents, locale, t],
  );

  useEffect(() => {
    void search(activeQuery, category);
    return () => {
      searchControllerRef.current?.abort();
      detailControllerRef.current?.abort();
    };
  }, [activeQuery, category, search]);

  useEffect(() => {
    if (loading || categories.length === 0) return;
    const missing = missingAgentMarketCountCategories(
      activeQuery,
      categories.map((item) => item.id),
      availableCountsRef.current,
    );
    if (missing.length === 0) return;

    const controller = new AbortController();
    let cursor = 0;
    const worker = async () => {
      while (!controller.signal.aborted) {
        const index = cursor++;
        const categoryID = missing[index];
        if (!categoryID) return;
        try {
          const result = await client.agents.searchMarket!({
            query: activeQuery,
            category: categoryID,
            locale,
            page: 1,
            pageSize: PAGE_SIZE,
            signal: controller.signal,
          });
          if (controller.signal.aborted) return;
          availableCountsRef.current = {
            ...availableCountsRef.current,
            [agentMarketCountKey(activeQuery, categoryID)]: result.totalCount,
          };
          setAvailableCounts((current) => ({
            ...current,
            [agentMarketCountKey(activeQuery, categoryID)]: result.totalCount,
          }));
        } catch {
          if (controller.signal.aborted) return;
        }
      }
    };

    void Promise.all(
      Array.from({ length: Math.min(3, missing.length) }, worker),
    );
    return () => controller.abort();
  }, [activeQuery, categories, client.agents, loading, locale]);

  const loadMore = useCallback(async () => {
    if (loading || loadingMoreRef.current || page < 1 || page >= totalPages)
      return;
    loadingMoreRef.current = true;
    setLoadingMore(true);
    setNextError("");
    const generation = searchGenerationRef.current;
    try {
      const result = await client.agents.searchMarket!({
        query: activeQuery,
        category: category || undefined,
        locale,
        page: page + 1,
        pageSize: PAGE_SIZE,
      });
      if (generation !== searchGenerationRef.current) return;
      setAgents((current) => dedupeAgents([...current, ...result.agents]));
      setPage(result.page);
      setTotalPages(result.totalPages);
    } catch (loadError) {
      if (generation === searchGenerationRef.current) {
        setNextError(errorMessage(loadError, t("loadMoreFailed")));
      }
    } finally {
      if (generation === searchGenerationRef.current) setLoadingMore(false);
      loadingMoreRef.current = false;
    }
  }, [
    activeQuery,
    category,
    client.agents,
    loading,
    locale,
    page,
    t,
    totalPages,
  ]);

  useEffect(() => {
    const target = sentinelRef.current;
    const root = scrollRef.current;
    if (!target || !root || page >= totalPages) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) void loadMore();
      },
      { root, rootMargin: "240px" },
    );
    observer.observe(target);
    return () => observer.disconnect();
  }, [loadMore, page, totalPages]);

  const submitSearch = (event: FormEvent) => {
    event.preventDefault();
    setActiveQuery(query.trim());
  };

  const openDetail = async (agent: LobeAgent) => {
    detailControllerRef.current?.abort();
    const controller = new AbortController();
    detailControllerRef.current = controller;
    setDetailLoading(true);
    setDetail(agent);
    try {
      const result = await client.agents.getMarketDetail!({
        identifier: agent.identifier,
        locale,
        signal: controller.signal,
      });
      if (controller.signal.aborted) return;
      setDetail(result.assistant);
      setCanReview(result.canReview);
    } catch (detailError) {
      if (controller.signal.aborted) return;
      setError(errorMessage(detailError, t("detailLoadFailed")));
      setDetail(null);
    } finally {
      if (!controller.signal.aborted) setDetailLoading(false);
    }
  };

  return (
    <div className="flex h-full overflow-hidden">
      <aside className="hidden w-52 shrink-0 border-r border-gray-100 bg-gray-50/60 p-4 dark:border-border dark:bg-muted/30 md:block">
        <p className="mb-3 px-2 text-[11px] font-semibold uppercase tracking-wider text-gray-400">
          {t("categories")}
        </p>
        <div className="space-y-1">
          <CategoryButton
            active={!category}
            label={t("allAssistants")}
            count={formatOptionalCount(
              availableCounts[agentMarketCountKey(activeQuery, "")],
              locale,
            )}
            onClick={() => setCategory("")}
          />
          {categories.map((item) => (
            <CategoryButton
              key={item.id}
              active={category === item.id}
              label={formatAgentMarketCategory(item.id, locale)}
              count={formatOptionalCount(
                availableCounts[agentMarketCountKey(activeQuery, item.id)],
                locale,
              )}
              onClick={() => setCategory(item.id)}
            />
          ))}
        </div>
      </aside>
      <div className="flex min-w-0 flex-1 flex-col">
        <div className="shrink-0 border-b border-gray-100 p-4 dark:border-border">
          <form className="flex gap-2" onSubmit={submitSearch}>
            <label htmlFor={searchInputId} className="sr-only">
              {t("searchLabel")}
            </label>
            <div className="flex min-w-0 flex-1 items-center rounded-xl border border-gray-200 bg-gray-50 px-3 focus-within:border-rose-400 focus-within:ring-2 focus-within:ring-rose-500/20 dark:border-border dark:bg-muted">
              <Search
                size={16}
                className="mr-2 shrink-0 text-gray-400"
                aria-hidden="true"
              />
              <input
                id={searchInputId}
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder={t("searchPlaceholder")}
                className="min-w-0 flex-1 bg-transparent py-2.5 text-sm outline-none"
              />
              {query && (
                <button
                  type="button"
                  onClick={() => {
                    setQuery("");
                    setActiveQuery("");
                  }}
                  aria-label={t("clearSearch")}
                  className="rounded p-1 text-gray-400 hover:text-gray-700"
                >
                  <X size={14} />
                </button>
              )}
            </div>
            <button
              type="submit"
              className="rounded-xl bg-gray-900 px-4 text-sm font-medium text-white hover:bg-black dark:bg-white dark:text-gray-900"
            >
              {t("search")}
            </button>
          </form>
          <div className="mt-3 flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-muted-foreground">
            <span>{t("browseableCount", { count: totalCount })}</span>
            {source === "legacy-registry" && <span>{t("legacyFallback")}</span>}
          </div>
        </div>
        <div
          ref={scrollRef}
          className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-4 custom-scrollbar [scrollbar-gutter:stable]"
        >
          {error && (
            <ErrorNotice
              message={error}
              action={t("retry")}
              onAction={() => void search(activeQuery, category)}
            />
          )}
          {loading ? (
            <LoadingState label={t("loadingAssistants")} />
          ) : agents.length === 0 ? (
            <div className="flex min-h-72 items-center justify-center text-sm text-gray-500">
              {canReview ? t("noLiveResults") : t("noAdmittedAssistants")}
            </div>
          ) : (
            <div className="grid grid-cols-1 gap-3 lg:grid-cols-2 2xl:grid-cols-3">
              {agents.map((agent) => (
                <button
                  key={agent.identifier}
                  type="button"
                  onClick={() => void openDetail(agent)}
                  className="group flex min-h-44 flex-col rounded-2xl border border-gray-200 bg-white p-4 text-left [contain:paint] transition-[border-color,box-shadow] hover:border-rose-300 hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-rose-500/60 dark:border-border dark:bg-muted dark:hover:border-rose-800"
                >
                  <div className="flex items-start gap-3">
                    <AssistantAvatar
                      avatar={agent.meta.avatar}
                      title={agent.meta.title}
                    />
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-1.5">
                        <h3 className="truncate font-semibold text-gray-900 group-hover:text-rose-600 dark:text-foreground">
                          {agent.meta.title}
                        </h3>
                        {(agent.installed ||
                          installedIdentifiers.has(agent.identifier)) && (
                          <span className="rounded-full bg-emerald-100 px-2 py-0.5 text-[10px] text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300">
                            {t("installed")}
                          </span>
                        )}
                      </div>
                      <p className="mt-1 truncate text-xs text-gray-500">
                        {agent.author ||
                          formatAgentMarketCategory(
                            agent.meta.category,
                            locale,
                          )}
                      </p>
                    </div>
                  </div>
                  <p className="mt-3 line-clamp-3 flex-1 text-sm leading-6 text-gray-600 dark:text-muted-foreground">
                    {agent.meta.description || t("noDescription")}
                  </p>
                  <div className="mt-3 flex items-center justify-between text-[11px] text-gray-400">
                    <span>
                      {formatAgentMarketCategory(agent.meta.category, locale)}
                    </span>
                    {canReview && (
                      <span
                        className={
                          agent.admitted ? "text-emerald-600" : "text-amber-600"
                        }
                      >
                        {agent.admitted ? t("admitted") : t("pendingAdmission")}
                      </span>
                    )}
                  </div>
                </button>
              ))}
            </div>
          )}
          <div
            ref={sentinelRef}
            className="flex min-h-16 items-center justify-center"
          >
            {loadingMore && (
              <Loader2 size={18} className="animate-spin text-rose-500" />
            )}
            {nextError && (
              <button
                type="button"
                onClick={() => void loadMore()}
                className="text-sm text-rose-600 hover:underline"
              >
                {t("loadMoreRetry")}
              </button>
            )}
          </div>
        </div>
      </div>
      {detail && (
        <MarketDetailDialog
          agent={detail}
          locale={locale}
          canReview={canReview}
          loading={detailLoading}
          installed={
            installedIdentifiers.has(detail.identifier) ||
            detail.installed === true
          }
          onClose={() => {
            detailControllerRef.current?.abort();
            setDetail(null);
          }}
          onInstalled={(entry) => {
            onInstalled(entry);
            setDetail((current) =>
              current
                ? { ...current, installed: true, libraryId: entry.id }
                : current,
            );
          }}
          onOpenLibrary={onOpenLibrary}
          onOpenTools={onOpenTools}
        />
      )}
    </div>
  );
}

function MarketDetailDialog({
  agent,
  locale,
  canReview,
  loading,
  installed,
  onClose,
  onInstalled,
  onOpenLibrary,
  onOpenTools,
}: {
  agent: LobeAgent;
  locale: "en" | "zh" | "ja";
  canReview: boolean;
  loading: boolean;
  installed: boolean;
  onClose: () => void;
  onInstalled: (entry: AssistantLibraryEntry) => void;
  onOpenLibrary: () => void;
  onOpenTools: () => void;
}) {
  const t = useTranslations("Assistant");
  const client = useMemo(() => createNeoChatApiClient(), []);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [confirmInstall, setConfirmInstall] = useState(false);
  const install = async () => {
    if (!agent.fingerprint) {
      setError(t("promptUnavailable"));
      return;
    }
    setBusy(true);
    setError("");
    try {
      if (canReview && !agent.admitted) {
        await client.agents.reviewMarket!({
          identifier: agent.identifier,
          locale,
          fingerprint: agent.fingerprint,
          status: "admitted",
        });
      }
      const entry = await client.agents.installMarket!({
        identifier: agent.identifier,
        fingerprint: agent.fingerprint,
      });
      onInstalled(entry);
      setConfirmInstall(false);
    } catch (installError) {
      setError(errorMessage(installError, t("installFailed")));
    } finally {
      setBusy(false);
    }
  };
  const reject = async () => {
    if (!agent.fingerprint) return;
    setBusy(true);
    setError("");
    try {
      await client.agents.reviewMarket!({
        identifier: agent.identifier,
        locale,
        fingerprint: agent.fingerprint,
        status: "rejected",
      });
      onClose();
    } catch (reviewError) {
      setError(errorMessage(reviewError, t("reviewFailed")));
    } finally {
      setBusy(false);
    }
  };
  const admitUpdate = async () => {
    if (!agent.fingerprint) return;
    setBusy(true);
    setError("");
    try {
      await client.agents.reviewMarket!({
        identifier: agent.identifier,
        locale,
        fingerprint: agent.fingerprint,
        status: "admitted",
      });
      onClose();
      onOpenLibrary();
    } catch (reviewError) {
      setError(errorMessage(reviewError, t("reviewFailed")));
    } finally {
      setBusy(false);
    }
  };
  return (
    <Dialog
      open
      onClose={onClose}
      title={agent.meta.title}
      closeLabel={t("closeDetail")}
      className="flex max-h-[90vh] max-w-2xl flex-col rounded-2xl dark:bg-card"
    >
      <div className="flex items-start gap-4 border-b border-gray-100 p-5 dark:border-border">
        <AssistantAvatar
          avatar={agent.meta.avatar}
          title={agent.meta.title}
          large
        />
        <div className="min-w-0 flex-1">
          <p className="mt-1 text-sm text-gray-500">
            {agent.author ||
              formatAgentMarketCategory(agent.meta.category, locale)}
          </p>
        </div>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-5 custom-scrollbar">
        {loading ? (
          <LoadingState label={t("loadingDetail")} />
        ) : (
          <>
            <p className="text-sm leading-6 text-gray-700 dark:text-muted-foreground">
              {agent.meta.description || t("noDescription")}
            </p>
            <div className="mt-4 flex flex-wrap gap-2">
              {agent.meta.tags.map((tag) => (
                <span
                  key={tag}
                  className="rounded-lg bg-gray-100 px-2 py-1 text-xs text-gray-600 dark:bg-muted"
                >
                  #{tag}
                </span>
              ))}
            </div>
            <DeclaredTools
              requiredTools={agent.requiredTools}
              onOpenTools={onOpenTools}
            />
            <div className="mt-5 rounded-xl border border-blue-200 bg-blue-50 p-3 text-sm text-blue-800 dark:border-blue-900/60 dark:bg-blue-950/20 dark:text-blue-200">
              <div className="flex gap-2">
                <ShieldCheck size={17} className="mt-0.5 shrink-0" />
                <p>{t("compatibilityNotice")}</p>
              </div>
            </div>
            <div className="mt-5">
              <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-400">
                {t("promptPreview")}
              </p>
              <pre className="max-h-72 overflow-auto whitespace-pre-wrap rounded-xl bg-gray-950 p-4 text-xs leading-5 text-gray-100 custom-scrollbar">
                {agent.meta.systemRole || t("promptUnavailable")}
              </pre>
            </div>
          </>
        )}
        {error && (
          <div
            role="alert"
            className="mt-4 rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/20 dark:text-red-200"
          >
            {error}
          </div>
        )}
      </div>
      <div className="flex flex-wrap items-center justify-between gap-3 border-t border-gray-100 bg-gray-50/70 p-5 dark:border-border dark:bg-muted/20">
        {canReview && !agent.admitted ? (
          <button
            type="button"
            disabled={busy}
            onClick={() => void reject()}
            className="rounded-xl px-3 py-2 text-sm text-red-600 hover:bg-red-50 disabled:opacity-50 dark:hover:bg-red-950/30"
          >
            {t("rejectEntry")}
          </button>
        ) : (
          <div />
        )}
        {installed && canReview && !agent.admitted ? (
          <button
            type="button"
            disabled={loading || busy || !agent.meta.systemRole}
            onClick={() => void admitUpdate()}
            className="inline-flex items-center gap-2 rounded-xl bg-amber-600 px-4 py-2 text-sm font-medium text-white hover:bg-amber-700 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {busy ? (
              <Loader2 size={15} className="animate-spin" />
            ) : (
              <RefreshCw size={15} />
            )}
            {t("admitUpdate")}
          </button>
        ) : installed ? (
          <button
            type="button"
            onClick={() => {
              onClose();
              onOpenLibrary();
            }}
            className="inline-flex items-center gap-2 rounded-xl bg-emerald-600 px-4 py-2 text-sm font-medium text-white"
          >
            <Check size={15} /> {t("viewInstalled")}
          </button>
        ) : (
          <button
            type="button"
            disabled={loading || busy || !agent.meta.systemRole}
            onClick={() => setConfirmInstall(true)}
            className="inline-flex items-center gap-2 rounded-xl bg-rose-600 px-4 py-2 text-sm font-medium text-white hover:bg-rose-700 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {busy ? (
              <Loader2 size={15} className="animate-spin" />
            ) : (
              <Plus size={15} />
            )}
            {canReview && !agent.admitted ? t("admitAndInstall") : t("install")}
          </button>
        )}
      </div>
      {confirmInstall && (
        <ConfirmDialog
          title={t("confirmInstallTitle")}
          body={t("compatibilityNotice")}
          confirmLabel={
            canReview && !agent.admitted
              ? t("admitAndInstall")
              : t("confirmInstall")
          }
          busy={busy}
          onCancel={() => setConfirmInstall(false)}
          onConfirm={() => void install()}
        />
      )}
    </Dialog>
  );
}

function AssistantEditor({
  entry,
  onClose,
  onSaved,
  onDeleted,
}: {
  entry: AssistantLibraryEntry | null;
  onClose: () => void;
  onSaved: (entry: AssistantLibraryEntry) => void;
  onDeleted: (id: string) => void;
}) {
  const t = useTranslations("Assistant");
  const client = useMemo(() => createNeoChatApiClient(), []);
  const [currentEntry, setCurrentEntry] = useState(entry);
  const [draft, setDraft] = useState<AssistantDraft>(
    entry ? draftFromEntry(entry) : emptyDraft,
  );
  const [tagInput, setTagInput] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [conflicted, setConflicted] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const save = async () => {
    if (!draft.title.trim() || !draft.systemPrompt.trim()) {
      setError(t("requiredFields"));
      return;
    }
    setBusy(true);
    setError("");
    try {
      const saved = currentEntry
        ? await client.agents.updateCustom!({
            assistantId: currentEntry.id,
            expectedRevision: currentEntry.revision,
            ...draft,
          })
        : await client.agents.createCustom!(draft);
      onSaved(saved);
    } catch (saveError) {
      if (
        saveError instanceof ApiClientError &&
        saveError.code === "ASSISTANT_REVISION_CONFLICT"
      ) {
        setConflicted(true);
        setError(t("revisionConflict"));
      } else setError(errorMessage(saveError, t("saveFailed")));
    } finally {
      setBusy(false);
    }
  };
  const remove = async () => {
    if (!currentEntry) return;
    setBusy(true);
    setError("");
    try {
      await client.agents.deleteLibraryEntry!({
        assistantId: currentEntry.id,
        revision: currentEntry.revision,
      });
      onDeleted(currentEntry.id);
    } catch (deleteError) {
      setError(errorMessage(deleteError, t("deleteFailed")));
    } finally {
      setBusy(false);
    }
  };
  const reloadCurrent = async () => {
    if (!currentEntry) return;
    setBusy(true);
    setError("");
    try {
      const latest = await client.agents.getLibraryEntry!(currentEntry.id);
      setCurrentEntry(latest);
      setDraft(draftFromEntry(latest));
      setConflicted(false);
    } catch (reloadError) {
      setError(errorMessage(reloadError, t("reloadFailed")));
    } finally {
      setBusy(false);
    }
  };
  const saveConflictCopy = async () => {
    setBusy(true);
    setError("");
    try {
      const copy = await client.agents.createCustom!(draft);
      onSaved(copy);
    } catch (copyError) {
      setError(errorMessage(copyError, t("copyFailed")));
    } finally {
      setBusy(false);
    }
  };
  const addTag = () => {
    const tag = tagInput.trim().slice(0, MARKET_LIMITS.maxAgentTagChars);
    if (
      tag &&
      draft.tags.length < MARKET_LIMITS.maxAgentTags &&
      !draft.tags.some((item) => item.toLowerCase() === tag.toLowerCase())
    )
      setDraft((current) => ({ ...current, tags: [...current.tags, tag] }));
    setTagInput("");
  };
  return (
    <Dialog
      open
      onClose={onClose}
      title={currentEntry ? t("editAssistant") : t("createAssistant")}
      closeLabel={t("closeEditor")}
      className="flex max-h-[92vh] flex-col rounded-2xl dark:bg-card"
    >
      <div className="min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain p-5 custom-scrollbar">
        <div className="grid grid-cols-[5rem_1fr] gap-3">
          <Field label={t("avatar")}>
            <input
              value={draft.avatar}
              maxLength={MARKET_LIMITS.maxAgentAvatarChars}
              onChange={(event) =>
                setDraft({ ...draft, avatar: event.target.value })
              }
              className="field-input text-center"
            />
          </Field>
          <Field label={t("name")}>
            <input
              value={draft.title}
              maxLength={MARKET_LIMITS.maxAgentTitleChars}
              onChange={(event) =>
                setDraft({ ...draft, title: event.target.value })
              }
              className="field-input"
            />
          </Field>
        </div>
        <Field label={t("description")}>
          <textarea
            value={draft.description}
            maxLength={MARKET_LIMITS.maxAgentDescriptionChars}
            onChange={(event) =>
              setDraft({ ...draft, description: event.target.value })
            }
            className="field-input h-20 resize-none"
          />
        </Field>
        <Field label={t("category")}>
          <input
            value={draft.category}
            maxLength={MARKET_LIMITS.maxAgentCategoryChars}
            onChange={(event) =>
              setDraft({ ...draft, category: event.target.value })
            }
            className="field-input"
          />
        </Field>
        <Field label={t("systemPrompt")}>
          <textarea
            value={draft.systemPrompt}
            maxLength={MARKET_LIMITS.maxAgentSystemRoleChars}
            onChange={(event) =>
              setDraft({ ...draft, systemPrompt: event.target.value })
            }
            className="field-input h-44 resize-none font-mono text-xs"
          />
        </Field>
        <Field label={t("tagsOptional")}>
          <div className="mb-2 flex flex-wrap gap-1.5">
            {draft.tags.map((tag) => (
              <button
                type="button"
                key={tag}
                onClick={() =>
                  setDraft((current) => ({
                    ...current,
                    tags: current.tags.filter((item) => item !== tag),
                  }))
                }
                className="rounded-lg bg-gray-100 px-2 py-1 text-xs text-gray-600 hover:text-red-600 dark:bg-muted"
              >
                #{tag} ×
              </button>
            ))}
          </div>
          <div className="flex gap-2">
            <input
              value={tagInput}
              onChange={(event) => setTagInput(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter") {
                  event.preventDefault();
                  addTag();
                }
              }}
              className="field-input flex-1"
            />
            <button
              type="button"
              onClick={addTag}
              className="rounded-xl border border-gray-200 px-3 hover:bg-gray-50 dark:border-border dark:hover:bg-muted"
            >
              <Plus size={15} />
            </button>
          </div>
        </Field>
        {error && (
          <div
            role="alert"
            className="rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/20 dark:text-red-200"
          >
            {error}
          </div>
        )}
        {conflicted && (
          <div className="flex flex-wrap gap-2">
            <button
              type="button"
              disabled={busy}
              onClick={() => void reloadCurrent()}
              className="rounded-xl border border-gray-200 px-3 py-2 text-sm hover:bg-gray-50 disabled:opacity-50 dark:border-border dark:hover:bg-muted"
            >
              {t("reloadLatest")}
            </button>
            <button
              type="button"
              disabled={busy}
              onClick={() => void saveConflictCopy()}
              className="rounded-xl border border-rose-200 px-3 py-2 text-sm text-rose-700 hover:bg-rose-50 disabled:opacity-50 dark:border-rose-900 dark:text-rose-300 dark:hover:bg-rose-950/20"
            >
              {t("saveAsCopy")}
            </button>
          </div>
        )}
      </div>
      <div className="flex items-center justify-between gap-3 border-t border-gray-100 bg-gray-50/70 p-5 dark:border-border dark:bg-muted/20">
        {currentEntry ? (
          <button
            type="button"
            disabled={busy}
            onClick={() => setConfirmDelete(true)}
            className="inline-flex items-center gap-1.5 rounded-xl px-3 py-2 text-sm text-red-600 hover:bg-red-50 disabled:opacity-50 dark:hover:bg-red-950/20"
          >
            <Trash2 size={15} />
            {t("delete")}
          </button>
        ) : (
          <div />
        )}
        <div className="flex gap-2">
          <button
            type="button"
            onClick={onClose}
            className="rounded-xl px-4 py-2 text-sm text-gray-600 hover:bg-gray-100 dark:hover:bg-muted"
          >
            {t("cancel")}
          </button>
          <button
            type="button"
            disabled={busy}
            onClick={() => void save()}
            className="inline-flex items-center gap-2 rounded-xl bg-rose-600 px-4 py-2 text-sm font-medium text-white hover:bg-rose-700 disabled:opacity-50"
          >
            {busy && <Loader2 size={15} className="animate-spin" />}
            {t("saveAssistant")}
          </button>
        </div>
      </div>
      {confirmDelete && (
        <ConfirmDialog
          title={t("deleteAssistantTitle")}
          body={t("historyPreserved")}
          confirmLabel={t("confirmDelete")}
          busy={busy}
          onCancel={() => setConfirmDelete(false)}
          onConfirm={() => void remove()}
        />
      )}
    </Dialog>
  );
}

function PageTab({
  active,
  label,
  icon,
  onClick,
}: {
  active: boolean;
  label: string;
  icon: ReactNode;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={active}
      onClick={onClick}
      className={`inline-flex items-center gap-1.5 border-b-2 px-3 py-2 text-sm font-medium ${active ? "border-rose-500 text-rose-700 dark:text-rose-300" : "border-transparent text-gray-500 hover:text-gray-800 dark:text-muted-foreground dark:hover:text-foreground"}`}
    >
      {icon}
      {label}
    </button>
  );
}
function CategoryButton({
  active,
  label,
  count,
  onClick,
}: {
  active: boolean;
  label: string;
  count?: ReactNode;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`flex w-full items-center justify-between gap-2 rounded-xl px-3 py-2 text-left text-sm ${active ? "bg-rose-100 font-medium text-rose-700 dark:bg-rose-950/40 dark:text-rose-300" : "text-gray-600 hover:bg-gray-100 dark:text-muted-foreground dark:hover:bg-accent"}`}
    >
      <span className="truncate">{label}</span>
      {count !== undefined && (
        <span className="text-[10px] opacity-65">{count}</span>
      )}
    </button>
  );
}
function SourceBadge({ source }: { source: "custom" | "lobehub" }) {
  const t = useTranslations("Assistant");
  return (
    <span
      className={`rounded-full px-2 py-0.5 text-[10px] font-medium ${source === "custom" ? "bg-blue-100 text-blue-700 dark:bg-blue-950/40 dark:text-blue-300" : "bg-violet-100 text-violet-700 dark:bg-violet-950/40 dark:text-violet-300"}`}
    >
      {source === "custom" ? t("custom") : t("storeSource")}
    </span>
  );
}
function DeclaredTools({
  requiredTools,
  onOpenTools,
}: {
  requiredTools?: string[];
  onOpenTools: () => void;
}) {
  const t = useTranslations("Assistant");
  if (!requiredTools || requiredTools.length === 0) return null;
  return (
    <div className="mt-4 rounded-xl border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/20 dark:text-amber-200">
      <p className="font-medium">{t("declaredTools")}</p>
      <p className="mt-1 break-words text-xs">{requiredTools.join(", ")}</p>
      <button
        type="button"
        onClick={onOpenTools}
        className="mt-2 text-xs font-semibold underline underline-offset-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-amber-500/60"
      >
        {t("openTools")}
      </button>
    </div>
  );
}
function AssistantAvatar({
  avatar,
  title,
  large = false,
}: {
  avatar: string;
  title: string;
  large?: boolean;
}) {
  const size = large ? "h-16 w-16 text-2xl" : "h-12 w-12 text-xl";
  const remote = avatar.startsWith("http://") || avatar.startsWith("https://");
  return (
    <div
      className={`flex ${size} shrink-0 items-center justify-center overflow-hidden rounded-xl border border-gray-100 bg-gray-50 dark:border-border dark:bg-accent`}
    >
      {remote ? (
        <SafeImage
          src={avatar}
          alt={`${title} avatar`}
          className="h-full w-full object-cover"
          fallback={<BotMessageSquare size={20} className="text-gray-400" />}
        />
      ) : (
        <span aria-hidden="true">{avatar || "🤖"}</span>
      )}
    </div>
  );
}
function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block space-y-1.5">
      <span className="text-xs font-semibold text-gray-500 dark:text-muted-foreground">
        {label}
      </span>
      {children}
    </label>
  );
}
function LoadingState({ label }: { label: string }) {
  return (
    <div
      role="status"
      className="flex min-h-72 items-center justify-center gap-2 text-sm text-gray-500"
    >
      <Loader2 size={18} className="animate-spin text-rose-500" />
      {label}
    </div>
  );
}
function ErrorNotice({
  message,
  action,
  onAction,
}: {
  message: string;
  action: string;
  onAction: () => void;
}) {
  return (
    <div
      role="alert"
      className="mb-4 flex items-center justify-between gap-3 rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/20 dark:text-red-200"
    >
      <span className="flex min-w-0 items-start gap-2">
        <AlertTriangle size={16} className="mt-0.5 shrink-0" />
        {message}
      </span>
      <button
        type="button"
        onClick={onAction}
        className="shrink-0 font-medium hover:underline"
      >
        {action}
      </button>
    </div>
  );
}
function ConfirmDialog({
  title,
  body,
  confirmLabel,
  busy = false,
  destructive = true,
  onCancel,
  onConfirm,
}: {
  title: string;
  body: string;
  confirmLabel: string;
  busy?: boolean;
  destructive?: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const t = useTranslations("Assistant");
  return (
    <Dialog
      open
      role="alertdialog"
      onClose={busy ? () => undefined : onCancel}
      title={title}
      className="z-10000 max-w-sm rounded-2xl dark:bg-card"
    >
      <div className="p-5">
        <p className="mt-2 text-sm leading-6 text-gray-600 dark:text-muted-foreground">
          {body}
        </p>
        <div className="mt-5 flex justify-end gap-2">
          <button
            type="button"
            disabled={busy}
            onClick={onCancel}
            className="rounded-xl px-4 py-2 text-sm text-gray-600 hover:bg-gray-100 disabled:opacity-50 dark:hover:bg-muted"
          >
            {t("cancel")}
          </button>
          <button
            type="button"
            disabled={busy}
            onClick={onConfirm}
            className={`inline-flex items-center gap-2 rounded-xl px-4 py-2 text-sm font-medium text-white disabled:opacity-50 ${destructive ? "bg-red-600 hover:bg-red-700" : "bg-rose-600 hover:bg-rose-700"}`}
          >
            {busy && <Loader2 size={14} className="animate-spin" />}
            {confirmLabel}
          </button>
        </div>
      </div>
    </Dialog>
  );
}
function draftFromEntry(entry: AssistantLibraryEntry): AssistantDraft {
  return {
    avatar: entry.avatar,
    title: entry.title,
    description: entry.description,
    category: entry.category,
    tags: entry.tags,
    systemPrompt: entry.systemPrompt,
  };
}
function dedupeAgents(agents: LobeAgent[]) {
  const seen = new Set<string>();
  return agents.filter((agent) => {
    if (!agent.identifier || seen.has(agent.identifier)) return false;
    seen.add(agent.identifier);
    return true;
  });
}
function formatCount(value: number, locale: "en" | "zh" | "ja") {
  const numberLocale = { en: "en-US", zh: "zh-CN", ja: "ja-JP" }[locale];
  return new Intl.NumberFormat(numberLocale, {
    notation: value >= 1000 ? "compact" : "standard",
    maximumFractionDigits: 1,
  }).format(value);
}
function formatOptionalCount(
  value: number | undefined,
  locale: "en" | "zh" | "ja",
) {
  return value === undefined ? undefined : formatCount(value, locale);
}
function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error && error.message ? error.message : fallback;
}
