import type {
  ApiPage,
  CreateMemoryProjectInput,
  DurableMemorySettingsDTO,
  GovernanceMemoryMutationInput,
  MemoryApi,
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
const memoryGovernancePath = "/v1/memory-governance";
const projectsPath = "/v1/projects";
const memoryReviewsPath = "/v1/memory-reviews";
const memoryActivitiesPath = "/v1/memory-activities";

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
