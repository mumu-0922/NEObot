import type {
  ApiPage,
  CreateMemoryProjectInput,
  DurableMemorySettingsDTO,
  GovernanceMemoryMutationInput,
  MemoryApi,
  MemoryHealthDTO,
  MemoryImportConfirmResult,
  MemoryImportDryRunResult,
  MemoryImportPackageInput,
  MemoryImportPlanResult,
  MemoryMutationInput,
  MemoryReviewDecision,
  UpdateConversationMemoryPolicyInput,
  UpdateDurableMemorySettingsInput,
  UpdateGovernanceMemoryInput,
  UpdateMemoryInput,
  UpdateMemoryProjectInput,
} from "../types";
import type {
  ConversationMemoryPolicy,
  GovernanceMemory,
  GovernanceMemoryDetail,
  L2SceneGovernanceDetail,
  L2SceneGovernanceScene,
  L2SceneRebuildResult,
  L3PersonaGovernanceDetail,
  L3PersonaGovernancePersona,
  L3PersonaRebuildResult,
  MemoryActivity,
  MemoryDeletionProgress,
  MemoryGovernanceSnapshot,
  MemoryProject,
  MemoryRecord,
  MemoryReviewDecisionResult,
  MemoryReviewSuggestion,
} from "../../../../lib/memory/types";
import type { HttpClient } from "./httpClient";

const memoriesPath = "/v1/memories";
const memorySettingsPath = "/v1/memory-settings";
const memoryHealthPath = "/v1/memory-health";
const memoryGovernancePath = "/v1/memory-governance";
const projectsPath = "/v1/projects";
const memoryReviewsPath = "/v1/memory-reviews";
const memoryActivitiesPath = "/v1/memory-activities";
const memoryExportPath = "/v1/memory-export";
const memoryImportDryRunPath = "/v1/memory-import/dry-run";
const memoryImportConfirmPath = "/v1/memory-import/confirm";

