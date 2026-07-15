import React from "react";
import {
  AlertTriangle,
  CheckCircle2,
  Download,
  Loader2,
  RotateCcw,
  UploadCloud,
} from "lucide-react";
import { useTranslations } from "next-intl";
import {
  BrowserImportPackageError,
  createBrowserImportPackage,
  type BrowserImportPackageResult,
} from "@/lib/data/browserImportPackage";
import { ApiClientError } from "@/services/api/client";
import {
  createBrowserImportService,
  type BrowserImportService,
} from "@/services/api/importService";
import { useChatStore } from "@/store/core/chatStore";
import type {
  BrowserImportCommitResponse,
  BrowserImportIssue,
  BrowserImportPreviewResponse,
} from "@/services/api/client";

type MigrationStep = "idle" | "previewing" | "committing" | "rollingBack";

const BrowserDataMigrationPanel = () => {
  const t = useTranslations("System");
  const importService = React.useMemo<BrowserImportService>(
    () => createBrowserImportService(),
    [],
  );
  const currentSessionId = useChatStore((state) => state.currentSessionId);
  const syncActiveSession = useChatStore((state) => state.syncActiveSession);
  const refreshServerSessions = useChatStore(
    (state) => state.refreshServerSessions,
  );
  const [step, setStep] = React.useState<MigrationStep>("idle");
  const [packageResult, setPackageResult] =
    React.useState<BrowserImportPackageResult | null>(null);
  const [preview, setPreview] =
    React.useState<BrowserImportPreviewResponse | null>(null);
  const [commit, setCommit] =
    React.useState<BrowserImportCommitResponse | null>(null);
  const [error, setError] = React.useState<string | null>(null);
  const [rollbackDone, setRollbackDone] = React.useState(false);

  const isBusy = step !== "idle";
  const canPreview = importService.serverEnabled && !isBusy;
  const canCommit =
    importService.serverEnabled &&
    !isBusy &&
    Boolean(packageResult && preview?.commitAllowed);
  const canRollback =
    importService.serverEnabled && !isBusy && Boolean(commit?.batchId);

  const handlePreview = async () => {
    if (!canPreview) return;
    setStep("previewing");
    setError(null);
    setPreview(null);
    setCommit(null);
    setRollbackDone(false);

    try {
      if (currentSessionId) {
        await syncActiveSession(currentSessionId);
      }
      const nextPackage = await createBrowserImportPackage();
      const nextPreview = await importService.preview(nextPackage.blob, {
        fileName: nextPackage.fileName,
      });
      setPackageResult(nextPackage);
      setPreview(nextPreview);
    } catch (nextError) {
      setPackageResult(null);
      setError(formatMigrationError(nextError, t));
    } finally {
      setStep("idle");
    }
  };

  const handleCommit = async () => {
    if (!canCommit || !packageResult) return;
    setStep("committing");
    setError(null);
    setRollbackDone(false);

    try {
      const response = await importService.commit(packageResult.blob, {
        fileName: packageResult.fileName,
      });
      setCommit(response);
      await refreshServerSessions();
    } catch (nextError) {
      setError(formatMigrationError(nextError, t));
    } finally {
      setStep("idle");
    }
  };

  const handleRollback = async () => {
    if (!canRollback || !commit?.batchId) return;
    setStep("rollingBack");
    setError(null);

    try {
      await importService.rollbackBatch(commit.batchId);
      setRollbackDone(true);
      await refreshServerSessions();
    } catch (nextError) {
      setError(formatMigrationError(nextError, t));
    } finally {
      setStep("idle");
    }
  };

  const handleDownloadPackage = () => {
    if (!packageResult) return;
    const url = URL.createObjectURL(packageResult.blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = packageResult.fileName;
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
    URL.revokeObjectURL(url);
  };

  return (
    <div className="rounded-xl border border-blue-200 bg-blue-50/60 p-4 dark:border-blue-900/50 dark:bg-blue-950/20">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <div className="font-medium text-gray-800 dark:text-foreground">
            {t("serverImportTitle")}
          </div>
          <div className="mt-1 text-xs text-gray-600 dark:text-muted-foreground">
            {t("serverImportDesc")}
          </div>
        </div>
        <button
          type="button"
          onClick={handlePreview}
          disabled={!canPreview}
          aria-busy={step === "previewing"}
          className="inline-flex items-center gap-2 rounded-lg border border-blue-200 bg-white px-4 py-2 text-sm font-medium text-blue-700 shadow-sm transition-colors hover:bg-blue-50 disabled:cursor-not-allowed disabled:opacity-60 dark:border-blue-800 dark:bg-muted dark:text-blue-200 dark:hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60"
        >
          {step === "previewing" ? (
            <Loader2 size={14} className="animate-spin" aria-hidden="true" />
          ) : (
            <UploadCloud size={14} aria-hidden="true" />
          )}
          {step === "previewing"
            ? t("serverImportPreviewing")
            : t("serverImportPreview")}
        </button>
      </div>

      {!importService.serverEnabled ? (
        <StatusMessage tone="warning" className="mt-3">
          {t("serverImportServerModeRequired")}
        </StatusMessage>
      ) : null}

      {preview ? (
        <div className="mt-4 space-y-3">
          <div className="grid gap-2 sm:grid-cols-4">
            <SummaryPill
              label={t("serverImportConversations")}
              value={preview.summary.conversations}
            />
            <SummaryPill
              label={t("serverImportMessages")}
              value={preview.summary.messages}
            />
            <SummaryPill
              label={t("serverImportFiles")}
              value={preview.summary.files}
            />
            <SummaryPill
              label={t("serverImportBytes")}
              value={formatBytes(preview.summary.bytes)}
            />
          </div>

          {preview.warnings.length > 0 ? (
            <IssueList
              title={t("serverImportWarnings")}
              issues={preview.warnings}
              tone="warning"
            />
          ) : null}
          {preview.errors.length > 0 ? (
            <IssueList
              title={t("serverImportErrors")}
              issues={preview.errors}
              tone="danger"
            />
          ) : null}

          <div className="flex flex-wrap gap-2">
            <button
              type="button"
              onClick={handleCommit}
              disabled={!canCommit}
              aria-busy={step === "committing"}
              className="inline-flex items-center gap-2 rounded-lg border border-green-200 bg-white px-4 py-2 text-sm font-medium text-green-700 shadow-sm transition-colors hover:bg-green-50 disabled:cursor-not-allowed disabled:opacity-60 dark:border-green-900 dark:bg-muted dark:text-green-200 dark:hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-green-500/60"
            >
              {step === "committing" ? (
                <Loader2
                  size={14}
                  className="animate-spin"
                  aria-hidden="true"
                />
              ) : (
                <CheckCircle2 size={14} aria-hidden="true" />
              )}
              {step === "committing"
                ? t("serverImportCommitting")
                : t("serverImportCommit")}
            </button>
            <button
              type="button"
              onClick={handleDownloadPackage}
              disabled={!packageResult || isBusy}
              className="inline-flex items-center gap-2 rounded-lg border border-gray-200 bg-white px-4 py-2 text-sm font-medium text-gray-700 shadow-sm transition-colors hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-60 dark:border-border dark:bg-muted dark:text-foreground dark:hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60"
            >
              <Download size={14} aria-hidden="true" />
              {t("serverImportDownloadPackage")}
            </button>
          </div>
        </div>
      ) : null}

      {commit ? (
        <StatusMessage tone="success" className="mt-3">
          {t("serverImportCommitSuccess", {
            batchId: commit.batchId,
            conversations: commit.created.conversations,
            messages: commit.created.messages,
            files: commit.created.files,
          })}
          <button
            type="button"
            onClick={handleRollback}
            disabled={!canRollback || rollbackDone}
            aria-busy={step === "rollingBack"}
            className="ml-3 inline-flex items-center gap-1 rounded-md border border-green-300 bg-white px-2 py-1 text-xs font-medium text-green-700 transition-colors hover:bg-green-50 disabled:cursor-not-allowed disabled:opacity-60 dark:border-green-800 dark:bg-muted dark:text-green-200 dark:hover:bg-accent"
          >
            {step === "rollingBack" ? (
              <Loader2 size={12} className="animate-spin" aria-hidden="true" />
            ) : (
              <RotateCcw size={12} aria-hidden="true" />
            )}
            {rollbackDone
              ? t("serverImportRolledBack")
              : t("serverImportRollback")}
          </button>
        </StatusMessage>
      ) : null}

      {error ? (
        <StatusMessage tone="danger" className="mt-3">
          {error}
        </StatusMessage>
      ) : null}
    </div>
  );
};

const SummaryPill = ({
  label,
  value,
}: {
  label: string;
  value: React.ReactNode;
}) => (
  <div className="rounded-lg border border-white/70 bg-white/80 px-3 py-2 dark:border-border dark:bg-card/80">
    <div className="text-[11px] font-medium uppercase tracking-wide text-gray-500 dark:text-muted-foreground">
      {label}
    </div>
    <div className="mt-1 text-sm font-semibold text-gray-800 dark:text-foreground">
      {value}
    </div>
  </div>
);

const IssueList = ({
  title,
  issues,
  tone,
}: {
  title: string;
  issues: BrowserImportIssue[];
  tone: "warning" | "danger";
}) => (
  <div
    className={`rounded-lg border px-3 py-2 text-xs ${
      tone === "danger"
        ? "border-red-200 bg-red-50 text-red-800 dark:border-red-900/60 dark:bg-red-950/40 dark:text-red-100"
        : "border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/40 dark:text-amber-100"
    }`}
  >
    <div className="mb-1 flex items-center gap-1 font-medium">
      <AlertTriangle size={13} aria-hidden="true" />
      {title}
    </div>
    <ul className="space-y-1">
      {issues.slice(0, 5).map((issue, index) => (
        <li key={`${issue.code}:${issue.path}:${index}`}>
          <span className="font-mono">{issue.code}</span>
          {issue.path ? ` · ${issue.path}` : ""}: {issue.message}
        </li>
      ))}
      {issues.length > 5 ? <li>+{issues.length - 5}</li> : null}
    </ul>
  </div>
);

const StatusMessage = ({
  tone,
  className = "",
  children,
}: {
  tone: "success" | "warning" | "danger";
  className?: string;
  children: React.ReactNode;
}) => {
  const classes = {
    success:
      "border-green-200 bg-green-50 text-green-800 dark:border-green-900/60 dark:bg-green-950/40 dark:text-green-100",
    warning:
      "border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/40 dark:text-amber-100",
    danger:
      "border-red-200 bg-red-50 text-red-800 dark:border-red-900/60 dark:bg-red-950/40 dark:text-red-100",
  }[tone];

  return (
    <div
      role={tone === "danger" ? "alert" : "status"}
      aria-live="polite"
      className={`rounded-lg border px-3 py-2 text-xs ${classes} ${className}`}
    >
      {children}
    </div>
  );
};

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  let value = bytes;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }
  return `${value.toFixed(value >= 10 || unitIndex === 0 ? 0 : 1)} ${units[unitIndex]}`;
}

function formatMigrationError(
  error: unknown,
  t: ReturnType<typeof useTranslations>,
): string {
  if (error instanceof BrowserImportPackageError) {
    if (error.code === "MISSING_OPFS_FILE") {
      return t("serverImportMissingOpfs", {
        count: error.missingOpfsUrls.length,
      });
    }
    return error.message;
  }
  if (error instanceof ApiClientError) {
    return `${error.code}: ${error.message}`;
  }
  if (error instanceof Error) return error.message;
  return t("serverImportError");
}

export default BrowserDataMigrationPanel;
