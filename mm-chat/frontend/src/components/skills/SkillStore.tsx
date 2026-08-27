"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import {
  ArrowLeft,
  Download,
  Loader2,
  PackageCheck,
  RefreshCw,
  Search,
  Store,
  Trash2,
  X,
} from "lucide-react";
import { useTranslations } from "next-intl";

import {
  ApiClientError,
  createNeoChatApiClient,
  type AgentPackageInstallationDTO,
  type SkillCatalogItemDTO,
  type SkillCatalogSummaryDTO,
} from "@/services/api/client";

interface SkillStoreProps {
  selectedId: string | null;
  initialQuery?: string;
  onNavigate: (id: string | null, historyMode?: "push" | "replace") => void;
  onClose: () => void;
}

type SkillStoreTab = "installed" | "store";

export default function SkillStore({
  selectedId,
  initialQuery = "",
  onNavigate,
  onClose,
}: SkillStoreProps) {
  const t = useTranslations("SkillStore");
  const client = useMemo(() => createNeoChatApiClient(), []);
  const [tab, setTab] = useState<SkillStoreTab>(() =>
    initialQuery || selectedId ? "store" : "installed",
  );
  const [items, setItems] = useState<SkillCatalogSummaryDTO[]>([]);
  const [installed, setInstalled] = useState<AgentPackageInstallationDTO[]>([]);
  const [storeLoading, setStoreLoading] = useState(true);
  const [installedLoading, setInstalledLoading] = useState(true);
  const [storeError, setStoreError] = useState("");
  const [installedError, setInstalledError] = useState("");
  const [actionId, setActionId] = useState("");
  const [announcement, setAnnouncement] = useState("");
  const [query, setQuery] = useState(initialQuery);
  const [selectedDetail, setSelectedDetail] =
    useState<SkillCatalogItemDTO | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState("");
  const [detailReload, setDetailReload] = useState(0);
  const restoreFocus = useRef<HTMLButtonElement | null>(null);

  const loadStore = useCallback(async () => {
    setStoreLoading(true);
    setStoreError("");
    try {
      const store = await client.skillStore.listCatalog();
      setItems(store.items);
    } catch (loadError) {
      setStoreError(errorMessage(loadError, t("loadStoreFailed")));
    } finally {
      setStoreLoading(false);
    }
  }, [client.skillStore, t]);

  const loadInstalled = useCallback(async () => {
    setInstalledLoading(true);
    setInstalledError("");
    try {
      setInstalled(await client.skillStore.listPackageLibrary());
    } catch (loadError) {
      setInstalledError(errorMessage(loadError, t("loadInstalledFailed")));
    } finally {
      setInstalledLoading(false);
    }
  }, [client.skillStore, t]);

  const loadAll = useCallback(
    async () => Promise.all([loadStore(), loadInstalled()]),
    [loadInstalled, loadStore],
  );

  useEffect(() => {
    queueMicrotask(() => void loadAll());
  }, [loadAll]);

  useEffect(() => setQuery(initialQuery), [initialQuery]);

  useEffect(() => {
    if (selectedId) setTab("store");
  }, [selectedId]);

  useEffect(() => {
    const controller = new AbortController();
    if (!selectedId) {
      setSelectedDetail(null);
      setDetailError("");
      setDetailLoading(false);
      return () => controller.abort();
    }
    setSelectedDetail(null);
    setDetailError("");
    setDetailLoading(true);
    void client.skillStore
      .getCatalogSkill(selectedId, { signal: controller.signal })
      .then(setSelectedDetail)
      .catch((loadError) => {
        if (!controller.signal.aborted) {
          setDetailError(errorMessage(loadError, t("loadStoreFailed")));
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setDetailLoading(false);
      });
    return () => controller.abort();
  }, [client.skillStore, detailReload, selectedId, t]);

  const selected = selectedDetail;
  const installedFingerprints = new Set(
    installed.map((item) => item.packageFingerprint),
  );
  const normalizedQuery = query.trim().toLowerCase();
  const filteredItems = normalizedQuery
    ? items.filter((item) =>
        [item.id, item.name, item.path, item.repository].some((value) =>
          value.toLowerCase().includes(normalizedQuery),
        ),
      )
    : items;

  const install = async (item: SkillCatalogItemDTO) => {
    setActionId(item.id);
    try {
      await client.skillStore.installCatalogSkill({
        id: item.id,
        resolvedCommit: item.resolvedCommit,
        packageFingerprint: item.packageFingerprint,
      });
      setAnnouncement(t("installedAnnouncement", { name: item.name }));
      await loadAll();
      onNavigate(null, "replace");
      setTab("installed");
    } catch (actionError) {
      setAnnouncement(errorMessage(actionError, t("actionFailed")));
      if (isStale(actionError)) {
        await loadAll();
        setDetailReload((value) => value + 1);
      }
    } finally {
      setActionId("");
    }
  };

  const uninstall = async (entry: AgentPackageInstallationDTO) => {
    setActionId(entry.id);
    try {
      await client.skillStore.uninstallPackageSkill({
        installationId: entry.id,
        revision: entry.revision,
      });
      setAnnouncement(t("uninstalledAnnouncement", { name: entry.name }));
      await loadAll();
    } catch (actionError) {
      setAnnouncement(errorMessage(actionError, t("actionFailed")));
      if (isStale(actionError)) await loadAll();
    } finally {
      setActionId("");
    }
  };

  return (
    <div className="flex h-full w-full flex-col overflow-hidden bg-slate-50/80 dark:bg-background">
      <a className="skip-link" href="#skill-store-content">
        {t("skipToContent")}
      </a>
      <header className="shrink-0 border-b border-slate-200/80 bg-white/95 px-4 pt-4 dark:border-border dark:bg-card/95 md:px-6">
        <div className="flex items-center justify-between gap-3 pb-4">
          <div className="flex min-w-0 items-center gap-3">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-slate-950 text-white dark:bg-cyan-500 dark:text-slate-950">
              <PackageCheck size={20} aria-hidden="true" />
            </div>
            <div className="min-w-0">
              <h1 className="truncate text-lg font-bold">{t("title")}</h1>
              <p className="mt-0.5 truncate text-xs text-muted-foreground">
                {t("subtitle")}
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label={t("close")}
            className="rounded-full p-2 text-muted-foreground hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/70"
          >
            <X size={20} aria-hidden="true" />
          </button>
        </div>
        <div role="tablist" aria-label={t("pageTabs")} className="flex gap-1">
          <PageTab
            active={tab === "installed"}
            label={t("installedTab")}
            icon={<PackageCheck size={14} />}
            onClick={() => {
              onNavigate(null);
              setTab("installed");
            }}
          />
          <PageTab
            active={tab === "store"}
            label={t("storeTab")}
            icon={<Store size={14} />}
            onClick={() => setTab("store")}
          />
        </div>
      </header>

      <main
        id="skill-store-content"
        tabIndex={-1}
        className="mx-auto min-h-0 w-full max-w-7xl flex-1 p-4 md:p-6"
      >
        <section className="relative h-full overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm dark:border-border dark:bg-card">
          {tab === "installed" ? (
            <InstalledSkills
              items={installed}
              loading={installedLoading}
              error={installedError}
              actionId={actionId}
              onReload={() => void loadInstalled()}
              onUninstall={uninstall}
            />
          ) : (
            <SplitShell
              selected={Boolean(selectedId)}
              list={
                <>
                  <PanelHeader
                    title={t("packageStore")}
                    onReload={() => void loadStore()}
                  />
                  <div className="space-y-5 p-4">
                    <label className="relative block">
                      <span className="sr-only">{t("searchLabel")}</span>
                      <Search
                        size={16}
                        aria-hidden="true"
                        className="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-muted-foreground"
                      />
                      <input
                        type="search"
                        value={query}
                        onChange={(event) => setQuery(event.target.value)}
                        placeholder={t("searchPlaceholder")}
                        className="w-full rounded-xl border bg-card py-2 pr-3 pl-9 text-sm outline-none focus:border-cyan-500 focus:ring-2 focus:ring-cyan-500/20"
                      />
                    </label>
                    {storeLoading ? (
                      <Loading />
                    ) : storeError ? (
                      <InlineError
                        message={storeError}
                        retry={() => void loadStore()}
                      />
                    ) : filteredItems.length ? (
                      <div className="space-y-2">
                        {filteredItems.map((item) => (
                          <button
                            key={item.id}
                            type="button"
                            onClick={(event) => {
                              restoreFocus.current = event.currentTarget;
                              onNavigate(item.id);
                            }}
                            aria-current={
                              selectedId === item.id ? "true" : undefined
                            }
                            className={`w-full rounded-xl border p-3 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500 ${
                              selectedId === item.id
                                ? "border-cyan-500 bg-cyan-50 dark:bg-cyan-950/20"
                                : "bg-card hover:border-slate-300 dark:hover:border-slate-700"
                            }`}
                          >
                            <div className="flex items-center justify-between gap-2">
                              <span className="font-medium">{item.name}</span>
                              <StatusPill value={t("curated")} />
                            </div>
                            <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">
                              {item.repository}/{item.path}
                            </p>
                          </button>
                        ))}
                      </div>
                    ) : (
                      <EmptyState title={t("emptyStore")} />
                    )}
                  </div>
                </>
              }
              detail={
                selected ? (
                  <DetailFrame
                    title={selected.name}
                    onBack={() => closeDetail(onNavigate, restoreFocus)}
                  >
                    <p className="text-sm text-muted-foreground">
                      {selected.description}
                    </p>
                    <DetailGrid
                      rows={[
                        [t("version"), selected.version],
                        [t("source"), selected.catalogSource],
                        [t("sourcePath"), selected.path],
                        [
                          t("runtime"),
                          selected.hasRuntime
                            ? t("localDirectPackage")
                            : t("textOnlyPackage"),
                        ],
                        [
                          t("compatibility"),
                          selected.compatibility || t("none"),
                        ],
                        [t("license"), selected.license || t("none")],
                      ]}
                    />
                    <a
                      href={selected.sourceUrl}
                      target="_blank"
                      rel="noreferrer"
                      className="inline-flex text-sm font-medium text-cyan-700 underline-offset-4 hover:underline dark:text-cyan-300"
                    >
                      {t("viewSource")}
                    </a>
                    <div>
                      <h3 className="text-sm font-semibold">
                        {t("declaredTools")}
                      </h3>
                      <p className="mt-1 text-sm text-muted-foreground">
                        {selected.allowedTools.join(", ") || t("none")}
                      </p>
                    </div>
                    <button
                      type="button"
                      disabled={
                        installedFingerprints.has(
                          selected.packageFingerprint,
                        ) || actionId === selected.id
                      }
                      onClick={() => void install(selected)}
                      className="inline-flex items-center gap-2 rounded-xl bg-slate-950 px-4 py-2.5 text-sm font-medium text-white disabled:cursor-not-allowed disabled:opacity-50 dark:bg-cyan-500 dark:text-slate-950"
                    >
                      {actionId === selected.id ? (
                        <Loader2 size={16} className="animate-spin" />
                      ) : (
                        <Download size={16} />
                      )}
                      {installedFingerprints.has(selected.packageFingerprint)
                        ? t("installed")
                        : t("installPackage")}
                    </button>
                  </DetailFrame>
                ) : detailLoading ? (
                  <Loading />
                ) : detailError ? (
                  <div className="p-4">
                    <InlineError
                      message={detailError}
                      retry={() => setDetailReload((value) => value + 1)}
                    />
                  </div>
                ) : (
                  <EmptyDetail title={t("selectPackage")} />
                )
              }
            />
          )}
        </section>
      </main>
      <div
        className="sr-only"
        role="status"
        aria-live="polite"
        aria-atomic="true"
      >
        {announcement}
      </div>
    </div>
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
      className={`inline-flex items-center gap-1.5 border-b-2 px-3 py-2 text-sm font-medium transition-colors ${
        active
          ? "border-cyan-500 text-cyan-700 dark:text-cyan-300"
          : "border-transparent text-gray-500 hover:text-gray-800 dark:text-muted-foreground dark:hover:text-foreground"
      }`}
    >
      {icon}
      {label}
    </button>
  );
}

function InstalledSkills({
  items,
  loading,
  error,
  actionId,
  onReload,
  onUninstall,
}: {
  items: AgentPackageInstallationDTO[];
  loading: boolean;
  error: string;
  actionId: string;
  onReload: () => void;
  onUninstall: (item: AgentPackageInstallationDTO) => Promise<void>;
}) {
  const t = useTranslations("SkillStore");
  return (
    <div className="h-full overflow-y-auto">
      <PanelHeader
        title={t("installedPackages", { count: items.length })}
        onReload={onReload}
      />
      <div className="p-4 md:p-5">
        {loading ? (
          <Loading />
        ) : error ? (
          <InlineError message={error} retry={onReload} />
        ) : items.length ? (
          <div className="space-y-3">
            {items.map((entry) => (
              <article key={entry.id} className="rounded-xl border bg-card p-4">
                <div className="flex items-start justify-between gap-4">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <h2 className="font-semibold">{entry.name}</h2>
                      <span className="rounded-full border px-2 py-0.5 text-[11px] text-muted-foreground">
                        v{entry.version}
                      </span>
                    </div>
                    <p className="mt-1 text-sm text-muted-foreground">
                      {entry.description}
                    </p>
                    {entry.allowedTools.length ? (
                      <p className="mt-2 text-xs text-muted-foreground">
                        {t("declaredTools")}: {entry.allowedTools.join(", ")}
                      </p>
                    ) : null}
                  </div>
                  <button
                    type="button"
                    disabled={actionId === entry.id}
                    onClick={() => void onUninstall(entry)}
                    aria-label={t("uninstallAria", { name: entry.name })}
                    className="shrink-0 rounded-lg p-2 text-red-600 hover:bg-red-50 disabled:opacity-50 dark:hover:bg-red-950/30"
                  >
                    {actionId === entry.id ? (
                      <Loader2 size={15} className="animate-spin" />
                    ) : (
                      <Trash2 size={15} aria-hidden="true" />
                    )}
                  </button>
                </div>
              </article>
            ))}
          </div>
        ) : (
          <EmptyState title={t("noInstalledPackages")} />
        )}
      </div>
    </div>
  );
}

function SplitShell({
  selected,
  list,
  detail,
}: {
  selected: boolean;
  list: ReactNode;
  detail: ReactNode;
}) {
  return (
    <div className="grid h-full min-h-0 md:grid-cols-[minmax(17rem,22rem)_1fr]">
      <aside
        className={`${selected ? "hidden md:block" : "block"} min-h-0 overflow-y-auto border-r border-border/70`}
      >
        {list}
      </aside>
      <section
        className={`${selected ? "block" : "hidden md:block"} min-h-0 overflow-y-auto`}
      >
        {detail}
      </section>
    </div>
  );
}

function PanelHeader({
  title,
  onReload,
}: {
  title: string;
  onReload: () => void;
}) {
  const t = useTranslations("SkillStore");
  return (
    <div className="sticky top-0 z-10 flex items-center justify-between border-b bg-background/95 px-4 py-3 backdrop-blur">
      <h2 className="font-semibold">{title}</h2>
      <button
        type="button"
        onClick={onReload}
        aria-label={t("reloadAria", { title })}
        className="rounded-lg p-2 text-muted-foreground hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500"
      >
        <RefreshCw size={16} aria-hidden="true" />
      </button>
    </div>
  );
}

function DetailFrame({
  title,
  onBack,
  children,
}: {
  title: string;
  onBack: () => void;
  children: ReactNode;
}) {
  const t = useTranslations("SkillStore");
  return (
    <div className="mx-auto max-w-5xl space-y-5 p-4 md:p-6">
      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={onBack}
          aria-label={t("backToList")}
          className="rounded-lg p-2 hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500 md:hidden"
        >
          <ArrowLeft size={18} aria-hidden="true" />
        </button>
        <h2 className="min-w-0 truncate text-lg font-bold">{title}</h2>
      </div>
      {children}
    </div>
  );
}

function DetailGrid({ rows }: { rows: Array<[string, string]> }) {
  return (
    <dl className="grid gap-3 rounded-xl border bg-card p-4 sm:grid-cols-2">
      {rows.map(([label, value]) => (
        <div key={label}>
          <dt className="text-xs text-muted-foreground">{label}</dt>
          <dd className="mt-1 break-words text-sm font-medium">{value}</dd>
        </div>
      ))}
    </dl>
  );
}

function StatusPill({ value }: { value: string }) {
  return (
    <span className="inline-flex rounded-full border border-slate-300 bg-slate-100 px-2 py-0.5 text-[11px] font-medium text-slate-700 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200">
      {value}
    </span>
  );
}

function Loading() {
  const t = useTranslations("SkillStore");
  return (
    <div className="flex items-center gap-2 p-4 text-sm text-muted-foreground">
      <Loader2 size={16} className="animate-spin" aria-hidden="true" />
      {t("loading")}
    </div>
  );
}

function InlineError({
  message,
  retry,
}: {
  message: string;
  retry: () => void;
}) {
  const t = useTranslations("SkillStore");
  return (
    <div
      role="alert"
      className="rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-800 dark:border-red-900 dark:bg-red-950/25 dark:text-red-200"
    >
      <p>{message}</p>
      <button
        type="button"
        onClick={retry}
        className="mt-2 font-medium underline"
      >
        {t("retry")}
      </button>
    </div>
  );
}

function EmptyState({ title }: { title: string }) {
  return (
    <div className="rounded-xl border border-dashed p-6 text-center text-sm text-muted-foreground">
      {title}
    </div>
  );
}

function EmptyDetail({ title }: { title: string }) {
  return (
    <div className="flex h-full items-center justify-center p-6">
      <EmptyState title={title} />
    </div>
  );
}

function closeDetail(
  onNavigate: (id: string | null, historyMode?: "push" | "replace") => void,
  restoreFocus: React.RefObject<HTMLButtonElement | null>,
) {
  onNavigate(null);
  requestAnimationFrame(() =>
    restoreFocus.current?.focus({ preventScroll: true }),
  );
}

function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error && error.message ? error.message : fallback;
}

function isStale(error: unknown) {
  return (
    error instanceof ApiClientError &&
    (error.code === "SKILL_REVISION_CONFLICT" ||
      error.code === "SKILL_PACKAGE_CHANGED")
  );
}
