"use client";

import React, {
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";
import { Check, Folder, Library, Loader2, RefreshCw, X } from "lucide-react";
import { useTranslations } from "next-intl";
import { logDevError } from "@/lib/utils/devLogger";
import { createNeoChatApiClient } from "@/services/api/client";
import type { KnowledgeCollectionDTO } from "@/services/api/client";

interface KnowledgeSelectionModalProps {
  onClose: () => void;
  onSelectCollections: (collectionIds: string[]) => void | Promise<void>;
  initialSelectedCollectionIds?: readonly string[];
  maxSelectedCollections?: number;
}

const KnowledgeSelectionModal = ({
  onClose,
  onSelectCollections,
  initialSelectedCollectionIds = [],
  maxSelectedCollections = 8,
}: KnowledgeSelectionModalProps) => {
  const t = useTranslations("Knowledge");
  const apiClient = useMemo(() => createNeoChatApiClient(), []);
  const [collections, setCollections] = useState<KnowledgeCollectionDTO[]>([]);
  const [selectedIds, setSelectedIds] = useState(
    () =>
      new Set(initialSelectedCollectionIds.slice(0, maxSelectedCollections)),
  );
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const titleId = useId();
  const dialogRef = useRef<HTMLDivElement>(null);

  const refreshCollections = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const response = await apiClient.knowledge.listCollections({
        limit: 100,
      });
      setCollections(response.items);
    } catch (cause) {
      logDevError("Failed to load server knowledge collections", cause);
      setError(t("serverLoadCollectionsFailed"));
    } finally {
      setLoading(false);
    }
  }, [apiClient, t]);

  useEffect(() => {
    void refreshCollections();
  }, [refreshCollections]);

  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    dialogRef.current?.focus({ preventScroll: true });
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => {
      window.removeEventListener("keydown", handleKeyDown);
      previous?.focus?.({ preventScroll: true });
    };
  }, [onClose]);

  const toggleCollection = (collectionId: string) => {
    setSelectedIds((current) => {
      const next = new Set(current);
      if (next.has(collectionId)) {
        next.delete(collectionId);
        setError("");
        return next;
      }
      if (next.size >= maxSelectedCollections) {
        setError(t("selectionLimit", { max: maxSelectedCollections }));
        return current;
      }
      next.add(collectionId);
      setError("");
      return next;
    });
  };

  const confirmSelection = async () => {
    const collectionIds = Array.from(selectedIds).slice(
      0,
      maxSelectedCollections,
    );
    setSaving(true);
    setError("");
    try {
      await onSelectCollections(collectionIds);
      onClose();
    } catch (cause) {
      logDevError("Failed to save conversation knowledge selection", cause);
      setError(t("saveSelectionFailed"));
    } finally {
      setSaving(false);
    }
  };

  return createPortal(
    <div
      className="fixed inset-0 z-9999 flex items-center justify-center bg-black/35 p-4 dark:bg-black/65"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
        className="glass-popover flex max-h-[82vh] w-full max-w-xl flex-col rounded-2xl border outline-none"
      >
        <div className="flex items-center justify-between border-b border-gray-200/50 px-5 py-4 dark:border-border">
          <h3
            id={titleId}
            className="flex items-center gap-2 text-lg font-semibold"
          >
            <Library size={20} className="text-purple-500" aria-hidden="true" />
            {t("selectKnowledgeBase")}
          </h3>
          <button
            type="button"
            aria-label={t("closeSelection")}
            onClick={onClose}
            className="rounded-full p-1.5 text-gray-500 hover:bg-gray-200/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-purple-500/60 dark:hover:bg-accent/50"
          >
            <X size={18} aria-hidden="true" />
          </button>
        </div>

        <div className="custom-scrollbar min-h-0 flex-1 space-y-3 overflow-y-auto p-5">
          <div className="flex items-center justify-between gap-3">
            <span className="text-xs text-gray-500 dark:text-muted-foreground">
              {t("selectedCount", { count: selectedIds.size })}
            </span>
            <button
              type="button"
              onClick={() => void refreshCollections()}
              disabled={loading}
              className="inline-flex items-center gap-1.5 rounded-lg px-2 py-1 text-xs text-gray-600 hover:bg-gray-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-purple-500/60 disabled:opacity-50 dark:text-muted-foreground dark:hover:bg-muted"
            >
              <RefreshCw size={13} className={loading ? "animate-spin" : ""} />
              {t("serverRefreshCollections")}
            </button>
          </div>

          {loading ? (
            <div
              role="status"
              className="flex items-center justify-center gap-2 py-10 text-sm text-gray-500"
            >
              <Loader2 size={18} className="animate-spin" aria-hidden="true" />
              {t("serverLoadingCollections")}
            </div>
          ) : collections.length === 0 ? (
            <div className="rounded-xl border border-dashed border-gray-200 py-10 text-center text-sm text-gray-500 dark:border-border">
              {t("serverNoCollections")}
            </div>
          ) : (
            collections.map((collection) => {
              const selected = selectedIds.has(collection.id);
              return (
                <button
                  key={collection.id}
                  type="button"
                  aria-pressed={selected}
                  aria-label={
                    selected
                      ? t("unselectCollectionAria", { name: collection.name })
                      : t("selectCollectionAria", { name: collection.name })
                  }
                  onClick={() => toggleCollection(collection.id)}
                  className={`flex w-full items-center gap-3 rounded-xl border p-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-purple-500/60 ${
                    selected
                      ? "border-purple-500/50 bg-purple-50 dark:bg-purple-900/20"
                      : "border-gray-200 bg-white hover:border-purple-300 dark:border-border dark:bg-muted"
                  }`}
                >
                  <span
                    className={`flex h-5 w-5 shrink-0 items-center justify-center rounded-full border ${selected ? "border-purple-500 bg-purple-500 text-white" : "border-gray-300 dark:border-input"}`}
                  >
                    {selected ? <Check size={12} strokeWidth={3} /> : null}
                  </span>
                  <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-purple-100 text-purple-600 dark:bg-purple-900/40 dark:text-purple-300">
                    <Folder size={20} aria-hidden="true" />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-medium">
                      {collection.name}
                    </span>
                    <span className="mt-1 block truncate text-xs text-gray-500 dark:text-muted-foreground">
                      {collection.description || t("noDescription")}
                    </span>
                  </span>
                </button>
              );
            })
          )}

          {error ? (
            <div
              role="alert"
              className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-200"
            >
              {error}
            </div>
          ) : null}
        </div>

        <div className="flex justify-end gap-3 border-t border-gray-200/50 p-5 dark:border-border">
          <button
            type="button"
            onClick={onClose}
            className="rounded-xl px-4 py-2 text-sm font-medium text-gray-600 hover:bg-gray-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-purple-500/60 dark:text-muted-foreground dark:hover:bg-muted"
          >
            {t("cancel")}
          </button>
          <button
            type="button"
            onClick={() => void confirmSelection()}
            disabled={saving}
            className="inline-flex items-center gap-2 rounded-xl bg-purple-600 px-5 py-2 text-sm font-medium text-white hover:bg-purple-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-purple-500/60 disabled:opacity-50"
          >
            {saving ? (
              <Loader2 size={15} className="animate-spin" />
            ) : (
              <Check size={15} />
            )}
            {saving ? t("savingSelection") : t("saveSelection")}
          </button>
        </div>
      </div>
    </div>,
    document.body,
  );
};

export default KnowledgeSelectionModal;
