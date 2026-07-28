"use client";

import {
  Archive,
  Brain,
  ChevronDown,
  ChevronUp,
  CircleAlert,
  Database,
  Download,
  FileCheck2,
  FolderKanban,
  History,
  Loader2,
  Pencil,
  Plus,
  RefreshCw,
  Save,
  Search,
  ShieldCheck,
  Trash2,
  Upload,
  X,
} from "lucide-react";
import { useTranslations } from "next-intl";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { MEMORY_LIMITS } from "@/config/limits";
import type {
  ConversationMemoryPolicy,
  GovernanceMemory,
  GovernanceMemoryDetail,
  MemoryGovernanceSnapshot,
  MemoryPolicyMode,
  MemoryProject,
  MemoryReviewSuggestion,
  MemoryScopeType,
  MemorySensitivity,
  MemoryType,
} from "@/lib/memory/types";
import type {
  MemoryImportConversationMapping,
  MemoryImportDryRunResult,
  MemoryImportMappings,
  MemoryImportProjectMapping,
  MemoryImportScopeRequirement,
  MemoryReviewDecision,
  NeoChatApiClient,
} from "@/services/api/client";
import { SimpleSwitch } from "./SettingsUI";

interface ServerMemoryGovernanceProps {
  apiClient: NeoChatApiClient;
}

type Section = "memories" | "projects" | "reviews" | "operations";

interface MemoryDraft {
  id?: string;
  expectedRevision?: number;
  type: MemoryType;
  content: string;
  importance: number;
  tags: string;
  scopeType: MemoryScopeType;
  projectId: string;
  conversationId: string;
  sensitivity: MemorySensitivity;
}

const EMPTY_MEMORY_DRAFT: MemoryDraft = {
  type: "fact",
  content: "",
  importance: 3,
  tags: "",
  scopeType: "global",
  projectId: "",
  conversationId: "",
  sensitivity: "normal",
};

const MEMORY_TYPES: MemoryType[] = [
  "fact",
  "preference",
  "instruction",
  "project",
  "warning",
  "decision",
  "context",
];

const POLICY_MODES: MemoryPolicyMode[] = ["inherit", "on", "off"];
const REVIEW_DECISIONS: MemoryReviewDecision[] = [
  "keep_current",
  "accept_new",
  "edit_merge",
  "keep_both",
  "reject",
];

function formatDate(value: number | undefined): string {
  if (!value) return "—";
  return new Date(value).toLocaleString();
}

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback;
}

function parseTags(value: string): string[] {
  return value
    .split(",")
    .map((tag) => tag.trim())
    .filter(Boolean)
    .slice(0, MEMORY_LIMITS.maxTags);
}

