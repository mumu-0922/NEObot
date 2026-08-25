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
  Trash2,
  X,
} from "lucide-react";
import { useTranslations } from "next-intl";

import {
  ApiClientError,
  createNeoChatApiClient,
  type AgentPackageCandidateDTO,
  type AgentPackageInstallationDTO,
} from "@/services/api/client";

interface SkillStoreProps {
  selectedId: string | null;
  initialQuery?: string;
  onNavigate: (id: string | null, historyMode?: "push" | "replace") => void;
  onClose: () => void;
}

export default function SkillStore({
  selectedId,
  initialQuery = "",
  onNavigate,
  onClose,
}: SkillStoreProps) {
  const t = useTranslations("SkillStore");
  const client = useMemo(() => createNeoChatApiClient(), []);
  const [items, setItems] = useState<AgentPackageCandidateDTO[]>([]);
  const [installed, setInstalled] = useState<AgentPackageInstallationDTO[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [actionId, setActionId] = useState("");
  const [announcement, setAnnouncement] = useState("");
  const [query, setQuery] = useState(initialQuery);
  const [selectedFallback, setSelectedFallback] =
    useState<AgentPackageCandidateDTO | null>(null);
  const restoreFocus = useRef<HTMLButtonElement | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [store, library] = await Promise.all([
        client.skillStore.listPackageStore(),
        client.skillStore.listPackageLibrary(),
      ]);
      setItems(store.items);
      setInstalled(library);
    } catch (loadError) {
      setError(errorMessage(loadError, t("loadFailed")));
    } finally {
      setLoading(false);
    }
  }, [client.skillStore, t]);

  useEffect(() => {
    queueMicrotask(() => void load());
  }, [load]);

  useEffect(() => setQuery(initialQuery), [initialQuery]);

  const listedSelected = items.find((item) => item.id === selectedId) ?? null;
  useEffect(() => {
    const controller = new AbortController();
    if (!selectedId || listedSelected) {
      setSelectedFallback(null);
      return () => controller.abort();
    }
    void client.skillStore
      .getPackageSkill(selectedId, { signal: controller.signal })
      .then(setSelectedFallback)
      .catch((loadError) => {
        if (!controller.signal.aborted) {
          setError(errorMessage(loadError, t("loadFailed")));
        }
      });
    return () => controller.abort();
  }, [client.skillStore, listedSelected, selectedId, t]);

  const selected = listedSelected ?? selectedFallback;
  const installedByAdmission = new Map(
    installed.map((item) => [item.admissionId, item]),
  );
  const normalizedQuery = query.trim().toLowerCase();
  const filteredItems = normalizedQuery
    ? items.filter((item) =>
        [
          item.id,
          item.package.name,
          item.package.description,
          item.package.version,
        ].some((value) => value.toLowerCase().includes(normalizedQuery)),
      )
    : items;

  const install = async (item: AgentPackageCandidateDTO) => {
    setActionId(item.id);
    try {
      await client.skillStore.installPackageSkill({
        candidateId: item.id,
        packageFingerprint: item.package.packageFingerprint,
      });
      setAnnouncement(t("installedAnnouncement", { name: item.package.name }));
      await load();
    } catch (actionError) {
      setAnnouncement(errorMessage(actionError, t("actionFailed")));
      if (isStale(actionError)) await load();
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
      await load();
    } catch (actionError) {
      setAnnouncement(errorMessage(actionError, t("actionFailed")));
      if (isStale(actionError)) await load();
    } finally {
      setActionId("");
    }
  };

  return (
    <div className="flex h-full w-full flex-col overflow-hidden bg-slate-50/80 dark:bg-background">
      <a className="skip-link" href="#skill-store-content">
        {t("skipToContent")}
      </a>
      <header className="flex shrink-0 items-center justify-between gap-3 border-b border-slate-200/80 bg-white/95 px-4 py-4 dark:border-border dark:bg-card/95 md:px-6">
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
      </header>

      <div id="skill-store-content" tabIndex={-1} className="min-h-0 flex-1">
        <SplitShell
          selected={Boolean(selected)}
          list={
            <>
              <PanelHeader
                title={t("packageSkills")}
                onReload={() => void load()}
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
                <section>
                  <h2 className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                    {t("installedPackages", { count: installed.length })}
                  </h2>
                  {installed.length ? (
                    <div className="space-y-2">
                      {installed.map((entry) => (
                        <div
                          key={entry.id}
                          className="rounded-xl border bg-card p-3"
                        >
                          <div className="flex items-start justify-between gap-2">
                            <div>
                              <p className="font-medium">{entry.name}</p>
                              <p className="text-xs text-muted-foreground">
                                v{entry.version}
                              </p>
                            </div>
                            <button
                              type="button"
                              disabled={actionId === entry.id}
                              onClick={() => void uninstall(entry)}
                              aria-label={t("uninstallAria", {
                                name: entry.name,
                              })}
                              className="rounded-lg p-2 text-red-600 hover:bg-red-50 disabled:opacity-50 dark:hover:bg-red-950/30"
                            >
                              <Trash2 size={15} aria-hidden="true" />
                            </button>
                          </div>
                          <Fingerprint value={entry.packageFingerprint} />
                        </div>
                      ))}
                    </div>
                  ) : (
                    <p className="text-sm text-muted-foreground">
                      {t("noInstalledPackages")}
                    </p>
                  )}
                </section>

                <section>
                  <h2 className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                    {t("packageStore")}
                  </h2>
                  {loading ? (
                    <Loading />
                  ) : error ? (
                    <InlineError message={error} retry={() => void load()} />
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
                            <span className="font-medium">
                              {item.package.name}
                            </span>
                            <StatusPill value={item.status} />
                          </div>
                          <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">
                            {item.package.description}
                          </p>
                        </button>
                      ))}
                    </div>
                  ) : (
                    <EmptyState title={t("emptyStore")} />
                  )}
                </section>
              </div>
            </>
          }
          detail={
            selected ? (
              <DetailFrame
                title={selected.package.name}
                onBack={() => closeDetail(onNavigate, restoreFocus)}
              >
                <p className="text-sm text-muted-foreground">
                  {selected.package.description}
                </p>
                <DetailGrid
                  rows={[
                    [t("version"), selected.package.version],
                    [t("source"), selected.sourceType],
                    [
                      t("runtime"),
                      selected.package.hasRuntime
                        ? t("localDirectPackage")
                        : t("textOnlyPackage"),
                    ],
                    [t("validation"), selected.validationSummary],
                  ]}
                />
                <Fingerprint
                  label={t("packageFingerprint")}
                  value={selected.package.packageFingerprint}
                  full
                />
                <Fingerprint
                  label={t("runtimeFingerprint")}
                  value={selected.package.runtimeBundleFingerprint ?? t("none")}
                  full
                />
                <Fingerprint
                  label={t("sbomFingerprint")}
                  value={selected.package.sbomFingerprint}
                  full
                />
                <div>
                  <h3 className="text-sm font-semibold">
                    {t("declaredTools")}
                  </h3>
                  <p className="mt-1 text-sm text-muted-foreground">
                    {selected.package.allowedTools.join(", ") || t("none")}
                  </p>
                </div>
                <button
                  type="button"
                  disabled={
                    Boolean(installedByAdmission.get(selected.id)) ||
                    actionId === selected.id
                  }
                  onClick={() => void install(selected)}
                  className="inline-flex items-center gap-2 rounded-xl bg-slate-950 px-4 py-2.5 text-sm font-medium text-white disabled:cursor-not-allowed disabled:opacity-50 dark:bg-cyan-500 dark:text-slate-950"
                >
                  {actionId === selected.id ? (
                    <Loader2 size={16} className="animate-spin" />
                  ) : (
                    <Download size={16} />
                  )}
                  {installedByAdmission.has(selected.id)
                    ? t("installed")
                    : t("installPackage")}
                </button>
              </DetailFrame>
            ) : (
              <EmptyDetail title={t("selectPackage")} />
            )
          }
        />
      </div>
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

function Fingerprint({
  value,
  label,
  full = false,
}: {
  value: string;
  label?: string;
  full?: boolean;
}) {
  const shown =
    full || value.length <= 24
      ? value
      : `${value.slice(0, 18)}…${value.slice(-6)}`;
  return (
    <div className="mt-2 min-w-0">
      <span className="block text-[11px] text-muted-foreground">{label}</span>
      <code
        className="block break-all text-[11px] text-muted-foreground"
        title={value}
      >
        {shown}
      </code>
    </div>
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
    error instanceof ApiClientError && error.code === "SKILL_REVISION_CONFLICT"
  );
}
