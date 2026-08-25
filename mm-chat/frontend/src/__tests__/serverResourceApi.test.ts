import { afterEach, describe, expect, it, vi } from "vitest";

import { createNeoChatApiClient } from "../services/api/client";

const fingerprint = `sha256:${"a".repeat(64)}`;

describe("server Resource API", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("searches and installs through the unified Resource contract", async () => {
    const requests: Array<{ url: string; method: string; body?: string }> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        requests.push({
          url,
          method: init?.method ?? "GET",
          body: typeof init?.body === "string" ? init.body : undefined,
        });
        if (url.includes("/v1/resources/search?")) {
          return jsonResponse({
            kind: "skill",
            query: "excel",
            items: [
              {
                kind: "skill",
                id: "candidate-id",
                name: "office-xlsx",
                description: "Create Excel workbooks",
                version: "1.0.0",
                exactRevision: fingerprint,
                status: "admitted",
                source: "official",
                permissionScopes: ["write"],
              },
            ],
          });
        }
        return jsonResponse({
          kind: "skill",
          id: "installation-id",
          name: "office-xlsx",
          revision: fingerprint,
          status: "installed",
          refreshRequired: true,
        });
      }),
    );
    const client = createNeoChatApiClient({
      env: {
        NEXT_PUBLIC_API_MODE: "server",
        NEXT_PUBLIC_API_BASE_URL: "/mm-api",
      },
    });

    const search = await client.resources.search({
      kind: "skill",
      query: "excel",
    });
    await expect(
      client.resources.install({
        kind: "skill",
        id: search.items[0].id,
        version: search.items[0].version,
        exactRevision: search.items[0].exactRevision,
        conversationId: "conversation-id",
      }),
    ).resolves.toMatchObject({ name: "office-xlsx", refreshRequired: true });

    expect(requests.map(({ url, method }) => `${method} ${url}`)).toEqual([
      "GET /mm-api/v1/resources/search?kind=skill&q=excel",
      "POST /mm-api/v1/resources/install",
    ]);
    expect(JSON.parse(requests[1].body ?? "{}")).toEqual({
      kind: "skill",
      id: "candidate-id",
      version: "1.0.0",
      exactRevision: fingerprint,
      conversationId: "conversation-id",
    });
  });

  it("fails closed on malformed install responses", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse({ status: "ok" })),
    );
    const client = createNeoChatApiClient({
      env: {
        NEXT_PUBLIC_API_MODE: "server",
        NEXT_PUBLIC_API_BASE_URL: "/mm-api",
      },
    });
    await expect(
      client.resources.install({
        kind: "skill",
        id: "candidate-id",
        exactRevision: fingerprint,
        conversationId: "conversation-id",
      }),
    ).rejects.toMatchObject({ code: "INVALID_SERVER_RESPONSE" });
  });
});

function jsonResponse(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}