const ServerMemoryGovernance = ({ apiClient }: ServerMemoryGovernanceProps) => {
  const t = useTranslations("Memory");
  const [snapshot, setSnapshot] = useState<MemoryGovernanceSnapshot | null>(
    null,
  );
  const [section, setSection] = useState<Section>("memories");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const [memoryDraft, setMemoryDraft] =
    useState<MemoryDraft>(EMPTY_MEMORY_DRAFT);
  const [projectDraft, setProjectDraft] = useState({
    id: "",
    expectedRevision: 0,
    name: "",
    description: "",
  });
  const [detail, setDetail] = useState<GovernanceMemoryDetail | null>(null);
  const [detailLoadingId, setDetailLoadingId] = useState<string | null>(null);
  const [pendingDeleteId, setPendingDeleteId] = useState<string | null>(null);
  const [reviewDrafts, setReviewDrafts] = useState<Record<string, string>>({});
  const loadVersionRef = useRef(0);
  const detailLoadVersionRef = useRef(0);
  const mutationInFlightRef = useRef(false);

  const load = useCallback(
    async (signal?: AbortSignal) => {
      const loadVersion = ++loadVersionRef.current;
      setError(null);
      try {
        const next = await apiClient.memories.getGovernance({ signal });
        if (signal?.aborted || loadVersion !== loadVersionRef.current) return;
        setSnapshot(next);
      } catch (nextError) {
        if (signal?.aborted || loadVersion !== loadVersionRef.current) return;
        setError(errorMessage(nextError, t("requestFailed")));
      }
    },
    [apiClient, t],
  );

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    void load(controller.signal).finally(() => {
      if (!controller.signal.aborted) setLoading(false);
    });
    return () => controller.abort();
  }, [load]);

  const mutate = useCallback(
    async (operation: () => Promise<unknown>) => {
      if (mutationInFlightRef.current) return false;
      mutationInFlightRef.current = true;
      setSaving(true);
      setError(null);
      try {
        await operation();
        await load();
        return true;
      } catch (nextError) {
        setError(errorMessage(nextError, t("staleOrFailed")));
        return false;
      } finally {
        mutationInFlightRef.current = false;
        setSaving(false);
      }
    },
    [load, t],
  );

  const filteredMemories = useMemo(() => {
    const term = query.trim().toLocaleLowerCase();
    if (!term) return snapshot?.memories ?? [];
    return (snapshot?.memories ?? []).filter((memory) =>
      [
        memory.content,
        memory.type,
        memory.scopeType,
        memory.projectName ?? "",
        memory.conversationTitle ?? "",
        ...memory.tags,
      ].some((value) => value.toLocaleLowerCase().includes(term)),
    );
  }, [query, snapshot?.memories]);

  const updateSettings = async (
    patch: Parameters<typeof apiClient.memories.updateSettings>[0],
  ) => {
    const ok = await mutate(() => apiClient.memories.updateSettings(patch));
    if (!ok) return;
  };

  const saveMemory = async () => {
    const content = memoryDraft.content.trim();
    if (!content) return;
    const common = {
      type: memoryDraft.type,
      content,
      importance: memoryDraft.importance,
      tags: parseTags(memoryDraft.tags),
      scopeType: memoryDraft.scopeType,
      projectId:
        memoryDraft.scopeType === "project" ? memoryDraft.projectId : "",
      conversationId:
        memoryDraft.scopeType === "conversation"
          ? memoryDraft.conversationId
          : "",
      sensitivity: memoryDraft.sensitivity,
    };
    const ok = await mutate(() =>
      memoryDraft.id
        ? apiClient.memories.updateGovernanceMemory({
            ...common,
            memoryId: memoryDraft.id,
            expectedRevision: memoryDraft.expectedRevision ?? 0,
          })
        : apiClient.memories.createGovernanceMemory(common),
    );
    if (ok) setMemoryDraft(EMPTY_MEMORY_DRAFT);
  };

  const editMemory = (memory: GovernanceMemory) => {
    setMemoryDraft({
      id: memory.id,
      expectedRevision: memory.revision,
      type: memory.type,
      content: memory.content,
      importance: memory.importance,
      tags: memory.tags.join(", "),
      scopeType: memory.scopeType,
      projectId: memory.projectId ?? "",
      conversationId: memory.conversationId ?? "",
      sensitivity: memory.sensitivity,
    });
    setSection("memories");
  };

  const forgetMemory = async (memory: GovernanceMemory) => {
    if (pendingDeleteId !== memory.id) {
      setPendingDeleteId(memory.id);
      return;
    }
    const ok = await mutate(() =>
      apiClient.memories.deleteGovernanceMemory({
        memoryId: memory.id,
        expectedRevision: memory.revision,
      }),
    );
    setPendingDeleteId(null);
    if (ok && detail?.memory.id === memory.id) setDetail(null);
  };

  const loadDetail = async (memoryId: string) => {
    const loadVersion = ++detailLoadVersionRef.current;
    setDetailLoadingId(memoryId);
    setError(null);
    try {
      const next = await apiClient.memories.getGovernanceMemoryDetail({
        memoryId,
      });
      if (loadVersion === detailLoadVersionRef.current) setDetail(next);
    } catch (nextError) {
      if (loadVersion === detailLoadVersionRef.current) {
        setError(errorMessage(nextError, t("requestFailed")));
      }
    } finally {
      if (loadVersion === detailLoadVersionRef.current) {
        setDetailLoadingId(null);
      }
    }
  };

  const closeDetail = () => {
    detailLoadVersionRef.current += 1;
    setDetailLoadingId(null);
    setDetail(null);
  };

  const saveProject = async () => {
    if (!projectDraft.name.trim()) return;
    const ok = await mutate(() =>
      projectDraft.id
        ? apiClient.memories.updateProject({
            projectId: projectDraft.id,
            expectedRevision: projectDraft.expectedRevision,
            name: projectDraft.name.trim(),
            description: projectDraft.description.trim(),
            lifecycleStatus:
              snapshot?.projects.find(
                (project) => project.id === projectDraft.id,
              )?.lifecycleStatus ?? "active",
          })
        : apiClient.memories.createProject({
            name: projectDraft.name.trim(),
            description: projectDraft.description.trim(),
          }),
    );
    if (ok) {
      setProjectDraft({
        id: "",
        expectedRevision: 0,
        name: "",
        description: "",
      });
    }
  };

  const toggleProject = async (project: MemoryProject) => {
    await mutate(() =>
      apiClient.memories.updateProject({
        projectId: project.id,
        expectedRevision: project.revision,
        name: project.name,
        description: project.description,
        lifecycleStatus:
          project.lifecycleStatus === "active" ? "archived" : "active",
      }),
    );
  };

  const updatePolicy = async (
    policy: ConversationMemoryPolicy,
    patch: Partial<{
      projectId: string;
      useMode: MemoryPolicyMode;
      learnMode: MemoryPolicyMode;
    }>,
  ) => {
    await mutate(() =>
      apiClient.memories.updateConversationPolicy({
        conversationId: policy.conversationId,
        expectedScopeGeneration: policy.scopeGeneration,
        projectId: patch.projectId ?? policy.projectId ?? "",
        useMode: patch.useMode ?? policy.useMode,
        learnMode: patch.learnMode ?? policy.learnMode,
      }),
    );
  };

  const decideReview = async (
    review: MemoryReviewSuggestion,
    decision: MemoryReviewDecision,
  ) => {
    const ok = await mutate(() =>
      apiClient.memories.decideMemoryReview({
        suggestionId: review.id,
        decision,
        editedContent:
          decision === "edit_merge" ? reviewDrafts[review.id] : undefined,
      }),
    );
    if (ok) {
      setReviewDrafts((current) => {
        const next = { ...current };
        delete next[review.id];
        return next;
      });
    }
  };

  if (loading) {
    return (
      <div
        role="status"
        className="flex min-h-64 items-center justify-center gap-2 text-sm text-muted-foreground"
      >
        <Loader2 className="animate-spin" size={18} aria-hidden="true" />
        {t("loadingGovernance")}
      </div>
    );
  }

  if (!snapshot) {
    return (
      <div className="space-y-3 rounded-xl border border-red-200 bg-red-50 p-5 dark:border-red-900/60 dark:bg-red-950/20">
        <p role="alert" className="text-sm text-red-700 dark:text-red-200">
          {error ?? t("requestFailed")}
        </p>
        <button
          type="button"
          onClick={() => void load()}
          className="inline-flex items-center gap-2 rounded-lg border border-red-300 px-3 py-2 text-sm text-red-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring dark:border-red-800 dark:text-red-200"
        >
          <RefreshCw size={15} aria-hidden="true" />
          {t("retry")}
        </button>
      </div>
    );
  }

  const activeProjects = snapshot.projects.filter(
    (project) => project.lifecycleStatus === "active",
  );

  return (
    <div className="space-y-5" aria-busy={saving}>
      <header className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div>
          <h3 className="flex items-center gap-2 text-lg font-semibold text-foreground">
            <Brain size={20} className="text-cyan-500" aria-hidden="true" />
            {t("governanceTitle")}
          </h3>
          <p className="mt-1 max-w-3xl text-xs leading-relaxed text-muted-foreground">
            {t("governanceSubtitle")}
          </p>
        </div>
        <button
          type="button"
          onClick={() => void load()}
          disabled={saving}
          className="inline-flex items-center gap-2 self-start rounded-lg border border-border px-3 py-2 text-xs text-muted-foreground transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
        >
          <RefreshCw size={14} aria-hidden="true" />
          {t("refresh")}
        </button>
      </header>

      <section
        aria-label={t("globalPolicy")}
        className="grid gap-3 md:grid-cols-2 xl:grid-cols-4"
      >
        <PolicySwitch
          label={t("masterToggle")}
          description={t("masterToggleDesc")}
          checked={snapshot.settings.enabled}
          disabled={saving}
          onChange={() =>
            void updateSettings({ enabled: !snapshot.settings.enabled })
          }
        />
        <PolicySwitch
          label={t("globalUse")}
          description={t("globalUseDesc")}
          checked={snapshot.settings.searchEnabled}
          disabled={saving}
          onChange={() =>
            void updateSettings({
              searchEnabled: !snapshot.settings.searchEnabled,
            })
          }
        />
        <PolicySwitch
          label={t("globalLearn")}
          description={t("globalLearnDesc")}
          checked={snapshot.settings.autoRecordEnabled}
          disabled={saving}
          onChange={() =>
            void updateSettings({
              autoRecordEnabled: !snapshot.settings.autoRecordEnabled,
            })
          }
        />
        <PolicySwitch
          label={t("sensitiveMemory")}
          description={t("sensitiveMemoryDesc")}
          checked={snapshot.settings.sensitiveMemoryEnabled}
          disabled={saving}
          onChange={() =>
            void updateSettings({
              sensitiveMemoryEnabled: !snapshot.settings.sensitiveMemoryEnabled,
            })
          }
        />
      </section>

      {error && (
        <div
          role="alert"
          className="flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 p-3 text-xs text-red-700 dark:border-red-900/60 dark:bg-red-950/20 dark:text-red-200"
        >
          <CircleAlert size={15} className="mt-0.5 shrink-0" aria-hidden />
          <span>{error}</span>
        </div>
      )}

      <nav
        aria-label={t("governanceSections")}
        className="flex gap-1 overflow-x-auto border-b border-border"
      >
        {(["memories", "projects", "reviews", "operations"] as Section[]).map(
          (item) => (
            <button
              key={item}
              type="button"
              aria-current={section === item ? "page" : undefined}
              onClick={() => setSection(item)}
              className={`whitespace-nowrap border-b-2 px-3 py-2 text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring ${
                section === item
                  ? "border-cyan-500 font-medium text-cyan-700 dark:text-cyan-300"
                  : "border-transparent text-muted-foreground hover:text-foreground"
              }`}
            >
              {t(`section${item[0].toUpperCase()}${item.slice(1)}`)}
              {item === "reviews" && snapshot.reviews.length > 0
                ? ` (${snapshot.reviews.length})`
                : ""}
            </button>
          ),
        )}
      </nav>

      {section === "memories" && (
        <div className="grid gap-4 xl:grid-cols-[minmax(18rem,0.75fr)_minmax(0,1.5fr)]">
          <section className="space-y-3 rounded-xl border border-border bg-card p-4">
            <div className="flex items-center justify-between gap-2">
              <h4 className="font-semibold text-foreground">
                {memoryDraft.id ? t("editMemory") : t("addMemory")}
              </h4>
              {memoryDraft.id && (
                <button
                  type="button"
                  onClick={() => setMemoryDraft(EMPTY_MEMORY_DRAFT)}
                  aria-label={t("cancelEdit")}
                  className="rounded-lg p-1.5 text-muted-foreground hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  <X size={16} aria-hidden />
                </button>
              )}
            </div>
            <label className="block text-xs font-medium text-muted-foreground">
              {t("typeLabel")}
              <select
                value={memoryDraft.type}
                onChange={(event) =>
                  setMemoryDraft((current) => ({
                    ...current,
                    type: event.target.value as MemoryType,
                  }))
                }
                className="mt-1 w-full rounded-lg border border-input bg-background px-3 py-2 text-sm text-foreground"
              >
                {MEMORY_TYPES.map((value) => (
                  <option key={value} value={value}>
                    {t(`type${value[0].toUpperCase()}${value.slice(1)}`)}
                  </option>
                ))}
              </select>
            </label>
            <label className="block text-xs font-medium text-muted-foreground">
              {t("memoryContent")}
              <textarea
                value={memoryDraft.content}
                onChange={(event) =>
                  setMemoryDraft((current) => ({
                    ...current,
                    content: event.target.value,
                  }))
                }
                maxLength={MEMORY_LIMITS.maxContentChars}
                className="mt-1 h-28 w-full resize-none rounded-lg border border-input bg-background p-3 text-sm text-foreground"
                placeholder={t("contentPlaceholder")}
              />
            </label>
            <div className="grid gap-3 sm:grid-cols-2">
              <label className="block text-xs font-medium text-muted-foreground">
                {t("scope")}
                <select
                  value={memoryDraft.scopeType}
                  onChange={(event) =>
                    setMemoryDraft((current) => ({
                      ...current,
                      scopeType: event.target.value as MemoryScopeType,
                      projectId: "",
                      conversationId: "",
                    }))
                  }
                  className="mt-1 w-full rounded-lg border border-input bg-background px-3 py-2 text-sm text-foreground"
                >
                  <option value="global">{t("scopeGlobal")}</option>
                  <option value="project">{t("scopeProject")}</option>
                  <option value="conversation">{t("scopeConversation")}</option>
                </select>
              </label>
              <label className="block text-xs font-medium text-muted-foreground">
                {t("sensitivity")}
                <select
                  value={memoryDraft.sensitivity}
                  onChange={(event) =>
                    setMemoryDraft((current) => ({
                      ...current,
                      sensitivity: event.target.value as MemorySensitivity,
                    }))
                  }
                  className="mt-1 w-full rounded-lg border border-input bg-background px-3 py-2 text-sm text-foreground"
                >
                  <option value="normal">{t("normal")}</option>
                  <option
                    value="sensitive"
                    disabled={!snapshot.settings.sensitiveMemoryEnabled}
                  >
                    {t("sensitive")}
                  </option>
                </select>
              </label>
            </div>
            {memoryDraft.scopeType === "project" && (
              <label className="block text-xs font-medium text-muted-foreground">
                {t("project")}
                <select
                  required
                  value={memoryDraft.projectId}
                  onChange={(event) =>
                    setMemoryDraft((current) => ({
                      ...current,
                      projectId: event.target.value,
                    }))
                  }
                  className="mt-1 w-full rounded-lg border border-input bg-background px-3 py-2 text-sm text-foreground"
                >
                  <option value="">{t("selectProject")}</option>
                  {activeProjects.map((project) => (
                    <option key={project.id} value={project.id}>
                      {project.name}
                    </option>
                  ))}
                </select>
              </label>
            )}
            {memoryDraft.scopeType === "conversation" && (
              <label className="block text-xs font-medium text-muted-foreground">
                {t("conversation")}
                <select
                  required
                  value={memoryDraft.conversationId}
                  onChange={(event) =>
                    setMemoryDraft((current) => ({
                      ...current,
                      conversationId: event.target.value,
                    }))
                  }
                  className="mt-1 w-full rounded-lg border border-input bg-background px-3 py-2 text-sm text-foreground"
                >
                  <option value="">{t("selectConversation")}</option>
                  {snapshot.conversations.map((conversation) => (
                    <option
                      key={conversation.conversationId}
                      value={conversation.conversationId}
                    >
                      {conversation.title}
                    </option>
                  ))}
                </select>
              </label>
            )}
            <label className="block text-xs font-medium text-muted-foreground">
              {t("tags")}
              <input
                value={memoryDraft.tags}
                onChange={(event) =>
                  setMemoryDraft((current) => ({
                    ...current,
                    tags: event.target.value,
                  }))
                }
                className="mt-1 w-full rounded-lg border border-input bg-background px-3 py-2 text-sm text-foreground"
                placeholder={t("tagsPlaceholder")}
              />
            </label>
            <button
              type="button"
              disabled={
                saving ||
                !memoryDraft.content.trim() ||
                (memoryDraft.scopeType === "project" &&
                  !memoryDraft.projectId) ||
                (memoryDraft.scopeType === "conversation" &&
                  !memoryDraft.conversationId)
              }
              onClick={() => void saveMemory()}
              className="inline-flex w-full items-center justify-center gap-2 rounded-lg bg-cyan-600 px-3 py-2 text-sm font-medium text-white hover:bg-cyan-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
            >
              {memoryDraft.id ? (
                <Save size={16} aria-hidden />
              ) : (
                <Plus size={16} aria-hidden />
              )}
              {memoryDraft.id ? t("saveEdit") : t("add")}
            </button>
          </section>

          <section className="space-y-3">
            <label className="relative block">
              <span className="sr-only">{t("filterPlaceholder")}</span>
              <Search
                size={16}
                className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"
                aria-hidden
              />
              <input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder={t("filterPlaceholder")}
                className="w-full rounded-xl border border-input bg-background py-2.5 pl-9 pr-3 text-sm text-foreground"
              />
            </label>
            {filteredMemories.length === 0 ? (
              <EmptyState text={query ? t("emptyFiltered") : t("empty")} />
            ) : (
              filteredMemories.map((memory) => (
                <article
                  key={memory.id}
                  className="rounded-xl border border-border bg-card p-4"
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0 space-y-2">
                      <div className="flex flex-wrap gap-1.5 text-[11px]">
                        <Badge>
                          {t(
                            `scope${memory.scopeType[0].toUpperCase()}${memory.scopeType.slice(1)}`,
                          )}
                        </Badge>
                        <Badge>{memory.authorityKind}</Badge>
                        <Badge>{memory.lifecycleStatus}</Badge>
                        <Badge>{memory.recallStatus}</Badge>
                        {memory.sensitivity === "sensitive" && (
                          <Badge tone="amber">{t("sensitive")}</Badge>
                        )}
                      </div>
                      <p className="whitespace-pre-wrap break-words text-sm leading-relaxed text-foreground">
                        {memory.content}
                      </p>
                      <p className="text-xs text-muted-foreground">
                        {memory.projectName ||
                          memory.conversationTitle ||
                          t("scopeGlobal")}{" "}
                        · {formatDate(memory.updatedAt)} · r{memory.revision}
                      </p>
                    </div>
                    <div className="flex shrink-0 gap-1">
                      <IconButton
                        label={t("details")}
                        onClick={() => void loadDetail(memory.id)}
                        disabled={detailLoadingId === memory.id}
                      >
                        {detailLoadingId === memory.id ? (
                          <Loader2 size={15} className="animate-spin" />
                        ) : (
                          <History size={15} />
                        )}
                      </IconButton>
                      <IconButton
                        label={t("editAria")}
                        onClick={() => editMemory(memory)}
                      >
                        <Pencil size={15} />
                      </IconButton>
                      <IconButton
                        label={
                          pendingDeleteId === memory.id
                            ? t("confirmForget")
                            : t("forget")
                        }
                        danger
                        onBlur={() =>
                          setPendingDeleteId((current) =>
                            current === memory.id ? null : current,
                          )
                        }
                        onClick={() => void forgetMemory(memory)}
                      >
                        <Trash2 size={15} />
                      </IconButton>
                    </div>
                  </div>
                </article>
              ))
            )}
          </section>
        </div>
      )}

      {section === "projects" && (
        <div className="space-y-5">
          <section className="grid gap-3 rounded-xl border border-border bg-card p-4 md:grid-cols-[minmax(10rem,0.7fr)_minmax(12rem,1fr)_auto] md:items-end">
            <label className="block text-xs font-medium text-muted-foreground">
              {t("projectName")}
              <input
                value={projectDraft.name}
                maxLength={200}
                onChange={(event) =>
                  setProjectDraft((current) => ({
                    ...current,
                    name: event.target.value,
                  }))
                }
                className="mt-1 w-full rounded-lg border border-input bg-background px-3 py-2 text-sm text-foreground"
              />
            </label>
            <label className="block text-xs font-medium text-muted-foreground">
              {t("projectDescription")}
              <input
                value={projectDraft.description}
                maxLength={4000}
                onChange={(event) =>
                  setProjectDraft((current) => ({
                    ...current,
                    description: event.target.value,
                  }))
                }
                className="mt-1 w-full rounded-lg border border-input bg-background px-3 py-2 text-sm text-foreground"
              />
            </label>
            <div className="flex gap-2">
              <button
                type="button"
                onClick={() => void saveProject()}
                disabled={saving || !projectDraft.name.trim()}
                className="inline-flex items-center justify-center gap-2 rounded-lg bg-cyan-600 px-4 py-2 text-sm font-medium text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
              >
                {projectDraft.id ? <Save size={15} /> : <Plus size={15} />}
                {projectDraft.id ? t("saveEdit") : t("createProject")}
              </button>
              {projectDraft.id && (
                <IconButton
                  label={t("cancelEdit")}
                  onClick={() =>
                    setProjectDraft({
                      id: "",
                      expectedRevision: 0,
                      name: "",
                      description: "",
                    })
                  }
                >
                  <X size={15} />
                </IconButton>
              )}
            </div>
          </section>

          <section aria-label={t("projects")} className="space-y-2">
            {snapshot.projects.length === 0 ? (
              <EmptyState text={t("noProjects")} />
            ) : (
              snapshot.projects.map((project) => (
                <article
                  key={project.id}
                  className="flex flex-col gap-3 rounded-xl border border-border bg-card p-4 md:flex-row md:items-center md:justify-between"
                >
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <FolderKanban size={16} className="text-cyan-500" />
                      <h5 className="font-medium text-foreground">
                        {project.name}
                      </h5>
                      <Badge
                        tone={
                          project.lifecycleStatus === "archived"
                            ? "amber"
                            : "default"
                        }
                      >
                        {t(project.lifecycleStatus)}
                      </Badge>
                    </div>
                    {project.description && (
                      <p className="mt-1 text-xs text-muted-foreground">
                        {project.description}
                      </p>
                    )}
                    <p className="mt-2 text-xs text-muted-foreground">
                      {t("projectCounts", {
                        conversations: project.conversationCount,
                        memories: project.memoryCount,
                      })}
                    </p>
                  </div>
                  <div className="flex gap-2">
                    <button
                      type="button"
                      onClick={() =>
                        setProjectDraft({
                          id: project.id,
                          expectedRevision: project.revision,
                          name: project.name,
                          description: project.description,
                        })
                      }
                      className="inline-flex items-center gap-2 rounded-lg border border-border px-3 py-2 text-xs text-muted-foreground hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    >
                      <Pencil size={14} />
                      {t("edit")}
                    </button>
                    <button
                      type="button"
                      onClick={() => void toggleProject(project)}
                      disabled={saving}
                      className="inline-flex items-center gap-2 rounded-lg border border-border px-3 py-2 text-xs text-muted-foreground hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
                    >
                      {project.lifecycleStatus === "active" ? (
                        <Archive size={14} />
                      ) : (
                        <RefreshCw size={14} />
                      )}
                      {project.lifecycleStatus === "active"
                        ? t("archive")
                        : t("restore")}
                    </button>
                  </div>
                </article>
              ))
            )}
          </section>

          <section className="space-y-2">
            <h4 className="font-semibold text-foreground">
              {t("conversationPolicies")}
            </h4>
            {snapshot.conversations.length === 0 ? (
              <EmptyState text={t("noConversations")} />
            ) : (
              snapshot.conversations.map((policy) => (
                <article
                  key={policy.conversationId}
                  className="grid gap-3 rounded-xl border border-border bg-card p-4 lg:grid-cols-[minmax(12rem,1fr)_repeat(3,minmax(8rem,0.55fr))] lg:items-end"
                >
                  <div>
                    <h5 className="truncate text-sm font-medium text-foreground">
                      {policy.title}
                    </h5>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {t("effectivePolicy", {
                        use: policy.effectiveUse ? t("on") : t("off"),
                        learn: policy.effectiveLearn ? t("on") : t("off"),
                      })}
                      {policy.learnForcedOff
                        ? ` · ${t("archiveLearnOff")}`
                        : ""}
                    </p>
                  </div>
                  <PolicySelect
                    label={t("project")}
                    value={policy.projectId ?? ""}
                    disabled={saving}
                    onChange={(value) =>
                      void updatePolicy(policy, { projectId: value })
                    }
                    options={[
                      { value: "", label: t("unassigned") },
                      ...snapshot.projects
                        .filter(
                          (project) =>
                            project.lifecycleStatus === "active" ||
                            project.id === policy.projectId,
                        )
                        .map((project) => ({
                          value: project.id,
                          label: `${project.name}${
                            project.lifecycleStatus === "archived"
                              ? ` (${t("archived")})`
                              : ""
                          }`,
                        })),
                    ]}
                  />
                  <PolicySelect
                    label={t("usePolicy")}
                    value={policy.useMode}
                    disabled={saving}
                    onChange={(value) =>
                      void updatePolicy(policy, {
                        useMode: value as MemoryPolicyMode,
                      })
                    }
                    options={POLICY_MODES.map((value) => ({
                      value,
                      label: t(value),
                    }))}
                  />
                  <PolicySelect
                    label={t("learnPolicy")}
                    value={policy.learnMode}
                    disabled={saving}
                    onChange={(value) =>
                      void updatePolicy(policy, {
                        learnMode: value as MemoryPolicyMode,
                      })
                    }
                    options={POLICY_MODES.map((value) => ({
                      value,
                      label: t(value),
                    }))}
                  />
                </article>
              ))
            )}
          </section>
        </div>
      )}

      {section === "reviews" && (
        <section className="space-y-3">
          {snapshot.reviews.length === 0 ? (
            <EmptyState text={t("noReviews")} />
          ) : (
            snapshot.reviews.map((review) => (
              <ReviewCard
                key={review.id}
                review={review}
                editedContent={reviewDrafts[review.id] ?? review.content}
                disabled={saving}
                onEditedContentChange={(value) =>
                  setReviewDrafts((current) => ({
                    ...current,
                    [review.id]: value,
                  }))
                }
                onDecision={(decision) => void decideReview(review, decision)}
                t={t}
              />
            ))
          )}
        </section>
      )}

      {section === "operations" && (
        <div className="grid gap-4 lg:grid-cols-2">
          <MemoryPortabilityPanel
            apiClient={apiClient}
            snapshot={snapshot}
            disabled={saving}
            onImported={load}
            t={t}
          />
          <section className="space-y-3 rounded-xl border border-border bg-card p-4">
            <h4 className="flex items-center gap-2 font-semibold text-foreground">
              <Database size={16} className="text-cyan-500" />
              {t("deletionProgress")}
            </h4>
            {snapshot.deletions.length === 0 ? (
              <p className="text-xs text-muted-foreground">
                {t("noDeletions")}
              </p>
            ) : (
              snapshot.deletions.map((deletion) => (
                <div
                  key={deletion.manifestId}
                  className="rounded-lg border border-border p-3 text-xs text-muted-foreground"
                >
                  <div className="flex flex-wrap gap-1.5">
                    <Badge>{deletion.onlinePurgeStatus}</Badge>
                    <Badge>{deletion.backupExpiryStatus}</Badge>
                  </div>
                  <p className="mt-2 font-mono text-[11px]">
                    {deletion.memoryId}
                  </p>
                  <p className="mt-1">
                    {t("backupExpires", {
                      date: formatDate(deletion.backupExpiresAt),
                    })}
                  </p>
                </div>
              ))
            )}
          </section>
          <section className="space-y-3 rounded-xl border border-border bg-card p-4">
            <h4 className="flex items-center gap-2 font-semibold text-foreground">
              <ShieldCheck size={16} className="text-cyan-500" />
              {t("searchDiagnostics")}
            </h4>
            <p className="text-xs text-muted-foreground">
              {t("diagnosticsPrivacy")}
            </p>
            {snapshot.diagnostics.length === 0 ? (
              <p className="text-xs text-muted-foreground">
                {t("noDiagnostics")}
              </p>
            ) : (
              snapshot.diagnostics.map((diagnostic, index) => (
                <div
                  key={`${diagnostic.assistantMessageId}-${diagnostic.profile}-${index}`}
                  className="rounded-lg border border-border p-3 text-xs text-muted-foreground"
                >
                  <div className="flex flex-wrap gap-1.5">
                    <Badge>{diagnostic.profile}</Badge>
                    <Badge>{diagnostic.status}</Badge>
                    {diagnostic.fallbackCode !== "NONE" && (
                      <Badge tone="amber">{diagnostic.fallbackCode}</Badge>
                    )}
                  </div>
                  <p className="mt-2">
                    {t("diagnosticCounts", {
                      baseline: diagnostic.baselineCount,
                      final: diagnostic.finalCount,
                      overlap: diagnostic.overlapCount,
                      duration: diagnostic.durationMillis,
                    })}
                  </p>
                </div>
              ))
            )}
          </section>
        </div>
      )}

      {detail && (
        <MemoryDetailPanel detail={detail} onClose={closeDetail} t={t} />
      )}

      {saving && (
        <div
          role="status"
          aria-live="polite"
          className="fixed bottom-5 right-5 z-50 inline-flex items-center gap-2 rounded-full border border-border bg-background px-4 py-2 text-xs text-muted-foreground shadow-lg"
        >
          <Loader2 size={14} className="animate-spin" />
          {t("saving")}
        </div>
      )}
    </div>
  );
};

