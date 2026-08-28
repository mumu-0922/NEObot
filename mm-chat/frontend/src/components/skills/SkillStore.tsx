"use client";

import { useRef } from "react";
import {
  Download,
  ExternalLink,
  Link2,
  Loader2,
  RefreshCw,
  Search,
} from "lucide-react";
import { useTranslations } from "next-intl";

import type { SkillMarketplaceSortDTO } from "@/services/api/client";

import {
  closeDetail,
  EmptyState,
  InlineError,
  InstalledSkills,
  Loading,
  SkillStoreShell,
} from "./SkillStorePrimitives";
import { SkillDetailDialog } from "./SkillDetailDialog";
import {
  CuratedSkillCard,
  MarketplaceCategoryNav,
  MarketplaceSkillCard,
} from "./SkillMarketplacePrimitives";
import { marketSkillKey, useSkillStore } from "./useSkillStore";

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
  const restoreFocus = useRef<HTMLButtonElement | null>(null);
  const controller = useSkillStore({ selectedId, initialQuery, onNavigate });
  const {
    actionId,
    announcement,
    categories,
    category,
    directInstalling,
    directURL,
    filteredCatalog,
    installLink,
    installed,
    installedError,
    installedLoading,
    loadingMore,
    loadInstalled,
    loadMarketplace,
    marketItems,
    marketPage,
    marketSort,
    marketSourceURL,
    marketTotalCount,
    marketTotalPages,
    query,
    reloadStore,
    setCategory,
    setDirectURL,
    setMarketSort,
    setQuery,
    setSource,
    setTab,
    source,
    storeError,
    storeLoading,
    submitSearch,
    tab,
    uninstall,
  } = controller;

  const dismissDetail = () => closeDetail(onNavigate, restoreFocus);

  return (
    <SkillStoreShell
      tab={tab}
      onTabChange={(nextTab) => {
        if (nextTab === "installed") onNavigate(null);
        setTab(nextTab);
      }}
      onClose={onClose}
      announcement={announcement}
      installed={
        <InstalledSkills
          items={installed}
          loading={installedLoading}
          error={installedError}
          actionId={actionId}
          onReload={() => void loadInstalled()}
          onUninstall={uninstall}
        />
      }
      store={
        <div className="flex h-full min-h-0 flex-col">
          <div className="shrink-0 space-y-3 border-b border-gray-200 p-4 dark:border-border">
            <div className="flex flex-col gap-3 xl:flex-row xl:items-center">
              <div
                role="tablist"
                aria-label={t("sourceTabs")}
                className="grid shrink-0 grid-cols-2 rounded-lg bg-slate-100 p-1 dark:bg-slate-900"
              >
                {(["lobehub", "openai"] as const).map((value) => (
                  <button
                    key={value}
                    type="button"
                    role="tab"
                    aria-selected={source === value}
                    onClick={() => {
                      setSource(value);
                      onNavigate(null);
                    }}
                    className={`rounded-md px-3 py-2 text-xs font-medium transition-colors ${
                      source === value
                        ? "bg-white text-cyan-700 shadow-sm dark:bg-slate-800 dark:text-cyan-300"
                        : "text-muted-foreground hover:text-foreground"
                    }`}
                  >
                    {value === "lobehub"
                      ? t("lobehubSource")
                      : t("openaiSource")}
                  </button>
                ))}
              </div>

              <form
                onSubmit={installLink}
                className="flex min-w-0 flex-1 items-center gap-2"
              >
                <label className="relative min-w-0 flex-1">
                  <Link2
                    size={15}
                    aria-hidden="true"
                    className="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-muted-foreground"
                  />
                  <span className="sr-only">{t("linkLabel")}</span>
                  <input
                    type="url"
                    required
                    value={directURL}
                    onChange={(event) => setDirectURL(event.target.value)}
                    placeholder={t("linkPlaceholder")}
                    className="h-10 w-full rounded-lg border bg-card pr-3 pl-9 text-sm outline-none transition-colors focus:border-cyan-500"
                  />
                </label>
                <button
                  type="submit"
                  disabled={directInstalling}
                  className="inline-flex h-10 shrink-0 items-center gap-2 rounded-lg border px-3 text-xs font-medium hover:border-cyan-300 hover:text-cyan-700 disabled:opacity-50"
                >
                  {directInstalling ? (
                    <Loader2 size={14} className="animate-spin" />
                  ) : (
                    <Download size={14} aria-hidden="true" />
                  )}
                  {t("install")}
                </button>
              </form>

              <button
                type="button"
                onClick={() => void reloadStore()}
                aria-label={t("reloadAria", { title: t("packageStore") })}
                className="hidden h-10 w-10 shrink-0 items-center justify-center rounded-lg border text-muted-foreground hover:border-cyan-300 hover:text-cyan-700 xl:inline-flex"
              >
                <RefreshCw size={16} aria-hidden="true" />
              </button>
            </div>

            <form onSubmit={submitSearch} className="flex gap-2">
              <label className="relative min-w-0 flex-1">
                <Search
                  size={15}
                  aria-hidden="true"
                  className="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-muted-foreground"
                />
                <span className="sr-only">{t("searchLabel")}</span>
                <input
                  type="search"
                  maxLength={200}
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  placeholder={
                    source === "lobehub"
                      ? t("marketplaceSearchPlaceholder")
                      : t("searchPlaceholder")
                  }
                  className="h-10 w-full rounded-lg border bg-card pr-3 pl-9 text-sm outline-none transition-colors focus:border-cyan-500"
                />
              </label>
              {source === "lobehub" ? (
                <>
                  <select
                    aria-label={t("sortLabel")}
                    value={marketSort}
                    onChange={(event) => {
                      const nextSort = event.target
                        .value as SkillMarketplaceSortDTO;
                      setMarketSort(nextSort);
                      void loadMarketplace(query, category, 1, false, nextSort);
                    }}
                    className="hidden h-10 rounded-lg border bg-card px-3 text-xs text-foreground outline-none focus:border-cyan-500 sm:block"
                  >
                    <option value="relevance">{t("sortRelevance")}</option>
                    <option value="recommended">{t("sortRecommended")}</option>
                    <option value="installCount">
                      {t("sortInstallCount")}
                    </option>
                    <option value="ratingAverage">{t("sortRating")}</option>
                    <option value="updatedAt">{t("sortUpdated")}</option>
                  </select>
                  <button
                    type="submit"
                    disabled={storeLoading}
                    className="inline-flex h-10 shrink-0 items-center gap-2 rounded-lg bg-cyan-600 px-4 text-sm font-medium text-white disabled:opacity-50"
                  >
                    {storeLoading ? (
                      <Loader2 size={15} className="animate-spin" />
                    ) : null}
                    {t("search")}
                  </button>
                </>
              ) : null}
            </form>
          </div>

          <div className="flex min-h-0 flex-1 flex-col md:flex-row">
            {source === "lobehub" ? (
              <MarketplaceCategoryNav
                categories={categories}
                activeCategory={category}
                totalCount={marketTotalCount}
                disabled={storeLoading}
                onSelect={(nextCategory) => {
                  setCategory(nextCategory);
                  void loadMarketplace(query, nextCategory, 1, false);
                }}
              />
            ) : null}

            <div className="min-h-0 flex-1 overflow-y-auto p-4 custom-scrollbar">
              {storeError ? (
                <InlineError
                  message={storeError}
                  retry={() => void reloadStore()}
                />
              ) : storeLoading ? (
                <div className="flex justify-center py-16">
                  <Loading />
                </div>
              ) : source === "lobehub" ? (
                marketItems.length ? (
                  <>
                    <div className="mb-3 flex items-center justify-between gap-3 text-xs text-muted-foreground">
                      <span>
                        {t("marketplaceResults", {
                          loaded: marketItems.length,
                          count: marketTotalCount,
                        })}
                      </span>
                      <a
                        href={marketSourceURL}
                        target="_blank"
                        rel="noreferrer noopener"
                        className="inline-flex shrink-0 items-center gap-1 hover:text-cyan-600"
                      >
                        LobeHub <ExternalLink size={11} aria-hidden="true" />
                      </a>
                    </div>
                    <div className="grid gap-3 md:grid-cols-2">
                      {marketItems.map((item) => {
                        const key = marketSkillKey(item.identifier);
                        return (
                          <MarketplaceSkillCard
                            key={item.identifier}
                            item={item}
                            active={selectedId === key}
                            onOpen={(button) => {
                              restoreFocus.current = button;
                              onNavigate(key);
                            }}
                          />
                        );
                      })}
                    </div>
                    <div className="flex min-h-16 items-center justify-center py-4">
                      {marketPage < marketTotalPages ? (
                        <button
                          type="button"
                          disabled={loadingMore}
                          onClick={() =>
                            void loadMarketplace(
                              query,
                              category,
                              marketPage + 1,
                              true,
                            )
                          }
                          className="inline-flex items-center gap-2 rounded-lg border px-4 py-2 text-xs font-medium hover:border-cyan-300 hover:text-cyan-700 disabled:opacity-50"
                        >
                          {loadingMore ? (
                            <Loader2 size={14} className="animate-spin" />
                          ) : null}
                          {loadingMore ? t("loading") : t("loadMore")}
                        </button>
                      ) : null}
                    </div>
                  </>
                ) : (
                  <div className="py-12">
                    <EmptyState title={t("emptyMarketplace")} />
                  </div>
                )
              ) : filteredCatalog.length ? (
                <div className="grid gap-3 md:grid-cols-2">
                  {filteredCatalog.map((item) => (
                    <CuratedSkillCard
                      key={item.id}
                      item={item}
                      active={selectedId === item.id}
                      onOpen={(button) => {
                        restoreFocus.current = button;
                        onNavigate(item.id);
                      }}
                    />
                  ))}
                </div>
              ) : (
                <div className="py-12">
                  <EmptyState title={t("emptyStore")} />
                </div>
              )}
            </div>
          </div>

          <SkillDetailDialog
            controller={controller}
            selectedId={selectedId}
            onClose={dismissDetail}
          />
        </div>
      }
    />
  );
}