export function createServerMemoryApiShell(httpClient: HttpClient): MemoryApi {
  return {
    async listMemories(input = {}): Promise<MemoryRecord[]> {
      const page = await httpClient.requestJson<ApiPage<MemoryRecord>>(
        memoriesPath,
        { signal: input.signal },
      );
      return Array.isArray(page.items) ? page.items : [];
    },

    async createMemory(input: MemoryMutationInput): Promise<MemoryRecord> {
      return httpClient.requestJson<MemoryRecord>(memoriesPath, {
        method: "POST",
        body: memoryBody(input),
        signal: input.signal,
      });
    },

    async updateMemory(input: UpdateMemoryInput): Promise<MemoryRecord> {
      return httpClient.requestJson<MemoryRecord>(memoryPath(input.memoryId), {
        method: "PATCH",
        body: memoryBody(input),
        signal: input.signal,
      });
    },

    async deleteMemory(input): Promise<void> {
      await httpClient.requestJson<void>(memoryPath(input.memoryId), {
        method: "DELETE",
        signal: input.signal,
      });
    },

    async getSettings(input = {}): Promise<DurableMemorySettingsDTO> {
      return httpClient.requestJson<DurableMemorySettingsDTO>(
        memorySettingsPath,
        { signal: input.signal },
      );
    },

    async getHealth(input = {}): Promise<MemoryHealthDTO> {
      return normalizeMemoryHealth(
        await httpClient.requestJson<unknown>(memoryHealthPath, {
          signal: input.signal,
        }),
      );
    },

    async updateSettings(
      input: UpdateDurableMemorySettingsInput,
    ): Promise<DurableMemorySettingsDTO> {
      const { signal, ...body } = input;
      return httpClient.requestJson<DurableMemorySettingsDTO>(
        memorySettingsPath,
        { method: "PATCH", body, signal },
      );
    },

    async getGovernance(input = {}): Promise<MemoryGovernanceSnapshot> {
      return httpClient.requestJson<MemoryGovernanceSnapshot>(
        memoryGovernancePath,
        { signal: input.signal },
      );
    },

    async listProjects(input = {}): Promise<MemoryProject[]> {
      const page = await httpClient.requestJson<ApiPage<MemoryProject>>(
        projectsPath,
        { signal: input.signal },
      );
      return Array.isArray(page.items) ? page.items : [];
    },

    async createProject(
      input: CreateMemoryProjectInput,
    ): Promise<MemoryProject> {
      const { signal, ...body } = input;
      return httpClient.requestJson<MemoryProject>(projectsPath, {
        method: "POST",
        body,
        signal,
      });
    },

    async updateProject(
      input: UpdateMemoryProjectInput,
    ): Promise<MemoryProject> {
      const { projectId, signal, ...body } = input;
      return httpClient.requestJson<MemoryProject>(projectPath(projectId), {
        method: "PATCH",
        body,
        signal,
      });
    },

    async getConversationPolicy(input): Promise<ConversationMemoryPolicy> {
      return httpClient.requestJson<ConversationMemoryPolicy>(
        conversationPolicyPath(input.conversationId),
        { signal: input.signal },
      );
    },

    async updateConversationPolicy(
      input: UpdateConversationMemoryPolicyInput,
    ): Promise<ConversationMemoryPolicy> {
      const { conversationId, signal, ...body } = input;
      return httpClient.requestJson<ConversationMemoryPolicy>(
        conversationPolicyPath(conversationId),
        { method: "PATCH", body, signal },
      );
    },

    async createGovernanceMemory(
      input: GovernanceMemoryMutationInput,
    ): Promise<GovernanceMemory> {
      const { signal, ...body } = input;
      return httpClient.requestJson<GovernanceMemory>(
        `${memoryGovernancePath}/memories`,
        { method: "POST", body: governanceMemoryBody(body), signal },
      );
    },

    async updateGovernanceMemory(
      input: UpdateGovernanceMemoryInput,
    ): Promise<GovernanceMemory> {
      const { memoryId, signal, ...body } = input;
      return httpClient.requestJson<GovernanceMemory>(
        governanceMemoryPath(memoryId),
        { method: "PATCH", body: governanceMemoryBody(body), signal },
      );
    },

    async deleteGovernanceMemory(input): Promise<MemoryDeletionProgress> {
      return httpClient.requestJson<MemoryDeletionProgress>(
        governanceMemoryPath(input.memoryId),
        {
          method: "DELETE",
          body: { expectedRevision: input.expectedRevision },
          signal: input.signal,
        },
      );
    },

    async getGovernanceMemoryDetail(input): Promise<GovernanceMemoryDetail> {
      return httpClient.requestJson<GovernanceMemoryDetail>(
        `${governanceMemoryPath(input.memoryId)}/details`,
        { signal: input.signal },
      );
    },

    async getL2SceneDetail(input): Promise<L2SceneGovernanceDetail> {
      return httpClient.requestJson<L2SceneGovernanceDetail>(
        `${governanceL2ScenePath(input.sceneId)}/details`,
        { signal: input.signal },
      );
    },

    async setL2SceneEnabled(input): Promise<L2SceneGovernanceScene> {
      return httpClient.requestJson<L2SceneGovernanceScene>(
        `${governanceL2ScenePath(input.sceneId)}/enabled`,
        {
          method: "POST",
          body: {
            expectedRevision: input.expectedRevision,
            enabled: input.enabled,
          },
          signal: input.signal,
        },
      );
    },

    async rebuildL2Scene(input): Promise<L2SceneRebuildResult> {
      return httpClient.requestJson<L2SceneRebuildResult>(
        `${governanceL2ScenePath(input.sceneId)}/rebuild`,
        { method: "POST", signal: input.signal },
      );
    },

    async rebuildL2Scenes(input = {}): Promise<L2SceneRebuildResult> {
      return httpClient.requestJson<L2SceneRebuildResult>(
        `${memoryGovernancePath}/scenes/rebuild`,
        { method: "POST", signal: input.signal },
      );
    },

    async getL3PersonaDetail(input): Promise<L3PersonaGovernanceDetail> {
      return httpClient.requestJson<L3PersonaGovernanceDetail>(
        `${governanceL3PersonaPath(input.personaId)}/details`,
        { signal: input.signal },
      );
    },

    async setL3PersonaEnabled(input): Promise<L3PersonaGovernancePersona> {
      return httpClient.requestJson<L3PersonaGovernancePersona>(
        `${governanceL3PersonaPath(input.personaId)}/enabled`,
        {
          method: "POST",
          body: {
            expectedRevision: input.expectedRevision,
            enabled: input.enabled,
          },
          signal: input.signal,
        },
      );
    },

    async rebuildL3Persona(input): Promise<L3PersonaRebuildResult> {
      return httpClient.requestJson<L3PersonaRebuildResult>(
        `${governanceL3PersonaPath(input.personaId)}/rebuild`,
        { method: "POST", signal: input.signal },
      );
    },

    async rebuildL3Personas(input = {}): Promise<L3PersonaRebuildResult> {
      return httpClient.requestJson<L3PersonaRebuildResult>(
        `${memoryGovernancePath}/personas/rebuild`,
        { method: "POST", signal: input.signal },
      );
    },

    async listMemoryReviews(input = {}): Promise<MemoryReviewSuggestion[]> {
      const page = await httpClient.requestJson<
        ApiPage<MemoryReviewSuggestion>
      >(memoryReviewsPath, { signal: input.signal });
      return Array.isArray(page.items) ? page.items : [];
    },

    async decideMemoryReview(input): Promise<MemoryReviewDecisionResult> {
      return httpClient.requestJson<MemoryReviewDecisionResult>(
        memoryReviewDecisionPath(input.suggestionId),
        {
          method: "POST",
          body: reviewDecisionBody(input.decision, input.editedContent),
          signal: input.signal,
        },
      );
    },

    async listMessageMemoryActivities(input): Promise<MemoryActivity[]> {
      const query = new URLSearchParams({
        assistantMessageId: input.assistantMessageId,
        limit: String(input.limit ?? 20),
      });
      const page = await httpClient.requestJson<ApiPage<MemoryActivity>>(
        `${memoryActivitiesPath}?${query.toString()}`,
        { signal: input.signal },
      );
      return Array.isArray(page.items) ? page.items : [];
    },

    async undoMemoryActivity(input) {
      return httpClient.requestJson(
        `${memoryActivitiesPath}/${encodeURIComponent(input.activityId)}/undo`,
        {
          method: "POST",
          body: { expectedRevision: input.expectedRevision },
          signal: input.signal,
        },
      );
    },

    async exportMemoryPackage(input): Promise<Blob> {
      const response = await httpClient.requestBinary(memoryExportPath, {
        method: "POST",
        body: {
          passphrase: input.passphrase,
          includeHistory: input.includeHistory,
        },
        signal: input.signal,
      });
      return response.blob;
    },

    async dryRunMemoryImport(
      input: MemoryImportPackageInput,
    ): Promise<MemoryImportDryRunResult> {
      return normalizeMemoryImportDryRun(
        await httpClient.requestMultipartJson<unknown>(memoryImportDryRunPath, {
          method: "POST",
          formData: memoryImportFormData(input),
          signal: input.signal,
        }),
      );
    },

    async confirmMemoryImport(input): Promise<MemoryImportConfirmResult> {
      const formData = memoryImportFormData(input);
      formData.append("planToken", input.planToken);
      return normalizeMemoryImportConfirm(
        await httpClient.requestMultipartJson<unknown>(
          memoryImportConfirmPath,
          { method: "POST", formData, signal: input.signal },
        ),
      );
    },
  };
}

