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
import { useTranslations } from "next-intl";
import {
  AlertCircle,
  Archive,
  Atom,
  BookText,
  Cat,
  ChartLine,
  Check,
  CheckCircle2,
  ChessKnight,
  ChevronRight,
  CodeXml,
  Coffee,
  FileText,
  Folder,
  GraduationCap,
  Library,
  MessagesSquare,
  Microscope,
  Plus,
  RefreshCw,
  Save,
  Search,
  Settings,
  Trash2,
  UploadCloud,
  X,
} from "lucide-react";
import { createNeoChatApiClient } from "@/services/api/client";
import type {
  KnowledgeCollectionDTO,
  KnowledgeDocumentDTO,
} from "@/services/api/client";
import {
  formatBytes as formatLimitBytes,
  KNOWLEDGE_LIMITS,
} from "@/config/limits";
import { logDevError } from "@/lib/utils/devLogger";

interface ServerKnowledgeBaseProps {
  onClose?: () => void;
}

type CollectionFormData = {
  name: string;
  description: string;
  icon: string;
  color: string;
};

const COLLECTION_ICONS = [
  { name: "Folder", icon: Folder },
  { name: "Atom", icon: Atom },
  { name: "BookText", icon: BookText },
  { name: "Microscope", icon: Microscope },
  { name: "Cat", icon: Cat },
  { name: "ChartLine", icon: ChartLine },
  { name: "ChessKnight", icon: ChessKnight },
  { name: "CodeXml", icon: CodeXml },
  { name: "Coffee", icon: Coffee },
  { name: "GraduationCap", icon: GraduationCap },
  { name: "MessagesSquare", icon: MessagesSquare },
  { name: "Archive", icon: Archive },
];

const COLLECTION_COLORS = [
  { name: "blue", className: "bg-blue-500" },
  { name: "purple", className: "bg-purple-500" },
  { name: "green", className: "bg-green-500" },
  { name: "orange", className: "bg-orange-500" },
  { name: "red", className: "bg-red-500" },
  { name: "pink", className: "bg-pink-500" },
  { name: "cyan", className: "bg-cyan-500" },
  { name: "gray", className: "bg-gray-500" },
];

