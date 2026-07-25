"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import {
  AlertCircle,
  AlertTriangle,
  Check,
  CheckCircle2,
  Cpu,
  FileScan,
  Loader2,
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
  RAGProviderRuntimeStatus,
  RAGProviderStatusDTO,
} from "@/services/api/client";

const RAG_PROVIDERS: Array<{
  id: RAGProviderId;
  name: string;
  roleKey: "mineruRole" | "siliconflowRole";
  Icon: typeof FileScan;
}> = [
  { id: "mineru", name: "MinerU", roleKey: "mineruRole", Icon: FileScan },
  {
    id: "siliconflow",
    name: "SiliconFlow",
    roleKey: "siliconflowRole",
    Icon: Cpu,
  },
];

type BusyAction = "configure" | "remove";

type Feedback = {
  providerId: RAGProviderId;
  kind: "success" | "error";
  message: string;
};

const mapProviders = (response: AdminRAGProviderConfigsDTO) => {
  const next: Partial<Record<RAGProviderId, AdminRAGProviderConfigDTO>> = {};
  for (const provider of response.providers) {
    next[provider.provider] = provider;
  }
  return next;
};

const fallbackRuntimeStatus = (
  config: AdminRAGProviderConfigDTO | undefined,
): RAGProviderRuntimeStatus => {
  if (!config?.hasApiKey) return "missing_secret";
  if (config.enabled && config.connectionTestValid) return "ready";
  if (config.connectionTestValid) return "activation_required";
  return "unavailable";
};

