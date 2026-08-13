import { afterEach, describe, expect, it, vi } from "vitest";

import { createNeoChatApiClient } from "../services/api/client";

const entry = {
  id: "00000000-0000-4000-8000-000000000001",
  source: "custom",
  avatar: "🤖",
  title: "Writer",
  description: "Writes",
  category: "writing",
  tags: ["writing"],
  systemPrompt: "You are a writer.",
  author: "",
  homepage: "",
  requiredTools: ["search"],
  contentFingerprint: "a".repeat(64),
  revision: 1,
  createdAt: "2026-08-13T00:00:00Z",
  updatedAt: "2026-08-13T00:00:00Z",
};

describe("server assistant library API", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("routes library CRUD with revision authority", async () => {
    const requests: Array<{ url: string; method: string; body?: unknown }> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method ?? "GET";
        const body = init?.body ? JSON.parse(String(init.body)) : undefined;
        requests.push({ url, method, body });
        if (method === "DELETE") return new Response(null, { status: 204 });
        if (url.endsWith("/v1/assistants/library") && method === "GET") {
          return json({ assistants: [entry] });
        }
        return json({ assistant: entry }, method === "POST" ? 201 : 200);
      }),
    );
    const agents = createNeoChatApiClient({
      env: {
        NEXT_PUBLIC_API_MODE: "server",
        NEXT_PUBLIC_API_BASE_URL: "/mm-api",
      },
    }).agents;

    await expect(agents.listLibrary!()).resolves.toEqual([
      { ...entry, updateAvailable: false },
    ]);
    await agents.createCustom!({
      avatar: "🤖",
      title: "Writer",
      description: "Writes",
      category: "writing",
      tags: ["writing"],
      systemPrompt: "You are a writer.",
    });
    await agents.updateCustom!({
      assistantId: entry.id,
      expectedRevision: 1,
      avatar: "🤖",
      title: "Writer 2",
      description: "Writes",
      category: "writing",
      tags: [],
      systemPrompt: "You are a careful writer.",
    });
    await agents.deleteLibraryEntry!({
      assistantId: entry.id,
      revision: 2,
    });
    await agents.copyToCustom!({
      assistantId: entry.id,
      expectedRevision: 2,
    });
    await agents.updateInstalled!({
      assistantId: entry.id,
      expectedRevision: 3,
    });

    expect(requests).toEqual([
      { url: "/mm-api/v1/assistants/library", method: "GET", body: undefined },
      {
        url: "/mm-api/v1/assistants/library",
        method: "POST",
        body: {
          avatar: "🤖",
          title: "Writer",
          description: "Writes",
          category: "writing",
          tags: ["writing"],
          systemPrompt: "You are a writer.",
        },
      },
      {
        url: `/mm-api/v1/assistants/library/${entry.id}`,
        method: "PUT",
        body: {
          expectedRevision: 1,
          avatar: "🤖",
          title: "Writer 2",
          description: "Writes",
          category: "writing",
          tags: [],
          systemPrompt: "You are a careful writer.",
        },
      },
      {
        url: `/mm-api/v1/assistants/library/${entry.id}?revision=2`,
        method: "DELETE",
        body: undefined,
      },
      {
        url: `/mm-api/v1/assistants/library/${entry.id}/copy`,
        method: "POST",
        body: { expectedRevision: 2 },
      },
      {
        url: `/mm-api/v1/assistants/library/${entry.id}/update`,
        method: "POST",
        body: { expectedRevision: 3 },
      },
    ]);
  });

  it("routes paged market search and fingerprint-bound install", async () => {
    const requests: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        requests.push(url);
        if (url.includes("/market?")) {
          return json({
            agents: [],
            categories: [],
            page: 2,
            pageSize: 20,
            totalCount: 41,
            totalPages: 3,
            source: "lobehub-live",
          });
        }
        expect(JSON.parse(String(init?.body))).toEqual({
          fingerprint: "a".repeat(64),
        });
        return json({ assistant: entry }, 201);
      }),
    );
    const agents = createNeoChatApiClient({
      env: {
        NEXT_PUBLIC_API_MODE: "server",
        NEXT_PUBLIC_API_BASE_URL: "/mm-api",
      },
    }).agents;

    await agents.searchMarket!({
      query: "writer",
      category: "copywriting",
      locale: "zh",
      page: 2,
      pageSize: 20,
    });
    await agents.installMarket!({
      identifier: "writer",
      fingerprint: "a".repeat(64),
    });

    expect(requests[0]).toContain(
      "/mm-api/v1/assistants/market?page=2&pageSize=20&q=writer&category=copywriting&locale=zh",
    );
    expect(requests[1]).toBe(
      "/mm-api/v1/assistants/market/items/writer/install",
    );
  });

  it("rejects malformed nested market entries", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        json({
          agents: [{ identifier: "broken", meta: null }],
          categories: [],
          page: 1,
          pageSize: 20,
          totalCount: 1,
          totalPages: 1,
          source: "lobehub-live",
        }),
      ),
    );
    const agents = createNeoChatApiClient({
      env: {
        NEXT_PUBLIC_API_MODE: "server",
        NEXT_PUBLIC_API_BASE_URL: "/mm-api",
      },
    }).agents;

    await expect(
      agents.searchMarket!({ page: 1, pageSize: 20 }),
    ).rejects.toMatchObject({ code: "INVALID_SERVER_RESPONSE" });
  });
});

function json(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), {
    status,
    headers: { "content-type": "application/json" },
  });
}