function MemoryPortabilityPanel({
  apiClient,
  snapshot,
  disabled,
  onImported,
  t,
}: {
  apiClient: NeoChatApiClient;
  snapshot: MemoryGovernanceSnapshot;
  disabled: boolean;
  onImported: () => Promise<void>;
  t: ReturnType<typeof useTranslations<"Memory">>;
}) {
  const [exportPassphrase, setExportPassphrase] = useState("");
  const [includeHistory, setIncludeHistory] = useState(true);
  const [packageFile, setPackageFile] = useState<File | null>(null);
  const [importPassphrase, setImportPassphrase] = useState("");
  const [mappings, setMappings] = useState<MemoryImportMappings>(() =>
    emptyImportMappings(),
  );
  const [plan, setPlan] = useState<MemoryImportDryRunResult | null>(null);
  const [planStale, setPlanStale] = useState(false);
  const [busy, setBusy] = useState<"export" | "dry-run" | "confirm" | null>(
    null,
  );
  const [portabilityError, setPortabilityError] = useState<string | null>(null);
  const [status, setStatus] = useState<string | null>(null);
  const packageInputRef = useRef<HTMLInputElement>(null);

  const exportPackage = async () => {
    if (exportPassphrase.length < 12 || busy) return;
    setBusy("export");
    setPortabilityError(null);
    setStatus(null);
    try {
      const blob = await apiClient.memories.exportMemoryPackage({
        passphrase: exportPassphrase,
        includeHistory,
      });
      const url = URL.createObjectURL(blob);
      try {
        const anchor = document.createElement("a");
        anchor.href = url;
        anchor.download = `neo-chat-memory-${new Date()
          .toISOString()
          .replaceAll(":", "-")}.mm-memory`;
        anchor.click();
      } finally {
        URL.revokeObjectURL(url);
      }
      setExportPassphrase("");
      setStatus(t("exportComplete"));
    } catch (nextError) {
      setPortabilityError(errorMessage(nextError, t("exportFailed")));
    } finally {
      setBusy(null);
    }
  };

  const dryRunImport = async () => {
    if (!packageFile || importPassphrase.length < 12 || busy) return;
    setBusy("dry-run");
    setPortabilityError(null);
    setStatus(null);
    try {
      const next = await apiClient.memories.dryRunMemoryImport({
        packageFile,
        passphrase: importPassphrase,
        mappings,
      });
      setPlan(next);
      setPlanStale(false);
      setStatus(
        next.scopeRequirements.length > 0
          ? t("mappingRequired")
          : t("dryRunComplete"),
      );
    } catch (nextError) {
      setPlan(null);
      setPlanStale(false);
      setPortabilityError(errorMessage(nextError, t("importFailed")));
    } finally {
      setBusy(null);
    }
  };

  const confirmImport = async () => {
    if (
      !packageFile ||
      !plan ||
      planStale ||
      plan.scopeRequirements.length > 0 ||
      busy
    ) {
      return;
    }
    setBusy("confirm");
    setPortabilityError(null);
    setStatus(null);
    try {
      const result = await apiClient.memories.confirmMemoryImport({
        packageFile,
        passphrase: importPassphrase,
        mappings,
        planToken: plan.planToken,
      });
      setStatus(
        t("importComplete", {
          memories: result.addedMemories,
          projects: result.addedProjects,
        }),
      );
      setPackageFile(null);
      setImportPassphrase("");
      setMappings(emptyImportMappings());
      setPlan(null);
      setPlanStale(false);
      if (packageInputRef.current) packageInputRef.current.value = "";
      await onImported();
    } catch (nextError) {
      setPortabilityError(errorMessage(nextError, t("confirmImportFailed")));
    } finally {
      setBusy(null);
    }
  };

  const updateRequirement = (
    requirement: MemoryImportScopeRequirement,
    selection: string,
  ) => {
    setMappings((current) => {
      if (requirement.kind === "project") {
        const projects = { ...current.projects };
        const mapping = parseProjectMappingSelection(selection);
        if (mapping) projects[requirement.portableRef] = mapping;
        else delete projects[requirement.portableRef];
        return { ...current, projects };
      }
      const conversations = { ...current.conversations };
      const mapping = parseConversationMappingSelection(selection);
      if (mapping) conversations[requirement.portableRef] = mapping;
      else delete conversations[requirement.portableRef];
      return { ...current, conversations };
    });
    setPlanStale(true);
    setStatus(t("rerunDryRun"));
  };

  const activeProjects = snapshot.projects.filter(
    (project) => project.lifecycleStatus === "active",
  );

  return (
    <section className="space-y-5 rounded-xl border border-border bg-card p-4 lg:col-span-2">
      <div>
        <h4 className="flex items-center gap-2 font-semibold text-foreground">
          <FileCheck2 size={16} className="text-cyan-500" aria-hidden />
          {t("portabilityTitle")}
        </h4>
        <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
          {t("portabilityPrivacy")}
        </p>
      </div>

      {portabilityError && (
        <p role="alert" className="text-xs text-red-700 dark:text-red-300">
          {portabilityError}
        </p>
      )}
      {status && (
        <p
          role="status"
          aria-live="polite"
          className="text-xs text-cyan-700 dark:text-cyan-300"
        >
          {status}
        </p>
      )}

      <div className="grid gap-5 xl:grid-cols-2">
        <div className="space-y-3 rounded-lg border border-border p-4">
          <h5 className="flex items-center gap-2 text-sm font-semibold text-foreground">
            <Download size={15} aria-hidden />
            {t("exportTitle")}
          </h5>
          <label className="block text-xs font-medium text-muted-foreground">
            {t("packagePassphrase")}
            <input
              type="password"
              autoComplete="new-password"
              value={exportPassphrase}
              minLength={12}
              maxLength={1024}
              onChange={(event) => setExportPassphrase(event.target.value)}
              className="mt-1 w-full rounded-lg border border-input bg-background px-3 py-2 text-sm text-foreground"
            />
          </label>
          <label className="flex items-center gap-2 text-xs text-muted-foreground">
            <input
              type="checkbox"
              checked={includeHistory}
              onChange={(event) => setIncludeHistory(event.target.checked)}
              className="size-4 rounded border-input"
            />
            {t("includeHistory")}
          </label>
          <button
            type="button"
            disabled={disabled || busy !== null || exportPassphrase.length < 12}
            onClick={() => void exportPackage()}
            className="inline-flex items-center gap-2 rounded-lg bg-cyan-600 px-3 py-2 text-sm font-medium text-white hover:bg-cyan-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
          >
            {busy === "export" ? (
              <Loader2 size={15} className="animate-spin" aria-hidden />
            ) : (
              <Download size={15} aria-hidden />
            )}
            {t("downloadPackage")}
          </button>
        </div>

        <div className="space-y-3 rounded-lg border border-border p-4">
          <h5 className="flex items-center gap-2 text-sm font-semibold text-foreground">
            <Upload size={15} aria-hidden />
            {t("importTitle")}
          </h5>
          <label className="block text-xs font-medium text-muted-foreground">
            {t("encryptedPackage")}
            <input
              ref={packageInputRef}
              type="file"
              accept=".mm-memory,application/octet-stream"
              onChange={(event) => {
                setPackageFile(event.target.files?.[0] ?? null);
                setMappings(emptyImportMappings());
                setPlan(null);
                setPlanStale(false);
                setStatus(null);
              }}
              className="mt-1 block w-full text-xs text-muted-foreground file:mr-3 file:rounded-lg file:border-0 file:bg-muted file:px-3 file:py-2 file:text-xs file:font-medium file:text-foreground"
            />
          </label>
          <label className="block text-xs font-medium text-muted-foreground">
            {t("packagePassphrase")}
            <input
              type="password"
              autoComplete="off"
              value={importPassphrase}
              minLength={12}
              maxLength={1024}
              onChange={(event) => {
                setImportPassphrase(event.target.value);
                if (plan) setPlanStale(true);
              }}
              className="mt-1 w-full rounded-lg border border-input bg-background px-3 py-2 text-sm text-foreground"
            />
          </label>
          <button
            type="button"
            disabled={
              disabled ||
              busy !== null ||
              !packageFile ||
              importPassphrase.length < 12
            }
            onClick={() => void dryRunImport()}
            className="inline-flex items-center gap-2 rounded-lg border border-border px-3 py-2 text-sm font-medium text-foreground hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
          >
            {busy === "dry-run" && (
              <Loader2 size={15} className="animate-spin" aria-hidden />
            )}
            {plan ? t("rerunDryRunButton") : t("startDryRun")}
          </button>
        </div>
      </div>

      {plan && (
        <div className="space-y-4 border-t border-border pt-4">
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-5">
            {(
              ["NOOP", "ADD", "REVIEW", "REJECT", "SCOPE_REQUIRED"] as const
            ).map((result) => (
              <div key={result} className="rounded-lg bg-muted p-3 text-center">
                <div className="text-lg font-semibold text-foreground">
                  {plan.counts[result]}
                </div>
                <div className="text-[10px] text-muted-foreground">
                  {result}
                </div>
              </div>
            ))}
          </div>

          {plan.scopeRequirements.length > 0 && (
            <div className="space-y-3">
              <h5 className="text-sm font-semibold text-foreground">
                {t("scopeMappings")}
              </h5>
              {plan.scopeRequirements.map((requirement) => (
                <label
                  key={`${requirement.kind}:${requirement.portableRef}`}
                  className="block text-xs font-medium text-muted-foreground"
                >
                  {requirement.name || requirement.portableRef}
                  <select
                    value={mappingSelection(requirement, mappings)}
                    onChange={(event) =>
                      updateRequirement(requirement, event.target.value)
                    }
                    className="mt-1 w-full rounded-lg border border-input bg-background px-3 py-2 text-sm text-foreground"
                  >
                    <option value="">{t("selectMapping")}</option>
                    {requirement.kind === "project" ? (
                      <>
                        <option value="create">{t("mappingCreate")}</option>
                        {activeProjects.map((project) => (
                          <option
                            key={project.id}
                            value={`existing:${project.id}`}
                          >
                            {t("mappingExisting", { name: project.name })}
                          </option>
                        ))}
                        <option value="skip">{t("mappingSkip")}</option>
                      </>
                    ) : (
                      <>
                        <option value="global">{t("mappingGlobal")}</option>
                        {snapshot.conversations.map((conversation) => (
                          <option
                            key={conversation.conversationId}
                            value={`existing:${conversation.conversationId}`}
                          >
                            {t("mappingExisting", {
                              name: conversation.title,
                            })}
                          </option>
                        ))}
                        {activeProjects.map((project) => (
                          <option
                            key={project.id}
                            value={`project-local:${project.id}`}
                          >
                            {t("mappingProject", { name: project.name })}
                          </option>
                        ))}
                        {plan.scopeRequirements
                          .filter((item) => item.kind === "project")
                          .map((project) => (
                            <option
                              key={project.portableRef}
                              value={`project-ref:${project.portableRef}`}
                            >
                              {t("mappingPackageProject", {
                                name: project.name || project.portableRef,
                              })}
                            </option>
                          ))}
                        <option value="skip">{t("mappingSkip")}</option>
                      </>
                    )}
                  </select>
                </label>
              ))}
              <p className="text-xs text-amber-700 dark:text-amber-300">
                {t("rerunAfterMapping")}
              </p>
            </div>
          )}

          {plan.settingsSuggestion && (
            <div className="rounded-lg border border-amber-300 bg-amber-50 p-3 text-xs text-amber-900 dark:border-amber-800 dark:bg-amber-950/20 dark:text-amber-200">
              <p className="font-semibold">{t("settingsSuggestion")}</p>
              <p className="mt-1">{t("settingsNeverApplied")}</p>
            </div>
          )}

          <button
            type="button"
            disabled={
              disabled ||
              busy !== null ||
              planStale ||
              plan.scopeRequirements.length > 0
            }
            onClick={() => void confirmImport()}
            className="inline-flex items-center gap-2 rounded-lg bg-cyan-600 px-3 py-2 text-sm font-medium text-white hover:bg-cyan-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
          >
            {busy === "confirm" ? (
              <Loader2 size={15} className="animate-spin" aria-hidden />
            ) : (
              <FileCheck2 size={15} aria-hidden />
            )}
            {t("confirmImport")}
          </button>
        </div>
      )}
    </section>
  );
}

