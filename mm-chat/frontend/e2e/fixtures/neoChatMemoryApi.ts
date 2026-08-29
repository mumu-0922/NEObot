import type { Route } from "@playwright/test";

import type {
  GovernanceMemory,
  MemoryActivity,
  MemoryGovernanceSnapshot,
  MemoryType,
} from "../../src/lib/memory/types";
import type {
  DurableMemorySettingsDTO,
  MemoryHealthDTO,
} from "../../src/services/api/client/types";

import { json, NOW } from "./neoChatApiSupport";

const NOW_MILLIS = Date.parse(NOW);

export class NeoChatMemoryApiFixture {
  readonly snapshot: MemoryGovernanceSnapshot = {
    settings: {
      enabled: true,
      searchEnabled: true,
      autoRecordEnabled: true,
      sensitiveMemoryEnabled: false,
      l2Mode: "inherit",
      l3Mode: "inherit",
    },
    projects: [],
    conversations: [],
    memories: [],
    reviews: [],
    deletions: [],
    diagnostics: [],
  };
  readonly activities = new Map<string, MemoryActivity[]>();
  readonly health: MemoryHealthDTO = {
    status: "ready",
    reasonCode: "MEMORY_READY",
    workerAvailable: true,
    embeddingWorkerAvailable: true,
    readyCount: 0,
    pendingCount: 0,
    failedCount: 0,
    judgeProviderId: "SUB",
    judgeModelId: "gpt-5.6-luna",
    judgeFixed: true,
    judgeProviderConfigured: true,
    judgeAvailable: true,
  };
  private sequence = 0;

  seedDirectActionActivity(
    assistantMessageId: string,
    content: string,
  ): MemoryActivity {
    const memory = this.createMemory({
      type: "preference",
      content,
      importance: 4,
      tags: ["drink"],
      scopeType: "global",
      sensitivity: "normal",
      source: "direct_user",
      authorityKind: "direct_user",
    });
    const activity: MemoryActivity = {
      id: this.nextUuid("82000000"),
      assistantMessageId,
      ordinal: 1,
      subjectType: "memory",
      subjectId: memory.id,
      subjectRevision: memory.revision,
      action: "created",
      status: "completed",
      reasonCode: "DIRECT_CREATED",
      undoKind: "created",
      undoStatus: "available",
      sourceKind: "direct_action",
      scopeType: memory.scopeType,
      memoryType: memory.type,
      memoryContent: memory.content,
      memoryRevision: memory.revision,
      createdAt: NOW_MILLIS,
      updatedAt: NOW_MILLIS,
    };
    this.activities.set(assistantMessageId, [activity]);
    return activity;
  }

  async handle(route: Route, path: string, method: string): Promise<boolean> {
    if (path === "/v1/memory-governance" && method === "GET") {
      await json(route, this.snapshot);
      return true;
    }

    if (path === "/v1/memory-health" && method === "GET") {
      await json(route, {
        ...this.health,
        readyCount: this.snapshot.memories.length,
      });
      return true;
    }

    if (path === "/v1/memory-settings" && method === "PATCH") {
      const body = route.request().postDataJSON() as Record<string, unknown>;
      const keys = [
        "enabled",
        "searchEnabled",
        "autoRecordEnabled",
        "sensitiveMemoryEnabled",
        "l2Mode",
        "l3Mode",
      ] as const;
      for (const key of keys) {
        if (body[key] !== undefined) {
          Object.assign(this.snapshot.settings, { [key]: body[key] });
        }
      }
      await json(
        route,
        this.snapshot.settings satisfies DurableMemorySettingsDTO,
      );
      return true;
    }

    if (path === "/v1/memory-governance/memories" && method === "POST") {
      const body = route.request().postDataJSON() as Record<string, unknown>;
      const memory = this.createMemory({
        type: memoryType(body.type),
        content: typeof body.content === "string" ? body.content : "",
        importance: typeof body.importance === "number" ? body.importance : 3,
        tags: stringList(body.tags),
        scopeType: "global",
        sensitivity: body.sensitivity === "sensitive" ? "sensitive" : "normal",
      });
      await json(route, memory, 201);
      return true;
    }

    if (path === "/v1/memory-activities" && method === "GET") {
      const assistantMessageId = new URL(
        route.request().url(),
      ).searchParams.get("assistantMessageId");
      await json(route, {
        items: assistantMessageId
          ? (this.activities.get(assistantMessageId) ?? [])
          : [],
      });
      return true;
    }

    const undoMatch = path.match(/^\/v1\/memory-activities\/([^/]+)\/undo$/);
    if (undoMatch && method === "POST") {
      const activityId = decodeURIComponent(undoMatch[1]);
      const body = route.request().postDataJSON() as Record<string, unknown>;
      const activity = this.findActivity(activityId);
      if (!activity || body.expectedRevision !== activity.subjectRevision) {
        await json(
          route,
          {
            error: {
              code: "MEMORY_ACTIVITY_STALE",
              message: "Memory Activity revision is stale",
            },
          },
          409,
        );
        return true;
      }
      activity.undoStatus = "undone";
      activity.updatedAt = NOW_MILLIS + 1;
      const memory = this.snapshot.memories.find(
        (item) => item.id === activity.subjectId,
      );
      if (memory) {
        this.snapshot.memories.splice(
          this.snapshot.memories.indexOf(memory),
          1,
        );
      }
      await json(route, {
        status: "undone",
        resultCode: "UNDO_APPLIED",
        memoryId: activity.subjectId,
        memoryRevision: (activity.memoryRevision ?? 0) + 1,
      });
      return true;
    }

    return false;
  }

