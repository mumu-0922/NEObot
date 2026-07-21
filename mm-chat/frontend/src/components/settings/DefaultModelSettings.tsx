import React, { useEffect, useMemo, useRef, useState } from "react";
import {
  MessageSquarePlus,
  MessageSquareQuote,
  FoldVertical,
  Sparkles,
  Search,
  Brain,
} from "lucide-react";
import { useTranslations } from "next-intl";
import { useSettingsStore, formatModelName } from "@/store/core/settingsStore";
import { useCoreSettingsStore } from "@/store/core/coreSettingsStore";
import { useDefaultModels } from "@/store/hooks/useShallowStore";
import { CustomSelect, GroupedSelectOption } from "./SettingsUI";
import { DefaultModels } from "@/types";
import { getDefaultModelSelectValue } from "@/lib/utils/defaultModels";
import { createNeoChatApiClient } from "@/services/api/client";

type SaveStatus = "idle" | "loading" | "saving" | "saved" | "error";

const DefaultModelSettings = () => {
  const t = useTranslations("DefaultModels");
  const { modelMetadata, customModelMetadata } = useSettingsStore();
  const { providers } = useCoreSettingsStore();
  const { defaultModels, updateDefaultModels } = useDefaultModels();
  const apiClient = useMemo(() => createNeoChatApiClient(), []);
  const serverMode = apiClient.mode === "server";
  const statusTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [saveStatus, setSaveStatus] = useState<SaveStatus>(
    serverMode ? "loading" : "idle",
  );
  const [savingKey, setSavingKey] = useState<keyof DefaultModels>();
  const [saveError, setSaveError] = useState("");

  const groupedOptions: GroupedSelectOption[] = useMemo(() => {
    return providers
      .filter((p) => p.enabled && p.models.length > 0)
      .map((p) => ({
        label: p.name,
        options: p.models.map((mId) => {
          const displayName = formatModelName(
            mId,
            modelMetadata,
            customModelMetadata,
          );
          return {
            value: `${p.id}:${mId}`,
            label: displayName,
          };
        }),
      }));
  }, [providers, modelMetadata, customModelMetadata]);

  useEffect(() => {
    if (!serverMode) return;
    const controller = new AbortController();
    let active = true;
    setSaveStatus("loading");
    setSaveError("");

    apiClient.settings
      .getTaskModels({ signal: controller.signal })
      .then(async (response) => {
        if (!active) return;
        if (response.configured) {
          updateDefaultModels(response.models);
        }
        setSaveStatus("idle");
      })
      .catch((cause) => {
        if (!active || controller.signal.aborted) return;
        setSaveStatus("error");
        setSaveError(cause instanceof Error ? cause.message : t("saveFailed"));
      });

    return () => {
      active = false;
      controller.abort();
    };
  }, [apiClient, serverMode, t, updateDefaultModels]);

  useEffect(
    () => () => {
      if (statusTimerRef.current) clearTimeout(statusTimerRef.current);
    },
    [],
  );

  const saveTaskModel = async (
    valueKey: keyof DefaultModels,
    value: string,
  ) => {
    const previous = useCoreSettingsStore.getState().defaultModels[valueKey];
    updateDefaultModels({ [valueKey]: value });
    if (!serverMode) return;

    if (statusTimerRef.current) clearTimeout(statusTimerRef.current);
    setSavingKey(valueKey);
    setSaveStatus("saving");
    setSaveError("");
    try {
      const response = await apiClient.settings.updateTaskModels({
        [valueKey]: value,
      });
      updateDefaultModels(response.models);
      setSaveStatus("saved");
      statusTimerRef.current = setTimeout(() => setSaveStatus("idle"), 1600);
    } catch (cause) {
      updateDefaultModels({ [valueKey]: previous });
      setSaveStatus("error");
      setSaveError(cause instanceof Error ? cause.message : t("saveFailed"));
    } finally {
      setSavingKey(undefined);
    }
  };

  const getEffectiveValue = (taskKey: keyof DefaultModels) => {
    return getDefaultModelSelectValue(defaultModels, taskKey, providers);
  };

  const renderSettingRow = (
    icon: React.ReactNode,
    label: string,
    description: string,
    valueKey: keyof DefaultModels,
    colorClass: string,
  ) => (
    <div className="flex flex-col md:flex-row md:items-center justify-between p-4 bg-gray-50/50 dark:bg-muted/30 border border-gray-200 dark:border-border rounded-xl gap-4">
      <div className="flex items-start gap-3 flex-1">
        <div className={`p-2 rounded-lg ${colorClass} bg-opacity-10 shrink-0`}>
          {icon}
        </div>
        <div>
          <div className="font-medium text-sm text-gray-800 dark:text-foreground">
            {label}
          </div>
          <div className="text-xs text-gray-500 dark:text-muted-foreground mt-0.5 leading-relaxed">
            {description}
          </div>
        </div>
      </div>
      <div className="w-full md:w-64 shrink-0">
        <CustomSelect
          ariaLabel={t("defaultModelForAria", { label })}
          value={getEffectiveValue(valueKey)}
          onChange={(val) => void saveTaskModel(valueKey, val)}
          options={groupedOptions}
          disabled={savingKey !== undefined || saveStatus === "loading"}
        />
      </div>
    </div>
  );

  return (
    <div className="space-y-6 animate-in fade-in slide-in-from-bottom-2 duration-300">
      <div>
        <h3 className="text-lg font-semibold text-gray-800 dark:text-foreground">
          {t("title")}
        </h3>
        <p className="text-xs text-gray-500 dark:text-muted-foreground mt-1">
          {t("subtitle")}
        </p>
        {serverMode && saveStatus !== "idle" && (
          <p
            role={saveStatus === "error" ? "alert" : "status"}
            className={`mt-2 text-xs ${
              saveStatus === "error"
                ? "text-red-600 dark:text-red-400"
                : "text-gray-500 dark:text-muted-foreground"
            }`}
          >
            {saveStatus === "loading"
              ? t("loading")
              : saveStatus === "saving"
                ? t("saving")
                : saveStatus === "saved"
                  ? t("saved")
                  : saveError || t("saveFailed")}
          </p>
        )}
      </div>

      <div className="flex flex-col gap-3">
        {renderSettingRow(
          <MessageSquareQuote
            size={18}
            className="text-blue-500"
            aria-hidden="true"
          />,
          t("conversationTitle"),
          t("conversationTitleDesc"),
          "titleGeneration",
          "bg-blue-50 text-blue-600 dark:bg-blue-900/20 dark:text-blue-400",
        )}

        {renderSettingRow(
          <MessageSquarePlus
            size={18}
            className="text-green-500"
            aria-hidden="true"
          />,
          t("relatedQuestions"),
          t("relatedQuestionsDesc"),
          "relatedQuestions",
          "bg-green-50 text-green-600 dark:bg-green-900/20 dark:text-green-400",
        )}

        {renderSettingRow(
          <FoldVertical
            size={18}
            className="text-orange-500"
            aria-hidden="true"
          />,
          t("contextCompression"),
          t("contextCompressionDesc"),
          "contextCompression",
          "bg-orange-50 text-orange-600 dark:bg-orange-900/20 dark:text-orange-400",
        )}

        {renderSettingRow(
          <Sparkles size={18} className="text-purple-500" aria-hidden="true" />,
          t("promptOptimization"),
          t("promptOptimizationDesc"),
          "promptOptimization",
          "bg-purple-50 text-purple-600 dark:bg-purple-900/20 dark:text-purple-400",
        )}

        {renderSettingRow(
          <Search size={18} className="text-pink-500" aria-hidden="true" />,
          t("ragQuery"),
          t("ragQueryDesc"),
          "ragQuery",
          "bg-pink-50 text-pink-600 dark:bg-pink-900/20 dark:text-pink-400",
        )}

        {renderSettingRow(
          <Brain size={18} className="text-cyan-500" aria-hidden="true" />,
          t("memory"),
          t("memoryDesc"),
          "memory",
          "bg-cyan-50 text-cyan-600 dark:bg-cyan-900/20 dark:text-cyan-400",
        )}
      </div>
    </div>
  );
};

export default DefaultModelSettings;