const newIdempotencyKey = (prefix: string): string => {
  const randomId = globalThis.crypto?.randomUUID?.();
  return `${prefix}-${randomId ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`}`;
};

const statusClass = (status: string) => {
  switch (status) {
    case "active":
      return "border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900/60 dark:bg-emerald-950/40 dark:text-emerald-300";
    case "failed":
    case "tombstoned":
      return "border-red-200 bg-red-50 text-red-700 dark:border-red-900/60 dark:bg-red-950/40 dark:text-red-300";
    case "processing":
    case "uploaded":
      return "border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-900/60 dark:bg-amber-950/40 dark:text-amber-300";
    default:
      return "border-border bg-muted text-muted-foreground";
  }
};

const formatBytes = (value: number): string => formatLimitBytes(value);

export default function ServerKnowledgeBase({
  onClose,
}: ServerKnowledgeBaseProps) {
  const t = useTranslations("Knowledge");
  const apiClient = useMemo(() => createNeoChatApiClient(), []);
  const knowledgeSupported =
    apiClient.mode === "server" &&
    apiClient.capabilities.knowledge &&
    apiClient.capabilities.files;

  const [collections, setCollections] = useState<KnowledgeCollectionDTO[]>([]);
  const [documents, setDocuments] = useState<KnowledgeDocumentDTO[]>([]);
  const [documentCounts, setDocumentCounts] = useState<Record<string, number>>(
    {},
  );
  const [selectedCollectionId, setSelectedCollectionId] = useState<
    string | null
  >(null);
  const [searchTerm, setSearchTerm] = useState("");
  const [showNewModal, setShowNewModal] = useState(false);
  const [editingCollection, setEditingCollection] =
    useState<KnowledgeCollectionDTO | null>(null);
  const [loadingCollections, setLoadingCollections] = useState(false);
  const [loadingDocuments, setLoadingDocuments] = useState(false);
  const [busyAction, setBusyAction] = useState<string | null>(null);
  const [isDragging, setIsDragging] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement | null>(null);

  const selectedCollection = useMemo(
    () =>
      collections.find(
        (collection) => collection.id === selectedCollectionId,
      ) ?? null,
    [collections, selectedCollectionId],
  );
  const canManageCollection = selectedCollection?.permissions.manage === true;
  const filteredCollections = useMemo(() => {
    const query = searchTerm.trim().toLocaleLowerCase();
    if (!query) return collections;
    return collections.filter(
      (collection) =>
        collection.name.toLocaleLowerCase().includes(query) ||
        collection.description.toLocaleLowerCase().includes(query),
    );
  }, [collections, searchTerm]);

  const showError = useCallback((message: string, caught: unknown) => {
    logDevError(message, caught);
    setNotice(null);
    setError(message);
  }, []);

  const refreshCollections = useCallback(async () => {
    if (!knowledgeSupported) return;
    setLoadingCollections(true);
    setError(null);
    try {
      const page = await apiClient.knowledge.listCollections({ limit: 100 });
      setCollections(page.items);
      const countResults = await Promise.allSettled(
        page.items.map(async (collection) => {
          const documentsPage = await apiClient.knowledge.listDocuments({
            collectionId: collection.id,
            limit: KNOWLEDGE_LIMITS.maxFilesPerCollection,
          });
          return [collection.id, documentsPage.items.length] as const;
        }),
      );
      setDocumentCounts(
        Object.fromEntries(
          countResults.flatMap((result) =>
            result.status === "fulfilled" ? [result.value] : [],
          ),
        ),
      );
      setSelectedCollectionId((current) => {
        if (current && page.items.some((item) => item.id === current)) {
          return current;
        }
        return null;
      });
    } catch (caught) {
      showError(t("serverLoadCollectionsFailed"), caught);
    } finally {
      setLoadingCollections(false);
    }
  }, [apiClient, knowledgeSupported, showError, t]);

  const refreshDocuments = useCallback(async () => {
    if (!knowledgeSupported || !selectedCollectionId) {
      setDocuments([]);
      return;
    }
    setLoadingDocuments(true);
    setError(null);
    try {
      const page = await apiClient.knowledge.listDocuments({
        collectionId: selectedCollectionId,
        limit: 100,
      });
      setDocuments(page.items);
      setDocumentCounts((current) => ({
        ...current,
        [selectedCollectionId]: page.items.length,
      }));
    } catch (caught) {
      showError(t("serverLoadDocumentsFailed"), caught);
    } finally {
      setLoadingDocuments(false);
    }
  }, [apiClient, knowledgeSupported, selectedCollectionId, showError, t]);

  useEffect(() => {
    void refreshCollections();
  }, [refreshCollections]);

  useEffect(() => {
    void refreshDocuments();
  }, [refreshDocuments]);

  const runAction = async (key: string, action: () => Promise<void>) => {
    setBusyAction(key);
    setError(null);
    setNotice(null);
    try {
      await action();
    } finally {
      setBusyAction(null);
    }
  };

  const handleCreateCollection = async (data: CollectionFormData) => {
    await runAction("create-collection", async () => {
      try {
        const collection = await apiClient.knowledge.createCollection({
          name: data.name.trim(),
          description: data.description.trim(),
          icon: data.icon,
          color: data.color,
          scope: "personal",
          idempotencyKey: newIdempotencyKey("knowledge-collection"),
        });
        setCollections((current) => [collection, ...current]);
        setDocumentCounts((current) => ({ ...current, [collection.id]: 0 }));
        setShowNewModal(false);
        setNotice(t("serverCollectionCreated"));
      } catch (caught) {
        showError(t("serverCreateCollectionFailed"), caught);
      }
    });
  };

  const handleUpdateCollection = async (
    collection: KnowledgeCollectionDTO,
    data: CollectionFormData,
  ) => {
    await runAction(`update-${collection.id}`, async () => {
      try {
        const updated = await apiClient.knowledge.updateCollection({
          collectionId: collection.id,
          name: data.name.trim(),
          description: data.description.trim(),
          icon: data.icon,
          color: data.color,
        });
        setCollections((current) =>
          current.map((item) => (item.id === updated.id ? updated : item)),
        );
        setEditingCollection(null);
        setNotice(t("serverCollectionUpdated"));
      } catch (caught) {
        showError(t("serverUpdateCollectionFailed"), caught);
      }
    });
  };

  const handleDeleteCollection = async (collection: KnowledgeCollectionDTO) => {
    if (
      !window.confirm(
        t("serverConfirmDeleteCollection", { name: collection.name }),
      )
    ) {
      return;
    }
    await runAction(`delete-${collection.id}`, async () => {
      try {
        await apiClient.knowledge.deleteCollection({
          collectionId: collection.id,
        });
        setCollections((current) =>
          current.filter((item) => item.id !== collection.id),
        );
        setDocumentCounts((current) => {
          const next = { ...current };
          delete next[collection.id];
          return next;
        });
        setSelectedCollectionId((current) =>
          current === collection.id ? null : current,
        );
        setEditingCollection(null);
        setDocuments([]);
        setNotice(t("serverCollectionDeleted"));
        await refreshCollections();
      } catch (caught) {
        showError(t("serverDeleteCollectionFailed"), caught);
      }
    });
  };

  const uploadAndBindFiles = async (files: File[]) => {
    if (!selectedCollection || files.length === 0) return;
    const collectionId = selectedCollection.id;
    await runAction("upload-documents", async () => {
      try {
        const accepted = files.slice(
          0,
          Math.max(
            0,
            KNOWLEDGE_LIMITS.maxFilesPerCollection - documents.length,
          ),
        );
        for (const file of accepted) {
          const fileRecord = await apiClient.files.uploadFile({
            file,
            fileName: file.name,
            purpose: "knowledge",
            knowledgeCollectionId: collectionId,
          });
          try {
            await apiClient.knowledge.bindDocument({
              collectionId,
              fileId: fileRecord.id,
              idempotencyKey: newIdempotencyKey("knowledge-document"),
            });
          } catch (caught) {
            try {
              await apiClient.files.deleteFile(fileRecord.id);
            } catch (cleanupError) {
              logDevError("Failed to clean up unbound knowledge file", {
                cleanupError,
                fileId: fileRecord.id,
              });
            }
            throw caught;
          }
        }
        setNotice(t("serverDocumentsBound", { count: accepted.length }));
        await refreshDocuments();
      } catch (caught) {
        showError(t("serverBindDocumentFailed"), caught);
      } finally {
        if (fileInputRef.current) fileInputRef.current.value = "";
      }
    });
  };

  const handleFileInput = (files: FileList | null) => {
    if (!files) return;
    void uploadAndBindFiles(Array.from(files));
  };

  const handleDrop = (event: React.DragEvent<HTMLDivElement>) => {
    event.preventDefault();
    setIsDragging(false);
    if (event.dataTransfer.files.length > 0) {
      void uploadAndBindFiles(Array.from(event.dataTransfer.files));
    }
  };

  const handleDeleteDocument = (document: KnowledgeDocumentDTO) => {
    if (!window.confirm(t("serverConfirmDeleteDocument"))) return;
    const previousDocuments = documents;
    setDocuments((current) =>
      current.filter((item) => item.id !== document.id),
    );
    if (selectedCollectionId) {
      setDocumentCounts((current) => ({
        ...current,
        [selectedCollectionId]: Math.max(
          0,
          (current[selectedCollectionId] ?? documents.length) - 1,
        ),
      }));
    }
    void runAction(`delete-document-${document.id}`, async () => {
      try {
        await apiClient.knowledge.deleteDocument({ documentId: document.id });
        setNotice(t("serverDocumentDeleted"));
      } catch (caught) {
        setDocuments(previousDocuments);
        if (selectedCollectionId) {
          setDocumentCounts((current) => ({
            ...current,
            [selectedCollectionId]: previousDocuments.length,
          }));
        }
        showError(t("serverDeleteDocumentFailed"), caught);
      }
    });
  };

  const handleReprocessDocument = (document: KnowledgeDocumentDTO) => {
    void runAction(`reprocess-${document.id}`, async () => {
      try {
        const updated = await apiClient.knowledge.reprocessDocument({
          documentId: document.id,
          idempotencyKey: newIdempotencyKey("knowledge-reprocess"),
        });
        setDocuments((current) =>
          current.map((item) => (item.id === updated.id ? updated : item)),
        );
        setNotice(t("serverDocumentReprocessQueued"));
      } catch (caught) {
        showError(t("serverReprocessDocumentFailed"), caught);
      }
    });
  };

  if (!knowledgeSupported) {
    return (
      <div className="flex h-full w-full flex-col overflow-hidden bg-gray-50/50 dark:bg-background">
        <Header
          onClose={onClose}
          subtitle={t("serverUnsupportedDescription")}
        />
        <div className="flex flex-1 items-center justify-center p-6">
          <div className="max-w-md rounded-2xl border border-border bg-card p-6 text-center shadow-sm">
            <Library className="mx-auto mb-3 text-muted-foreground" size={28} />
            <h2 className="text-base font-semibold text-foreground">
              {t("title")}
            </h2>
            <p className="mt-2 text-sm text-muted-foreground">
              {t("serverUnsupportedDescription")}
            </p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="relative flex h-full w-full animate-in flex-col overflow-hidden bg-gray-50/50 duration-300 fade-in dark:bg-background">
      {showNewModal && (
        <ServerCollectionModal
          title={t("newCollection")}
          busy={busyAction !== null}
          onSubmit={handleCreateCollection}
          onClose={() => setShowNewModal(false)}
        />
      )}
      {editingCollection && (
        <ServerCollectionModal
          title={t("editCollection")}
          initialData={editingCollection}
          busy={busyAction !== null}
          onSubmit={(data) => handleUpdateCollection(editingCollection, data)}
          onDelete={() => handleDeleteCollection(editingCollection)}
          onClose={() => setEditingCollection(null)}
        />
      )}

      <Header
        onClose={onClose}
        subtitle={
          selectedCollection
            ? selectedCollection.description || t("manageDocsSubtitle")
            : t("manageCollectionsSubtitle")
        }
        activeName={selectedCollection?.name}
        onBack={
          selectedCollection ? () => setSelectedCollectionId(null) : undefined
        }
      />

      {(error || notice) && (
        <div className="mx-6 mt-4">
          <div
            role="status"
            className={`flex items-start gap-2 rounded-xl border px-4 py-3 text-sm ${
              error
                ? "border-red-200 bg-red-50 text-red-700 dark:border-red-900/60 dark:bg-red-950/40 dark:text-red-300"
                : "border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900/60 dark:bg-emerald-950/40 dark:text-emerald-300"
            }`}
          >
            {error ? <AlertCircle size={16} /> : <CheckCircle2 size={16} />}
            <span>{error ?? notice}</span>
          </div>
        </div>
      )}

      {!selectedCollection && (
        <div className="mx-auto flex w-full max-w-7xl shrink-0 gap-3 px-6 pb-6 pt-6">
          <div className="group relative min-w-0 flex-1">
            <div className="relative flex items-center rounded-2xl border border-gray-200 bg-white px-4 py-3 shadow-sm focus-within:border-purple-500/50 focus-within:ring-2 focus-within:ring-purple-500/30 dark:border-border dark:bg-muted">
              <Search
                size={20}
                className="mr-3 text-gray-400"
                aria-hidden="true"
              />
              <input
                type="text"
                autoComplete="off"
                spellCheck={false}
                aria-label={t("searchCollectionsLabel")}
                placeholder={t("searchCollectionsPlaceholder")}
                className="min-w-0 flex-1 border-none bg-transparent text-base text-gray-800 outline-none placeholder-gray-400 dark:text-foreground"
                value={searchTerm}
                onChange={(event) => setSearchTerm(event.target.value)}
              />
              <button
                type="button"
                onClick={() => void refreshCollections()}
                disabled={loadingCollections || busyAction !== null}
                className="rounded-lg p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-purple-500/60 disabled:opacity-50 dark:hover:bg-muted dark:hover:text-foreground"
                aria-label={t("serverRefreshCollections")}
              >
                <RefreshCw
                  size={16}
                  className={loadingCollections ? "animate-spin" : ""}
                  aria-hidden="true"
                />
              </button>
            </div>
          </div>
        </div>
      )}

      <div
        className={`custom-scrollbar min-h-0 flex-1 overflow-y-auto overscroll-contain px-6 [scrollbar-gutter:stable] ${selectedCollection ? "py-6" : "pb-10"}`}
      >
        <div className="mx-auto flex min-h-full max-w-7xl flex-col">
          {!selectedCollection ? (
            loadingCollections ? (
              <EmptyState text={t("serverLoadingCollections")} />
            ) : (
              <>
                <div className="grid content-start grid-cols-1 gap-6 animate-in duration-500 fade-in slide-in-from-bottom-4 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
                  <CreateCollectionCard onClick={() => setShowNewModal(true)} />
                  {filteredCollections.map((collection) => (
                    <ServerCollectionCard
                      key={collection.id}
                      collection={collection}
                      fileCount={documentCounts[collection.id] ?? 0}
                      onClick={() => setSelectedCollectionId(collection.id)}
                      onEdit={(event) => {
                        event.stopPropagation();
                        setEditingCollection(collection);
                      }}
                    />
                  ))}
                </div>
                {filteredCollections.length === 0 && searchTerm && (
                  <div className="py-20 text-center text-gray-400">
                    <p>{t("noCollectionsMatch", { term: searchTerm })}</p>
                  </div>
                )}
              </>
            )
          ) : (
            <div className="animate-in duration-300 fade-in slide-in-from-right-8">
              <div className="flex flex-col gap-6">
                <section
                  onDragOver={(event) => {
                    event.preventDefault();
                    setIsDragging(true);
                  }}
                  onDragLeave={() => setIsDragging(false)}
                  onDrop={handleDrop}
                  className={`group relative flex flex-col items-center justify-center overflow-hidden rounded-xl border-2 border-dashed p-10 text-center transition-[border-color,background-color,transform] duration-300 ${
                    isDragging
                      ? "scale-[1.01] border-purple-500 bg-purple-50 dark:bg-purple-900/10"
                      : "border-gray-300 bg-white/50 hover:border-purple-400 dark:border-border dark:bg-muted/20 dark:hover:border-purple-600"
                  }`}
                >
                  <input
                    ref={fileInputRef}
                    type="file"
                    multiple
                    className="sr-only"
                    onChange={(event) => handleFileInput(event.target.files)}
                  />
                  <button
                    type="button"
                    onClick={() => fileInputRef.current?.click()}
                    disabled={!canManageCollection || busyAction !== null}
                    className="group/upload flex max-w-full flex-col items-center rounded-xl px-4 py-3 text-center hover:bg-white/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-purple-500/70 focus-visible:ring-offset-2 disabled:opacity-60 dark:hover:bg-muted/50"
                  >
                    <span className="mb-4 flex h-16 w-16 items-center justify-center rounded-2xl border border-gray-100 bg-white text-purple-500 shadow-sm transition-transform duration-300 group-hover/upload:scale-110 dark:border-border dark:bg-muted">
                      <UploadCloud size={32} aria-hidden="true" />
                    </span>
                    <span className="mb-1 text-base font-bold text-gray-800 dark:text-foreground">
                      {t("chooseFiles")}
                    </span>
                    <span className="text-xs text-gray-500 dark:text-muted-foreground">
                      {t("chooseFilesHint")}
                    </span>
                  </button>
                  <p className="mt-3 text-xs text-gray-500 dark:text-muted-foreground">
                    {t("uploadSupportedRag")}
                  </p>
                  <p className="mt-2 text-[11px] text-gray-400 dark:text-muted-foreground/70">
                    {t("uploadLimits", {
                      max: KNOWLEDGE_LIMITS.maxFilesPerCollection,
                      size: formatBytes(KNOWLEDGE_LIMITS.maxFileBytes),
                    })}
                  </p>
                </section>

                <section className="overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-border dark:bg-card/50">
                  <div className="flex items-center justify-between gap-3 border-b border-gray-100 bg-gray-50/80 p-4 dark:border-border dark:bg-muted/80">
                    <h3 className="truncate text-sm font-bold text-gray-700 dark:text-foreground">
                      {t("documentsHeading", { count: documents.length })}
                    </h3>
                    <button
                      type="button"
                      onClick={() => void refreshDocuments()}
                      disabled={loadingDocuments || busyAction !== null}
                      className="rounded-lg p-1.5 text-gray-400 hover:bg-white hover:text-gray-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60 disabled:opacity-50 dark:hover:bg-background dark:hover:text-foreground"
                      aria-label={t("serverRefreshDocuments")}
                    >
                      <RefreshCw
                        size={16}
                        className={loadingDocuments ? "animate-spin" : ""}
                        aria-hidden="true"
                      />
                    </button>
                  </div>
                  {loadingDocuments ? (
                    <EmptyState text={t("loadingKnowledgeBase")} />
                  ) : documents.length === 0 ? (
                    <EmptyState text={t("noDocuments")} />
                  ) : (
                    <div className="space-y-1 p-1">
                      {documents.map((document) => (
                        <ServerDocumentRow
                          key={document.id}
                          document={document}
                          busy={busyAction !== null}
                          canManage={canManageCollection}
                          onReprocess={() => handleReprocessDocument(document)}
                          onDelete={() => handleDeleteDocument(document)}
                        />
                      ))}
                    </div>
                  )}
                </section>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function CreateCollectionCard({ onClick }: { onClick: () => void }) {
  const t = useTranslations("Knowledge");
  return (
    <button
      type="button"
      aria-label={t("createNewCollectionAria")}
      onClick={onClick}
      className="group flex h-full min-h-45 flex-col items-center justify-center rounded-3xl border-2 border-dashed border-gray-300 p-6 duration-300 hover:border-blue-500 hover:bg-blue-50/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 dark:border-border dark:hover:border-blue-400 dark:hover:bg-blue-900/10"
    >
      <div className="mb-4 flex h-14 w-14 items-center justify-center rounded-full bg-gray-100 text-gray-400 shadow-sm duration-300 group-hover:bg-blue-500 group-hover:text-white dark:bg-muted dark:text-muted-foreground/70">
        <Plus size={28} aria-hidden="true" />
      </div>
      <h3 className="mb-1 font-bold text-gray-700 group-hover:text-blue-600 dark:text-foreground dark:group-hover:text-blue-400">
        {t("newCollection")}
      </h3>
      <p className="max-w-37.5 text-center text-xs text-gray-400">
        {t("createFolderHint")}
      </p>
    </button>
  );
}

function ServerCollectionCard({
  collection,
  fileCount,
  onClick,
  onEdit,
}: {
  collection: KnowledgeCollectionDTO;
  fileCount: number;
  onClick: () => void;
  onEdit: (event: React.MouseEvent) => void;
}) {
  const t = useTranslations("Knowledge");
  const Icon =
    COLLECTION_ICONS.find((item) => item.name === collection.icon)?.icon ??
    Folder;
  const color =
    COLLECTION_COLORS.find((item) => item.name === collection.color)
      ?.className ?? "bg-blue-500";
  return (
    <div className="group relative flex h-full min-h-45 flex-col overflow-hidden rounded-3xl border border-gray-200 bg-white p-5 shadow-sm duration-300 hover:border-gray-300 hover:shadow-xl dark:border-border dark:bg-muted/40 dark:shadow-none">
      <button
        type="button"
        aria-label={t("openCollectionAria", { name: collection.name })}
        onClick={onClick}
        className="absolute inset-0 rounded-3xl focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
      />
      <div className="pointer-events-none relative mb-3 flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3 overflow-hidden">
          <div
            className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-xl text-white shadow-md ${color}`}
          >
            <Icon size={20} aria-hidden="true" />
          </div>
          <h3 className="truncate text-base font-bold text-gray-800 group-hover:text-blue-600 dark:text-foreground dark:group-hover:text-blue-400">
            {collection.name}
          </h3>
        </div>
        <div className="h-7 w-7 shrink-0" />
      </div>
      <p className="pointer-events-none relative mb-4 line-clamp-3 flex-1 text-xs leading-relaxed text-gray-500 dark:text-muted-foreground">
        {collection.description || t("noDescriptionProvided")}
      </p>
      <div className="pointer-events-none relative mt-auto flex items-center gap-1.5 border-t border-gray-100 pt-3 text-xs font-medium text-gray-400 dark:border-border dark:text-muted-foreground/70">
        <FileText size={14} aria-hidden="true" />
        <span>{t("fileCount", { count: fileCount })}</span>
      </div>
      <button
        type="button"
        aria-label={t("editCollectionAria", { name: collection.name })}
        onClick={onEdit}
        className="absolute right-5 top-5 z-10 rounded-lg p-1.5 text-gray-400 opacity-0 hover:bg-gray-100 hover:text-gray-600 focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 group-hover:opacity-100 dark:hover:bg-accent dark:hover:text-foreground"
      >
        <Settings size={16} aria-hidden="true" />
      </button>
    </div>
  );
}

