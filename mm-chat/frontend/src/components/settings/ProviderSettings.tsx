"use client";
import { useState, useEffect, useRef } from "react";
import {
  Server,
  Trash2,
  Plus,
  RefreshCw,
  Settings,
  AlertCircle,
  Check,
  Eye,
  Paperclip,
  Mic,
  Lightbulb,
  Wrench,
  ExternalLink,
} from "lucide-react";
import { useTranslations } from "next-intl";
import {
  useSettingsStore,
  formatModelName,
  getEffectiveBaseUrl,
} from "@/store/core/settingsStore";
import { useCoreSettingsStore } from "@/store/core/coreSettingsStore";
import Tooltip from "../ui/Tooltip";
import ModelEditor from "./ModelEditor";
import { SecretInput } from "./SettingsUI";
import { PROVIDER_CONFIG_LIMITS } from "@/config/limits";
import { buildProviderRuntimeConfig, encryptSecret } from "@/lib/byok/client";
import { BYOK_CONTEXTS } from "@/lib/byok/shared";
import { SERVER_DEFAULT_PROVIDER_ID } from "@/lib/defaultConfig/shared";
import { createNeoChatApiClient } from "@/services/api/client";
import {
  encryptLocalSecret,
  LOCAL_SECRET_CONTEXTS,
} from "@/lib/security/localSecrets";

