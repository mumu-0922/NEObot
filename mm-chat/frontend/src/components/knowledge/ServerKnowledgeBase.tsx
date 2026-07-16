"use client";

import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useTranslations } from "next-intl";
import {
  AlertCircle,
  CheckCircle2,
  ChevronRight,
  FileText,
  Library,
  Plus,
  RefreshCw,
  Save,
  ShieldCheck,
  ShieldOff,
  Trash2,
  UploadCloud,
  X,
} from "lucide-react";
import { createNeoChatApiClient } from "@/services/api/client";
import type {
  KnowledgeCollectionDTO,
  KnowledgeCollectionScope,
  KnowledgeDocumentDTO,
  ProcessingConsentDTO,
} from "@/services/api/client";
import {
  formatBytes as formatLimitBytes,
  KNOWLEDGE_LIMITS,
} from "@/config/limits";
import { logDevError } from "@/lib/utils/devLogger";

interface ServerKnowledgeBaseProps {
  onClose?: () => void;
}

type ConsentFormState = {
  processor: string;
  endpointId: string;
  modelId: string;
  purposes: string;
  dataTypes: string;
  policyVersion: string;
  expiresAt: string;
};

type ConsentPayload = {
  processor: string;
  endpointId?: string;
  modelId?: string;
  purposes: string[];
  dataTypes: string[];
  policyVersion: string;
  expiresAt?: string;
};

type ConsentPreset = {
  key: string;
  labelKey: string;
  form: ConsentFormState;
};

const defaultCollectionConsentForm: ConsentFormState = {
  processor: "mineru",
  endpointId: "hosted-main",
  modelId: "model-stable-20260712",
  purposes: "parse",
  dataTypes: "application/pdf",
  policyVersion: "v1",
  expiresAt: "",
};

const defaultQueryConsentForm: ConsentFormState = {
  processor: "jina",
  endpointId: "hosted-main",
  modelId: "jina-embeddings-v4",
  purposes: "query_embedding, rerank",
  dataTypes: "text/plain",
  policyVersion: "v1",
  expiresAt: "",
};

const collectionConsentPresets: ConsentPreset[] = [
  {
    key: "mineruPdfParse",
    labelKey: "serverConsentPresetMineruPdfParse",
    form: defaultCollectionConsentForm,
  },
  {
    key: "jinaPassageEmbedding",
    labelKey: "serverConsentPresetJinaPassageEmbedding",
    form: {
      processor: "jina",
      endpointId: "hosted-main",
      modelId: "jina-embeddings-v4",
      purposes: "passage_embedding",
      dataTypes: "text/plain",
      policyVersion: "v1",
      expiresAt: "",
    },
  },
];

const queryConsentPresets: ConsentPreset[] = [
  {
    key: "jinaQueryEmbedding",
    labelKey: "serverConsentPresetJinaQueryEmbedding",
    form: defaultQueryConsentForm,
  },
  {
    key: "jinaAnswer",
    labelKey: "serverConsentPresetJinaAnswer",
    form: {
      processor: "jina",
      endpointId: "hosted-main",
      modelId: "jina-reranker-v3",
      purposes: "rerank, answer",
      dataTypes: "text/plain",
      policyVersion: "v1",
      expiresAt: "",
    },
  },
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

const consentStatusClass = (status: string) => {
  switch (status) {
    case "granted":
    case "active":
      return "border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900/60 dark:bg-emerald-950/40 dark:text-emerald-300";
    case "expired":
      return "border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-900/60 dark:bg-amber-950/40 dark:text-amber-300";
    case "revoked":
      return "border-red-200 bg-red-50 text-red-700 dark:border-red-900/60 dark:bg-red-950/40 dark:text-red-300";
    default:
      return "border-border bg-muted text-muted-foreground";
  }
};

const formatDate = (value: string): string => {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
};

const formatBytes = (value: number): string => formatLimitBytes(value);

const splitCsv = (value: string): string[] =>
  value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);

const hasCompleteIdentity = (form: ConsentFormState): boolean => {
  const hasEndpoint = Boolean(form.endpointId.trim());
  const hasModel = Boolean(form.modelId.trim());
  return hasEndpoint === hasModel;
};

const canSubmitConsent = (form: ConsentFormState): boolean =>
  Boolean(form.processor.trim()) &&
  Boolean(form.policyVersion.trim()) &&
  splitCsv(form.purposes).length > 0 &&
  splitCsv(form.dataTypes).length > 0 &&
  hasCompleteIdentity(form);