function ServerCollectionModal({
  title,
  initialData,
  busy,
  onSubmit,
  onDelete,
  onClose,
}: {
  title: string;
  initialData?: KnowledgeCollectionDTO;
  busy: boolean;
  onSubmit: (data: CollectionFormData) => Promise<void>;
  onDelete?: () => Promise<void>;
  onClose: () => void;
}) {
  const t = useTranslations("Knowledge");
  const id = useId();
  const [name, setName] = useState(initialData?.name ?? "");
  const [description, setDescription] = useState(
    initialData?.description ?? "",
  );
  const [icon, setIcon] = useState(initialData?.icon ?? "Folder");
  const [color, setColor] = useState(initialData?.color ?? "blue");
  const inputRef = useRef<HTMLInputElement | null>(null);
  useEffect(() => {
    inputRef.current?.focus({ preventScroll: true });
  }, []);
  return createPortal(
    <div
      className="fixed inset-0 z-9999 flex items-center justify-center bg-black/50 p-4"
      onClick={(event) => {
        if (event.target === event.currentTarget && !busy) onClose();
      }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={`${id}-title`}
        onKeyDown={(event) => {
          if (event.key === "Escape" && !busy) onClose();
        }}
        className="flex max-h-[90vh] w-full max-w-lg flex-col overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-xl dark:border-border dark:bg-card"
      >
        <div className="flex items-center justify-between border-b border-gray-100 px-5 py-4 dark:border-border">
          <h2
            id={`${id}-title`}
            className="text-lg font-bold text-gray-800 dark:text-foreground"
          >
            {title}
          </h2>
          <button
            type="button"
            aria-label={t("closeEditor")}
            onClick={onClose}
            disabled={busy}
            className="rounded-full p-1 text-gray-500 hover:bg-gray-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60 disabled:opacity-50 dark:hover:bg-muted"
          >
            <X size={20} aria-hidden="true" />
          </button>
        </div>
        <div className="custom-scrollbar min-h-0 flex-1 space-y-5 overflow-y-auto overscroll-contain p-5">
          <label className="grid gap-1.5 text-xs font-semibold text-gray-500 dark:text-muted-foreground">
            <span>{t("name")}</span>
            <input
              ref={inputRef}
              value={name}
              onChange={(event) => setName(event.target.value)}
              maxLength={KNOWLEDGE_LIMITS.maxCollectionNameChars}
              placeholder={t("namePlaceholder")}
              className="rounded-xl border border-gray-200 bg-gray-50 px-3 py-2 text-sm font-medium text-gray-800 focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-500/20 dark:border-border dark:bg-muted dark:text-foreground"
            />
          </label>
          <label className="grid gap-1.5 text-xs font-semibold text-gray-500 dark:text-muted-foreground">
            <span>{t("description")}</span>
            <textarea
              value={description}
              onChange={(event) => setDescription(event.target.value)}
              maxLength={KNOWLEDGE_LIMITS.maxCollectionDescriptionChars}
              placeholder={t("descriptionPlaceholder")}
              className="h-20 resize-none rounded-xl border border-gray-200 bg-gray-50 px-3 py-2 text-sm font-normal text-gray-700 focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-500/20 dark:border-border dark:bg-muted dark:text-foreground/85"
            />
          </label>
          <div className="space-y-2">
            <p className="text-xs font-semibold text-gray-500 dark:text-muted-foreground">
              {t("themeColor")}
            </p>
            <div className="flex flex-wrap gap-3">
              {COLLECTION_COLORS.map((item) => (
                <button
                  key={item.name}
                  type="button"
                  aria-label={t("useThemeColorAria", { color: item.name })}
                  aria-pressed={color === item.name}
                  onClick={() => setColor(item.name)}
                  className={`flex h-8 w-8 items-center justify-center rounded-full ${item.className} ${color === item.name ? "scale-110 shadow-md" : "opacity-40 hover:opacity-100"}`}
                >
                  {color === item.name && (
                    <Check
                      size={14}
                      className="text-white"
                      strokeWidth={3}
                      aria-hidden="true"
                    />
                  )}
                </button>
              ))}
            </div>
          </div>
          <div className="space-y-2">
            <p className="text-xs font-semibold text-gray-500 dark:text-muted-foreground">
              {t("icon")}
            </p>
            <div className="flex flex-wrap gap-2">
              {COLLECTION_ICONS.map((item) => (
                <button
                  key={item.name}
                  type="button"
                  aria-label={t("useIconAria", { icon: item.name })}
                  aria-pressed={icon === item.name}
                  onClick={() => setIcon(item.name)}
                  className={`flex h-10 w-10 items-center justify-center rounded-xl border p-2 ${icon === item.name ? "border-blue-200 bg-blue-50 text-blue-500 dark:border-blue-800 dark:bg-blue-900/20" : "border-transparent text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-muted"}`}
                >
                  <item.icon size={20} aria-hidden="true" />
                </button>
              ))}
            </div>
          </div>
        </div>
        <div className="flex justify-between gap-3 border-t border-gray-100 bg-gray-50/50 p-5 dark:border-border dark:bg-card/50">
          <div>
            {onDelete && (
              <button
                type="button"
                onClick={() => void onDelete()}
                disabled={busy}
                className="flex items-center gap-2 rounded-xl px-4 py-2 text-sm text-red-500 hover:bg-red-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-500/60 disabled:opacity-50 dark:hover:bg-red-900/20"
              >
                <Trash2 size={16} aria-hidden="true" />
                {t("delete")}
              </button>
            )}
          </div>
          <div className="flex gap-3">
            <button
              type="button"
              onClick={onClose}
              disabled={busy}
              className="rounded-xl px-4 py-2 text-sm text-gray-600 hover:bg-gray-100 disabled:opacity-50 dark:text-muted-foreground dark:hover:bg-muted"
            >
              {t("cancel")}
            </button>
            <button
              type="button"
              onClick={() => void onSubmit({ name, description, icon, color })}
              disabled={!name.trim() || busy}
              className="flex items-center gap-2 rounded-xl bg-blue-600 px-6 py-2 text-sm font-medium text-white shadow-lg shadow-blue-500/20 hover:bg-blue-700 disabled:opacity-50"
            >
              <Save size={16} aria-hidden="true" />
              {t("save")}
            </button>
          </div>
        </div>
      </div>
    </div>,
    document.body,
  );
}

function ServerDocumentRow({
  document,
  busy,
  canManage,
  onReprocess,
  onDelete,
}: {
  document: KnowledgeDocumentDTO;
  busy: boolean;
  canManage: boolean;
  onReprocess: () => void;
  onDelete: () => void;
}) {
  const t = useTranslations("Knowledge");
  const version = document.currentVersion ?? document.pendingVersion;
  return (
    <div className="group flex items-center gap-3 rounded-lg p-3 hover:bg-gray-50 dark:hover:bg-muted/50">
      <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-blue-50 text-blue-500 dark:bg-blue-900/20">
        <FileText size={20} aria-hidden="true" />
      </div>
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium text-gray-700 dark:text-foreground">
          {version?.file.name ?? document.id}
        </p>
        <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-gray-400 dark:text-muted-foreground">
          <span
            className={`rounded-full border px-2 py-0.5 ${statusClass(document.status)}`}
          >
            {t(`serverDocumentStatus.${document.status}`)}
          </span>
          {version && (
            <>
              <span>{version.file.mimeType}</span>
              <span>{formatBytes(version.file.byteSize)}</span>
            </>
          )}
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-1 opacity-100 md:opacity-0 md:group-hover:opacity-100 md:group-focus-within:opacity-100">
        {document.pendingVersion?.status === "failed" && (
          <button
            type="button"
            onClick={onReprocess}
            disabled={!canManage || busy}
            aria-label={t("serverReprocessDocument")}
            className="rounded-lg p-2 text-gray-400 hover:bg-white hover:text-blue-500 disabled:opacity-50 dark:hover:bg-background"
          >
            <RefreshCw size={16} aria-hidden="true" />
          </button>
        )}
        <button
          type="button"
          onClick={onDelete}
          disabled={!canManage || busy}
          aria-label={t("serverDeleteDocument")}
          className="rounded-lg p-2 text-gray-400 hover:bg-red-50 hover:text-red-500 disabled:opacity-50 dark:hover:bg-red-950/30"
        >
          <Trash2 size={16} aria-hidden="true" />
        </button>
      </div>
    </div>
  );
}

function Header({
  onClose,
  subtitle,
  activeName,
  onBack,
}: {
  onClose?: () => void;
  subtitle: string;
  activeName?: string;
  onBack?: () => void;
}) {
  const t = useTranslations("Knowledge");
  return (
    <div className="sticky top-0 z-10 flex shrink-0 items-center justify-between gap-3 border-b border-gray-200/50 bg-white/95 px-6 py-4 dark:border-border dark:bg-card/95">
      <div className="flex min-w-0 items-center gap-3">
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-linear-to-tr from-purple-600 to-indigo-600 text-white shadow-lg shadow-purple-500/20">
          <Library size={20} aria-hidden="true" />
        </div>
        <div className="min-w-0">
          <h1 className="flex min-w-0 items-center gap-2 text-lg font-bold text-gray-800 dark:text-foreground">
            {activeName && onBack ? (
              <>
                <button
                  type="button"
                  onClick={onBack}
                  className="shrink-0 rounded opacity-60 transition-opacity hover:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  {t("title")}
                </button>
                <ChevronRight size={16} className="shrink-0 text-gray-400" />
                <span className="truncate">{activeName}</span>
              </>
            ) : (
              <span className="truncate">{t("title")}</span>
            )}
          </h1>
          <p className="mt-0.5 truncate text-xs text-gray-500 dark:text-muted-foreground">
            {subtitle}
          </p>
        </div>
      </div>
      {onClose && (
        <button
          type="button"
          aria-label={t("closeKnowledgeBase")}
          onClick={onClose}
          className="shrink-0 rounded-full p-2 text-gray-500 transition-colors hover:bg-gray-200/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-purple-500/60 dark:text-muted-foreground dark:hover:bg-accent/50"
        >
          <X size={20} aria-hidden="true" />
        </button>
      )}
    </div>
  );
}

function EmptyState({ text }: { text: string }) {
  return (
    <div className="flex min-h-40 flex-col items-center justify-center gap-2 p-8 text-center text-sm text-muted-foreground">
      <FileText size={24} className="opacity-50" aria-hidden="true" />
      <p>{text}</p>
    </div>
  );
}
