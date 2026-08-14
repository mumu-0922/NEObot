"use client";

import { useCallback, useEffect, useState } from "react";
import { Archive, FileSearch, ShieldCheck } from "lucide-react";
import { useTranslations } from "next-intl";

import {
  createLegacySkillBackup,
  createLegacySkillDeletionDryRun,
  createLegacySkillInventory,
  serializeLegacySkillArtifact,
  type LegacySkillDeletionPlan,
  type LegacySkillInventory,
} from "@/lib/skills/legacyCutover";
import { appDb, STORAGE_KEYS } from "@/store/storage/storageConfig";

export default function LegacySkillCutoverCard() {
  const t = useTranslations("AgentCenter");
  const [raw, setRaw] = useState<unknown>(null);
  const [inventory, setInventory] = useState<LegacySkillInventory | null>(null);
  const [plan, setPlan] = useState<LegacySkillDeletionPlan | null>(null);
  const [error, setError] = useState("");

  const inspect = useCallback(async () => {
    setError("");
    try {
      const value = await appDb.getItem<unknown>(STORAGE_KEYS.SETTINGS);
      setRaw(value);
      setInventory(await createLegacySkillInventory(value));
      setPlan(null);
    } catch {
      setError(t("legacyInspectFailed"));
    }
  }, [t]);

  useEffect(() => {
    queueMicrotask(() => void inspect());
  }, [inspect]);

  const downloadBackup = () => {
    if (raw === null) return;
    downloadJson(
      "neo-chat-legacy-skills-backup.json",
      createLegacySkillBackup(raw),
    );
  };

  return (
    <section className="rounded-2xl border border-amber-200 bg-amber-50/70 p-4 dark:border-amber-900/60 dark:bg-amber-950/20">
      <div className="flex items-start gap-3">
        <div className="rounded-xl bg-amber-100 p-2 text-amber-700 dark:bg-amber-900/50 dark:text-amber-200">
          <Archive size={18} aria-hidden="true" />
        </div>
        <div className="min-w-0 flex-1">
          <h3 className="font-semibold text-amber-950 dark:text-amber-100">
            {t("legacyTitle")}
          </h3>
          <p className="mt-1 text-sm text-amber-900/75 dark:text-amber-100/70">
            {t("legacyDescription")}
          </p>
        </div>
      </div>

      {error ? (
        <p role="alert" className="mt-3 text-sm text-red-700 dark:text-red-300">
          {error}
        </p>
      ) : inventory ? (
        <div className="mt-4 grid grid-cols-2 gap-2 text-sm sm:grid-cols-4">
          <Metric
            label={t("legacyInstalled")}
            value={inventory.counts.installed}
          />
          <Metric label={t("legacyActive")} value={inventory.counts.active} />
          <Metric
            label={t("legacyCached")}
            value={
              inventory.counts.catalogEntries + inventory.counts.definitions
            }
          />
          <Metric
            label={t("legacyInvalidOrphan")}
            value={inventory.counts.invalid + inventory.counts.orphanActive}
          />
        </div>
      ) : (
        <p className="mt-3 text-sm text-amber-900/70 dark:text-amber-100/70">
          {t("loading")}
        </p>
      )}

      <div className="mt-4 flex flex-wrap gap-2">
        <ActionButton
          onClick={() => void inspect()}
          icon={<FileSearch size={15} />}
        >
          {t("inventoryRefresh")}
        </ActionButton>
        <ActionButton
          onClick={downloadBackup}
          disabled={raw === null}
          icon={<Archive size={15} />}
        >
          {t("downloadBackup")}
        </ActionButton>
        <ActionButton
          onClick={() =>
            inventory && setPlan(createLegacySkillDeletionDryRun(inventory))
          }
          disabled={!inventory}
          icon={<ShieldCheck size={15} />}
        >
          {t("dryRun")}
        </ActionButton>
      </div>

      {plan && (
        <div
          role="status"
          className="mt-4 rounded-xl border border-amber-200 bg-white/70 p-3 text-xs text-amber-950 dark:border-amber-900 dark:bg-black/15 dark:text-amber-100"
        >
          <p className="font-semibold">{t("dryRunReady")}</p>
          <p className="mt-1 break-all font-mono">
            {plan.sourceManifestFingerprint}
          </p>
          <p className="mt-2">
            {t("dryRunFields", { count: plan.deleteStateFields.length })}
          </p>
          <p className="mt-1">
            {t("dryRunPreserved", {
              domains: plan.preservedDomains.join(", "),
            })}
          </p>
          <button
            type="button"
            className="mt-2 font-medium underline underline-offset-2"
            onClick={() =>
              downloadJson("neo-chat-legacy-skills-dry-run.json", plan)
            }
          >
            {t("downloadDryRun")}
          </button>
        </div>
      )}
    </section>
  );
}

function Metric({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-xl bg-white/70 px-3 py-2 dark:bg-black/15">
      <div className="text-lg font-bold tabular-nums">{value}</div>
      <div className="text-xs opacity-70">{label}</div>
    </div>
  );
}

function ActionButton({
  children,
  icon,
  disabled,
  onClick,
}: {
  children: React.ReactNode;
  icon: React.ReactNode;
  disabled?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className="inline-flex items-center gap-1.5 rounded-lg border border-amber-300 bg-white px-3 py-2 text-xs font-medium text-amber-950 transition-colors hover:bg-amber-100 disabled:cursor-not-allowed disabled:opacity-50 dark:border-amber-800 dark:bg-amber-950/30 dark:text-amber-100 dark:hover:bg-amber-900/40"
    >
      {icon}
      {children}
    </button>
  );
}

function downloadJson(fileName: string, value: unknown) {
  const blob = new Blob([serializeLegacySkillArtifact(value)], {
    type: "application/json",
  });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = fileName;
  anchor.click();
  setTimeout(() => URL.revokeObjectURL(url), 0);
}