const toConsentPayload = (form: ConsentFormState): ConsentPayload => {
  const endpointId = form.endpointId.trim();
  const modelId = form.modelId.trim();
  return {
    processor: form.processor.trim(),
    endpointId: endpointId || undefined,
    modelId: modelId || undefined,
    purposes: splitCsv(form.purposes),
    dataTypes: splitCsv(form.dataTypes),
    policyVersion: form.policyVersion.trim(),
    expiresAt: form.expiresAt.trim() || undefined,
  };
};

const sameConsentIdentity = (
  left: ProcessingConsentDTO,
  right: ProcessingConsentDTO,
): boolean =>
  left.processor === right.processor &&
  left.endpointId === right.endpointId &&
  left.modelId === right.modelId;

const upsertConsent = (
  values: ProcessingConsentDTO[],
  next: ProcessingConsentDTO,
): ProcessingConsentDTO[] => {
  const existing = values.some((item) => sameConsentIdentity(item, next));
  if (!existing) return [next, ...values];
  return values.map((item) => (sameConsentIdentity(item, next) ? next : item));
};

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
  const [collectionConsents, setCollectionConsents] = useState<
    ProcessingConsentDTO[]
  >([]);
  const [queryConsents, setQueryConsents] = useState<ProcessingConsentDTO[]>(
    [],
  );
  const [selectedCollectionId, setSelectedCollectionId] = useState<
    string | null
  >(null);
  const [collectionName, setCollectionName] = useState("");
  const [collectionDescription, setCollectionDescription] = useState("");
  const [collectionScope, setCollectionScope] =
    useState<KnowledgeCollectionScope>("personal");
  const [collectionTeamId, setCollectionTeamId] = useState("");
  const [editName, setEditName] = useState("");
  const [editDescription, setEditDescription] = useState("");
  const [loadingCollections, setLoadingCollections] = useState(false);
  const [loadingDocuments, setLoadingDocuments] = useState(false);
  const [loadingCollectionConsents, setLoadingCollectionConsents] =
    useState(false);
  const [loadingQueryConsents, setLoadingQueryConsents] = useState(false);
  const [collectionConsentForm, setCollectionConsentForm] =
    useState<ConsentFormState>(defaultCollectionConsentForm);
  const [queryConsentForm, setQueryConsentForm] = useState<ConsentFormState>(
    defaultQueryConsentForm,
  );
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
  const canManageCollectionConsent =
    selectedCollection?.permissions.manageConsent === true;
  const trimmedCollectionName = collectionName.trim();
  const trimmedCollectionDescription = collectionDescription.trim();
  const trimmedTeamId = collectionTeamId.trim();
  const trimmedEditName = editName.trim();
  const trimmedEditDescription = editDescription.trim();
  const collectionConsentReady = canSubmitConsent(collectionConsentForm);
  const queryConsentReady = canSubmitConsent(queryConsentForm);

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
      setSelectedCollectionId((current) => {
        if (current && page.items.some((item) => item.id === current)) {
          return current;
        }
        return page.items[0]?.id ?? null;
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
    } catch (caught) {
      showError(t("serverLoadDocumentsFailed"), caught);
    } finally {
      setLoadingDocuments(false);
    }
  }, [apiClient, knowledgeSupported, selectedCollectionId, showError, t]);

  const refreshCollectionConsents = useCallback(async () => {
    if (!knowledgeSupported || !selectedCollectionId) {
      setCollectionConsents([]);
      return;
    }
    setLoadingCollectionConsents(true);
    setError(null);
    try {
      const values = await apiClient.knowledge.listCollectionConsents({
        collectionId: selectedCollectionId,
      });
      setCollectionConsents(values);
    } catch (caught) {
      showError(t("serverLoadCollectionConsentsFailed"), caught);
    } finally {
      setLoadingCollectionConsents(false);
    }
  }, [apiClient, knowledgeSupported, selectedCollectionId, showError, t]);

  const refreshQueryConsents = useCallback(async () => {
    if (!knowledgeSupported) {
      setQueryConsents([]);
      return;
    }
    setLoadingQueryConsents(true);
    setError(null);
    try {
      const values = await apiClient.knowledge.listQueryConsents();
      setQueryConsents(values);
    } catch (caught) {
      showError(t("serverLoadQueryConsentsFailed"), caught);
    } finally {
      setLoadingQueryConsents(false);
    }
  }, [apiClient, knowledgeSupported, showError, t]);

  useEffect(() => {
    void refreshCollections();
  }, [refreshCollections]);

  useEffect(() => {
    setEditName(selectedCollection?.name ?? "");
    setEditDescription(selectedCollection?.description ?? "");
  }, [selectedCollection?.description, selectedCollection?.name]);

  useEffect(() => {
    void refreshDocuments();
  }, [refreshDocuments]);

  useEffect(() => {
    void refreshCollectionConsents();
  }, [refreshCollectionConsents]);

  useEffect(() => {
    void refreshQueryConsents();
  }, [refreshQueryConsents]);

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

  const handleCreateCollection = () => {
    if (!trimmedCollectionName) return;
    if (collectionScope === "team" && !trimmedTeamId) {
      setError(t("serverTeamIdRequired"));
      return;
    }
    void runAction("create-collection", async () => {
      try {
        const collection = await apiClient.knowledge.createCollection({
          name: trimmedCollectionName,
          description: trimmedCollectionDescription,
          scope: collectionScope,
          teamId: collectionScope === "team" ? trimmedTeamId : undefined,
          idempotencyKey: newIdempotencyKey("knowledge-collection"),
        });
        setCollectionName("");
        setCollectionDescription("");
        setCollectionTeamId("");
        setCollectionScope("personal");
        setCollections((current) => [collection, ...current]);
        setSelectedCollectionId(collection.id);
        setNotice(t("serverCollectionCreated"));
      } catch (caught) {
        showError(t("serverCreateCollectionFailed"), caught);
      }
    });
  };

  const handleUpdateCollection = () => {
    if (!selectedCollection || !trimmedEditName) return;
    void runAction(`update-${selectedCollection.id}`, async () => {
      try {
        const updated = await apiClient.knowledge.updateCollection({
          collectionId: selectedCollection.id,
          name: trimmedEditName,
          description: trimmedEditDescription,
        });
        setCollections((current) =>
          current.map((item) => (item.id === updated.id ? updated : item)),
        );
        setNotice(t("serverCollectionUpdated"));
      } catch (caught) {
        showError(t("serverUpdateCollectionFailed"), caught);
      }
    });
  };

  const handleDeleteCollection = () => {
    if (!selectedCollection) return;
    if (
      !window.confirm(
        t("serverConfirmDeleteCollection", { name: selectedCollection.name }),
      )
    ) {
      return;
    }
    const collection = selectedCollection;
    void runAction(`delete-${collection.id}`, async () => {
      try {
        await apiClient.knowledge.deleteCollection({
          collectionId: collection.id,
        });
        setCollections((current) =>
          current.filter((item) => item.id !== collection.id),
        );
        setSelectedCollectionId(null);
        setDocuments([]);
        setCollectionConsents([]);
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
          await apiClient.knowledge.bindDocument({
            collectionId,
            fileId: fileRecord.id,
            idempotencyKey: newIdempotencyKey("knowledge-document"),
          });
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
    void runAction(`delete-document-${document.id}`, async () => {
      try {
        await apiClient.knowledge.deleteDocument({ documentId: document.id });
        setNotice(t("serverDocumentDeleted"));
      } catch (caught) {
        setDocuments(previousDocuments);
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

  const handleGrantCollectionConsent = () => {
    if (
      !selectedCollection ||
      !canManageCollectionConsent ||
      !collectionConsentReady
    ) {
      return;
    }
    const payload = toConsentPayload(collectionConsentForm);
    void runAction("grant-collection-consent", async () => {
      try {
        const consent = await apiClient.knowledge.putCollectionConsent({
          collectionId: selectedCollection.id,
          ...payload,
        });
        setCollectionConsents((current) => upsertConsent(current, consent));
        setNotice(t("serverCollectionConsentGranted"));
      } catch (caught) {
        showError(t("serverGrantCollectionConsentFailed"), caught);
      }
    });
  };

  const handleRevokeCollectionConsent = (consent: ProcessingConsentDTO) => {
    if (!selectedCollection || !canManageCollectionConsent) return;
    if (!window.confirm(t("serverConfirmRevokeConsent"))) return;
    void runAction(
      `revoke-collection-consent-${consent.processor}-${consent.modelId}`,
      async () => {
        try {
          await apiClient.knowledge.revokeCollectionConsent({
            collectionId: selectedCollection.id,
            processor: consent.processor,
            endpointId: consent.endpointId,
            modelId: consent.modelId,
          });
          setCollectionConsents((current) =>
            current.map((item) =>
              sameConsentIdentity(item, consent)
                ? {
                    ...item,
                    decision: "revoked",
                    effectiveStatus: "revoked",
                    expiresAt: undefined,
                    decidedAt: new Date().toISOString(),
                  }
                : item,
            ),
          );
          setNotice(t("serverCollectionConsentRevoked"));
          await refreshCollectionConsents();
        } catch (caught) {
          showError(t("serverRevokeCollectionConsentFailed"), caught);
        }
      },
    );
  };

  const handleGrantQueryConsent = () => {
    if (!queryConsentReady) return;
    const payload = toConsentPayload(queryConsentForm);
    void runAction("grant-query-consent", async () => {
      try {
        const consent = await apiClient.knowledge.putQueryConsent(payload);
        setQueryConsents((current) => upsertConsent(current, consent));
        setNotice(t("serverQueryConsentGranted"));
      } catch (caught) {
        showError(t("serverGrantQueryConsentFailed"), caught);
      }
    });
  };

  const handleRevokeQueryConsent = (consent: ProcessingConsentDTO) => {
    if (!window.confirm(t("serverConfirmRevokeConsent"))) return;
    void runAction(
      `revoke-query-consent-${consent.processor}-${consent.modelId}`,
      async () => {
        try {
          await apiClient.knowledge.revokeQueryConsent({
            processor: consent.processor,
            endpointId: consent.endpointId,
            modelId: consent.modelId,
          });
          setQueryConsents((current) =>
            current.map((item) =>
              sameConsentIdentity(item, consent)
                ? {
                    ...item,
                    decision: "revoked",
                    effectiveStatus: "revoked",
                    expiresAt: undefined,
                    decidedAt: new Date().toISOString(),
                  }
                : item,
            ),
          );
          setNotice(t("serverQueryConsentRevoked"));
          await refreshQueryConsents();
        } catch (caught) {
          showError(t("serverRevokeQueryConsentFailed"), caught);
        }
      },
    );
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
    <div className="flex h-full w-full flex-col overflow-hidden bg-gray-50/50 dark:bg-background">
      <Header
        onClose={onClose}
        subtitle={
          selectedCollection
            ? selectedCollection.description || t("serverManageDocsSubtitle")
            : t("serverManageCollectionsSubtitle")
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

      <div className="grid min-h-0 flex-1 gap-5 overflow-hidden p-6 lg:grid-cols-[minmax(260px,340px)_1fr]">
        <aside className="flex min-h-0 flex-col rounded-2xl border border-border bg-card p-4 shadow-sm">
          <div className="mb-4 space-y-3">
            <div className="flex items-center justify-between gap-3">
              <h2 className="text-sm font-semibold text-foreground">
                {t("serverCollections")}
              </h2>
              <button
                type="button"
                onClick={() => void refreshCollections()}
                disabled={loadingCollections || busyAction !== null}
                className="rounded-lg p-2 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60"
                aria-label={t("serverRefreshCollections")}
              >
                <RefreshCw
                  size={16}
                  className={loadingCollections ? "animate-spin" : ""}
                  aria-hidden="true"
                />
              </button>
            </div>

            <div className="space-y-2 rounded-xl border border-border bg-muted/30 p-3">
              <input
                value={collectionName}
                onChange={(event) => setCollectionName(event.target.value)}
                placeholder={t("serverCollectionNamePlaceholder")}
                className="w-full rounded-lg border border-input bg-background px-3 py-2 text-sm text-foreground outline-none transition focus-visible:ring-2 focus-visible:ring-ring"
              />
              <textarea
                value={collectionDescription}
                onChange={(event) =>
                  setCollectionDescription(event.target.value)
                }
                placeholder={t("serverCollectionDescriptionPlaceholder")}
                className="h-20 w-full resize-none rounded-lg border border-input bg-background px-3 py-2 text-sm text-foreground outline-none transition focus-visible:ring-2 focus-visible:ring-ring"
              />
              <div className="grid gap-2 sm:grid-cols-2">
                <select
                  value={collectionScope}
                  onChange={(event) =>
                    setCollectionScope(
                      event.target.value as KnowledgeCollectionScope,
                    )
                  }
                  className="rounded-lg border border-input bg-background px-3 py-2 text-sm text-foreground outline-none transition focus-visible:ring-2 focus-visible:ring-ring"
                >
                  <option value="personal">{t("serverScopePersonal")}</option>
                  <option value="team">{t("serverScopeTeam")}</option>
                </select>
                <input
                  value={collectionTeamId}
                  onChange={(event) => setCollectionTeamId(event.target.value)}
                  disabled={collectionScope !== "team"}
                  placeholder={t("serverTeamIdPlaceholder")}
                  className="rounded-lg border border-input bg-background px-3 py-2 text-sm text-foreground outline-none transition focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
                />
              </div>
              <button
                type="button"
                onClick={handleCreateCollection}
                disabled={!trimmedCollectionName || busyAction !== null}
                className="inline-flex w-full items-center justify-center gap-2 rounded-lg bg-brand px-3 py-2 text-sm font-semibold text-white transition hover:bg-brand/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60"
              >
                <Plus size={16} aria-hidden="true" />
                {t("serverCreateCollection")}
              </button>
            </div>
          </div>

          <div className="min-h-0 flex-1 overflow-y-auto custom-scrollbar">
            {loadingCollections ? (
              <EmptyState text={t("serverLoadingCollections")} />
            ) : collections.length === 0 ? (
              <EmptyState text={t("serverNoCollections")} />
            ) : (
              <div className="space-y-2">
                {collections.map((collection) => {
                  const selected = collection.id === selectedCollectionId;
                  return (
                    <button
                      key={collection.id}
                      type="button"
                      onClick={() => setSelectedCollectionId(collection.id)}
                      className={`w-full rounded-xl border p-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${
                        selected
                          ? "border-brand bg-brand/10"
                          : "border-border bg-background hover:bg-muted"
                      }`}
                    >
                      <span className="block truncate text-sm font-semibold text-foreground">
                        {collection.name}
                      </span>
                      <span className="mt-2 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                        <span className="rounded-full border border-border bg-muted px-2 py-0.5">
                          {t(`serverScope.${collection.scope}`)}
                        </span>
                        {collection.permissions.manage && (
                          <span className="rounded-full border border-emerald-200 bg-emerald-50 px-2 py-0.5 text-emerald-700 dark:border-emerald-900/60 dark:bg-emerald-950/40 dark:text-emerald-300">
                            {t("serverCanManage")}
                          </span>
                        )}
                      </span>
                    </button>
                  );
                })}
              </div>
            )}
          </div>
        </aside>

        <main className="min-h-0 overflow-y-auto rounded-2xl border border-border bg-card p-5 shadow-sm custom-scrollbar">
          <div className="space-y-6">
            {!selectedCollection ? (
              <EmptyState text={t("serverSelectCollectionEmpty")} />
            ) : (
              <>
                <section className="space-y-3 border-b border-border pb-5">
                  <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
                    <div className="min-w-0">
                      <h2 className="truncate text-xl font-semibold text-foreground">
                        {selectedCollection.name}
                      </h2>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {t("serverCollectionUpdatedAt", {
                          value: formatDate(selectedCollection.updatedAt),
                        })}
                      </p>
                    </div>
                    <button
                      type="button"
                      onClick={handleDeleteCollection}
                      disabled={!canManageCollection || busyAction !== null}
                      className="inline-flex items-center justify-center gap-2 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm font-medium text-red-700 transition-colors hover:bg-red-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300 dark:hover:bg-red-950/50"
                    >
                      <Trash2 size={16} aria-hidden="true" />
                      {t("serverDeleteCollection")}
                    </button>
                  </div>

                  <div className="grid gap-3 md:grid-cols-[1fr_auto]">
                    <div className="grid gap-2">
                      <input
                        value={editName}
                        onChange={(event) => setEditName(event.target.value)}
                        disabled={!canManageCollection}
                        className="rounded-lg border border-input bg-background px-3 py-2 text-sm text-foreground outline-none transition focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60"
                      />
                      <textarea
                        value={editDescription}
                        onChange={(event) =>
                          setEditDescription(event.target.value)
                        }
                        disabled={!canManageCollection}
                        className="h-20 resize-none rounded-lg border border-input bg-background px-3 py-2 text-sm text-foreground outline-none transition focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60"
                      />
                    </div>
                    <button
                      type="button"
                      onClick={handleUpdateCollection}
                      disabled={
                        !canManageCollection ||
                        !trimmedEditName ||
                        busyAction !== null ||
                        (trimmedEditName === selectedCollection.name &&
                          trimmedEditDescription ===
                            selectedCollection.description)
                      }
                      className="inline-flex items-center justify-center gap-2 self-start rounded-lg border border-border bg-background px-4 py-2 text-sm font-medium text-foreground transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60"
                    >
                      <Save size={16} aria-hidden="true" />
                      {t("serverSaveCollection")}
                    </button>
                  </div>
                </section>

                <ConsentSection
                  kind="collection"
                  title={t("serverCollectionConsentHeading")}
                  description={t("serverCollectionConsentDescription")}
                  envBackedNote={t("serverConsentEnvBackedNote")}
                  failClosedNote={t("serverConsentFailClosedNote")}
                  noConsentText={t("serverNoCollectionConsents")}
                  loadingText={t("serverLoadingConsents")}
                  consents={collectionConsents}
                  form={collectionConsentForm}
                  presets={collectionConsentPresets}
                  setForm={setCollectionConsentForm}
                  canGrant={canManageCollectionConsent}
                  isLoading={loadingCollectionConsents}
                  busyAction={busyAction}
                  isReady={collectionConsentReady}
                  onGrant={handleGrantCollectionConsent}
                  onRevoke={handleRevokeCollectionConsent}
                  onRefresh={refreshCollectionConsents}
                />

                <section
                  onDragOver={(event) => {
                    event.preventDefault();
                    setIsDragging(true);
                  }}
                  onDragLeave={() => setIsDragging(false)}
                  onDrop={handleDrop}
                  className={`rounded-2xl border-2 border-dashed p-8 text-center transition-colors ${
                    isDragging
                      ? "border-purple-500 bg-purple-50 dark:bg-purple-950/20"
                      : "border-border bg-muted/20 hover:border-purple-400"
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
                    className="mx-auto flex max-w-sm flex-col items-center rounded-xl px-4 py-3 text-center transition-colors hover:bg-background/70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60"
                  >
                    <span className="mb-3 flex h-14 w-14 items-center justify-center rounded-2xl border border-border bg-background text-purple-500 shadow-sm">
                      <UploadCloud size={28} aria-hidden="true" />
                    </span>
                    <span className="text-sm font-semibold text-foreground">
                      {t("serverChooseFiles")}
                    </span>
                    <span className="mt-1 text-xs text-muted-foreground">
                      {t("serverChooseFilesHint")}
                    </span>
                  </button>
                  <p className="mt-3 text-xs text-muted-foreground">
                    {t("uploadLimits", {
                      max: KNOWLEDGE_LIMITS.maxFilesPerCollection,
                      size: formatBytes(KNOWLEDGE_LIMITS.maxFileBytes),
                    })}
                  </p>
                </section>

                <section className="overflow-hidden rounded-2xl border border-border">
                  <div className="flex items-center justify-between gap-3 border-b border-border bg-muted/60 px-4 py-3">
                    <h3 className="text-sm font-semibold text-foreground">
                      {t("serverDocumentsHeading", { count: documents.length })}
                    </h3>
                    <button
                      type="button"
                      onClick={() => void refreshDocuments()}
                      disabled={loadingDocuments || busyAction !== null}
                      className="rounded-lg p-2 text-muted-foreground transition-colors hover:bg-background hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60"
                      aria-label={t("serverRefreshDocuments")}
                    >
                      <RefreshCw
                        size={16}
                        className={loadingDocuments ? "animate-spin" : ""}
                        aria-hidden="true"
                      />
                    </button>
                  </div>
                  {documents.length === 0 ? (
                    <EmptyState text={t("serverNoDocuments")} />
                  ) : (
                    <div className="divide-y divide-border">
                      {documents.map((document) => {
                        const version =
                          document.currentVersion ?? document.pendingVersion;
                        return (
                          <div
                            key={document.id}
                            className="grid gap-3 p-4 md:grid-cols-[1fr_auto] md:items-center"
                          >
                            <div className="min-w-0">
                              <div className="flex min-w-0 items-center gap-2">
                                <FileText
                                  size={18}
                                  className="shrink-0 text-blue-500"
                                  aria-hidden="true"
                                />
                                <p className="truncate text-sm font-semibold text-foreground">
                                  {version?.file.name ?? document.id}
                                </p>
                              </div>
                              <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                                <span
                                  className={`rounded-full border px-2 py-0.5 ${statusClass(document.status)}`}
                                >
                                  {t(`serverDocumentStatus.${document.status}`)}
                                </span>
                                {version && (
                                  <>
                                    <span>{version.file.mimeType}</span>
                                    <span>
                                      {formatBytes(version.file.byteSize)}
                                    </span>
                                    <span>
                                      {t("serverSourceVersion", {
                                        version: version.sourceVersion,
                                      })}
                                    </span>
                                  </>
                                )}
                              </div>
                            </div>
                            <div className="flex flex-wrap items-center gap-2">
                              <button
                                type="button"
                                onClick={() =>
                                  handleReprocessDocument(document)
                                }
                                disabled={
                                  !canManageCollection || busyAction !== null
                                }
                                className="inline-flex items-center gap-1.5 rounded-lg border border-border bg-background px-3 py-1.5 text-xs font-medium text-foreground transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
                              >
                                <RefreshCw size={14} aria-hidden="true" />
                                {t("serverReprocessDocument")}
                              </button>
                              <button
                                type="button"
                                onClick={() => handleDeleteDocument(document)}
                                disabled={
                                  !canManageCollection || busyAction !== null
                                }
                                className="inline-flex items-center gap-1.5 rounded-lg border border-border bg-background px-3 py-1.5 text-xs font-medium text-red-600 transition-colors hover:bg-red-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50 dark:text-red-300 dark:hover:bg-red-950/30"
                              >
                                <Trash2 size={14} aria-hidden="true" />
                                {t("serverDeleteDocument")}
                              </button>
                            </div>
                          </div>
                        );
                      })}
                    </div>
                  )}
                </section>
              </>
            )}

            <ConsentSection
              kind="query"
              title={t("serverQueryConsentHeading")}
              description={t("serverQueryConsentDescription")}
              envBackedNote={t("serverConsentEnvBackedNote")}
              failClosedNote={t("serverConsentFailClosedNote")}
              noConsentText={t("serverNoQueryConsents")}
              loadingText={t("serverLoadingConsents")}
              consents={queryConsents}
              form={queryConsentForm}
              presets={queryConsentPresets}
              setForm={setQueryConsentForm}
              canGrant
              isLoading={loadingQueryConsents}
              busyAction={busyAction}
              isReady={queryConsentReady}
              onGrant={handleGrantQueryConsent}
              onRevoke={handleRevokeQueryConsent}
              onRefresh={refreshQueryConsents}
            />
          </div>
        </main>
      </div>
    </div>
  );
}

function ConsentSection({
  kind,
  title,
  description,
  envBackedNote,
  failClosedNote,
  noConsentText,
  loadingText,
  consents,
  form,
  presets,
  setForm,
  canGrant,
  isLoading,
  busyAction,
  isReady,
  onGrant,
  onRevoke,
  onRefresh,
}: {
  kind: "collection" | "query";
  title: string;
  description: string;
  envBackedNote: string;
  failClosedNote: string;
  noConsentText: string;
  loadingText: string;
  consents: ProcessingConsentDTO[];
  form: ConsentFormState;
  presets: ConsentPreset[];
  setForm: React.Dispatch<React.SetStateAction<ConsentFormState>>;
  canGrant: boolean;
  isLoading: boolean;
  busyAction: string | null;
  isReady: boolean;
  onGrant: () => void;
  onRevoke: (consent: ProcessingConsentDTO) => void;
  onRefresh: () => Promise<void>;
}) {
  const t = useTranslations("Knowledge");
  const disabled = busyAction !== null;
  const cannotManage = !canGrant && kind === "collection";
  return (
    <section className="overflow-hidden rounded-2xl border border-border">
      <div className="border-b border-border bg-muted/50 px-4 py-3">
        <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
          <div className="min-w-0">
            <h3 className="flex items-center gap-2 text-sm font-semibold text-foreground">
              <ShieldCheck
                size={16}
                className="text-emerald-500"
                aria-hidden="true"
              />
              {title}
            </h3>
            <p className="mt-1 text-xs text-muted-foreground">{description}</p>
          </div>
          <button
            type="button"
            onClick={() => void onRefresh()}
            disabled={isLoading || disabled}
            className="inline-flex items-center gap-1.5 self-start rounded-lg border border-border bg-background px-3 py-1.5 text-xs font-medium text-foreground transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
          >
            <RefreshCw
              size={14}
              className={isLoading ? "animate-spin" : ""}
              aria-hidden="true"
            />
            {t("serverRefreshConsents")}
          </button>
        </div>
        <div className="mt-3 grid gap-2 text-xs md:grid-cols-2">
          <p className="rounded-xl border border-emerald-200 bg-emerald-50 px-3 py-2 text-emerald-700 dark:border-emerald-900/60 dark:bg-emerald-950/30 dark:text-emerald-300">
            {envBackedNote}
          </p>
          <p className="rounded-xl border border-amber-200 bg-amber-50 px-3 py-2 text-amber-700 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-300">
            {failClosedNote}
          </p>
        </div>
      </div>

      <div className="space-y-4 p-4">
        {cannotManage && (
          <div className="flex items-start gap-2 rounded-xl border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-300">
            <ShieldOff size={14} className="mt-0.5 shrink-0" />
            <span>{t("serverCollectionConsentReadOnly")}</span>
          </div>
        )}

        <div className="flex flex-wrap gap-2">
          {presets.map((preset) => (
            <button
              key={preset.key}
              type="button"
              onClick={() => setForm(preset.form)}
              disabled={disabled || !canGrant}
              className="rounded-full border border-border bg-background px-3 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
            >
              {t(preset.labelKey)}
            </button>
          ))}
        </div>

        <div className="grid gap-3 md:grid-cols-3">
          <ConsentInput
            label={t("serverConsentProcessor")}
            value={form.processor}
            onChange={(processor) =>
              setForm((current) => ({ ...current, processor }))
            }
            disabled={disabled || !canGrant}
          />
          <ConsentInput
            label={t("serverConsentEndpointId")}
            value={form.endpointId}
            onChange={(endpointId) =>
              setForm((current) => ({ ...current, endpointId }))
            }
            disabled={disabled || !canGrant}
          />
          <ConsentInput
            label={t("serverConsentModelId")}
            value={form.modelId}
            onChange={(modelId) =>
              setForm((current) => ({ ...current, modelId }))
            }
            disabled={disabled || !canGrant}
          />
          <ConsentInput
            label={t("serverConsentPurposes")}
            value={form.purposes}
            onChange={(purposes) =>
              setForm((current) => ({ ...current, purposes }))
            }
            disabled={disabled || !canGrant}
          />
          <ConsentInput
            label={t("serverConsentDataTypes")}
            value={form.dataTypes}
            onChange={(dataTypes) =>
              setForm((current) => ({ ...current, dataTypes }))
            }
            disabled={disabled || !canGrant}
          />
          <ConsentInput
            label={t("serverConsentPolicyVersion")}
            value={form.policyVersion}
            onChange={(policyVersion) =>
              setForm((current) => ({ ...current, policyVersion }))
            }
            disabled={disabled || !canGrant}
          />
          <ConsentInput
            label={t("serverConsentExpiresAt")}
            value={form.expiresAt}
            onChange={(expiresAt) =>
              setForm((current) => ({ ...current, expiresAt }))
            }
            disabled={disabled || !canGrant}
            className="md:col-span-2"
          />
          <button
            type="button"
            onClick={onGrant}
            disabled={!canGrant || !isReady || disabled}
            className="inline-flex items-center justify-center gap-2 self-end rounded-lg bg-emerald-600 px-4 py-2 text-sm font-semibold text-white transition hover:bg-emerald-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60"
          >
            <ShieldCheck size={16} aria-hidden="true" />
            {t("serverGrantConsent")}
          </button>
        </div>

        {!hasCompleteIdentity(form) && (
          <p className="text-xs text-red-600 dark:text-red-300">
            {t("serverConsentIdentityRequired")}
          </p>
        )}

        <div className="overflow-hidden rounded-xl border border-border">
          {isLoading ? (
            <EmptyState text={loadingText} />
          ) : consents.length === 0 ? (
            <EmptyState text={noConsentText} />
          ) : (
            <div className="divide-y divide-border">
              {consents.map((consent) => (
                <ConsentRow
                  key={`${consent.processor}:${consent.endpointId}:${consent.modelId}`}
                  consent={consent}
                  disabled={disabled || (kind === "collection" && !canGrant)}
                  onRevoke={() => onRevoke(consent)}
                />
              ))}
            </div>
          )}
        </div>
      </div>
    </section>
  );
}

function ConsentInput({
  label,
  value,
  onChange,
  disabled,
  className = "",
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  disabled: boolean;
  className?: string;
}) {
  return (
    <label className={`grid gap-1 text-xs font-medium ${className}`}>
      <span className="text-muted-foreground">{label}</span>
      <input
        value={value}
        onChange={(event) => onChange(event.target.value)}
        disabled={disabled}
        className="rounded-lg border border-input bg-background px-3 py-2 text-sm font-normal text-foreground outline-none transition focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60"
      />
    </label>
  );
}

function ConsentRow({
  consent,
  disabled,
  onRevoke,
}: {
  consent: ProcessingConsentDTO;
  disabled: boolean;
  onRevoke: () => void;
}) {
  const t = useTranslations("Knowledge");
  const status = consent.effectiveStatus || consent.decision;
  return (
    <div className="grid gap-3 p-4 md:grid-cols-[1fr_auto] md:items-center">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-sm font-semibold text-foreground">
            {consent.processor}
          </span>
          <span
            className={`rounded-full border px-2 py-0.5 text-xs ${consentStatusClass(status)}`}
          >
            {t(`serverConsentStatus.${status}`)}
          </span>
        </div>
        <div className="mt-2 flex flex-wrap gap-2 text-xs text-muted-foreground">
          <span>{consent.endpointId || "default"}</span>
          <span>{consent.modelId || "default"}</span>
          <span>{consent.purposes.join(", ")}</span>
          <span>{consent.dataTypes.join(", ")}</span>
          <span>{consent.policyVersion}</span>
          {consent.expiresAt && (
            <span>
              {t("serverConsentExpiresAtValue", {
                value: formatDate(consent.expiresAt),
              })}
            </span>
          )}
        </div>
        <p className="mt-1 truncate text-xs text-muted-foreground">
          {t("serverConsentDecidedAt", {
            value: formatDate(consent.decidedAt),
          })}
        </p>
      </div>
      <button
        type="button"
        onClick={onRevoke}
        disabled={disabled || status === "revoked"}
        className="inline-flex items-center justify-center gap-1.5 rounded-lg border border-border bg-background px-3 py-1.5 text-xs font-medium text-red-600 transition-colors hover:bg-red-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50 dark:text-red-300 dark:hover:bg-red-950/30"
      >
        <ShieldOff size={14} aria-hidden="true" />
        {t("serverRevokeConsent")}
      </button>
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
    <div className="sticky top-0 z-10 flex shrink-0 items-center justify-between gap-3 border-b border-gray-200/50 bg-white/40 px-6 py-4 backdrop-blur-md dark:border-border dark:bg-card/40">
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
