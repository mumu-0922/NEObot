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
