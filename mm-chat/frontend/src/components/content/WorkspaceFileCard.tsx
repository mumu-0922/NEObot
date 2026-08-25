"use client";

import React, { useEffect, useMemo, useState } from "react";
import { createPortal } from "react-dom";
import {
  AlertTriangle,
  Download,
  FileSpreadsheet,
  FileText,
  LoaderCircle,
  X,
} from "lucide-react";
import { useTranslations } from "next-intl";
import type { MessageOutputBlock } from "@/types";
import {
  createNeoChatApiClient,
  type WorkspaceFilePreviewDTO,
} from "@/services/api/client";
import { formatArtifactSize } from "@/lib/utils/artifactAttachments";
import { sanitizeDownloadFilename } from "@/lib/utils/filename";

type WorkspaceFileBlock = Extract<
  MessageOutputBlock,
  { type: "workspace_file" }
>;

interface WorkspaceFileCardProps {
  file: WorkspaceFileBlock;
}

type BinaryPreview = {
  kind: "image" | "audio" | "pdf";
  url: string;
};

const focusClass =
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-400/50 focus-visible:ring-offset-2 dark:focus-visible:ring-offset-background";

export default function WorkspaceFileCard({ file }: WorkspaceFileCardProps) {
  const t = useTranslations("Content");
  const messageT = useTranslations("Message");
  const api = useMemo(() => createNeoChatApiClient().workspaces, []);
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [downloading, setDownloading] = useState(false);
  const [error, setError] = useState(false);
  const [preview, setPreview] = useState<WorkspaceFilePreviewDTO | null>(null);
  const [binaryPreview, setBinaryPreview] = useState<BinaryPreview | null>(
    null,
  );
  const [activeSheet, setActiveSheet] = useState(0);

  useEffect(
    () => () => {
      if (binaryPreview) URL.revokeObjectURL(binaryPreview.url);
    },
    [binaryPreview],
  );

  useEffect(() => {
    if (!open) return;
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpen(false);
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [open]);

  const loadPreview = async () => {
    setOpen(true);
    setLoading(true);
    setError(false);
    setPreview(null);
    setActiveSheet(0);
    try {
      if (!api) throw new Error("Workspace API unavailable");
      const nextPreview = await api.previewFile({
        workspaceId: file.workspaceId,
        path: file.path,
      });
      setPreview(nextPreview);
      const binaryKind = workspaceBinaryPreviewKind(nextPreview.mimeType);
      if (binaryKind) {
        const blob = await api.readFile({
          workspaceId: file.workspaceId,
          path: file.path,
        });
        setBinaryPreview((previous) => {
          if (previous) URL.revokeObjectURL(previous.url);
          return { kind: binaryKind, url: URL.createObjectURL(blob) };
        });
      } else {
        setBinaryPreview((previous) => {
          if (previous) URL.revokeObjectURL(previous.url);
          return null;
        });
      }
    } catch {
      setError(true);
    } finally {
      setLoading(false);
    }
  };

  const download = async () => {
    setDownloading(true);
    try {
      if (!api) throw new Error("Workspace API unavailable");
      const blob = await api.readFile({
        workspaceId: file.workspaceId,
        path: file.path,
        download: true,
      });
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = sanitizeDownloadFilename(
        file.fileName,
        "workspace-file",
      );
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
      URL.revokeObjectURL(url);
    } catch {
      setError(true);
      setOpen(true);
    } finally {
      setDownloading(false);
    }
  };

  const icon = file.fileName.toLowerCase().endsWith(".xlsx") ? (
    <FileSpreadsheet size={20} aria-hidden="true" />
  ) : (
    <FileText size={20} aria-hidden="true" />
  );

  return (
    <>
      <div className="markdown-file-card my-2 inline-flex min-w-60 max-w-full items-center gap-2 rounded-xl p-2 md:w-88">
        <button
          type="button"
          onClick={loadPreview}
          aria-label={t("openWorkspaceFileAria", { name: file.fileName })}
          className={`markdown-file-card-interactive flex min-w-0 flex-1 items-center gap-3 rounded-lg p-1.5 text-left ${focusClass}`}
        >
          <span className="markdown-file-card-icon">{icon}</span>
          <span className="flex min-w-0 flex-1 flex-col">
            <span className="markdown-strong-text truncate text-sm font-medium">
              {file.fileName}
            </span>
            <span className="markdown-file-card-meta truncate text-xs">
              {t("openWorkspaceFile")} · {file.path} ·{" "}
              {formatArtifactSize(file.size)}
            </span>
          </span>
        </button>
        <button
          type="button"
          onClick={download}
          disabled={downloading}
          aria-label={messageT("downloadFileAria", { name: file.fileName })}
          className={`inline-flex size-9 shrink-0 items-center justify-center rounded-lg border border-gray-200 bg-white text-gray-700 hover:bg-gray-100 disabled:cursor-wait disabled:opacity-60 dark:border-border dark:bg-background dark:text-foreground dark:hover:bg-muted ${focusClass}`}
        >
          {downloading ? (
            <LoaderCircle
              className="animate-spin"
              size={17}
              aria-hidden="true"
            />
          ) : (
            <Download size={17} aria-hidden="true" />
          )}
        </button>
      </div>
      {open && typeof document !== "undefined"
        ? createPortal(
            <div
              className="fixed inset-0 z-[100] flex items-center justify-center bg-black/55 p-4 backdrop-blur-sm"
              role="presentation"
              onMouseDown={(event) => {
                if (event.target === event.currentTarget) setOpen(false);
              }}
            >
              <section
                role="dialog"
                aria-modal="true"
                aria-label={file.fileName}
                className="flex max-h-[90vh] w-full max-w-6xl flex-col overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-2xl dark:border-border dark:bg-background"
              >
                <header className="flex items-center gap-3 border-b border-gray-200 px-4 py-3 dark:border-border">
                  <span className="markdown-file-card-icon">{icon}</span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate font-medium">
                      {file.fileName}
                    </span>
                    <span className="block truncate text-xs text-gray-500 dark:text-muted-foreground">
                      {file.path}
                    </span>
                  </span>
                  <button
                    type="button"
                    onClick={download}
                    disabled={downloading}
                    aria-label={messageT("downloadFileAria", {
                      name: file.fileName,
                    })}
                    className={`inline-flex size-9 items-center justify-center rounded-lg hover:bg-gray-100 dark:hover:bg-muted ${focusClass}`}
                  >
                    <Download size={18} aria-hidden="true" />
                  </button>
                  <button
                    type="button"
                    onClick={() => setOpen(false)}
                    aria-label={t("closeWorkspacePreview")}
                    className={`inline-flex size-9 items-center justify-center rounded-lg hover:bg-gray-100 dark:hover:bg-muted ${focusClass}`}
                  >
                    <X size={19} aria-hidden="true" />
                  </button>
                </header>
                <WorkspacePreviewBody
                  file={file}
                  loading={loading}
                  error={error}
                  preview={preview}
                  binaryPreview={binaryPreview}
                  activeSheet={activeSheet}
                  setActiveSheet={setActiveSheet}
                />
              </section>
            </div>,
            document.body,
          )
        : null}
    </>
  );
}

function WorkspacePreviewBody({
  file,
  loading,
  error,
  preview,
  binaryPreview,
  activeSheet,
  setActiveSheet,
}: {
  file: WorkspaceFileBlock;
  loading: boolean;
  error: boolean;
  preview: WorkspaceFilePreviewDTO | null;
  binaryPreview: BinaryPreview | null;
  activeSheet: number;
  setActiveSheet: (index: number) => void;
}) {
  const t = useTranslations("Content");
  if (loading) {
    return (
      <div className="flex min-h-80 items-center justify-center gap-2 text-sm text-gray-500">
        <LoaderCircle className="animate-spin" size={18} aria-hidden="true" />
        {t("workspacePreviewLoading")}
      </div>
    );
  }
  if (error || !preview) {
    return (
      <div className="flex min-h-80 items-center justify-center gap-2 text-sm text-red-600 dark:text-red-300">
        <AlertTriangle size={18} aria-hidden="true" />
        {t("workspacePreviewFailed")}
      </div>
    );
  }
  const changed = preview.version !== file.version;
  const notice = changed || preview.truncated;
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {notice ? (
        <div className="border-b border-amber-200 bg-amber-50 px-4 py-2 text-xs text-amber-900 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-200">
          {changed ? t("workspaceFileChanged") : t("workspacePreviewTruncated")}
        </div>
      ) : null}
      <div className="min-h-0 flex-1 overflow-auto p-4">
        {binaryPreview?.kind === "image" ? (
          <img
            src={binaryPreview.url}
            alt={file.fileName}
            className="mx-auto max-h-[72vh] max-w-full object-contain"
          />
        ) : binaryPreview?.kind === "audio" ? (
          <audio controls src={binaryPreview.url} className="w-full" />
        ) : binaryPreview?.kind === "pdf" ? (
          <iframe
            title={file.fileName}
            src={binaryPreview.url}
            className="h-[72vh] w-full rounded-lg border border-gray-200 dark:border-border"
          />
        ) : preview.kind === "xlsx" ? (
          <SpreadsheetPreview
            preview={preview}
            activeSheet={activeSheet}
            setActiveSheet={setActiveSheet}
          />
        ) : preview.kind === "text" || preview.kind === "docx" ? (
          <pre className="whitespace-pre-wrap break-words font-mono text-sm leading-6">
            {preview.text}
          </pre>
        ) : (
          <div className="flex min-h-64 items-center justify-center text-sm text-gray-500 dark:text-muted-foreground">
            {t("workspacePreviewUnsupported")}
          </div>
        )}
      </div>
    </div>
  );
}