const RAGProviderAdmin = () => {
  const t = useTranslations("RAG");
  const client = useMemo(() => createNeoChatApiClient(), []);
  const [providerConfigs, setProviderConfigs] = useState<
    Partial<Record<RAGProviderId, AdminRAGProviderConfigDTO>>
  >({});
  const [providerStatus, setProviderStatus] = useState<RAGProviderStatusDTO>();
  const [drafts, setDrafts] = useState<Record<RAGProviderId, string>>({
    mineru: "",
    siliconflow: "",
  });
  const [busy, setBusy] = useState<{
    providerId: RAGProviderId;
    action: BusyAction;
  }>();
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string>();
  const [feedback, setFeedback] = useState<Feedback>();
  const [removeConfirmProviderId, setRemoveConfirmProviderId] =
    useState<RAGProviderId>();

  const refreshStatus = useCallback(async () => {
    const status = await client.ragProviders.getRAGProviderStatus();
    setProviderStatus(status);
  }, [client]);

  useEffect(() => {
    let active = true;
    setLoading(true);
    setLoadError(undefined);
    Promise.all([
      client.ragProviders.listAdminRAGProviderConfigs(),
      client.ragProviders.getRAGProviderStatus(),
    ])
      .then(([configs, status]) => {
        if (!active) return;
        setProviderConfigs(mapProviders(configs));
        setProviderStatus(status);
      })
      .catch((cause) => {
        if (!active) return;
        setLoadError(
          cause instanceof Error ? cause.message : t("requestFailed"),
        );
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [client, t]);

  const configureProvider = async (
    event: FormEvent<HTMLFormElement>,
    providerId: RAGProviderId,
  ) => {
    event.preventDefault();
    const apiKey = drafts[providerId].trim();
    if (!apiKey || busy) return;

    setBusy({ providerId, action: "configure" });
    setFeedback(undefined);
    setLoadError(undefined);
    setRemoveConfirmProviderId(undefined);
    try {
      const apiKeySecret = await encryptSecret(
        apiKey,
        BYOK_CONTEXTS.ragProvider(providerId),
      );
      if (!apiKeySecret) throw new Error(t("providerKeyRequired"));
      const response = await client.ragProviders.configureAdminRAGProvider(
        providerId,
        { apiKeySecret },
      );
      setProviderConfigs((current) => ({
        ...current,
        [providerId]: response.provider,
      }));
      setDrafts((current) => ({ ...current, [providerId]: "" }));
      setFeedback({
        providerId,
        kind: "success",
        message: t("providerConfigured", { count: response.checks.length }),
      });
      try {
        await refreshStatus();
      } catch {
        setLoadError(t("statusRefreshFailed"));
      }
    } catch (cause) {
      setFeedback({
        providerId,
        kind: "error",
        message: cause instanceof Error ? cause.message : t("requestFailed"),
      });
    } finally {
      setBusy(undefined);
    }
  };

  const removeProvider = async (providerId: RAGProviderId) => {
    if (busy) return;
    if (removeConfirmProviderId !== providerId) {
      setRemoveConfirmProviderId(providerId);
      setFeedback(undefined);
      return;
    }

    setBusy({ providerId, action: "remove" });
    setFeedback(undefined);
    setLoadError(undefined);
    try {
      await client.ragProviders.deleteAdminRAGProviderConfig(providerId);
      setProviderConfigs((current) => {
        const next = { ...current };
        delete next[providerId];
        return next;
      });
      setDrafts((current) => ({ ...current, [providerId]: "" }));
      setRemoveConfirmProviderId(undefined);
      setFeedback({
        providerId,
        kind: "success",
        message: t("providerRemoved"),
      });
      try {
        await refreshStatus();
      } catch {
        setLoadError(t("statusRefreshFailed"));
      }
    } catch (cause) {
      setFeedback({
        providerId,
        kind: "error",
        message: cause instanceof Error ? cause.message : t("requestFailed"),
      });
    } finally {
      setBusy(undefined);
    }
  };

  const anyConfigured = RAG_PROVIDERS.some(
    ({ id }) =>
      providerStatus?.providers[id]?.configured ||
      providerConfigs[id]?.hasApiKey,
  );
  const serviceStatus = providerStatus?.status ?? "unavailable";
  const serviceMeta =
    serviceStatus === "ready"
      ? {
          label: t("serviceReady"),
          Icon: CheckCircle2,
          className:
            "border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-400/30 dark:bg-emerald-400/10 dark:text-emerald-200",
        }
      : serviceStatus === "partial"
        ? {
            label: t("servicePartial"),
            Icon: AlertTriangle,
            className:
              "border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-400/30 dark:bg-amber-400/10 dark:text-amber-200",
          }
        : {
            label: anyConfigured
              ? t("serviceUnavailable")
              : t("serviceNotConfigured"),
            Icon: AlertCircle,
            className:
              "border-gray-200 bg-gray-50 text-gray-600 dark:border-border dark:bg-muted/50 dark:text-muted-foreground",
          };
  const ServiceIcon = serviceMeta.Icon;

  return (
    <div className="space-y-4">
      <div
        className={`flex items-center gap-2 rounded-lg border px-3 py-2 text-sm font-medium ${serviceMeta.className}`}
        role="status"
      >
        {loading ? (
          <Loader2 size={16} className="animate-spin" aria-hidden="true" />
        ) : (
          <ServiceIcon size={16} aria-hidden="true" />
        )}
        <span>{loading ? t("providerLoading") : serviceMeta.label}</span>
      </div>

      {loadError ? (
        <div
          role="alert"
          className="flex items-start gap-2 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-500/10 dark:text-red-300"
        >
          <AlertCircle size={16} className="mt-0.5 shrink-0" />
          <span>{loadError}</span>
        </div>
      ) : null}

      <div className="space-y-3">
        {RAG_PROVIDERS.map((provider) => {
          const config = providerConfigs[provider.id];
          const runtimeStatus =
            providerStatus?.providers[provider.id]?.status ??
            fallbackRuntimeStatus(config);
          const isReady = runtimeStatus === "ready";
          const hasApiKey =
            providerStatus?.providers[provider.id]?.configured ??
            config?.hasApiKey === true;
          const isBusy = busy?.providerId === provider.id;
          const providerFeedback =
            feedback?.providerId === provider.id ? feedback : undefined;
          const Icon = provider.Icon;

          return (
            <section
              key={provider.id}
              data-rag-provider={provider.id}
              className="space-y-4 rounded-xl border border-gray-200 bg-background p-4 dark:border-border"
            >
              <div className="flex items-start justify-between gap-3">
                <div className="flex min-w-0 items-start gap-3">
                  <Icon
                    size={18}
                    className="mt-0.5 shrink-0 text-gray-500 dark:text-muted-foreground"
                    aria-hidden="true"
                  />
                  <div className="min-w-0">
                    <h3 className="font-medium text-gray-900 dark:text-foreground">
                      {provider.name}
                    </h3>
                    <p className="mt-1 text-xs text-gray-500 dark:text-muted-foreground">
                      {t(provider.roleKey)}
                    </p>
                  </div>
                </div>
                <span
                  className={`inline-flex shrink-0 items-center gap-1.5 rounded-full px-2 py-1 text-xs font-medium ${
                    isReady
                      ? "bg-emerald-50 text-emerald-700 dark:bg-emerald-400/10 dark:text-emerald-200"
                      : runtimeStatus === "missing_secret"
                        ? "bg-gray-100 text-gray-600 dark:bg-muted dark:text-muted-foreground"
                        : "bg-amber-50 text-amber-700 dark:bg-amber-400/10 dark:text-amber-200"
                  }`}
                >
                  {isReady ? <Check size={13} aria-hidden="true" /> : null}
                  {t(`providerStatus.${runtimeStatus}`)}
                </span>
              </div>

              <form
                className="space-y-2"
                onSubmit={(event) => configureProvider(event, provider.id)}
              >
                <label
                  htmlFor={`rag-provider-key-${provider.id}`}
                  className="text-sm font-medium text-gray-700 dark:text-foreground/85"
                >
                  API Key
                </label>
                <div className="flex flex-wrap gap-2 sm:flex-nowrap">
                  <input
                    id={`rag-provider-key-${provider.id}`}
                    name={`ragProviderKey-${provider.id}`}
                    type="password"
                    value={drafts[provider.id]}
                    onChange={(event) => {
                      setDrafts((current) => ({
                        ...current,
                        [provider.id]: event.target.value,
                      }));
                      setRemoveConfirmProviderId(undefined);
                    }}
                    maxLength={SEARCH_CONFIG_LIMITS.maxProviderApiKeyChars}
                    autoComplete="off"
                    spellCheck={false}
                    placeholder={
                      hasApiKey
                        ? t("providerReplaceKeyPlaceholder")
                        : t("providerApiKeyPlaceholder")
                    }
                    className="h-10 min-w-0 flex-1 rounded-lg border border-gray-200 bg-gray-50 px-3 font-mono text-sm text-gray-800 outline-none transition-[border-color,box-shadow] focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 dark:border-border dark:bg-muted dark:text-foreground"
                  />
                  <button
                    type="submit"
                    disabled={!drafts[provider.id].trim() || isBusy}
                    className="inline-flex h-10 shrink-0 items-center justify-center gap-2 rounded-lg bg-blue-500 px-4 text-sm font-medium text-white transition-colors hover:bg-blue-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60 disabled:cursor-not-allowed disabled:opacity-50"
                  >
                    {busy?.action === "configure" && isBusy ? (
                      <Loader2
                        size={15}
                        className="animate-spin"
                        aria-hidden="true"
                      />
                    ) : null}
                    {t("providerSaveAndTest")}
                  </button>
                  {hasApiKey ? (
                    <button
                      type="button"
                      disabled={isBusy}
                      onClick={() => removeProvider(provider.id)}
                      className="inline-flex h-10 shrink-0 items-center justify-center gap-2 rounded-lg border border-gray-200 px-3 text-sm font-medium text-gray-600 transition-colors hover:bg-red-50 hover:text-red-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-500/50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-border dark:text-muted-foreground dark:hover:bg-red-500/10 dark:hover:text-red-300"
                    >
                      {busy?.action === "remove" && isBusy ? (
                        <Loader2
                          size={15}
                          className="animate-spin"
                          aria-hidden="true"
                        />
                      ) : (
                        <Trash2 size={15} aria-hidden="true" />
                      )}
                      {removeConfirmProviderId === provider.id
                        ? t("providerConfirmRemove")
                        : t("providerRemove")}
                    </button>
                  ) : null}
                </div>
                <p className="text-xs text-gray-500 dark:text-muted-foreground">
                  {hasApiKey
                    ? t("providerKeyStoredOnServer")
                    : t("providerKeyNotStored")}
                </p>
              </form>

              {providerFeedback ? (
                <div
                  role={providerFeedback.kind === "error" ? "alert" : "status"}
                  className={`flex items-start gap-2 rounded-lg px-3 py-2 text-sm ${
                    providerFeedback.kind === "error"
                      ? "bg-red-50 text-red-700 dark:bg-red-500/10 dark:text-red-300"
                      : "bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300"
                  }`}
                >
                  {providerFeedback.kind === "error" ? (
                    <AlertCircle size={16} className="mt-0.5 shrink-0" />
                  ) : (
                    <Check size={16} className="mt-0.5 shrink-0" />
                  )}
                  <span>{providerFeedback.message}</span>
                </div>
              ) : null}
            </section>
          );
        })}
      </div>
    </div>
  );
};

export default RAGProviderAdmin;
