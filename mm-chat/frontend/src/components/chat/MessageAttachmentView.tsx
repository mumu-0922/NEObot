"use client";
import React, { useEffect, useRef, useState } from "react";
import {
  Download,
  FileCheck2,
  FileText,
  Library,
  LoaderCircle,
} from "lucide-react";
import { useTranslations } from "next-intl";
import type { Attachment } from "@/types";
import { isOPFSUrl, resolveOPFSUrl } from "@/utils/opfs";
import { resolveObjectUrlWithLifecycle } from "@/lib/utils/objectUrlLifecycle";
import AudioPlayer from "./AudioPlayer";
import {
  isKnowledgeCollectionAttachment,
  isKnowledgeFileAttachment,
} from "@/lib/utils/knowledgeAttachments";
import { isTextDocumentMimeType } from "@/lib/utils/documentAttachments";
import { isServerArtifactAttachment } from "@/lib/utils/serverAttachments";
import {
  downloadServerArtifact,
  formatArtifactSize,
} from "@/lib/utils/artifactAttachments";

interface MessageAttachmentViewProps {
  attachment: Attachment;
  onImageClick: () => void;
  onDocumentClick?: (attachment: Attachment) => void;
}

const actionButtonFocusClass =
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-400/40 focus-visible:ring-offset-2 focus-visible:ring-offset-white dark:focus-visible:ring-offset-background";

const documentCardClass =
  "group/attachment markdown-file-card inline-flex min-w-50 max-w-full select-none items-center gap-3 rounded-xl p-3 text-left transition-[border-color,background-color,box-shadow] md:w-72";

