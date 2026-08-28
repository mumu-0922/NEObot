"use client";

import type { ReactNode } from "react";
import {
  Loader2,
  PackageCheck,
  RefreshCw,
  Store,
  Trash2,
  X,
} from "lucide-react";
import { useTranslations } from "next-intl";

import type { AgentPackageInstallationDTO } from "@/services/api/client";

import { SkillIcon } from "./SkillIcon";

export function SkillStoreShell({
  tab,
  onTabChange,
  onClose,
  installed,
  store,
  announcement,
}: {
  tab: "installed" | "store";
  onTabChange: (tab: "installed" | "store") => void;
  onClose: () => void;
  installed: ReactNode;
  store: ReactNode;
  announcement: string;
}) {
  const t = useTranslations("SkillStore");
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
            onClick={() => onTabChange("installed")}
          />
          <PageTab
            active={tab === "store"}
            label={t("storeTab")}
            icon={<Store size={14} />}
            onClick={() => onTabChange("store")}
          />
        </div>
      </header>
      <main
        id="skill-store-content"
        tabIndex={-1}
        className="mx-auto min-h-0 w-full max-w-7xl flex-1 p-4 md:p-6"
      >
        <section className="relative h-full overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm dark:border-border dark:bg-card">
          {tab === "installed" ? installed : store}
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

export function PageTab({
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

export function InstalledSkills({
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
                  <div className="flex min-w-0 items-start gap-3">
                    <SkillIcon label={entry.name} />
                    <div className="min-w-0">
                      <h2 className="font-semibold">{entry.name}</h2>
                      <p className="mt-1 text-sm text-muted-foreground">
                        {entry.description}
                      </p>
                      {entry.allowedTools.length ? (
                        <p className="mt-2 text-xs text-muted-foreground">
                          {t("declaredTools")}: {entry.allowedTools.join(", ")}
                        </p>
                      ) : null}
                    </div>
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

export function PanelHeader({
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

export function DetailGrid({ rows }: { rows: Array<[string, string]> }) {
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

export function StatusPill({ value }: { value: string }) {
  return (
    <span className="inline-flex shrink-0 rounded-full border border-slate-300 bg-slate-100 px-2 py-0.5 text-[11px] font-medium text-slate-700 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200">
      {value}
    </span>
  );
}

export function Loading() {
  const t = useTranslations("SkillStore");
  return (
    <div className="flex items-center gap-2 p-4 text-sm text-muted-foreground">
      <Loader2 size={16} className="animate-spin" aria-hidden="true" />
      {t("loading")}
    </div>
  );
}

export function InlineError({
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

export function EmptyState({ title }: { title: string }) {
  return (
    <div className="rounded-xl border border-dashed p-6 text-center text-sm text-muted-foreground">
      {title}
    </div>
  );
}

export function closeDetail(
  onNavigate: (id: string | null, historyMode?: "push" | "replace") => void,
  restoreFocus: React.RefObject<HTMLButtonElement | null>,
) {
  onNavigate(null);
  requestAnimationFrame(() =>
    restoreFocus.current?.focus({ preventScroll: true }),
  );
}
