"use client";

import { useCallback, useEffect, useState } from "react";
import {
  AlertCircle,
  Check,
  Cpu,
  FileScan,
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
  AdminRAGProviderConfigDTO,
  AdminRAGProviderConfigsDTO,
  RAGProviderId,
} from "@/services/api/client";
import { SecretInput } from "./SettingsUI";

const RAG_PROVIDERS: Array<{
  id: RAGProviderId;
  name: string;
  profile: string;
}> = [
  { id: "mineru", name: "MinerU", profile: "VLM · OCR · Formula · Table" },
  {
    id: "jina",
    name: "Jina AI",
    profile: "jina-embeddings-v4 · 1024D · jina-reranker-v3",
  },
];

type BusyAction = "save" | "activate" | "deactivate" | "delete" | "clear";

const providerName = (providerId: RAGProviderId) =>
  RAG_PROVIDERS.find((provider) => provider.id === providerId)?.name ??
  providerId;

const RAGProviderAdmin = () => {
  const t = useTranslations("RAG");
  const [selectedProviderId, setSelectedProviderId] =
    useState<RAGProviderId>("mineru");
  const [providerConfigs, setProviderConfigs] = useState<
    Partial<Record<RAGProviderId, AdminRAGProviderConfigDTO>>
  >({});
  const [busy, setBusy] = useState<{
    providerId: RAGProviderId;
    action: BusyAction;
  }>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();
  const [success, setSuccess] = useState<string>();
  const [deleteConfirmProviderId, setDeleteConfirmProviderId] =
    useState<RAGProviderId>();

  const applyProviders = useCallback((response: AdminRAGProviderConfigsDTO) => {
    const next: Partial<Record<RAGProviderId, AdminRAGProviderConfigDTO>> = {};
    for (const provider of response.providers) {
      next[provider.provider] = provider;
    }
    setProviderConfigs(next);
  }, []);

  const refreshProviders = useCallback(async () => {
    const response =
      await createNeoChatApiClient().ragProviders.listAdminRAGProviderConfigs();
    applyProviders(response);
  }, [applyProviders]);

  useEffect(() => {
    let active = true;
    setLoading(true);
    createNeoChatApiClient()
      .ragProviders.listAdminRAGProviderConfigs()
      .then((response) => {
        if (active) applyProviders(response);
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
  }, [applyProviders, t]);

  useEffect(() => {
    setError(undefined);
    setSuccess(undefined);
    setDeleteConfirmProviderId(undefined);
  }, [selectedProviderId]);

  const runProviderAction = async (
    providerId: RAGProviderId,
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

  const saveAndTest = async (providerId: RAGProviderId, apiKey?: string) => {
    const current = providerConfigs[providerId];
    await runProviderAction(providerId, "save", async () => {
      const apiKeySecret = apiKey
        ? await encryptSecret(apiKey, BYOK_CONTEXTS.ragProvider(providerId))
        : undefined;
      const client = createNeoChatApiClient();
      await client.ragProviders.updateAdminRAGProviderConfig(providerId, {
        name: providerName(providerId),
        enabled: current?.enabled ?? false,
        ...(apiKeySecret ? { apiKeySecret } : {}),
      });
      const tested =
        await client.ragProviders.testAdminRAGProviderConnection(providerId);
      return t("providerTestPassed", { count: tested.checks.length });
    });
  };

  const activateProvider = async (providerId: RAGProviderId) => {
    await runProviderAction(providerId, "activate", async () => {
      await createNeoChatApiClient().ragProviders.activateAdminRAGProvider(
        providerId,
      );
      return t("providerActivated", { provider: providerName(providerId) });
    });
  };

  const deactivateProvider = async (providerId: RAGProviderId) => {
    const current = providerConfigs[providerId];
    if (!current) return;
    await runProviderAction(providerId, "deactivate", async () => {
      await createNeoChatApiClient().ragProviders.updateAdminRAGProviderConfig(
        providerId,
        { name: current.name, enabled: false },
      );
      return t("providerDeactivated", { provider: providerName(providerId) });
    });
  };

  const clearApiKey = async (providerId: RAGProviderId) => {
    const current = providerConfigs[providerId];
    if (!current) return;
    await runProviderAction(providerId, "clear", async () => {
      await createNeoChatApiClient().ragProviders.updateAdminRAGProviderConfig(
        providerId,
        { name: current.name, enabled: false, clearApiKey: true },
      );
      return t("providerKeyCleared");
    });
  };

  const deleteProvider = async (providerId: RAGProviderId) => {
    if (deleteConfirmProviderId !== providerId) {
      setDeleteConfirmProviderId(providerId);
      return;
    }
    setDeleteConfirmProviderId(undefined);
    await runProviderAction(providerId, "delete", async () => {
      await createNeoChatApiClient().ragProviders.deleteAdminRAGProviderConfig(
        providerId,
      );
      return t("providerDeleted", { provider: providerName(providerId) });
    });
  };

  const selectedProvider = RAG_PROVIDERS.find(
    (provider) => provider.id === selectedProviderId,
  )!;
  const selectedConfig = providerConfigs[selectedProviderId];
  const selectedBusy = busy?.providerId === selectedProviderId;
  const Icon = selectedProviderId === "mineru" ? FileScan : Cpu;

  return (
    <div className="space-y-4 rounded-xl border border-gray-200 p-4 dark:border-border">
      <div className="flex items-center justify-between gap-3">
        <h4 className="text-sm font-semibold text-gray-800 dark:text-foreground">
          {t("serverProviders")}
        </h4>
        {loading ? (
          <Loader2
            size={16}
            className="animate-spin text-gray-400"
            aria-label={t("providerLoading")}
          />
        ) : null}
      </div>

      <div className="grid grid-cols-2 gap-2">
        {RAG_PROVIDERS.map((provider) => {
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
                    aria-label={t("providerActive")}
                  />
                ) : null}
              </span>
            </button>
          );
        })}
      </div>

      <div className="space-y-4">
        <div className="flex items-start gap-3">
          <Icon
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
                ? t("providerActive")
                : selectedConfig?.connectionTestValid
                  ? t("providerTested")
                  : selectedConfig?.hasApiKey
                    ? t("providerSaved")
                    : t("providerNotConfigured")}
            </p>
            <p className="mt-1 text-[10px] text-gray-400 dark:text-muted-foreground">
              {selectedProvider.profile}
            </p>
          </div>
        </div>

        <div className="space-y-1.5">
          <label
            htmlFor={`rag-provider-api-key-${selectedProviderId}`}
            className="text-sm font-medium text-gray-700 dark:text-foreground/85"
          >
            API Key
          </label>
          <SecretInput
            id={`rag-provider-api-key-${selectedProviderId}`}
            name={`ragProviderApiKey-${selectedProviderId}`}
            placeholder={t("providerApiKeyPlaceholder")}
            maxLength={SEARCH_CONFIG_LIMITS.maxProviderApiKeyChars}
            hasSecret={selectedConfig?.hasApiKey === true}
            statusText={
              selectedConfig?.hasApiKey
                ? t("providerKeyStoredOnServer")
                : t("providerKeyNotStored")
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
            disabled={selectedBusy}
            onClick={() => saveAndTest(selectedProviderId)}
            className="inline-flex items-center gap-2 rounded-lg bg-blue-500 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {busy?.action === "save" && selectedBusy ? (
              <Loader2 size={15} className="animate-spin" aria-hidden="true" />
            ) : (
              <RefreshCw size={15} aria-hidden="true" />
            )}
            {t("providerSaveAndTest")}
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
            {selectedConfig?.enabled
              ? t("providerDeactivate")
              : t("providerActivate")}
          </button>
          <button
            type="button"
            disabled={selectedBusy || !selectedConfig}
            onClick={() => deleteProvider(selectedProviderId)}
            className="inline-flex items-center gap-2 rounded-lg border border-gray-200 px-3 py-2 text-sm font-medium text-gray-600 transition-colors hover:bg-red-50 hover:text-red-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-500/50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-border dark:text-muted-foreground dark:hover:bg-red-500/10 dark:hover:text-red-300"
          >
            <Trash2 size={15} aria-hidden="true" />
            {deleteConfirmProviderId === selectedProviderId
              ? t("providerConfirmDelete")
              : t("providerDelete")}
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
  );
};

export default RAGProviderAdmin;