function normalizeMemoryHealth(value: unknown): MemoryHealthDTO {
  const object = recordValue(value, "memory health");
  const statuses = ["ready", "indexing", "degraded", "disabled"] as const;
  if (!statuses.includes(object.status as (typeof statuses)[number])) {
    throw new Error("Server returned an invalid memory health status.");
  }
  if (
    typeof object.workerAvailable !== "boolean" ||
    typeof object.embeddingWorkerAvailable !== "boolean" ||
    object.judgeFixed !== true
  ) {
    throw new Error("Server returned invalid memory health authority.");
  }
  return {
    status: object.status as MemoryHealthDTO["status"],
    reasonCode: stringValue(object.reasonCode, "memory health reason"),
    workerAvailable: object.workerAvailable,
    embeddingWorkerAvailable: object.embeddingWorkerAvailable,
    readyCount: nonNegativeInteger(object.readyCount, "memory ready count"),
    pendingCount: nonNegativeInteger(
      object.pendingCount,
      "memory pending count",
    ),
    failedCount: nonNegativeInteger(object.failedCount, "memory failed count"),
    judgeModelId: stringValue(object.judgeModelId, "memory judge model"),
    judgeFixed: object.judgeFixed,
  };
}

function memoryPath(memoryId: string): string {
  return `${memoriesPath}/${encodeURIComponent(memoryId)}`;
}

