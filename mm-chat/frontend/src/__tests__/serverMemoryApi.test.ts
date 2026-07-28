import { afterEach, describe, expect, it, vi } from "vitest";
import { createNeoChatApiClient } from "../services/api/client";

describe("server durable memory API", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("persists settings and memory CRUD through Go routes", async () => {
    const requests: Array<{
      url: string;
      method: string;
      body?: unknown;
    }> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method ?? "GET";
        const body = init?.body ? JSON.parse(String(init.body)) : undefined;
        requests.push({ url, method, body });
        if (url.endsWith("/v1/memory-settings")) {
          return jsonResponse({
            enabled: method === "PATCH",
            searchEnabled: true,
            autoRecordEnabled: method === "PATCH",
            sensitiveMemoryEnabled: false,
            l2Mode: "inherit",
            l3Mode: "inherit",
          });
        }
        if (url.endsWith("/v1/memories") && method === "GET") {
          return jsonResponse({ items: [memoryRecord("memory-1", "manual")] });
        }
        if (url.endsWith("/v1/memories") && method === "POST") {
          return jsonResponse(memoryRecord("memory-2", "manual"), 201);
        }
        if (url.endsWith("/v1/memories/memory-2") && method === "PATCH") {
          return jsonResponse({
            ...memoryRecord("memory-2", "manual"),
            content: "Keep every answer concise",
          });
        }
        if (url.endsWith("/v1/memories/memory-2") && method === "DELETE") {
          return new Response(null, { status: 204 });
        }
        return jsonResponse(
          { error: { code: "NOT_FOUND", message: "missing" } },
          404,
        );
      }),
    );

    const client = createNeoChatApiClient({
      env: {
        NEXT_PUBLIC_API_MODE: "server",
        NEXT_PUBLIC_API_BASE_URL: "/mm-api",
      },
    });
    expect(client.capabilities.memories).toBe(true);
    await expect(client.memories.getSettings()).resolves.toMatchObject({
      enabled: false,
    });
    await expect(
      client.memories.updateSettings({
        enabled: true,
        autoRecordEnabled: true,
      }),
    ).resolves.toMatchObject({ enabled: true, autoRecordEnabled: true });
    await expect(client.memories.listMemories()).resolves.toHaveLength(1);
    await expect(
      client.memories.createMemory({
        type: "preference",
        content: "Keep answers concise",
        importance: 4,
        tags: ["style"],
      }),
    ).resolves.toMatchObject({ id: "memory-2", source: "manual" });
    await expect(
      client.memories.updateMemory({
        memoryId: "memory-2",
        type: "preference",
        content: "Keep every answer concise",
        importance: 5,
        tags: ["style"],
      }),
    ).resolves.toMatchObject({ content: "Keep every answer concise" });
    await client.memories.deleteMemory({ memoryId: "memory-2" });

    expect(requests).toEqual([
      { url: "/mm-api/v1/memory-settings", method: "GET", body: undefined },
      {
        url: "/mm-api/v1/memory-settings",
        method: "PATCH",
        body: { enabled: true, autoRecordEnabled: true },
      },
      { url: "/mm-api/v1/memories", method: "GET", body: undefined },
      {
        url: "/mm-api/v1/memories",
        method: "POST",
        body: {
          type: "preference",
          content: "Keep answers concise",
          importance: 4,
          tags: ["style"],
        },
      },
      {
        url: "/mm-api/v1/memories/memory-2",
        method: "PATCH",
        body: {
          type: "preference",
          content: "Keep every answer concise",
          importance: 5,
          tags: ["style"],
        },
      },
      {
        url: "/mm-api/v1/memories/memory-2",
        method: "DELETE",
        body: undefined,
      },
    ]);
  });

  it("routes Project, policy, scoped Memory, Review, and Activity governance", async () => {
    const requests: Array<{ url: string; method: string; body?: unknown }> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method ?? "GET";
        const body = init?.body ? JSON.parse(String(init.body)) : undefined;
        requests.push({ url, method, body });
        if (url.endsWith("/v1/memory-governance")) {
          return jsonResponse(governanceSnapshot());
        }
        if (url.endsWith("/v1/projects")) {
          return method === "POST"
            ? jsonResponse(memoryProject(), 201)
            : jsonResponse({ items: [memoryProject()] });
        }
        if (url.endsWith("/v1/projects/project-1")) {
          return jsonResponse({
            ...memoryProject(),
            lifecycleStatus: "archived",
          });
        }
        if (url.includes("/memory-policy")) {
          return jsonResponse(conversationPolicy());
        }
        if (url.endsWith("/v1/memory-governance/memories")) {
          return jsonResponse(governanceMemory(), 201);
        }
        if (url.endsWith("/v1/memory-governance/memories/memory-1")) {
          return jsonResponse(
            method === "DELETE"
              ? {
                  manifestId: "manifest-1",
                  memoryId: "memory-1",
                  immediateHidden: true,
                  onlinePurgeStatus: "pending",
                  backupExpiryStatus: "retention_pending",
                  backupExpiresAt: 1,
                  deletedAt: 1,
                }
              : governanceMemory(),
          );
        }
        if (url.includes("/v1/memory-reviews/review-1/decision")) {
          return jsonResponse({
            suggestionId: "review-1",
            decision: "reject",
            status: "rejected",
            resultCode: "USER_REJECTED",
          });
        }
        if (url.includes("/v1/memory-activities?")) {
          return jsonResponse({ items: [memoryActivity()] });
        }
        if (url.endsWith("/v1/memory-activities/activity-1/undo")) {
          return jsonResponse({
            status: "undone",
            resultCode: "UNDO_APPLIED",
            memoryId: "memory-1",
            memoryRevision: 2,
          });
        }
        return jsonResponse({ items: [] });
      }),
    );

    const memories = createNeoChatApiClient({
      env: {
        NEXT_PUBLIC_API_MODE: "server",
        NEXT_PUBLIC_API_BASE_URL: "/mm-api",
      },
    }).memories;
    await expect(memories.getGovernance()).resolves.toMatchObject({
      projects: [{ id: "project-1" }],
    });
    await expect(memories.listProjects()).resolves.toHaveLength(1);
    await memories.createProject({ name: "Neo Chat", description: "Memory" });
    await memories.updateProject({
      projectId: "project-1",
      expectedRevision: 1,
      name: "Neo Chat",
      description: "Memory",
      lifecycleStatus: "archived",
    });
    await memories.updateConversationPolicy({
      conversationId: "conversation-1",
      expectedScopeGeneration: 1,
      projectId: "project-1",
      useMode: "off",
      learnMode: "on",
    });
    await memories.createGovernanceMemory({
      type: "project",
      content: "Neo Chat uses Go",
      scopeType: "project",
      projectId: "project-1",
      sensitivity: "normal",
    });
    await memories.updateGovernanceMemory({
      memoryId: "memory-1",
      expectedRevision: 1,
      type: "project",
      content: "Neo Chat uses Go and PostgreSQL",
      scopeType: "project",
      projectId: "project-1",
      sensitivity: "normal",
    });
    await memories.deleteGovernanceMemory({
      memoryId: "memory-1",
      expectedRevision: 2,
    });
    await memories.decideMemoryReview({
      suggestionId: "review-1",
      decision: "reject",
    });
    await expect(
      memories.listMessageMemoryActivities({
        assistantMessageId: "assistant-1",
      }),
    ).resolves.toHaveLength(1);
    await memories.undoMemoryActivity({
      activityId: "activity-1",
      expectedRevision: 1,
    });

    expect(requests).toContainEqual({
      url: "/mm-api/v1/chat/conversations/conversation-1/memory-policy",
      method: "PATCH",
      body: {
        expectedScopeGeneration: 1,
        projectId: "project-1",
        useMode: "off",
        learnMode: "on",
      },
    });
    expect(requests).toContainEqual({
      url: "/mm-api/v1/memory-governance/memories/memory-1",
      method: "PATCH",
      body: {
        type: "project",
        content: "Neo Chat uses Go and PostgreSQL",
        importance: 3,
        tags: [],
        expectedRevision: 1,
        scopeType: "project",
        projectId: "project-1",
        conversationId: "",
        sensitivity: "normal",
      },
    });
  });

  it("keeps local mode on the browser store rather than a fake server adapter", async () => {
    const client = createNeoChatApiClient({
      env: {
        NEXT_PUBLIC_API_MODE: "local",
        NEXT_PUBLIC_API_BASE_URL: "",
      },
    });
    expect(client.capabilities.memories).toBe(false);
    await expect(client.memories.listMemories()).rejects.toMatchObject({
      code: "FEATURE_NOT_IMPLEMENTED",
    });
  });

  it("exports authenticated packages and keeps import dry-run/confirm multipart-bound", async () => {
    const requests: Array<{ url: string; init?: RequestInit }> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        requests.push({ url, init });
        if (url.endsWith("/v1/memory-export")) {
          return new Response(new Uint8Array([1, 2, 3]), {
            status: 200,
            headers: { "Content-Type": "application/octet-stream" },
          });
        }
        if (url.endsWith("/v1/memory-import/dry-run")) {
          return jsonResponse(memoryImportDryRun());
        }
        if (url.endsWith("/v1/memory-import/confirm")) {
          return jsonResponse({
            importId: "10000000-0000-4000-8000-000000000001",
            status: "completed",
            addedProjects: 1,
            addedMemories: 2,
            importedAt: 1_700_000_000_000,
          });
        }
        return jsonResponse(
          { error: { code: "NOT_FOUND", message: "missing" } },
          404,
        );
      }),
    );

    const memories = createNeoChatApiClient({
      env: {
        NEXT_PUBLIC_API_MODE: "server",
        NEXT_PUBLIC_API_BASE_URL: "/mm-api",
      },
    }).memories;
    const blob = await memories.exportMemoryPackage({
      passphrase: "fixture-passphrase",
      includeHistory: true,
    });
    expect(blob.size).toBe(3);
    const packageFile = new File(
      [new Uint8Array([4, 5, 6])],
      "fixture.mm-memory",
      {
        type: "application/octet-stream",
      },
    );
    const mappings = {
      projects: { "project-000001": { mode: "create" as const } },
      conversations: {},
    };
    const dryRun = await memories.dryRunMemoryImport({
      packageFile,
      passphrase: "fixture-passphrase",
      mappings,
    });
    expect(dryRun.counts).toEqual({
      NOOP: 0,
      ADD: 2,
      REVIEW: 0,
      REJECT: 0,
      SCOPE_REQUIRED: 1,
    });
    await expect(
      memories.confirmMemoryImport({
        packageFile,
        passphrase: "fixture-passphrase",
        mappings,
        planToken: dryRun.planToken,
      }),
    ).resolves.toMatchObject({ status: "completed", addedMemories: 2 });

    expect(JSON.parse(String(requests[0].init?.body))).toEqual({
      passphrase: "fixture-passphrase",
      includeHistory: true,
    });
    for (const request of requests.slice(1)) {
      expect(request.init?.body).toBeInstanceOf(FormData);
      const formData = request.init?.body as FormData;
      expect(formData.get("package")).toBeInstanceOf(File);
      expect(formData.get("passphrase")).toBe("fixture-passphrase");
      expect(formData.get("mappings")).toBe(JSON.stringify(mappings));
    }
    expect(
      (requests[1].init?.headers as Record<string, string>)["Content-Type"],
    ).toBeUndefined();
    expect((requests[2].init?.body as FormData).get("planToken")).toBe(
      "plan-token",
    );
  });

  it("rejects malformed memory import plans at the server boundary", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({ ...memoryImportDryRun(), packageSha256: "not-a-hash" }),
      ),
    );
    const memories = createNeoChatApiClient({
      env: {
        NEXT_PUBLIC_API_MODE: "server",
        NEXT_PUBLIC_API_BASE_URL: "/mm-api",
      },
    }).memories;
    await expect(
      memories.dryRunMemoryImport({
        packageFile: new File(["ciphertext"], "fixture.mm-memory"),
        passphrase: "fixture-passphrase",
        mappings: { projects: {}, conversations: {} },
      }),
    ).rejects.toThrow("invalid memory import package hash");
  });
});