function emptyImportMappings(): MemoryImportMappings {
  return { projects: {}, conversations: {} };
}

function parseProjectMappingSelection(
  selection: string,
): MemoryImportProjectMapping | undefined {
  if (selection === "create" || selection === "skip") {
    return { mode: selection };
  }
  if (selection.startsWith("existing:")) {
    return { mode: "existing", projectId: selection.slice(9) };
  }
  return undefined;
}

function parseConversationMappingSelection(
  selection: string,
): MemoryImportConversationMapping | undefined {
  if (selection === "global" || selection === "skip") {
    return { mode: selection };
  }
  if (selection.startsWith("existing:")) {
    return { mode: "existing", conversationId: selection.slice(9) };
  }
  if (selection.startsWith("project-local:")) {
    return { mode: "project", projectId: selection.slice(14) };
  }
  if (selection.startsWith("project-ref:")) {
    return { mode: "project", projectRef: selection.slice(12) };
  }
  return undefined;
}

function mappingSelection(
  requirement: MemoryImportScopeRequirement,
  mappings: MemoryImportMappings,
): string {
  if (requirement.kind === "project") {
    const mapping = mappings.projects[requirement.portableRef];
    if (!mapping) return "";
    return mapping.mode === "existing"
      ? `existing:${mapping.projectId ?? ""}`
      : mapping.mode;
  }
  const mapping = mappings.conversations[requirement.portableRef];
  if (!mapping) return "";
  if (mapping.mode === "existing") {
    return `existing:${mapping.conversationId ?? ""}`;
  }
  if (mapping.mode === "project") {
    return mapping.projectRef
      ? `project-ref:${mapping.projectRef}`
      : `project-local:${mapping.projectId ?? ""}`;
  }
  return mapping.mode;
}