function projectPath(projectId: string): string {
  return `${projectsPath}/${encodeURIComponent(projectId)}`;
}

function conversationPolicyPath(conversationId: string): string {
  return `/v1/chat/conversations/${encodeURIComponent(conversationId)}/memory-policy`;
}

function governanceMemoryPath(memoryId: string): string {
  return `${memoryGovernancePath}/memories/${encodeURIComponent(memoryId)}`;
}

function governanceL2ScenePath(sceneId: string): string {
  return `${memoryGovernancePath}/scenes/${encodeURIComponent(sceneId)}`;
}

function governanceL3PersonaPath(personaId: string): string {
  return `${memoryGovernancePath}/personas/${encodeURIComponent(personaId)}`;
}

function memoryReviewDecisionPath(suggestionId: string): string {
  return `${memoryReviewsPath}/${encodeURIComponent(suggestionId)}/decision`;
}

function memoryBody(input: MemoryMutationInput) {
  return {
    type: input.type,
    content: input.content,
    importance: input.importance ?? 3,
    tags: input.tags ?? [],
  };
}

function governanceMemoryBody(input: GovernanceMemoryMutationInput) {
  return {
    type: input.type,
    content: input.content,
    importance: input.importance ?? 3,
    tags: input.tags ?? [],
    ...(typeof (input as UpdateGovernanceMemoryInput).expectedRevision ===
    "number"
      ? {
          expectedRevision: (input as UpdateGovernanceMemoryInput)
            .expectedRevision,
        }
      : {}),
    scopeType: input.scopeType,
    projectId: input.projectId ?? "",
    conversationId: input.conversationId ?? "",
    sensitivity: input.sensitivity ?? "normal",
  };
}

function reviewDecisionBody(
  decision: MemoryReviewDecision,
  editedContent: string | undefined,
) {
  return {
    decision,
    editedContent:
      decision === "edit_merge" ? (editedContent?.trim() ?? "") : "",
  };
}

function memoryImportFormData(input: MemoryImportPackageInput): FormData {
  const formData = new FormData();
  formData.append("package", input.packageFile, input.packageFile.name);
  formData.append("passphrase", input.passphrase);
  formData.append("mappings", JSON.stringify(input.mappings));
  return formData;
}

const MEMORY_IMPORT_RESULTS: MemoryImportPlanResult[] = [
  "NOOP",
  "ADD",
  "REVIEW",
  "REJECT",
  "SCOPE_REQUIRED",
];

function normalizeMemoryImportDryRun(value: unknown): MemoryImportDryRunResult {
  const object = recordValue(value, "memory import dry-run");
  const rawCounts = recordValue(object.counts, "memory import counts");
  const counts = Object.fromEntries(
    MEMORY_IMPORT_RESULTS.map((result) => [
      result,
      nonNegativeInteger(rawCounts[result], `memory import ${result} count`),
    ]),
  ) as Record<MemoryImportPlanResult, number>;
  const items = arrayValue(object.items, "memory import items").map(
    (item, index) => {
      const candidate = recordValue(item, `memory import item ${index}`);
      const result = stringValue(candidate.result, "memory import result");
      if (!MEMORY_IMPORT_RESULTS.includes(result as MemoryImportPlanResult)) {
        throw new Error("Server returned an invalid memory import result.");
      }
      return {
        ordinal: positiveInteger(candidate.ordinal, "memory import ordinal"),
        memoryRef: stringValue(candidate.memoryRef, "memory import ref"),
        recordHash: sha256Value(
          candidate.recordHash,
          "memory import record hash",
        ),
        result: result as MemoryImportPlanResult,
        reasonCode: stringValue(candidate.reasonCode, "memory import reason"),
        ...(typeof candidate.currentHash === "string"
          ? {
              currentHash: sha256Value(
                candidate.currentHash,
                "memory import current hash",
              ),
            }
          : {}),
      };
    },
  );
  const scopeRequirements = arrayValue(
    object.scopeRequirements,
    "memory import scope requirements",
  ).map((requirement, index) => {
    const candidate = recordValue(
      requirement,
      `memory import scope requirement ${index}`,
    );
    const kind = stringValue(candidate.kind, "memory import scope kind");
    if (kind !== "project" && kind !== "conversation") {
      throw new Error("Server returned an invalid memory import scope kind.");
    }
    const normalizedKind: "project" | "conversation" = kind;
    return {
      kind: normalizedKind,
      portableRef: stringValue(
        candidate.portableRef,
        "memory import portable ref",
      ),
      ...(typeof candidate.name === "string" ? { name: candidate.name } : {}),
      ...(typeof candidate.description === "string"
        ? { description: candidate.description }
        : {}),
    };
  });
  const settingsSuggestion = normalizeSettingsSuggestion(
    object.settingsSuggestion,
  );
  return {
    importId: stringValue(object.importId, "memory import id"),
    packageSha256: sha256Value(
      object.packageSha256,
      "memory import package hash",
    ),
    manifestSha256: sha256Value(
      object.manifestSha256,
      "memory import manifest hash",
    ),
    planSha256: sha256Value(object.planSha256, "memory import plan hash"),
    planToken: stringValue(object.planToken, "memory import plan token"),
    expiresAt: positiveInteger(object.expiresAt, "memory import expiry"),
    counts,
    items,
    scopeRequirements,
    ...(settingsSuggestion ? { settingsSuggestion } : {}),
  };
}