function memoryRecord(id: string, source: "manual" | "ai") {
  return {
    id,
    type: "preference",
    content: "Keep answers concise",
    createdAt: 1_700_000_000_000,
    updatedAt: 1_700_000_000_000,
    importance: 4,
    tags: ["style"],
    source,
  };
}

function jsonResponse(payload: unknown, status = 200) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function memoryProject() {
  return {
    id: "project-1",
    name: "Neo Chat",
    description: "Memory",
    lifecycleStatus: "active",
    revision: 1,
    scopeGeneration: 1,
    conversationCount: 1,
    memoryCount: 1,
    createdAt: 1,
    updatedAt: 1,
  };
}

function conversationPolicy() {
  return {
    conversationId: "conversation-1",
    title: "Memory",
    projectId: "project-1",
    projectName: "Neo Chat",
    projectStatus: "active",
    useMode: "off",
    learnMode: "on",
    effectiveUse: false,
    effectiveLearn: true,
    learnForcedOff: false,
    scopeGeneration: 2,
    updatedAt: 1,
  };
}

function governanceMemory() {
  return {
    id: "memory-1",
    type: "project",
    content: "Neo Chat uses Go",
    importance: 3,
    tags: [],
    source: "manual",
    authorityKind: "manual",
    enabled: true,
    revision: 1,
    scopeType: "project",
    projectId: "project-1",
    lifecycleStatus: "active",
    sensitivity: "normal",
    recallStatus: "shadow_only",
    createdAt: 1,
    updatedAt: 1,
  };
}