function PolicySwitch({
  label,
  description,
  checked,
  disabled,
  onChange,
}: {
  label: string;
  description: string;
  checked: boolean;
  disabled: boolean;
  onChange: () => void;
}) {
  return (
    <div className="rounded-xl border border-border bg-card p-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="text-sm font-medium text-foreground">{label}</div>
          <div className="mt-1 text-xs leading-relaxed text-muted-foreground">
            {description}
          </div>
        </div>
        <SimpleSwitch
          ariaLabel={label}
          checked={checked}
          disabled={disabled}
          onChange={onChange}
        />
      </div>
    </div>
  );
}

function PolicySelect({
  label,
  value,
  options,
  disabled,
  onChange,
}: {
  label: string;
  value: string;
  options: Array<{ value: string; label: string }>;
  disabled: boolean;
  onChange: (value: string) => void;
}) {
  return (
    <label className="block text-xs font-medium text-muted-foreground">
      {label}
      <select
        value={value}
        disabled={disabled}
        onChange={(event) => onChange(event.target.value)}
        className="mt-1 w-full rounded-lg border border-input bg-background px-3 py-2 text-sm text-foreground"
      >
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    </label>
  );
}

function ReviewCard({
  review,
  editedContent,
  disabled,
  onEditedContentChange,
  onDecision,
  t,
}: {
  review: MemoryReviewSuggestion;
  editedContent: string;
  disabled: boolean;
  onEditedContentChange: (value: string) => void;
  onDecision: (decision: MemoryReviewDecision) => void;
  t: ReturnType<typeof useTranslations<"Memory">>;
}) {
  const [expanded, setExpanded] = useState(false);
  return (
    <article className="rounded-xl border border-border bg-card p-4">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap gap-1.5 text-[11px]">
            <Badge tone="amber">{review.proposedAction}</Badge>
            <Badge>{review.scopeType}</Badge>
            <Badge>{review.reasonCode}</Badge>
          </div>
          <p className="mt-3 whitespace-pre-wrap text-sm text-foreground">
            {review.content}
          </p>
          <p className="mt-2 text-xs text-muted-foreground">
            {t("reviewExpires", { date: formatDate(review.expiresAt) })}
          </p>
        </div>
        <button
          type="button"
          onClick={() => setExpanded((current) => !current)}
          aria-expanded={expanded}
          className="inline-flex items-center gap-1 self-start rounded-lg border border-border px-3 py-2 text-xs text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          {expanded ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
          {t("compare")}
        </button>
      </div>
      {expanded && (
        <div className="mt-4 space-y-3 border-t border-border pt-4">
          <div className="grid gap-3 md:grid-cols-2">
            <div>
              <p className="mb-1 text-xs font-medium text-muted-foreground">
                {t("currentValues")}
              </p>
              {review.targets.length === 0 ? (
                <p className="rounded-lg bg-muted p-3 text-xs text-muted-foreground">
                  {t("noCurrentTarget")}
                </p>
              ) : (
                review.targets.map((target) => (
                  <p
                    key={target.memoryId}
                    className="mb-2 rounded-lg bg-muted p-3 text-sm text-foreground"
                  >
                    {target.current ? target.content : t("targetChanged")}
                  </p>
                ))
              )}
            </div>
            <label className="block text-xs font-medium text-muted-foreground">
              {t("mergedValue")}
              <textarea
                value={editedContent}
                onChange={(event) => onEditedContentChange(event.target.value)}
                maxLength={MEMORY_LIMITS.maxContentChars}
                className="mt-1 h-24 w-full rounded-lg border border-input bg-background p-3 text-sm text-foreground"
              />
            </label>
          </div>
          <div className="flex flex-wrap gap-2">
            {REVIEW_DECISIONS.map((decision) => (
              <button
                key={decision}
                type="button"
                disabled={
                  disabled ||
                  (decision === "edit_merge" && !editedContent.trim())
                }
                onClick={() => onDecision(decision)}
                className={`rounded-lg border px-3 py-2 text-xs font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50 ${
                  decision === "reject"
                    ? "border-red-200 text-red-600 dark:border-red-900 dark:text-red-300"
                    : "border-border text-muted-foreground hover:bg-muted"
                }`}
              >
                {t(
                  `decision${decision
                    .split("_")
                    .map((part) => part[0].toUpperCase() + part.slice(1))
                    .join("")}`,
                )}
              </button>
            ))}
          </div>
        </div>
      )}
    </article>
  );
}