  private createMemory(
    input: Pick<
      GovernanceMemory,
      "type" | "content" | "importance" | "tags" | "scopeType" | "sensitivity"
    > &
      Partial<
        Pick<GovernanceMemory, "source" | "authorityKind" | "recallStatus">
      >,
  ): GovernanceMemory {
    const memory: GovernanceMemory = {
      id: this.nextUuid("81000000"),
      type: input.type,
      content: input.content,
      importance: input.importance,
      tags: input.tags,
      source: input.source ?? "manual",
      authorityKind: input.authorityKind ?? "manual",
      enabled: true,
      revision: 1,
      scopeType: input.scopeType,
      lifecycleStatus: "active",
      sensitivity: input.sensitivity,
      recallStatus: input.recallStatus ?? "ready",
      createdAt: NOW_MILLIS,
      updatedAt: NOW_MILLIS,
    };
    this.snapshot.memories.push(memory);
    return memory;
  }

  private findActivity(activityId: string): MemoryActivity | undefined {
    for (const activities of this.activities.values()) {
      const activity = activities.find((item) => item.id === activityId);
      if (activity) return activity;
    }
    return undefined;
  }

  private nextUuid(prefix: string): string {
    this.sequence += 1;
    return `${prefix}-0000-4000-8000-${String(this.sequence).padStart(12, "0")}`;
  }
}

export function memorySearchTranscriptEvents(
  conversationId: string,
  messageId: string,
  memoryContent: string,
) {
  const base = {
    turnId: "turn-memory-e2e",
    conversationId,
    messageId,
    runId: "run-memory-e2e",
    occurredAt: NOW,
  };
  const event = (
    sequence: number,
    type: string,
    payload: Record<string, unknown>,
  ) => ({
    ...base,
    eventId: `event-memory-e2e-${sequence}`,
    sequence,
    type,
    payload,
  });
  const step = (status: "running" | "completed") => ({
    id: "memory-search-e2e",
    kind: "tool",
    status,
    labelKey: "process.tool",
    detail: { toolName: "search_memory", mode: "memory", round: 1 },
    presentation: {
      version: 1,
      card: "search",
      title: "Memory search",
      provider: "neo-chat memory",
      query: "用户饮品偏好",
      count: status === "completed" ? 1 : 0,
      items:
        status === "completed"
          ? [{ label: "preference", detail: memoryContent }]
          : [],
    },
  });
  return [
    event(1, "turn.started", { status: "running", transcriptVersion: 2 }),
    event(2, "tool.called", { processSteps: [step("running")] }),
    event(3, "tool.result", { processSteps: [step("completed")] }),
    event(4, "turn.ended", { status: "completed" }),
  ];
}

function memoryType(value: unknown): MemoryType {
  const values: MemoryType[] = [
    "fact",
    "preference",
    "instruction",
    "project",
    "warning",
    "decision",
    "context",
  ];
  return values.includes(value as MemoryType) ? (value as MemoryType) : "fact";
}

function stringList(value: unknown): string[] {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === "string")
    : [];
}