function governanceSnapshot() {
  return {
    settings: {
      enabled: true,
      searchEnabled: true,
      autoRecordEnabled: false,
      sensitiveMemoryEnabled: false,
      l2Mode: "inherit",
      l3Mode: "inherit",
    },
    projects: [memoryProject()],
    conversations: [conversationPolicy()],
    memories: [governanceMemory()],
    reviews: [],
    deletions: [],
    diagnostics: [],
  };
}

function memoryActivity() {
  return {
    id: "activity-1",
    assistantMessageId: "assistant-1",
    ordinal: 1,
    subjectType: "memory",
    subjectId: "memory-1",
    subjectRevision: 1,
    action: "created",
    status: "completed",
    reasonCode: "DIRECT_CREATED",
    undoKind: "created",
    undoStatus: "available",
    sourceKind: "memory_job",
    scopeType: "project",
    memoryType: "project",
    memoryContent: "Neo Chat uses Go",
    memoryRevision: 1,
    createdAt: 1,
    updatedAt: 1,
  };
}

function memoryImportDryRun() {
  return {
    importId: "10000000-0000-4000-8000-000000000001",
    packageSha256: "a".repeat(64),
    manifestSha256: "b".repeat(64),
    planSha256: "c".repeat(64),
    planToken: "plan-token",
    expiresAt: 1_700_000_600_000,
    counts: {
      NOOP: 0,
      ADD: 2,
      REVIEW: 0,
      REJECT: 0,
      SCOPE_REQUIRED: 1,
    },
    items: [
      {
        ordinal: 1,
        memoryRef: "memory-000001",
        recordHash: "d".repeat(64),
        result: "ADD",
        reasonCode: "NEW_MEMORY",
      },
    ],
    scopeRequirements: [
      {
        kind: "project",
        portableRef: "project-000001",
        name: "Imported Project",
      },
    ],
    settingsSuggestion: {
      enabled: true,
      searchEnabled: true,
      autoRecordEnabled: false,
      sensitiveMemoryEnabled: false,
      l2Mode: "inherit",
      l3Mode: "inherit",
    },
  };
}