const MessageAttachmentView: React.FC<MessageAttachmentViewProps> = ({
  attachment,
  onImageClick,
  onDocumentClick,
}) => {
  const t = useTranslations("Message");
  const fallbackUrl =
    attachment.url ||
    (attachment.data
      ? `data:${attachment.mimeType};base64,${attachment.data}`
      : "");
  const [resolvedOpfsUrl, setResolvedOpfsUrl] = useState<{
    source: string;
    url: string;
  } | null>(null);
  const [artifactDownloadState, setArtifactDownloadState] = useState<
    "idle" | "downloading" | "error"
  >("idle");
  const artifactDownloadAbortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    if (!isOPFSUrl(attachment.url)) return;

    const source = attachment.url!;
    const resolution = resolveObjectUrlWithLifecycle({
      source,
      resolveObjectUrl: resolveOPFSUrl,
      onResolved: (url) => {
        setResolvedOpfsUrl(url ? { source, url } : null);
      },
      onError: () => setResolvedOpfsUrl(null),
    });
    return () => resolution.cancel();
  }, [attachment.url]);

  useEffect(
    () => () => {
      artifactDownloadAbortRef.current?.abort();
    },
    [],
  );

  const resolvedUrl =
    attachment.url && isOPFSUrl(attachment.url)
      ? resolvedOpfsUrl?.source === attachment.url
        ? resolvedOpfsUrl.url
        : ""
      : fallbackUrl;

  if (isServerArtifactAttachment(attachment)) {
    const sizeLabel = formatArtifactSize(attachment.size);
    const downloading = artifactDownloadState === "downloading";
    const handleArtifactDownload = async () => {
      artifactDownloadAbortRef.current?.abort();
      const controller = new AbortController();
      artifactDownloadAbortRef.current = controller;
      setArtifactDownloadState("downloading");
      try {
        await downloadServerArtifact(attachment, { signal: controller.signal });
        if (!controller.signal.aborted) setArtifactDownloadState("idle");
      } catch {
        if (!controller.signal.aborted) setArtifactDownloadState("error");
      } finally {
        if (artifactDownloadAbortRef.current === controller) {
          artifactDownloadAbortRef.current = null;
        }
      }
    };
    return (
      <div className="markdown-file-card inline-flex min-w-60 max-w-full items-center gap-3 rounded-xl p-3 md:w-80">
        <div className="markdown-file-card-icon">
          <FileCheck2 size={20} aria-hidden="true" />
        </div>
        <div className="flex min-w-0 flex-1 flex-col">
          <span className="markdown-strong-text truncate text-sm font-medium">
            {attachment.fileName}
          </span>
          <span className="markdown-file-card-meta truncate text-xs">
            {[attachment.mimeType, sizeLabel].filter(Boolean).join(" · ")}
          </span>
          <span className="sr-only" aria-live="polite">
            {downloading
              ? t("artifactDownloading")
              : artifactDownloadState === "error"
                ? t("artifactDownloadFailed")
                : t("artifactReady")}
          </span>
          {artifactDownloadState === "error" && (
            <span className="mt-1 text-xs text-red-600 dark:text-red-300">
              {t("artifactDownloadFailed")}
            </span>
          )}
        </div>
        <button
          type="button"
          onClick={handleArtifactDownload}
          disabled={downloading}
          aria-label={t("downloadFileAria", { name: attachment.fileName })}
          className={`inline-flex size-9 shrink-0 items-center justify-center rounded-lg border border-gray-200 bg-white text-gray-700 transition-colors hover:bg-gray-100 disabled:cursor-wait disabled:opacity-60 dark:border-border dark:bg-background dark:text-foreground dark:hover:bg-muted ${actionButtonFocusClass}`}
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
    );
  }

  if (
    isKnowledgeCollectionAttachment(attachment) ||
    isKnowledgeFileAttachment(attachment)
  ) {
    const isFile = isKnowledgeFileAttachment(attachment);
    return (
      <div className="group/attachment relative flex h-20 w-32 select-none flex-col justify-between overflow-hidden rounded-xl border border-purple-100 bg-purple-50/50 p-2.5 transition-colors hover:bg-purple-50 dark:border-purple-900/50 dark:bg-purple-900/20 dark:hover:bg-purple-900/30">
        <div className="flex items-center gap-2">
          <div className="rounded-lg bg-purple-100 p-1.5 text-purple-600 dark:bg-purple-500/20 dark:text-purple-300">
            {isFile ? (
              <FileText size={14} aria-hidden="true" />
            ) : (
              <Library size={14} aria-hidden="true" />
            )}
          </div>
          <span className="text-[9px] font-bold uppercase tracking-wider text-purple-400 dark:text-purple-500">
            {isFile ? t("knowledgeFile") : t("knowledgeBase")}
          </span>
        </div>
        <span className="truncate text-xs font-semibold text-purple-900 dark:text-purple-100">
          {attachment.fileName}
        </span>
      </div>
    );
  }

  if (attachment.mimeType.startsWith("audio/")) {
    return (
      <div className="w-full max-w-sm">
        <AudioPlayer src={resolvedUrl} fileName={attachment.fileName} />
      </div>
    );
  }

  if (attachment.mimeType.startsWith("image/")) {
    return (
      <button
        type="button"
        className={`group/attachment relative cursor-zoom-in overflow-hidden rounded-lg border border-gray-200 bg-gray-50 shadow-sm transition-shadow hover:shadow-md dark:border-border dark:bg-muted ${actionButtonFocusClass}`}
        onClick={onImageClick}
        aria-label={t("previewImageAria", {
          fileName: attachment.fileName,
        })}
      >
        <img
          src={resolvedUrl}
          alt={attachment.fileName}
          width={256}
          height={128}
          loading="lazy"
          decoding="async"
          referrerPolicy="no-referrer"
          className="h-32 w-auto rounded-lg object-cover transition-transform duration-300 group-hover/attachment:scale-110"
        />
      </button>
    );
  }

  const isReadableDocument =
    Boolean(attachment.data) && isTextDocumentMimeType(attachment.mimeType);
  const documentCardBody = (
    <>
      <div className="markdown-file-card-icon">
        <FileText size={20} aria-hidden="true" />
      </div>
      <div className="flex min-w-0 flex-1 flex-col">
        <span className="markdown-strong-text truncate text-sm font-medium">
          {attachment.fileName}
        </span>
        <div className="markdown-file-card-meta flex min-w-0 flex-wrap items-center gap-2 text-xs">
          <span className="markdown-file-card-action">
            {isReadableDocument
              ? t("openDocumentAttachment")
              : t("documentAttachment")}
          </span>
        </div>
      </div>
    </>
  );

  if (isReadableDocument && onDocumentClick) {
    return (
      <button
        type="button"
        aria-label={t("openDocumentAttachmentAria", {
          fileName: attachment.fileName,
        })}
        onClick={() => onDocumentClick(attachment)}
        className={`${documentCardClass} markdown-file-card-interactive markdown-focus-ring cursor-pointer`}
      >
        {documentCardBody}
      </button>
    );
  }

  return (
    <div
      aria-label={attachment.fileName}
      className={`${documentCardClass} cursor-default`}
    >
      {documentCardBody}
    </div>
  );
};

export default MessageAttachmentView;
