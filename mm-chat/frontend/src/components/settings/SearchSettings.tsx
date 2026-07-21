"use client";

import { useCallback, useEffect, useState } from "react";
import {
  AlertCircle,
  Check,
  Globe2,
  Loader2,
  Power,
  RefreshCw,
  Trash2,
} from "lucide-react";
import { useTranslations } from "next-intl";
import { SEARCH_CONFIG_LIMITS } from "@/config/limits";
import { encryptSecret } from "@/lib/byok/client";
import { BYOK_CONTEXTS } from "@/lib/byok/shared";
import { createNeoChatApiClient } from "@/services/api/client";
import type {
  AdminSearchProviderConfigDTO,
  AdminSearchProviderConfigsDTO,
  SearchProviderId,
} from "@/services/api/client";
import { useSettingsStore } from "@/store/core/settingsStore";
import { SecretInput } from "./SettingsUI";

const SEARCH_PROVIDERS: Array<{
  id: SearchProviderId;
  name: string;
  defaultBaseUrl: string;
}> = [
  {
    id: "tavily",
    name: "Tavily",
    defaultBaseUrl: "https://api.tavily.com",
  },
  {
    id: "firecrawl",
    name: "Firecrawl",
    defaultBaseUrl: "https://api.firecrawl.dev",
  },
  { id: "exa", name: "Exa", defaultBaseUrl: "https://api.exa.ai" },
  {
    id: "bocha",
    name: "Bocha",
    defaultBaseUrl: "https://api.bochaai.com",
  },
];

type BusyAction = "save" | "activate" | "deactivate" | "delete" | "clear";

const providerName = (providerId: SearchProviderId) =>
  SEARCH_PROVIDERS.find((provider) => provider.id === providerId)?.name ??
  providerId;