function normalizeMemoryImportConfirm(
  value: unknown,
): MemoryImportConfirmResult {
  const object = recordValue(value, "memory import confirmation");
  if (object.status !== "completed") {
    throw new Error("Server returned an invalid memory import status.");
  }
  return {
    importId: stringValue(object.importId, "memory import id"),
    status: "completed",
    addedProjects: nonNegativeInteger(
      object.addedProjects,
      "memory import added Projects",
    ),
    addedMemories: nonNegativeInteger(
      object.addedMemories,
      "memory import added Memories",
    ),
    importedAt: positiveInteger(object.importedAt, "memory import time"),
  };
}

function normalizeSettingsSuggestion(
  value: unknown,
): DurableMemorySettingsDTO | undefined {
  if (value === undefined) return undefined;
  const object = recordValue(value, "memory import settings suggestion");
  if (
    typeof object.enabled !== "boolean" ||
    typeof object.searchEnabled !== "boolean" ||
    typeof object.autoRecordEnabled !== "boolean" ||
    typeof object.sensitiveMemoryEnabled !== "boolean" ||
    !isPolicyMode(object.l2Mode) ||
    !isPolicyMode(object.l3Mode)
  ) {
    throw new Error("Server returned an invalid memory settings suggestion.");
  }
  return {
    enabled: object.enabled,
    searchEnabled: object.searchEnabled,
    autoRecordEnabled: object.autoRecordEnabled,
    sensitiveMemoryEnabled: object.sensitiveMemoryEnabled,
    l2Mode: object.l2Mode,
    l3Mode: object.l3Mode,
  };
}

function isPolicyMode(value: unknown): value is "inherit" | "on" | "off" {
  return value === "inherit" || value === "on" || value === "off";
}

function recordValue(value: unknown, label: string): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error(`Server returned an invalid ${label}.`);
  }
  return value as Record<string, unknown>;
}

function arrayValue(value: unknown, label: string): unknown[] {
  if (!Array.isArray(value)) {
    throw new Error(`Server returned invalid ${label}.`);
  }
  return value;
}

function stringValue(value: unknown, label: string): string {
  if (typeof value !== "string" || !value.trim()) {
    throw new Error(`Server returned an invalid ${label}.`);
  }
  return value;
}

function sha256Value(value: unknown, label: string): string {
  const text = stringValue(value, label);
  if (!/^[0-9a-f]{64}$/.test(text)) {
    throw new Error(`Server returned an invalid ${label}.`);
  }
  return text;
}

function nonNegativeInteger(value: unknown, label: string): number {
  if (typeof value !== "number" || !Number.isInteger(value) || value < 0) {
    throw new Error(`Server returned an invalid ${label}.`);
  }
  return value;
}

function positiveInteger(value: unknown, label: string): number {
  const number = nonNegativeInteger(value, label);
  if (number < 1) {
    throw new Error(`Server returned an invalid ${label}.`);
  }
  return number;
}
