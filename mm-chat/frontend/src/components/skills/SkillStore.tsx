"use client";

import { useRef } from "react";
import { Download, ExternalLink, Link2, Loader2, Search } from "lucide-react";
import { useTranslations } from "next-intl";

import type { SkillMarketplaceSortDTO } from "@/services/api/client";

import {
  CategoryButton,
  closeDetail,
  DetailFrame,
  DetailGrid,
  EmptyDetail,
  EmptyState,
  InlineError,
  InstalledSkills,
  Loading,
  PanelHeader,
  SkillStoreShell,
  SplitShell,
  StatusPill,
} from "./SkillStorePrimitives";
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
  const {
    actionId,
    announcement,
    catalogDetail,
    categories,
    category,
    detailError,
    detailLoading,
    directInstalling,
    directURL,
    filteredCatalog,
    installCatalog,
    installLink,
    installMarketplace,
    installed,
    installedError,
    installedFingerprints,
    installedLoading,
    loadingMore,
    loadInstalled,
    loadMarketplace,
    marketDetail,
    marketItems,
    marketPage,
    marketSort,
    marketTotalPages,
    query,
    reloadStore,
    selectedKey,
    setCategory,
    setDetailReload,
    setDirectURL,
    setQuery,
    setMarketSort,
    setSource,
    setTab,
    source,
    storeError,
    storeLoading,
    submitSearch,
    tab,
    uninstall,
  } = useSkillStore({ selectedId, initialQuery, onNavigate });

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
        <SplitShell
          selected={Boolean(selectedKey)}
          list={
            <>
              <PanelHeader
                title={t("packageStore")}
                onReload={() => void reloadStore()}
              />
              <div className="space-y-4 p-4">
                <form
                  onSubmit={installLink}
                  className="space-y-2 rounded-xl border bg-slate-50 p-3 dark:bg-slate-950/30"
                >
                  <div className="flex items-center gap-2 text-sm font-semibold">
                    <Link2 size={15} aria-hidden="true" />
                    {t("installFromLink")}
                  </div>
                  <div className="flex gap-2">
                    <input
                      type="url"
                      required
                      value={directURL}
                      onChange={(event) => setDirectURL(event.target.value)}
                      placeholder={t("linkPlaceholder")}
                      aria-label={t("linkLabel")}
                      className="min-w-0 flex-1 rounded-lg border bg-card px-3 py-2 text-xs outline-none focus:border-cyan-500 focus:ring-2 focus:ring-cyan-500/20"
                    />
                    <button
                      type="submit"
                      disabled={directInstalling}
                      className="rounded-lg bg-slate-950 px-3 py-2 text-xs font-medium text-white disabled:opacity-50 dark:bg-cyan-500 dark:text-slate-950"
                    >
                      {directInstalling ? (
                        <Loader2 size={14} className="animate-spin" />
                      ) : (
                        t("install")
                      )}
                    </button>
                  </div>
                  <p className="text-[11px] text-muted-foreground">
                    {t("linkHelp")}
                  </p>
                </form>

                <div
                  role="tablist"
                  aria-label={t("sourceTabs")}
                  className="grid grid-cols-2 rounded-xl bg-slate-100 p-1 dark:bg-slate-900"
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
                      className={`rounded-lg px-2 py-2 text-xs font-medium ${
                        source === value
                          ? "bg-white shadow-sm dark:bg-slate-800"
                          : "text-muted-foreground"
                      }`}
                    >
                      {value === "lobehub"
                        ? t("lobehubSource")
                        : t("openaiSource")}
                    </button>
                  ))}
                </div>

                <form onSubmit={submitSearch} className="flex gap-2">
                  <label className="relative min-w-0 flex-1">
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
                      placeholder={
                        source === "lobehub"
                          ? t("marketplaceSearchPlaceholder")
                          : t("searchPlaceholder")
                      }
                      className="w-full rounded-xl border bg-card py-2 pr-3 pl-9 text-sm outline-none focus:border-cyan-500 focus:ring-2 focus:ring-cyan-500/20"
                    />
                  </label>
                  {source === "lobehub" ? (
                    <button
                      type="submit"
                      className="rounded-xl border px-3 py-2 text-xs font-medium hover:bg-accent"
                    >
                      {t("search")}
                    </button>
                  ) : null}
                </form>

                {source === "lobehub" ? (
                  <label className="flex items-center gap-2 text-xs text-muted-foreground">
                    <span>{t("sortLabel")}</span>
                    <select
                      value={marketSort}
                      onChange={(event) => {
                        const nextSort = event.target
                          .value as SkillMarketplaceSortDTO;
                        setMarketSort(nextSort);
                        void loadMarketplace(
                          query,
                          category,
                          1,
                          false,
                          nextSort,
                        );
                      }}
                      className="min-w-0 flex-1 rounded-lg border bg-card px-3 py-2 text-foreground outline-none focus:border-cyan-500 focus:ring-2 focus:ring-cyan-500/20"
                    >
                      <option value="relevance">{t("sortRelevance")}</option>
                      <option value="recommended">
                        {t("sortRecommended")}
                      </option>
                      <option value="installCount">
                        {t("sortInstallCount")}
                      </option>
                      <option value="ratingAverage">{t("sortRating")}</option>
                      <option value="updatedAt">{t("sortUpdated")}</option>
                    </select>
                  </label>
                ) : null}

                {source === "lobehub" && categories.length ? (
                  <div className="flex flex-wrap gap-1.5">
                    <CategoryButton
                      active={!category}
                      label={t("allCategories")}
                      onClick={() => {
                        setCategory("");
                        void loadMarketplace(query, "", 1, false);
                      }}
                    />
                    {categories.map((item) => (
                      <CategoryButton
                        key={item.category}
                        active={category === item.category}
                        label={`${item.category} · ${item.count}`}
                        onClick={() => {
                          setCategory(item.category);
                          void loadMarketplace(query, item.category, 1, false);
                        }}
                      />
                    ))}
                  </div>
                ) : null}

                {storeLoading ? (
                  <Loading />
                ) : storeError ? (
                  <InlineError
                    message={storeError}
                    retry={() => void reloadStore()}
                  />
                ) : source === "lobehub" ? (
                  marketItems.length ? (
                    <>
                      <div className="space-y-2">
                        {marketItems.map((item) => {
                          const key = marketSkillKey(item.identifier);
                          return (
                            <button
                              key={item.identifier}
                              type="button"
                              onClick={(event) => {
                                restoreFocus.current = event.currentTarget;
                                onNavigate(key);
                              }}
                              aria-current={
                                selectedId === key ? "true" : undefined
                              }
                              className={`w-full rounded-xl border p-3 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500 ${
                                selectedId === key
                                  ? "border-cyan-500 bg-cyan-50 dark:bg-cyan-950/20"
                                  : "bg-card hover:border-slate-300 dark:hover:border-slate-700"
                              }`}
                            >
                              <div className="flex items-start justify-between gap-2">
                                <span className="font-medium">{item.name}</span>
                                <StatusPill
                                  value={
                                    item.official
                                      ? t("official")
                                      : item.validated
                                        ? t("validated")
                                        : `v${item.version}`
                                  }
                                />
                              </div>
                              <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">
                                {item.description}
                              </p>
                              <p className="mt-2 text-[11px] text-muted-foreground">
                                {item.author || item.identifier} ·{" "}
                                {t("installs", {
                                  count: item.installCount,
                                })}
                              </p>
                            </button>
                          );
                        })}
                      </div>
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
                          className="w-full rounded-xl border px-3 py-2 text-sm hover:bg-accent disabled:opacity-50"
                        >
                          {loadingMore ? t("loading") : t("loadMore")}
                        </button>
                      ) : null}
                    </>
                  ) : (
                    <EmptyState title={t("emptyMarketplace")} />
                  )
                ) : filteredCatalog.length ? (
                  <div className="space-y-2">
                    {filteredCatalog.map((item) => (
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
            marketDetail ? (
              <DetailFrame
                title={marketDetail.name}
                onBack={() => closeDetail(onNavigate, restoreFocus)}
              >
                <p className="text-sm text-muted-foreground">
                  {marketDetail.summary || marketDetail.description}
                </p>
                <DetailGrid
                  rows={[
                    [t("version"), marketDetail.version],
                    [t("source"), "LobeHub Marketplace"],
                    [t("identifier"), marketDetail.identifier],
                    [t("author"), marketDetail.author || t("none")],
                    [t("license"), marketDetail.license || t("none")],
                    [t("resources"), String(marketDetail.resources.length)],
                  ]}
                />
                <a
                  href={marketDetail.sourceUrl}
                  target="_blank"
                  rel="noreferrer"
                  className="inline-flex items-center gap-1 text-sm font-medium text-cyan-700 underline-offset-4 hover:underline dark:text-cyan-300"
                >
                  {t("viewMarketplace")}
                  <ExternalLink size={14} aria-hidden="true" />
                </a>
                <div>
                  <h3 className="text-sm font-semibold">{t("permissions")}</h3>
                  <p className="mt-1 text-sm text-muted-foreground">
                    {marketDetail.permissions.join(", ") || t("none")}
                  </p>
                </div>
                <button
                  type="button"
                  disabled={
                    marketDetail.installed ||
                    actionId === marketDetail.identifier
                  }
                  onClick={() => void installMarketplace(marketDetail)}
                  className="inline-flex items-center gap-2 rounded-xl bg-slate-950 px-4 py-2.5 text-sm font-medium text-white disabled:cursor-not-allowed disabled:opacity-50 dark:bg-cyan-500 dark:text-slate-950"
                >
                  {actionId === marketDetail.identifier ? (
                    <Loader2 size={16} className="animate-spin" />
                  ) : (
                    <Download size={16} />
                  )}
                  {marketDetail.installed
                    ? t("installed")
                    : t("installPackage")}
                </button>
              </DetailFrame>
            ) : catalogDetail ? (
              <DetailFrame
                title={catalogDetail.name}
                onBack={() => closeDetail(onNavigate, restoreFocus)}
              >
                <p className="text-sm text-muted-foreground">
                  {catalogDetail.description}
                </p>
                <DetailGrid
                  rows={[
                    [t("version"), catalogDetail.version],
                    [t("source"), catalogDetail.catalogSource],
                    [t("sourcePath"), catalogDetail.path],
                    [
                      t("runtime"),
                      catalogDetail.hasRuntime
                        ? t("localDirectPackage")
                        : t("textOnlyPackage"),
                    ],
                    [
                      t("compatibility"),
                      catalogDetail.compatibility || t("none"),
                    ],
                    [t("license"), catalogDetail.license || t("none")],
                  ]}
                />
                <a
                  href={catalogDetail.sourceUrl}
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
                    {catalogDetail.allowedTools.join(", ") || t("none")}
                  </p>
                </div>
                <button
                  type="button"
                  disabled={
                    installedFingerprints.has(
                      catalogDetail.packageFingerprint,
                    ) || actionId === catalogDetail.id
                  }
                  onClick={() => void installCatalog(catalogDetail)}
                  className="inline-flex items-center gap-2 rounded-xl bg-slate-950 px-4 py-2.5 text-sm font-medium text-white disabled:cursor-not-allowed disabled:opacity-50 dark:bg-cyan-500 dark:text-slate-950"
                >
                  {actionId === catalogDetail.id ? (
                    <Loader2 size={16} className="animate-spin" />
                  ) : (
                    <Download size={16} />
                  )}
                  {installedFingerprints.has(catalogDetail.packageFingerprint)
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
      }
    />
  );
}