function SpreadsheetPreview({
  preview,
  activeSheet,
  setActiveSheet,
}: {
  preview: WorkspaceFilePreviewDTO;
  activeSheet: number;
  setActiveSheet: (index: number) => void;
}) {
  const sheets = preview.sheets ?? [];
  const sheet = sheets[activeSheet] ?? sheets[0];
  return (
    <div className="min-w-max">
      {sheets.length > 1 ? (
        <div className="sticky top-0 z-10 mb-3 flex gap-1 bg-white pb-2 dark:bg-background">
          {sheets.map((item, index) => (
            <button
              type="button"
              key={`${item.name}-${index}`}
              onClick={() => setActiveSheet(index)}
              className={`rounded-lg px-3 py-1.5 text-xs font-medium ${
                index === activeSheet
                  ? "bg-blue-600 text-white"
                  : "bg-gray-100 text-gray-700 hover:bg-gray-200 dark:bg-muted dark:text-foreground"
              }`}
            >
              {item.name}
            </button>
          ))}
        </div>
      ) : null}
      {sheet ? (
        <table className="border-collapse text-sm">
          <tbody>
            {sheet.rows.map((row, rowIndex) => (
              <tr key={rowIndex}>
                <th className="sticky left-0 border border-gray-200 bg-gray-50 px-2 py-1 text-right text-xs font-normal text-gray-400 dark:border-border dark:bg-muted">
                  {rowIndex + 1}
                </th>
                {row.map((cell, columnIndex) => (
                  <td
                    key={columnIndex}
                    className="max-w-80 border border-gray-200 px-2 py-1 align-top whitespace-pre-wrap dark:border-border"
                  >
                    {cell}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}
    </div>
  );
}

function workspaceBinaryPreviewKind(
  mimeType: string,
): BinaryPreview["kind"] | null {
  if (mimeType.startsWith("image/")) return "image";
  if (mimeType.startsWith("audio/")) return "audio";
  if (mimeType === "application/pdf") return "pdf";
  return null;
}