function MemoryDetailPanel({
  detail,
  onClose,
  t,
}: {
  detail: GovernanceMemoryDetail;
  onClose: () => void;
  t: ReturnType<typeof useTranslations<"Memory">>;
}) {
  return (
    <section
      aria-label={t("memoryDetails")}
      className="rounded-xl border border-cyan-200 bg-cyan-50/40 p-4 dark:border-cyan-900/60 dark:bg-cyan-950/10"
    >
      <div className="flex items-start justify-between gap-3">
        <div>
          <h4 className="font-semibold text-foreground">
            {t("memoryDetails")}
          </h4>
          <p className="mt-1 text-sm text-foreground">
            {detail.memory.content}
          </p>
        </div>
        <IconButton label={t("closeDetails")} onClick={onClose}>
          <X size={16} />
        </IconButton>
      </div>
      <div className="mt-4 grid gap-4 lg:grid-cols-3">
        <DetailList title={t("provenance")} empty={t("noEvidence")}>
          {detail.evidence.map((evidence) => (
            <li key={`${evidence.messageId}-${evidence.role}`}>
              <strong>{evidence.role}</strong> ·{" "}
              {evidence.conversationTitle || evidence.conversationId}
              <br />
              {evidence.sourceDeleted
                ? t("sourceDeleted")
                : evidence.sourceExcerpt}
            </li>
          ))}
        </DetailList>
        <DetailList title={t("revisionHistory")} empty={t("noHistory")}>
          {detail.history.map((revision) => (
            <li key={`${revision.revision}-${revision.createdAt}`}>
              r{revision.revision} · {revision.operation} · {revision.actorType}
              <br />
              {revision.purged
                ? t("plaintextPurged")
                : revision.priorContent || "—"}
            </li>
          ))}
        </DetailList>
        <DetailList title={t("answerUsage")} empty={t("noUsage")}>
          {detail.usages.map((usage) => (
            <li key={`${usage.assistantMessageId}-${usage.memoryRevision}`}>
              {t("usedRevision", { revision: usage.memoryRevision })}
              <br />
              {formatDate(usage.createdAt)}
            </li>
          ))}
        </DetailList>
      </div>
    </section>
  );
}