const SearchSettings = () => {
  const t = useTranslations("Search");
  const { search, serverConfig, setSearchResultsLimit, applyServerConfig } =
    useSettingsStore();
  const [selectedProviderId, setSelectedProviderId] =
    useState<SearchProviderId>("tavily");
  const [providerConfigs, setProviderConfigs] = useState<
    Partial<Record<SearchProviderId, AdminSearchProviderConfigDTO>>
  >({});
  const [baseUrls, setBaseUrls] = useState<
    Partial<Record<SearchProviderId, string>>
  >({});
  const [activeProviderId, setActiveProviderId] = useState<SearchProviderId>();
  const [busy, setBusy] = useState<{
    providerId: SearchProviderId;
    action: BusyAction;
  }>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();
  const [success, setSuccess] = useState<string>();
  const [deleteConfirmProviderId, setDeleteConfirmProviderId] =
    useState<SearchProviderId>();

  const applyProviders = useCallback(
    (response: AdminSearchProviderConfigsDTO) => {
      const next: Partial<
        Record<SearchProviderId, AdminSearchProviderConfigDTO>
      > = {};
      for (const provider of response.providers) {
        next[provider.provider] = provider;
      }
      setProviderConfigs(next);
      setActiveProviderId(response.activeProviderId);
      setBaseUrls((current) => {
        const updated = { ...current };
        for (const provider of response.providers) {
          updated[provider.provider] = provider.baseUrl;
        }
        return updated;
      });
    },
    [],
  );

  const refreshProviders = useCallback(async () => {
    const client = createNeoChatApiClient();
    const response =
      await client.searchProviders.listAdminSearchProviderConfigs();
    applyProviders(response);
    try {
      const runtimeConfig = await client.settings.getRuntimeConfig();
      applyServerConfig(runtimeConfig);
    } catch {
      return;
    }
  }, [applyProviders, applyServerConfig]);

  useEffect(() => {
    let active = true;
    const client = createNeoChatApiClient();
    setLoading(true);
    client.searchProviders
      .listAdminSearchProviderConfigs()
      .then(async (response) => {
        if (!active) return;
        applyProviders(response);
        if (response.activeProviderId) {
          setSelectedProviderId(response.activeProviderId);
        }
        try {
          const runtimeConfig = await client.settings.getRuntimeConfig();
          if (active) applyServerConfig(runtimeConfig);
        } catch {
          return;
        }
      })
      .catch((cause) => {
        if (!active) return;
        setError(cause instanceof Error ? cause.message : t("requestFailed"));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [applyProviders, applyServerConfig, t]);

  useEffect(() => {
    setError(undefined);
    setSuccess(undefined);
    setDeleteConfirmProviderId(undefined);
  }, [selectedProviderId]);

  const runProviderAction = async (
    providerId: SearchProviderId,
    action: BusyAction,
    operation: () => Promise<string>,
  ) => {
    setBusy({ providerId, action });
    setError(undefined);
    setSuccess(undefined);
    let operationSucceeded = false;
    try {
      const message = await operation();
      operationSucceeded = true;
      setSuccess(message);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t("requestFailed"));
    } finally {
      try {
        await refreshProviders();
      } catch (cause) {
        if (operationSucceeded) {
          setSuccess(undefined);
          setError(cause instanceof Error ? cause.message : t("requestFailed"));
        }
      }
      setBusy(undefined);
    }
  };

  const saveAndTest = async (providerId: SearchProviderId, apiKey?: string) => {
    const current = providerConfigs[providerId];
    await runProviderAction(providerId, "save", async () => {
      const apiKeySecret = apiKey
        ? await encryptSecret(apiKey, BYOK_CONTEXTS.searchProvider(providerId))
        : undefined;
      const client = createNeoChatApiClient();
      await client.searchProviders.updateAdminSearchProviderConfig(providerId, {
        name: providerName(providerId),
        baseUrl: baseUrls[providerId]?.trim() ?? "",
        enabled: current?.enabled ?? false,
        ...(apiKeySecret ? { apiKeySecret } : {}),
      });
      const tested =
        await client.searchProviders.testAdminSearchProviderConnection(
          providerId,
        );
      return t("saveTestPassed", { count: tested.sourceCount });
    });
  };

  const activateProvider = async (providerId: SearchProviderId) => {
    await runProviderAction(providerId, "activate", async () => {
      await createNeoChatApiClient().searchProviders.activateAdminSearchProvider(
        providerId,
      );
      return t("activated", { provider: providerName(providerId) });
    });
  };

  const deactivateProvider = async (providerId: SearchProviderId) => {
    const current = providerConfigs[providerId];
    if (!current) return;
    await runProviderAction(providerId, "deactivate", async () => {
      await createNeoChatApiClient().searchProviders.updateAdminSearchProviderConfig(
        providerId,
        {
          name: current.name,
          baseUrl: baseUrls[providerId]?.trim() ?? current.baseUrl,
          enabled: false,
        },
      );
      return t("deactivated", { provider: providerName(providerId) });
    });
  };

  const clearApiKey = async (providerId: SearchProviderId) => {
    const current = providerConfigs[providerId];
    if (!current) return;
    await runProviderAction(providerId, "clear", async () => {
      await createNeoChatApiClient().searchProviders.updateAdminSearchProviderConfig(
        providerId,
        {
          name: current.name,
          baseUrl: baseUrls[providerId]?.trim() ?? current.baseUrl,
          enabled: false,
          clearApiKey: true,
        },
      );
      return t("keyCleared");
    });
  };

  const deleteProvider = async (providerId: SearchProviderId) => {
    if (deleteConfirmProviderId !== providerId) {
      setDeleteConfirmProviderId(providerId);
      return;
    }
    setDeleteConfirmProviderId(undefined);
    await runProviderAction(providerId, "delete", async () => {
      await createNeoChatApiClient().searchProviders.deleteAdminSearchProviderConfig(
        providerId,
      );
      return t("deleted", { provider: providerName(providerId) });
    });
  };

  const selectedProvider = SEARCH_PROVIDERS.find(
    (provider) => provider.id === selectedProviderId,
  )!;
  const selectedConfig = providerConfigs[selectedProviderId];
  const selectedBusy = busy?.providerId === selectedProviderId;
  const serviceAvailable = serverConfig?.search.available === true;
  const serviceStatus = activeProviderId
    ? providerName(activeProviderId)
    : serviceAvailable
      ? t("modelBuiltIn")
      : t("serverUnavailable");

  return (
    <div className="space-y-6">
      <div className="space-y-4">
        <div className="flex items-center justify-between gap-4">
          <h3 className="text-lg font-semibold text-gray-800 dark:text-foreground">
            {t("title")}
          </h3>
          <span
            className={
              serviceAvailable
                ? "text-xs font-medium text-emerald-600 dark:text-emerald-400"
                : "text-xs font-medium text-gray-500 dark:text-muted-foreground"
            }
          >
            {serviceStatus}
          </span>
        </div>

        <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
          {SEARCH_PROVIDERS.map((provider) => {
            const config = providerConfigs[provider.id];
            const selected = provider.id === selectedProviderId;
            return (
              <button
                key={provider.id}
                type="button"
                aria-pressed={selected}
                onClick={() => setSelectedProviderId(provider.id)}
                className={
                  selected
                    ? "rounded-lg border border-blue-500 bg-blue-50 px-3 py-2 text-left text-sm font-medium text-blue-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60 dark:bg-blue-500/10 dark:text-blue-300"
                    : "rounded-lg border border-gray-200 bg-background px-3 py-2 text-left text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring dark:border-border dark:text-foreground dark:hover:bg-muted"
                }
              >
                <span className="flex items-center justify-between gap-2">
                  {provider.name}
                  {config?.enabled ? (
                    <Check
                      size={14}
                      className="text-emerald-500"
                      aria-label={t("active")}
                    />
                  ) : null}
                </span>
              </button>
            );
          })}
        </div>

        <div className="space-y-4 rounded-lg border border-gray-200 p-4 dark:border-border">
          <div className="flex items-start justify-between gap-3">
            <div className="flex min-w-0 items-start gap-3">
              <Globe2
                size={18}
                className="mt-0.5 shrink-0 text-gray-500 dark:text-muted-foreground"
                aria-hidden="true"
              />
              <div>
                <p className="font-medium text-gray-800 dark:text-foreground">
                  {selectedProvider.name}
                </p>
                <p className="mt-1 text-xs text-gray-500 dark:text-muted-foreground">
                  {selectedConfig?.enabled
                    ? t("active")
                    : selectedConfig?.connectionTestValid
                      ? t("tested")
                      : selectedConfig?.hasApiKey
                        ? t("saved")
                        : t("notConfigured")}
                </p>
              </div>
            </div>
            {loading ? (
              <Loader2
                size={16}
                className="animate-spin text-gray-400"
                aria-label={t("loading")}
              />
            ) : null}
          </div>

          <div className="space-y-1.5">
            <label
              htmlFor={`search-base-url-${selectedProviderId}`}
              className="text-sm font-medium text-gray-700 dark:text-foreground/85"
            >
              {t("baseUrl")}
            </label>
            <input
              id={`search-base-url-${selectedProviderId}`}
              type="url"
              value={baseUrls[selectedProviderId] ?? ""}
              maxLength={SEARCH_CONFIG_LIMITS.maxBaseUrlChars}
              placeholder={selectedProvider.defaultBaseUrl}
              disabled={selectedBusy}
              onChange={(event) =>
                setBaseUrls((current) => ({
                  ...current,
                  [selectedProviderId]: event.target.value,
                }))
              }
              className="w-full rounded-lg border border-gray-200 bg-gray-50 px-3 py-2 text-sm text-gray-800 transition-[background-color,border-color,box-shadow,color] focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-500/20 disabled:opacity-60 dark:border-border dark:bg-muted dark:text-foreground"
            />
            <p className="text-[10px] text-gray-500 dark:text-muted-foreground">
              {t("baseUrlHint")}
            </p>
          </div>

          <div className="space-y-1.5">
            <label
              htmlFor={`search-api-key-${selectedProviderId}`}
              className="text-sm font-medium text-gray-700 dark:text-foreground/85"
            >
              API Key
            </label>
            <SecretInput
              id={`search-api-key-${selectedProviderId}`}
              name={`searchApiKey-${selectedProviderId}`}
              placeholder={t("apiKeyPlaceholder")}
              maxLength={SEARCH_CONFIG_LIMITS.maxProviderApiKeyChars}
              hasSecret={selectedConfig?.hasApiKey === true}
              statusText={
                selectedConfig?.hasApiKey
                  ? t("keyStoredOnServer")
                  : t("keyNotStored")
              }
              onSave={(value) => saveAndTest(selectedProviderId, value)}
              onClear={
                selectedConfig?.hasApiKey
                  ? () => clearApiKey(selectedProviderId)
                  : undefined
              }
            />
          </div>

          <div className="flex flex-wrap gap-2">
            <button
              type="button"
              disabled={selectedBusy || !selectedConfig?.hasApiKey}
              onClick={() => saveAndTest(selectedProviderId)}
              className="inline-flex items-center gap-2 rounded-lg bg-blue-500 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {busy?.action === "save" && selectedBusy ? (
                <Loader2
                  size={15}
                  className="animate-spin"
                  aria-hidden="true"
                />
              ) : (
                <RefreshCw size={15} aria-hidden="true" />
              )}
              {t("saveAndTest")}
            </button>
            <button
              type="button"
              disabled={selectedBusy || !selectedConfig?.hasApiKey}
              onClick={() =>
                selectedConfig?.enabled
                  ? deactivateProvider(selectedProviderId)
                  : activateProvider(selectedProviderId)
              }
              className="inline-flex items-center gap-2 rounded-lg border border-gray-200 px-3 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50 dark:border-border dark:text-foreground dark:hover:bg-muted"
            >
              <Power size={15} aria-hidden="true" />
              {selectedConfig?.enabled ? t("deactivate") : t("activate")}
            </button>
            <button
              type="button"
              disabled={selectedBusy || !selectedConfig}
              onClick={() => deleteProvider(selectedProviderId)}
              className="inline-flex items-center gap-2 rounded-lg border border-gray-200 px-3 py-2 text-sm font-medium text-gray-600 transition-colors hover:bg-red-50 hover:text-red-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-500/50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-border dark:text-muted-foreground dark:hover:bg-red-500/10 dark:hover:text-red-300"
            >
              <Trash2 size={15} aria-hidden="true" />
              {deleteConfirmProviderId === selectedProviderId
                ? t("confirmDelete")
                : t("delete")}
            </button>
          </div>

          {error ? (
            <div
              role="alert"
              className="flex items-start gap-2 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-500/10 dark:text-red-300"
            >
              <AlertCircle size={16} className="mt-0.5 shrink-0" />
              <span>{error}</span>
            </div>
          ) : null}
          {success ? (
            <div
              role="status"
              className="flex items-start gap-2 rounded-lg bg-emerald-50 px-3 py-2 text-sm text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300"
            >
              <Check size={16} className="mt-0.5 shrink-0" />
              <span>{success}</span>
            </div>
          ) : null}
        </div>
      </div>

      <div className="space-y-2 border-t border-gray-100 pt-4 dark:border-border">
        <div className="flex justify-between text-sm text-gray-700 dark:text-foreground/85">
          <label htmlFor="search-results-limit" className="font-medium">
            {t("resultLimit")}
          </label>
          <span className="rounded bg-gray-100 px-2 py-0.5 font-mono text-xs dark:bg-muted">
            {search.resultsLimit}
          </span>
        </div>
        <input
          id="search-results-limit"
          name="searchResultsLimit"
          type="range"
          min="1"
          max="10"
          step="1"
          value={search.resultsLimit}
          onChange={(event) =>
            setSearchResultsLimit(Number.parseInt(event.target.value, 10))
          }
          aria-describedby="search-results-limit-bounds"
          className="h-2 w-full cursor-pointer appearance-none rounded-lg bg-gray-200 accent-blue-500 dark:bg-accent"
        />
        <div
          id="search-results-limit-bounds"
          className="flex justify-between text-[10px] text-gray-400"
        >
          <span>1</span>
          <span>10</span>
        </div>
      </div>
    </div>
  );
};

export default SearchSettings;
