"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type FormEvent,
} from "react";
import { useLocale, useTranslations } from "next-intl";

import {
  ApiClientError,
  createNeoChatApiClient,
  type AgentPackageInstallationDTO,
  type SkillCatalogItemDTO,
  type SkillCatalogSummaryDTO,
  type SkillMarketplaceCategoryDTO,
  type SkillMarketplaceDetailDTO,
  type SkillMarketplaceLocaleDTO,
  type SkillMarketplaceSortDTO,
  type SkillMarketplaceSummaryDTO,
} from "@/services/api/client";

export type SkillStoreTab = "installed" | "store";
export type StoreSource = "lobehub" | "openai";

const MARKETPLACE_PAGE_SIZE = 20;
const LOBEHUB_KEY_PREFIX = "lobehub:";

export function useSkillStore({
  selectedId,
  initialQuery,
  onNavigate,
}: {
  selectedId: string | null;
  initialQuery: string;
  onNavigate: (id: string | null, historyMode?: "push" | "replace") => void;
}) {
  const t = useTranslations("SkillStore");
  const marketplaceLocale = toMarketplaceLocale(useLocale());
  const client = useMemo(() => createNeoChatApiClient(), []);
  const selectedKey = useMemo(() => parseSelectedKey(selectedId), [selectedId]);
  const [tab, setTab] = useState<SkillStoreTab>(() =>
    initialQuery || selectedId ? "store" : "installed",
  );
  const [source, setSource] = useState<StoreSource>(
    selectedKey?.source ?? "lobehub",
  );
  const [catalogItems, setCatalogItems] = useState<SkillCatalogSummaryDTO[]>(
    [],
  );
  const [marketItems, setMarketItems] = useState<SkillMarketplaceSummaryDTO[]>(
    [],
  );
  const [categories, setCategories] = useState<SkillMarketplaceCategoryDTO[]>(
    [],
  );
  const [category, setCategory] = useState("");
  const [marketPage, setMarketPage] = useState(0);
  const [marketTotalPages, setMarketTotalPages] = useState(0);
  const [marketSort, setMarketSort] =
    useState<SkillMarketplaceSortDTO>("relevance");
  const [installed, setInstalled] = useState<AgentPackageInstallationDTO[]>([]);
  const [catalogLoading, setCatalogLoading] = useState(true);
  const [marketplaceLoading, setMarketplaceLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [installedLoading, setInstalledLoading] = useState(true);
  const [catalogError, setCatalogError] = useState("");
  const [marketplaceError, setMarketplaceError] = useState("");
  const [installedError, setInstalledError] = useState("");
  const [actionId, setActionId] = useState("");
  const [announcement, setAnnouncement] = useState("");
  const [query, setQuery] = useState(initialQuery);
  const [directURL, setDirectURL] = useState("");
  const [directInstalling, setDirectInstalling] = useState(false);
  const [catalogDetail, setCatalogDetail] =
    useState<SkillCatalogItemDTO | null>(null);
  const [marketDetail, setMarketDetail] =
    useState<SkillMarketplaceDetailDTO | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState("");
  const [detailReload, setDetailReload] = useState(0);

  const loadCatalog = useCallback(async () => {
    setCatalogLoading(true);
    setCatalogError("");
    try {
      const store = await client.skillStore.listCatalog();
      setCatalogItems(store.items);
    } catch (loadError) {
      setCatalogItems([]);
      setCatalogError(errorMessage(loadError, t("loadStoreFailed")));
    } finally {
      setCatalogLoading(false);
    }
  }, [client.skillStore, t]);

  const loadMarketplace = useCallback(
    async (
      searchQuery: string,
      selectedCategory: string,
      page = 1,
      append = false,
      selectedSort = marketSort,
    ) => {
      if (append) setLoadingMore(true);
      else setMarketplaceLoading(true);
      setMarketplaceError("");
      try {
        const result = await client.skillStore.searchMarketplace({
          query: searchQuery.trim(),
          category: selectedCategory || undefined,
          locale: marketplaceLocale,
          sort: selectedSort,
          page,
          pageSize: MARKETPLACE_PAGE_SIZE,
        });
        setMarketItems((current) =>
          append ? appendUniqueSkills(current, result.items) : result.items,
        );
        setCategories(result.categories);
        setMarketPage(result.page);
        setMarketTotalPages(result.totalPages);
      } catch (loadError) {
        if (!append) setMarketItems([]);
        setMarketplaceError(
          errorMessage(loadError, t("loadMarketplaceFailed")),
        );
      } finally {
        setMarketplaceLoading(false);
        setLoadingMore(false);
      }
    },
    [client.skillStore, marketplaceLocale, marketSort, t],
  );

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

  const reloadStore = useCallback(async () => {
    if (source === "lobehub") {
      await loadMarketplace(query, category, 1, false);
      return;
    }
    await loadCatalog();
  }, [category, loadCatalog, loadMarketplace, query, source]);

  useEffect(() => {
    queueMicrotask(() => {
      void Promise.all([
        loadCatalog(),
        loadMarketplace(initialQuery, "", 1, false),
        loadInstalled(),
      ]);
    });
  }, [initialQuery, loadCatalog, loadInstalled, loadMarketplace]);

  useEffect(() => setQuery(initialQuery), [initialQuery]);

  useEffect(() => {
    if (!selectedKey) return;
    setTab("store");
    setSource(selectedKey.source);
  }, [selectedKey]);

  useEffect(() => {
    const controller = new AbortController();
    setCatalogDetail(null);
    setMarketDetail(null);
    setDetailError("");
    if (!selectedKey) {
      setDetailLoading(false);
      return () => controller.abort();
    }
    setDetailLoading(true);
    const request =
      selectedKey.source === "lobehub"
        ? client.skillStore
            .getMarketplaceSkill(selectedKey.id, {
              locale: marketplaceLocale,
              signal: controller.signal,
            })
            .then(setMarketDetail)
        : client.skillStore
            .getCatalogSkill(selectedKey.id, { signal: controller.signal })
            .then(setCatalogDetail);
    void request
      .catch((loadError) => {
        if (!controller.signal.aborted) {
          setDetailError(errorMessage(loadError, t("loadStoreFailed")));
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setDetailLoading(false);
      });
    return () => controller.abort();
  }, [client.skillStore, detailReload, marketplaceLocale, selectedKey, t]);

  const installedFingerprints = new Set(
    installed.map((item) => item.packageFingerprint),
  );
  const normalizedQuery = query.trim().toLowerCase();
  const filteredCatalog = normalizedQuery
    ? catalogItems.filter((item) =>
        [item.id, item.name, item.path, item.repository].some((value) =>
          value.toLowerCase().includes(normalizedQuery),
        ),
      )
    : catalogItems;

  const finishInstall = async (name: string) => {
    setAnnouncement(t("installedAnnouncement", { name }));
    await loadInstalled();
    onNavigate(null, "replace");
    setTab("installed");
  };

  const installCatalog = async (item: SkillCatalogItemDTO) => {
    setActionId(item.id);
    try {
      await client.skillStore.installCatalogSkill({
        id: item.id,
        resolvedCommit: item.resolvedCommit,
        packageFingerprint: item.packageFingerprint,
      });
      await finishInstall(item.name);
    } catch (actionError) {
      setAnnouncement(errorMessage(actionError, t("actionFailed")));
      if (isStale(actionError)) setDetailReload((value) => value + 1);
    } finally {
      setActionId("");
    }
  };

  const installMarketplace = async (item: SkillMarketplaceDetailDTO) => {
    setActionId(item.identifier);
    try {
      await client.skillStore.installMarketplaceSkill({
        identifier: item.identifier,
        version: item.version,
      });
      await finishInstall(item.name);
    } catch (actionError) {
      setAnnouncement(errorMessage(actionError, t("actionFailed")));
      if (isStale(actionError)) setDetailReload((value) => value + 1);
    } finally {
      setActionId("");
    }
  };

  const installLink = async (event: FormEvent) => {
    event.preventDefault();
    if (!directURL.trim() || directInstalling) return;
    setDirectInstalling(true);
    try {
      const item = await client.skillStore.installSkillLink({
        url: directURL.trim(),
      });
      setDirectURL("");
      await finishInstall(item.name);
    } catch (actionError) {
      setAnnouncement(errorMessage(actionError, t("directInstallFailed")));
    } finally {
      setDirectInstalling(false);
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
      await loadInstalled();
    } catch (actionError) {
      setAnnouncement(errorMessage(actionError, t("actionFailed")));
      if (isStale(actionError)) await loadInstalled();
    } finally {
      setActionId("");
    }
  };

  const submitSearch = (event: FormEvent) => {
    event.preventDefault();
    if (source === "lobehub") void loadMarketplace(query, category, 1, false);
  };

  return {
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
    storeError: source === "lobehub" ? marketplaceError : catalogError,
    storeLoading: source === "lobehub" ? marketplaceLoading : catalogLoading,
    submitSearch,
    tab,
    uninstall,
  };
}

function toMarketplaceLocale(locale: string): SkillMarketplaceLocaleDTO {
  if (locale.toLowerCase().startsWith("en")) return "en-US";
  if (locale.toLowerCase().startsWith("ja")) return "ja-JP";
  return "zh-CN";
}

export function marketSkillKey(identifier: string) {
  return `${LOBEHUB_KEY_PREFIX}${identifier}`;
}

function parseSelectedKey(value: string | null) {
  if (!value) return null;
  return value.startsWith(LOBEHUB_KEY_PREFIX)
    ? { source: "lobehub" as const, id: value.slice(LOBEHUB_KEY_PREFIX.length) }
    : { source: "openai" as const, id: value };
}

function appendUniqueSkills(
  current: SkillMarketplaceSummaryDTO[],
  incoming: SkillMarketplaceSummaryDTO[],
) {
  const identifiers = new Set(current.map((item) => item.identifier));
  return [
    ...current,
    ...incoming.filter((item) => {
      if (identifiers.has(item.identifier)) return false;
      identifiers.add(item.identifier);
      return true;
    }),
  ];
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