const ProviderSettings = () => {
  const t = useTranslations("Providers");
  const serverModeEnabled = createNeoChatApiClient().mode === "server";
  const { modelMetadata, customModelMetadata } = useSettingsStore();

  const {
    _hasHydrated,
    providers,
    addProvider,
    updateProvider,
    deleteProvider,
    replaceServerManagedProviders,
  } = useCoreSettingsStore();

  const [selectedProviderId, setSelectedProviderId] = useState<string | null>(
    null,
  );
  const [fetchingProviderId, setFetchingProviderId] = useState<string | null>(
    null,
  );
  const [editingModelId, setEditingModelId] = useState<string | null>(null);
  const [fetchError, setFetchError] = useState<string | null>(null);
  const [serverProviderHasApiKey, setServerProviderHasApiKey] = useState<
    Record<string, boolean>
  >({});
  const [savingProviderId, setSavingProviderId] = useState<string | null>(null);
  const [deleteConfirmProviderId, setDeleteConfirmProviderId] = useState<
    string | null
  >(null);
  const fetchAbortRef = useRef<AbortController | null>(null);
  const fetchRequestIdRef = useRef(0);
  const selectedProviderIdRef = useRef<string | null>(null);
  const serverProvidersLoadedRef = useRef(false);
  const deleteConfirmTimerRef = useRef<ReturnType<typeof setTimeout> | null>(
    null,
  );

  // Ensure selection validity - wait for hydration
  useEffect(() => {
    if (!_hasHydrated) return;

    if (providers.length > 0) {
      if (
        !selectedProviderId ||
        !providers.find((p) => p.id === selectedProviderId)
      ) {
        setSelectedProviderId(providers[0].id);
      }
    }
  }, [_hasHydrated, providers, selectedProviderId]);

  const currentProvider = providers.find((p) => p.id === selectedProviderId);
  const isServerDefaultProvider = Boolean(currentProvider?.isServerDefault);
  const isFetchingCurrentProvider = fetchingProviderId === currentProvider?.id;
  const currentProviderDomId = currentProvider
    ? currentProvider.id.replace(/[^a-zA-Z0-9_-]/g, "-")
    : "provider";
  const providerNameInputId = `${currentProviderDomId}-provider-name`;
  const providerTypeInputId = `${currentProviderDomId}-provider-type`;
  const providerBaseUrlInputId = `${currentProviderDomId}-provider-base-url`;
  const providerApiKeyInputId = `${currentProviderDomId}-provider-api-key`;
  const providerEnabledInputId = `${currentProviderDomId}-provider-enabled`;
  const providerApiKeyHelpUrl =
    currentProvider?.type === "Gemini"
      ? "https://aistudio.google.com/app/apikey"
      : currentProvider?.type === "OpenAI"
        ? "https://platform.openai.com/api-keys"
        : undefined;

  useEffect(() => {
    if (!_hasHydrated || !serverModeEnabled) return;
    if (serverProvidersLoadedRef.current) return;

    let active = true;
    serverProvidersLoadedRef.current = true;
    createNeoChatApiClient()
      .providers.listAdminProviderConfigs()
      .then(({ providers: serverProviders }) => {
        if (!active) return;
        const secretState: Record<string, boolean> = {};
        replaceServerManagedProviders(
          serverProviders.map((provider) => {
            secretState[provider.id] = provider.hasApiKey;
            return {
              id: provider.id,
              name: provider.name,
              type: provider.type as any,
              baseUrl: provider.baseUrl,
              apiKey: "",
              enabled: provider.enabled,
              models: provider.models,
              modelsList: provider.models,
              isServerDefault: provider.id === SERVER_DEFAULT_PROVIDER_ID,
              isServerManaged: true,
            };
          }),
        );
        setServerProviderHasApiKey(secretState);
      })
      .catch((error) => {
        if (!active) return;
        setFetchError(
          error instanceof Error ? error.message : t("failedToFetchModels"),
        );
      });

    return () => {
      active = false;
    };
  }, [_hasHydrated, replaceServerManagedProviders, serverModeEnabled, t]);

  const persistServerProvider = async (
    providerSnapshot: NonNullable<typeof currentProvider>,
    overrides: {
      models?: string[];
      apiKeySecret?: unknown;
      clearApiKey?: boolean;
    } = {},
  ) => {
    setSavingProviderId(providerSnapshot.id);
    try {
      const input = {
        name: providerSnapshot.name,
        type: providerSnapshot.type,
        baseUrl: providerSnapshot.baseUrl || "",
        models: overrides.models ?? providerSnapshot.models ?? [],
        enabled: providerSnapshot.isServerDefault
          ? true
          : providerSnapshot.enabled,
        ...(overrides.apiKeySecret
          ? { apiKeySecret: overrides.apiKeySecret }
          : {}),
        ...(overrides.clearApiKey ? { clearApiKey: true } : {}),
      };
      const response = providerSnapshot.isServerDefault
        ? await createNeoChatApiClient().providers.updateServerDefaultConfig(
            input,
          )
        : await createNeoChatApiClient().providers.updateAdminProviderConfig(
            providerSnapshot.id,
            input,
          );
      setServerProviderHasApiKey((current) => ({
        ...current,
        [response.id]: response.hasApiKey,
      }));
      updateProvider(response.id, {
        name: response.name,
        type: response.type as any,
        baseUrl: response.baseUrl,
        enabled: response.enabled,
        models: response.models,
        isServerManaged: true,
      });
      return response;
    } finally {
      setSavingProviderId(null);
    }
  };

  const clearDeleteConfirmation = () => {
    if (deleteConfirmTimerRef.current) {
      clearTimeout(deleteConfirmTimerRef.current);
      deleteConfirmTimerRef.current = null;
    }
    setDeleteConfirmProviderId(null);
  };

  useEffect(() => {
    selectedProviderIdRef.current = selectedProviderId;
    setFetchError(null);
    fetchAbortRef.current?.abort();
    fetchAbortRef.current = null;
    setFetchingProviderId(null);
    clearDeleteConfirmation();
  }, [selectedProviderId]);

  useEffect(() => {
    return () => {
      fetchAbortRef.current?.abort();
      if (deleteConfirmTimerRef.current) {
        clearTimeout(deleteConfirmTimerRef.current);
        deleteConfirmTimerRef.current = null;
      }
    };
  }, []);

  const handleDeleteProvider = async () => {
    if (!currentProvider || providers.length <= 1) return;

    if (deleteConfirmProviderId !== currentProvider.id) {
      setDeleteConfirmProviderId(currentProvider.id);
      if (deleteConfirmTimerRef.current) {
        clearTimeout(deleteConfirmTimerRef.current);
      }
      deleteConfirmTimerRef.current = setTimeout(() => {
        deleteConfirmTimerRef.current = null;
        setDeleteConfirmProviderId(null);
      }, 5000);
      return;
    }

    clearDeleteConfirmation();
    if (serverModeEnabled) {
      try {
        await createNeoChatApiClient().providers.deleteAdminProviderConfig(
          currentProvider.id,
        );
      } catch (error) {
        setFetchError(
          error instanceof Error ? error.message : t("errorFetchingModels"),
        );
        return;
      }
    }
    deleteProvider(currentProvider.id);
  };

  const handleFetchModels = async () => {
    if (!currentProvider) {
      setFetchError(t("failedToFetchModels"));
      return;
    }

    fetchAbortRef.current?.abort();
    const controller = new AbortController();
    fetchAbortRef.current = controller;
    const requestId = fetchRequestIdRef.current + 1;
    fetchRequestIdRef.current = requestId;
    const providerSnapshot = currentProvider;

    setFetchingProviderId(providerSnapshot.id);
    setFetchError(null);
    try {
      if (serverModeEnabled) {
        await persistServerProvider(providerSnapshot);
      }
      const data = await createNeoChatApiClient().providers.listModels({
        provider: await buildProviderRuntimeConfig(providerSnapshot),
        signal: controller.signal,
      });

      if (
        requestId !== fetchRequestIdRef.current ||
        controller.signal.aborted
      ) {
        return;
      }

      const models = data.models || [];

      if (models.length > 0) {
        const selectedModels = (providerSnapshot.models || []).filter((model) =>
          models.includes(model),
        );
        const nextModels = selectedModels.length > 0 ? selectedModels : models;
        updateProvider(providerSnapshot.id, {
          modelsList: models,
          models: nextModels,
        });
        if (serverModeEnabled) {
          await persistServerProvider(
            { ...providerSnapshot, models: nextModels, modelsList: models },
            { models: nextModels },
          );
        }
      } else {
        updateProvider(providerSnapshot.id, { modelsList: [] });
        if (selectedProviderIdRef.current === providerSnapshot.id) {
          setFetchError(t("noCompatibleModels"));
        }
      }
    } catch (error) {
      if (controller.signal.aborted) return;
      if (
        requestId === fetchRequestIdRef.current &&
        selectedProviderIdRef.current === providerSnapshot.id
      ) {
        setFetchError(
          error instanceof Error ? error.message : t("errorFetchingModels"),
        );
      }
    } finally {
      if (requestId === fetchRequestIdRef.current) {
        fetchAbortRef.current = null;
        setFetchingProviderId(null);
      }
    }
  };

  const handleAddProvider = async () => {
    const newId = addProvider();
    setSelectedProviderId(newId);
    if (!serverModeEnabled) return;

    const draft = useCoreSettingsStore
      .getState()
      .providers.find((provider) => provider.id === newId);
    if (!draft) return;
    updateProvider(newId, { isServerManaged: true });
    try {
      await persistServerProvider({ ...draft, isServerManaged: true });
    } catch (error) {
      deleteProvider(newId);
      setFetchError(
        error instanceof Error ? error.message : t("errorFetchingModels"),
      );
    }
  };

  const toggleModel = (model: string) => {
    if (!currentProvider) return;
    const currentModels = currentProvider.models || [];
    const newModels = currentModels.includes(model)
      ? currentModels.filter((m) => m !== model)
      : [...currentModels, model];

    updateProvider(currentProvider.id, { models: newModels });
  };

  const displayModels =
    currentProvider?.modelsList && currentProvider.modelsList.length > 0
      ? currentProvider.modelsList
      : currentProvider?.models || [];

  const renderModelCapabilities = (id: string) => {
    const meta = customModelMetadata[id] || modelMetadata[id];
    if (!meta) return null;

    const capabilities = [];

    if (meta.attachment) {
      capabilities.push({
        key: "att",
        icon: Paperclip,
        label: t("capAttachments"),
        className:
          "bg-blue-100 text-blue-600 dark:bg-blue-900/40 dark:text-blue-300 border-blue-200 dark:border-blue-800",
      });
    }
    if (meta.modalities?.input?.includes("image")) {
      capabilities.push({
        key: "vis",
        icon: Eye,
        label: t("capVision"),
        className:
          "bg-green-100 text-green-600 dark:bg-green-900/40 dark:text-green-300 border-green-200 dark:border-green-800",
      });
    }
    if (meta.modalities?.input?.includes("audio")) {
      capabilities.push({
        key: "aud",
        icon: Mic,
        label: t("capAudio"),
        className:
          "bg-orange-100 text-orange-600 dark:bg-orange-900/40 dark:text-orange-300 border-orange-200 dark:border-orange-800",
      });
    }
    if (meta.reasoning) {
      capabilities.push({
        key: "reas",
        icon: Lightbulb,
        label: t("capReasoning"),
        className:
          "bg-violet-100 text-violet-600 dark:bg-violet-900/40 dark:text-violet-300 border-violet-200 dark:border-violet-800",
      });
    }
    if (meta.tool_call) {
      capabilities.push({
        key: "tool",
        icon: Wrench,
        label: t("capFunctionCalling"),
        className:
          "bg-red-100 text-red-600 dark:bg-red-900/40 dark:text-red-300 border-red-200 dark:border-red-800",
      });
    }

    if (capabilities.length === 0) return null;

    return (
      <div
        className="flex flex-wrap gap-1.5 mt-1 md:mt-0 md:ml-2"
        aria-label={t("modelCapabilitiesAria")}
      >
        {capabilities.map((cap) => (
          <Tooltip key={cap.key} content={cap.label} position="top">
            <div
              aria-label={cap.label}
              className={`p-1 rounded-md border ${cap.className}`}
            >
              <cap.icon size={12} aria-hidden="true" />
            </div>
          </Tooltip>
        ))}
      </div>
    );
  };

  return (
    <div className="space-y-6 pb-6 relative">
      {!_hasHydrated && (
        <div className="flex items-center justify-center py-8">
          <div className="text-sm text-gray-500 dark:text-muted-foreground">
            {t("loading")}
          </div>
        </div>
      )}

      {_hasHydrated && (
        <>
          {editingModelId && (
            <ModelEditor
              modelId={editingModelId}
              onClose={() => setEditingModelId(null)}
            />
          )}

          <div className="flex items-center justify-between mb-2">
            <div className="text-sm font-medium text-gray-500 dark:text-muted-foreground uppercase tracking-wider">
              {t("configureProviders")}
            </div>
            <button
              type="button"
              onClick={handleAddProvider}
              className="text-sm text-blue-600 dark:text-blue-400 hover:text-blue-700 dark:hover:text-blue-300 font-medium flex items-center gap-1 bg-blue-50 dark:bg-blue-900/20 px-3 py-1.5 rounded-lg hover:bg-blue-100 dark:hover:bg-blue-900/30 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60"
            >
              <Plus size={16} aria-hidden="true" /> {t("add")}
            </button>
          </div>
          <div
            role="group"
            aria-label={t("modelProvidersAria")}
            className="flex items-center gap-2 overflow-x-auto pb-2 mb-4 custom-scrollbar"
          >
            {providers.map((provider) => (
              <button
                type="button"
                key={provider.id}
                aria-pressed={selectedProviderId === provider.id}
                aria-label={
                  provider.enabled
                    ? t("enabledProviderAria", { name: provider.name })
                    : t("disabledProviderAria", { name: provider.name })
                }
                onClick={() => setSelectedProviderId(provider.id)}
                className={`px-4 py-2 rounded-lg text-sm font-medium border transition-[color,background-color,border-color,box-shadow] flex items-center gap-2 shrink-0 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60 ${selectedProviderId === provider.id ? "bg-blue-50 dark:bg-blue-900/20 border-blue-200 dark:border-blue-800 text-blue-700 dark:text-blue-400" : "bg-white dark:bg-muted border-gray-200 dark:border-border text-gray-600 dark:text-muted-foreground hover:border-gray-300 dark:hover:border-border"}`}
              >
                <span
                  aria-hidden="true"
                  className={`w-2 h-2 rounded-full ${provider.enabled ? "bg-green-500" : "bg-gray-300 dark:bg-accent/80"}`}
                />
                <span>{provider.name}</span>
              </button>
            ))}
          </div>
          {currentProvider && (
            <div className="animate-in fade-in slide-in-from-bottom-2 duration-300 space-y-6">
              <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                {
                  <div className="space-y-2">
                    <label
                      htmlFor={providerNameInputId}
                      className="text-sm font-medium text-gray-700 dark:text-foreground/85"
                    >
                      {t("providerName")}
                    </label>
                    <input
                      id={providerNameInputId}
                      name="providerName"
                      type="text"
                      value={currentProvider.name}
                      maxLength={PROVIDER_CONFIG_LIMITS.maxProviderNameChars}
                      autoComplete="off"
                      spellCheck={false}
                      onChange={(e) =>
                        updateProvider(currentProvider.id, {
                          name: e.target.value,
                        })
                      }
                      className="w-full px-3 py-2 bg-gray-50 dark:bg-muted border border-gray-200 dark:border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 dark:focus:border-blue-400 transition-[border-color,box-shadow] text-gray-800 dark:text-foreground"
                    />
                  </div>
                }
                {
                  <div className="space-y-2">
                    <label
                      htmlFor={providerTypeInputId}
                      className="text-sm font-medium text-gray-700 dark:text-foreground/85"
                    >
                      {t("apiType")}
                    </label>
                    <div className="relative">
                      <select
                        id={providerTypeInputId}
                        name="providerType"
                        value={currentProvider.type}
                        onChange={(e) =>
                          updateProvider(currentProvider.id, {
                            type: e.target.value as any,
                          })
                        }
                        className="w-full px-3 py-2 bg-gray-50 dark:bg-muted border border-gray-200 dark:border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 dark:focus:border-blue-400 transition-[border-color,box-shadow] appearance-none text-gray-800 dark:text-foreground"
                      >
                        <option value="Gemini">Gemini</option>
                        <option value="OpenAI">{t("openaiResponses")}</option>
                        <option value="OpenAI Compatible">
                          {t("openaiCompatible")}
                        </option>
                      </select>
                      <div className="absolute right-3 top-1/2 -translate-y-1/2 pointer-events-none">
                        <Server
                          size={14}
                          className="text-gray-400"
                          aria-hidden="true"
                        />
                      </div>
                    </div>
                  </div>
                }
                {
                  <div className="col-span-1 md:col-span-2 space-y-2">
                    <label
                      htmlFor={providerBaseUrlInputId}
                      className="text-sm font-medium text-gray-700 dark:text-foreground/85"
                    >
                      {t("apiBaseUrl")}
                    </label>
                    <input
                      id={providerBaseUrlInputId}
                      name="providerBaseUrl"
                      type="url"
                      inputMode="url"
                      value={currentProvider.baseUrl}
                      maxLength={PROVIDER_CONFIG_LIMITS.maxBaseUrlChars}
                      autoComplete="off"
                      spellCheck={false}
                      onChange={(e) =>
                        updateProvider(currentProvider.id, {
                          baseUrl: e.target.value,
                        })
                      }
                      placeholder={
                        currentProvider.type === "Gemini"
                          ? t("geminiBaseUrlPlaceholder")
                          : t("openaiBaseUrlPlaceholder")
                      }
                      className="w-full px-3 py-2 bg-gray-50 dark:bg-muted border border-gray-200 dark:border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 dark:focus:border-blue-400 transition-[border-color,box-shadow] font-mono text-gray-600 dark:text-foreground/85"
                    />
                    {currentProvider.baseUrl ? (
                      <div className="text-[11px] text-gray-400 font-mono pl-1 flex items-start gap-1">
                        <span className="break-all text-gray-500 dark:text-muted-foreground">
                          {t("preview")}{" "}
                          {getEffectiveBaseUrl(
                            currentProvider.baseUrl,
                            currentProvider.type,
                          )}
                        </span>
                      </div>
                    ) : null}
                  </div>
                }
                {
                  <div className="col-span-1 md:col-span-2 space-y-2">
                    <label
                      htmlFor={providerApiKeyInputId}
                      className="text-sm font-medium text-gray-700 dark:text-foreground/85 flex items-center justify-between gap-2"
                    >
                      {t("apiKey")}
                      {providerApiKeyHelpUrl ? (
                        <a
                          href={providerApiKeyHelpUrl}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="text-xs text-blue-500 hover:underline flex items-center gap-1"
                        >
                          {t("getKey")}{" "}
                          <ExternalLink size={10} aria-hidden="true" />
                        </a>
                      ) : null}
                    </label>
                    <div className="relative">
                      <SecretInput
                        id={providerApiKeyInputId}
                        name="providerApiKey"
                        maxLength={PROVIDER_CONFIG_LIMITS.maxApiKeyChars}
                        placeholder={t("apiKeyPlaceholder")}
                        hasSecret={Boolean(
                          serverModeEnabled
                            ? serverProviderHasApiKey[currentProvider.id]
                            : currentProvider.apiKey ||
                                currentProvider.apiKeySecret,
                        )}
                        onSave={async (value) => {
                          if (serverModeEnabled) {
                            await persistServerProvider(currentProvider, {
                              apiKeySecret: await encryptSecret(
                                value,
                                BYOK_CONTEXTS.provider(currentProvider.type),
                              ),
                            });
                            return;
                          }
                          updateProvider(currentProvider.id, {
                            apiKey: "",
                            apiKeySecret: await encryptLocalSecret(
                              value,
                              LOCAL_SECRET_CONTEXTS.providerApiKey(
                                currentProvider.id,
                              ),
                            ),
                          });
                        }}
                        onClear={async () => {
                          if (serverModeEnabled) {
                            await persistServerProvider(currentProvider, {
                              clearApiKey: true,
                            });
                            return;
                          }
                          updateProvider(currentProvider.id, {
                            apiKey: "",
                            apiKeySecret: undefined,
                          });
                        }}
                        inputClassName="min-w-0 flex-1 px-3 py-2 bg-gray-50 dark:bg-muted border border-gray-200 dark:border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 dark:focus:border-blue-400 transition-[border-color,box-shadow] font-mono text-gray-800 dark:text-foreground"
                      />
                    </div>
                    <p className="text-xs text-gray-400">
                      {serverModeEnabled
                        ? t("keyStoredOnServer")
                        : t("keyStoredLocally")}
                    </p>
                  </div>
                }
                {serverModeEnabled && (
                  <div className="col-span-1 md:col-span-2 rounded-xl border border-blue-100 bg-blue-50/70 px-4 py-3 text-xs text-blue-700 dark:border-blue-900/40 dark:bg-blue-900/10 dark:text-blue-200">
                    {isServerDefaultProvider
                      ? t("serverDefaultProviderDesc")
                      : t("serverManagedProviderDesc")}
                  </div>
                )}
                <div className="col-span-1 md:col-span-2 flex items-center justify-between pt-2">
                  <label className="inline-flex items-center cursor-pointer group">
                    <div className="relative">
                      <input
                        id={providerEnabledInputId}
                        name="providerEnabled"
                        type="checkbox"
                        className="sr-only peer"
                        checked={currentProvider.enabled}
                        onChange={() =>
                          updateProvider(currentProvider.id, {
                            enabled: !currentProvider.enabled,
                          })
                        }
                      />
                      <div className="w-11 h-6 bg-gray-200 dark:bg-accent peer-focus-visible:ring-2 peer-focus-visible:ring-blue-500/60 peer-focus-visible:ring-offset-2 peer-focus-visible:ring-offset-white dark:peer-focus-visible:ring-offset-background rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-0.5 after:left-0.5 after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-transform peer-checked:bg-blue-500 peer-checked:shadow-[0_0_0_3px_rgba(59,130,246,0.18)] dark:peer-checked:bg-blue-400"></div>
                    </div>
                    <span className="ml-3 text-sm font-medium text-gray-700 dark:text-foreground/85 group-hover:text-gray-900 dark:group-hover:text-foreground transition-colors">
                      {t("enableProvider")}
                    </span>
                  </label>
                  {serverModeEnabled && (
                    <button
                      type="button"
                      disabled={savingProviderId === currentProvider.id}
                      onClick={() => persistServerProvider(currentProvider)}
                      className={`px-3 py-2 rounded-lg text-sm font-medium flex items-center gap-2 transition-[color,background-color,box-shadow] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60 ${
                        savingProviderId === currentProvider.id
                          ? "cursor-not-allowed opacity-70 bg-blue-50 text-blue-500 dark:bg-blue-900/20 dark:text-blue-300"
                          : "bg-blue-50 text-blue-700 hover:bg-blue-100 dark:bg-blue-900/20 dark:text-blue-200 dark:hover:bg-blue-900/30"
                      }`}
                    >
                      <Check size={16} aria-hidden="true" />
                      {savingProviderId === currentProvider.id
                        ? t("savingServerDefault")
                        : t("saveServerDefault")}
                    </button>
                  )}
                  {!isServerDefaultProvider && providers.length > 1 && (
                    <button
                      type="button"
                      aria-label={
                        deleteConfirmProviderId === currentProvider.id
                          ? t("confirmDeleteAria", {
                              name: currentProvider.name,
                            })
                          : t("deleteAria", { name: currentProvider.name })
                      }
                      onClick={handleDeleteProvider}
                      className={`px-3 py-2 rounded-lg text-sm font-medium flex items-center gap-2 transition-[color,background-color,box-shadow] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-500/60 ${
                        deleteConfirmProviderId === currentProvider.id
                          ? "bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-200"
                          : "text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20 hover:text-red-600"
                      }`}
                    >
                      {deleteConfirmProviderId === currentProvider.id ? (
                        <Check size={16} aria-hidden="true" />
                      ) : (
                        <Trash2 size={16} aria-hidden="true" />
                      )}
                      {deleteConfirmProviderId === currentProvider.id
                        ? t("confirmDelete")
                        : t("deleteProvider")}
                    </button>
                  )}
                </div>
              </div>
              <div className="pt-4 border-t border-gray-100 dark:border-border">
                <div className="flex items-center justify-between mb-3">
                  <div className="text-sm font-medium text-gray-700 dark:text-foreground/85">
                    {t("availableModels")}
                  </div>
                  <button
                    type="button"
                    aria-label={t("fetchModelsAria", {
                      name: currentProvider.name,
                    })}
                    onClick={handleFetchModels}
                    disabled={isFetchingCurrentProvider}
                    className={`px-3 py-1.5 rounded-lg border border-blue-200 dark:border-blue-800 bg-blue-50 dark:bg-blue-900/20 text-blue-600 dark:text-blue-400 hover:bg-blue-100 dark:hover:bg-blue-900/40 transition-[color,background-color,border-color,box-shadow] text-xs font-medium flex items-center gap-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60 ${isFetchingCurrentProvider ? "cursor-not-allowed opacity-70" : ""}`}
                  >
                    <RefreshCw
                      size={14}
                      aria-hidden="true"
                      className={
                        isFetchingCurrentProvider ? "animate-spin" : ""
                      }
                    />
                    <span>{t("fetchModels")}</span>
                  </button>
                </div>
                {fetchError ? (
                  <div
                    role="alert"
                    aria-live="polite"
                    className="mb-3 flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-200"
                  >
                    <AlertCircle
                      size={14}
                      className="mt-0.5 shrink-0"
                      aria-hidden="true"
                    />
                    <span>{fetchError}</span>
                  </div>
                ) : null}
                {displayModels.length > 0 ? (
                  <div className="space-y-2">
                    {displayModels.map((model) => (
                      <div
                        key={model}
                        className="flex items-center gap-3 p-2 hover:bg-gray-50 dark:hover:bg-muted rounded-lg transition-colors group"
                      >
                        <label className="flex items-center gap-3 cursor-pointer select-none flex-1 min-w-0 rounded-lg focus-within:ring-2 focus-within:ring-blue-500/60">
                          <span className="relative flex h-5 w-5 shrink-0 items-center justify-center">
                            <input
                              type="checkbox"
                              name="enabledProviderModels"
                              value={model}
                              checked={currentProvider.models.includes(model)}
                              onChange={() => toggleModel(model)}
                              className="peer sr-only"
                            />
                            <span
                              aria-hidden="true"
                              className={`w-5 h-5 rounded border flex items-center justify-center transition-[color,background-color,border-color] shrink-0 ${currentProvider.models.includes(model) ? "bg-blue-500 border-blue-500 text-white" : "border-gray-300 dark:border-input bg-white dark:bg-accent"}`}
                            >
                              {currentProvider.models.includes(model) && (
                                <Check size={12} aria-hidden="true" />
                              )}
                            </span>
                          </span>
                          <div className="flex flex-col md:flex-row md:items-center gap-1 md:gap-3 flex-1 min-w-0">
                            <span className="text-sm text-gray-700 dark:text-foreground font-mono truncate">
                              {formatModelName(
                                model,
                                modelMetadata,
                                customModelMetadata,
                              )}
                            </span>
                            {renderModelCapabilities(model)}
                          </div>
                        </label>
                        <button
                          type="button"
                          aria-label={t("editMetadataAria", {
                            name: formatModelName(
                              model,
                              modelMetadata,
                              customModelMetadata,
                            ),
                          })}
                          onClick={(e) => {
                            e.stopPropagation();
                            setEditingModelId(model);
                          }}
                          className="p-1.5 text-gray-400 hover:text-gray-600 dark:hover:text-foreground hover:bg-gray-200 dark:hover:bg-accent/80 rounded-lg transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60"
                        >
                          <Settings size={14} aria-hidden="true" />
                        </button>
                      </div>
                    ))}
                  </div>
                ) : (
                  <div className="flex flex-col items-center justify-center py-8 text-gray-400">
                    <AlertCircle
                      size={24}
                      className="mb-2 opacity-50"
                      aria-hidden="true"
                    />
                    <span className="text-xs">{t("noModels")}</span>
                  </div>
                )}
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
};

export default ProviderSettings;