function DetailList({
  title,
  empty,
  children,
}: {
  title: string;
  empty: string;
  children: React.ReactNode;
}) {
  const items = Array.isArray(children) ? children : children ? [children] : [];
  return (
    <div>
      <h5 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
        {title}
      </h5>
      {items.length === 0 ? (
        <p className="mt-2 text-xs text-muted-foreground">{empty}</p>
      ) : (
        <ul className="mt-2 space-y-2 text-xs text-muted-foreground">
          {children}
        </ul>
      )}
    </div>
  );
}

function Badge({
  children,
  tone = "default",
}: {
  children: React.ReactNode;
  tone?: "default" | "amber";
}) {
  return (
    <span
      className={`rounded-md px-2 py-0.5 font-medium ${
        tone === "amber"
          ? "bg-amber-100 text-amber-800 dark:bg-amber-950/40 dark:text-amber-200"
          : "bg-cyan-50 text-cyan-700 dark:bg-cyan-950/30 dark:text-cyan-200"
      }`}
    >
      {children}
    </span>
  );
}

function IconButton({
  label,
  danger = false,
  children,
  ...buttonProps
}: React.ButtonHTMLAttributes<HTMLButtonElement> & {
  label: string;
  danger?: boolean;
}) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      {...buttonProps}
      className={`rounded-lg p-2 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50 ${
        danger
          ? "text-red-500 hover:bg-red-50 dark:hover:bg-red-950/20"
          : "text-muted-foreground hover:bg-muted hover:text-foreground"
      }`}
    >
      {children}
    </button>
  );
}

function EmptyState({ text }: { text: string }) {
  return (
    <div className="rounded-xl border border-dashed border-border p-8 text-center text-sm text-muted-foreground">
      {text}
    </div>
  );
}

export default ServerMemoryGovernance;
