"use client";

import { useCallback, useEffect, useState } from "react";
import {
  AlertCircle,
  Check,
  Loader2,
  Power,
  RefreshCw,
  Server,
  Trash2,
} from "lucide-react";
import { useTranslations } from "next-intl";
import { encryptSecret } from "@/lib/byok/client";
import { BYOK_CONTEXTS } from "@/lib/byok/shared";
import { createNeoChatApiClient } from "@/services/api/client";
import type { AdminVoiceProviderConfigDTO } from "@/services/api/client";
import { useSettingsStore } from "@/store/core/settingsStore";
import { SecretInput } from "./SettingsUI";

const PROVIDER_ID = "siliconflow" as const;
const BASE_URL = "https://api.siliconflow.cn/v1";
const MODEL = "FunAudioLLM/CosyVoice2-0.5B";
const VOICE = "FunAudioLLM/CosyVoice2-0.5B:claire";

type BusyAction =
  "save" | "test" | "activate" | "deactivate" | "delete" | "clear";

export default function SiliconFlowVoiceProviderSettings() {
  const t = useTranslations("Voice");
  const applyServerConfig = useSettingsStore(
    (state) => state.applyServerConfig,
  );
  const [provider, setProvider] = useState<AdminVoiceProviderConfigDTO>();
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState<BusyAction>();
  const [error, setError] = useState<string>();
  const [success, setSuccess] = useState<string>();
  const [confirmDelete, setConfirmDelete] = useState(false);
  const serverMode = createNeoChatApiClient().mode === "server";

  const refresh = useCallback(async () => {
    const client = createNeoChatApiClient();
    const response =
      await client.voiceProviders.listAdminVoiceProviderConfigs();
    setProvider(
      response.providers.find((item) => item.provider === PROVIDER_ID),
    );
    try {
      applyServerConfig(await client.settings.getRuntimeConfig());
    } catch {
      return;
    }
  }, [applyServerConfig]);

  useEffect(() => {
    if (!serverMode) {
      setLoading(false);
      return;
    }
    let active = true;
    createNeoChatApiClient()
      .voiceProviders.listAdminVoiceProviderConfigs()
      .then((response) => {
        if (!active) return;
        setProvider(
          response.providers.find((item) => item.provider === PROVIDER_ID),
        );
      })
      .catch((cause) => {
        if (!active) return;
        setError(
          cause instanceof Error ? cause.message : t("hostedRequestFailed"),
        );
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [serverMode, t]);

  if (!serverMode) return null;

  const runAction = async (
    action: BusyAction,
    operation: () => Promise<string>,
  ) => {
    setBusy(action);
    setError(undefined);
    setSuccess(undefined);
    try {
      setSuccess(await operation());
      await refresh();
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : t("hostedRequestFailed"),
      );
    } finally {
      setBusy(undefined);
    }
  };

  const saveAndTest = async (apiKey?: string) => {
    await runAction(apiKey ? "save" : "test", async () => {
      const client = createNeoChatApiClient();
      const apiKeySecret = apiKey
        ? await encryptSecret(apiKey, BYOK_CONTEXTS.voiceProvider(PROVIDER_ID))
        : undefined;
      await client.voiceProviders.updateAdminVoiceProviderConfig(PROVIDER_ID, {
        enabled: provider?.enabled ?? false,
        ...(apiKeySecret ? { apiKeySecret } : {}),
      });
      const tested =
        await client.voiceProviders.testAdminVoiceProviderConnection(
          PROVIDER_ID,
        );
      return t("hostedTestPassed", { size: tested.size });
    });
  };

  const toggleActivation = async () => {
    if (provider?.enabled) {
      await runAction("deactivate", async () => {
        await createNeoChatApiClient().voiceProviders.updateAdminVoiceProviderConfig(
          PROVIDER_ID,
          { enabled: false },
        );
        return t("hostedDeactivated");
      });
      return;
    }
    await runAction("activate", async () => {
      await createNeoChatApiClient().voiceProviders.activateAdminVoiceProvider(
        PROVIDER_ID,
      );
      return t("hostedActivated");
    });
  };

  const clearKey = async () => {
    await runAction("clear", async () => {
      await createNeoChatApiClient().voiceProviders.updateAdminVoiceProviderConfig(
        PROVIDER_ID,
        { enabled: false, clearApiKey: true },
      );
      return t("hostedKeyCleared");
    });
  };

  const deleteProvider = async () => {
    if (!confirmDelete) {
      setConfirmDelete(true);
      return;
    }
    setConfirmDelete(false);
    await runAction("delete", async () => {
      await createNeoChatApiClient().voiceProviders.deleteAdminVoiceProviderConfig(
        PROVIDER_ID,
      );
      return t("hostedDeleted");
    });
  };

  return (
    <section className="space-y-4 rounded-lg border border-gray-200 p-4 dark:border-border">
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-3">
          <Server
            size={18}
            className="mt-0.5 shrink-0 text-blue-500"
            aria-hidden="true"
          />
          <div>
            <p className="font-medium text-gray-800 dark:text-foreground">
              {t("hostedTitle")}
            </p>
            <p className="mt-1 text-xs text-gray-500 dark:text-muted-foreground">
              {provider?.enabled
                ? t("hostedActive")
                : provider?.connectionTestValid
                  ? t("hostedTested")
                  : provider?.hasApiKey
                    ? t("hostedSaved")
                    : t("hostedNotConfigured")}
            </p>
          </div>
        </div>
        {loading ? (
          <Loader2
            size={16}
            className="animate-spin text-gray-400"
            aria-label={t("hostedLoading")}
          />
        ) : null}
      </div>

      <dl className="grid gap-2 rounded-md bg-gray-50 p-3 text-xs dark:bg-muted sm:grid-cols-[7rem_1fr]">
        <dt className="text-gray-500 dark:text-muted-foreground">Endpoint</dt>
        <dd className="break-all font-mono text-gray-700 dark:text-foreground/85">
          {provider?.baseUrl ?? BASE_URL}
        </dd>
        <dt className="text-gray-500 dark:text-muted-foreground">Model</dt>
        <dd className="break-all font-mono text-gray-700 dark:text-foreground/85">
          {provider?.model ?? MODEL}
        </dd>
        <dt className="text-gray-500 dark:text-muted-foreground">Voice</dt>
        <dd className="break-all font-mono text-gray-700 dark:text-foreground/85">
          {provider?.voice ?? VOICE}
        </dd>
      </dl>

      <div className="space-y-1.5">
        <label
          htmlFor="voice-siliconflow-api-key"
          className="text-sm font-medium text-gray-700 dark:text-foreground/85"
        >
          SiliconFlow API Key
        </label>
        <SecretInput
          id="voice-siliconflow-api-key"
          name="siliconFlowVoiceApiKey"
          placeholder="sk-…"
          hasSecret={provider?.hasApiKey === true}
          statusText={
            provider?.hasApiKey ? t("hostedKeyStored") : t("hostedKeyNotStored")
          }
          onSave={saveAndTest}
          onClear={provider?.hasApiKey ? clearKey : undefined}
        />
      </div>

      <div className="flex flex-wrap gap-2">
        <button
          type="button"
          disabled={Boolean(busy) || !provider?.hasApiKey}
          onClick={() => saveAndTest()}
          className="inline-flex items-center gap-2 rounded-lg bg-blue-500 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {busy === "test" ? (
            <Loader2 size={15} className="animate-spin" aria-hidden="true" />
          ) : (
            <RefreshCw size={15} aria-hidden="true" />
          )}
          {t("hostedRetest")}
        </button>
        <button
          type="button"
          disabled={Boolean(busy) || !provider?.hasApiKey}
          onClick={toggleActivation}
          className="inline-flex items-center gap-2 rounded-lg border border-gray-200 px-3 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50 dark:border-border dark:text-foreground dark:hover:bg-muted"
        >
          <Power size={15} aria-hidden="true" />
          {provider?.enabled ? t("hostedDeactivate") : t("hostedActivate")}
        </button>
        <button
          type="button"
          disabled={Boolean(busy) || !provider}
          onClick={deleteProvider}
          className="inline-flex items-center gap-2 rounded-lg border border-gray-200 px-3 py-2 text-sm font-medium text-gray-600 transition-colors hover:bg-red-50 hover:text-red-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-500/50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-border dark:text-muted-foreground dark:hover:bg-red-500/10 dark:hover:text-red-300"
        >
          <Trash2 size={15} aria-hidden="true" />
          {confirmDelete ? t("hostedConfirmDelete") : t("hostedDelete")}
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
    </section>
  );
}
