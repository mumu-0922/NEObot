"use client";

import { useCallback, useState, type ReactNode } from "react";
import { Store, Wrench, X } from "lucide-react";
import { useTranslations } from "next-intl";

import McpMarketplace from "./McpMarketplace";
import McpToolsControl from "./McpToolsControl";

interface McpToolsPageProps {
  conversationId?: string;
  enabled: boolean;
  onClose: () => void;
}

export default function McpToolsPage({
  conversationId,
  enabled,
  onClose,
}: McpToolsPageProps) {
  const t = useTranslations("Mcp");
  const [tab, setTab] = useState<"installed" | "marketplace">("installed");
  const [installedRevision, setInstalledRevision] = useState(0);
  const installed = useCallback(() => {
    setInstalledRevision((revision) => revision + 1);
    setTab("installed");
  }, []);

  return (
    <div className="flex h-full w-full flex-col overflow-hidden bg-gray-50/50 dark:bg-background">
      <header className="shrink-0 border-b border-gray-200/50 bg-white/95 px-6 pt-4 dark:border-border dark:bg-card/95">
        <div className="flex items-center justify-between gap-3 pb-4">
          <div className="flex min-w-0 items-center gap-3">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-linear-to-tr from-cyan-600 to-blue-600 text-white shadow-lg shadow-cyan-500/20">
              <Wrench size={20} aria-hidden="true" />
            </div>
            <div className="min-w-0">
              <h1 className="truncate text-lg font-bold text-gray-800 dark:text-foreground">
                {t("title")}
              </h1>
              <p className="mt-0.5 truncate text-xs text-gray-500 dark:text-muted-foreground">
                {t("pageSubtitle")}
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label={t("close")}
            className="shrink-0 rounded-full p-2 text-gray-500 transition-colors hover:bg-gray-200/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/60 dark:text-muted-foreground dark:hover:bg-accent/50"
          >
            <X size={20} aria-hidden="true" />
          </button>
        </div>
        <div role="tablist" aria-label={t("pageTabs")} className="flex gap-1">
          <PageTab
            active={tab === "installed"}
            label={t("installedTab")}
            icon={<Wrench size={14} />}
            onClick={() => setTab("installed")}
          />
          <PageTab
            active={tab === "marketplace"}
            label={t("marketplaceTab")}
            icon={<Store size={14} />}
            onClick={() => setTab("marketplace")}
          />
        </div>
      </header>
      <div className="mx-auto min-h-0 w-full max-w-7xl flex-1 p-4 md:p-6">
        <section className="relative h-full overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-border dark:bg-card">
          {tab === "installed" ? (
            <McpToolsControl
              key={installedRevision}
              conversationId={conversationId}
              enabled={enabled}
              variant="embedded"
            />
          ) : (
            <McpMarketplace
              conversationId={conversationId}
              enabled={enabled}
              onInstalled={installed}
            />
          )}
        </section>
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
