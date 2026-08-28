import { afterEach, describe, expect, it, vi } from "vitest";

import { createNeoChatApiClient } from "../services/api/client";

const fingerprint = `sha256:${"a".repeat(64)}`;
const commit = "b".repeat(40);

describe("server Skill Store API", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("lists the curated catalog, installs, and uninstalls only through /v1/skills", async () => {
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
        if (url.endsWith("/v1/skills/catalog")) {
          return jsonResponse({
            items: [catalogSummaryFixture()],
            totalCount: 1,
            source: "openai/skills curated",
          });
        }
        if (url.endsWith("/v1/skills/catalog/items/demo-skill")) {
          return jsonResponse({ skill: catalogItemFixture() });
        }
        if (url.endsWith("/v1/skills/library")) {
          return jsonResponse({ skills: [installationFixture()] });
        }
        if (url.endsWith("/install")) {
          return jsonResponse({ skill: installationFixture() });
        }
        if (init?.method === "DELETE")
          return new Response(null, { status: 204 });
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

    await expect(client.skillStore.listCatalog()).resolves.toMatchObject({
      totalCount: 1,
    });
    await expect(
      client.skillStore.getCatalogSkill("demo-skill"),
    ).resolves.toMatchObject({
      name: "demo-skill",
    });
    await expect(client.skillStore.listPackageLibrary()).resolves.toHaveLength(
      1,
    );
    await expect(
      client.skillStore.installCatalogSkill({
        id: "demo-skill",
        resolvedCommit: commit,
        packageFingerprint: fingerprint,
      }),
    ).resolves.toMatchObject({ name: "demo" });
    await expect(
      client.skillStore.uninstallPackageSkill({
        installationId: "installation_1234567890abcdef",
        revision: 1,
      }),
    ).resolves.toBeUndefined();

    expect(client.capabilities.skillStore).toBe(true);
    expect(requests.map(({ url }) => url)).toEqual([
      "/mm-api/v1/skills/catalog",
      "/mm-api/v1/skills/catalog/items/demo-skill",
      "/mm-api/v1/skills/library",
      "/mm-api/v1/skills/catalog/items/demo-skill/install",
      "/mm-api/v1/skills/library/installation_1234567890abcdef?revision=1",
    ]);
    expect(requests.every(({ url }) => !url.includes("/agent-center"))).toBe(
      true,
    );
  });

  it("fails closed on malformed server DTOs", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse({ items: [{ id: "leak" }] })),
    );
    const client = createNeoChatApiClient({
      env: {
        NEXT_PUBLIC_API_MODE: "server",
        NEXT_PUBLIC_API_BASE_URL: "/mm-api",
      },
    });
    await expect(client.skillStore.listCatalog()).rejects.toMatchObject({
      code: "INVALID_SERVER_RESPONSE",
    });
  });

  it("searches LobeHub, pins detail versions, and installs marketplace or exact links", async () => {
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
        if (url.includes("/v1/skills/marketplace?")) {
          return jsonResponse({
            items: [marketplaceSummaryFixture()],
            categories: [{ category: "productivity", count: 1 }],
            page: 1,
            pageSize: 20,
            totalCount: 1,
            totalPages: 1,
            source: "lobehub",
            sourceUrl: "https://lobehub.com/skills",
          });
        }
        if (url.includes("/v1/skills/marketplace/items/owner-demo?")) {
          return jsonResponse({ skill: marketplaceDetailFixture() });
        }
        return jsonResponse({ skill: installationFixture() }, 201);
      }),
    );
    const client = createNeoChatApiClient({
      env: {
        NEXT_PUBLIC_API_MODE: "server",
        NEXT_PUBLIC_API_BASE_URL: "/mm-api",
      },
    });

    await expect(
      client.skillStore.searchMarketplace({
        query: "demo",
        category: "productivity",
        locale: "en-US",
        sort: "installCount",
      }),
    ).resolves.toMatchObject({ source: "lobehub", totalCount: 1 });
    await expect(
      client.skillStore.getMarketplaceSkill("owner-demo", {
        version: "1.2.3",
        locale: "ja-JP",
      }),
    ).resolves.toMatchObject({ manifestName: "demo-skill" });
    await expect(
      client.skillStore.installMarketplaceSkill({
        identifier: "owner-demo",
        version: "1.2.3",
      }),
    ).resolves.toMatchObject({ name: "demo" });
    await expect(
      client.skillStore.installSkillLink({
        url: "https://lobehub.com/skills/owner-demo",
      }),
    ).resolves.toMatchObject({ name: "demo" });

    expect(requests).toEqual([
      {
        url: "/mm-api/v1/skills/marketplace?page=1&pageSize=20&q=demo&category=productivity&locale=en-US&sort=installCount",
        method: "GET",
        body: undefined,
      },
      {
        url: "/mm-api/v1/skills/marketplace/items/owner-demo?version=1.2.3&locale=ja-JP",
        method: "GET",
        body: undefined,
      },
      {
        url: "/mm-api/v1/skills/marketplace/items/owner-demo/install",
        method: "POST",
        body: JSON.stringify({ version: "1.2.3" }),
      },
      {
        url: "/mm-api/v1/skills/direct/install",
        method: "POST",
        body: JSON.stringify({
          url: "https://lobehub.com/skills/owner-demo",
        }),
      },
    ]);
  });

  it("rejects malformed LobeHub marketplace identity", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          skill: {
            ...marketplaceDetailFixture(),
            identifier: "owner/demo",
          },
        }),
      ),
    );
    const client = createNeoChatApiClient({
      env: {
        NEXT_PUBLIC_API_MODE: "server",
        NEXT_PUBLIC_API_BASE_URL: "/mm-api",
      },
    });
    await expect(
      client.skillStore.getMarketplaceSkill("owner-demo"),
    ).rejects.toMatchObject({ code: "INVALID_SERVER_RESPONSE" });
  });

  it("rejects a catalog that claims another source", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          items: [catalogSummaryFixture()],
          totalCount: 1,
          source: "untrusted marketplace",
        }),
      ),
    );
    const client = createNeoChatApiClient({
      env: {
        NEXT_PUBLIC_API_MODE: "server",
        NEXT_PUBLIC_API_BASE_URL: "/mm-api",
      },
    });

    await expect(client.skillStore.listCatalog()).rejects.toMatchObject({
      code: "INVALID_SERVER_RESPONSE",
    });
  });

  it("gets and replaces a revision-bound conversation selection", async () => {
    const requests: Array<{ url: string; method: string; body?: string }> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        requests.push({
          url: String(input),
          method: init?.method ?? "GET",
          body: typeof init?.body === "string" ? init.body : undefined,
        });
        return jsonResponse({
          selection: {
            conversationId: "conversation/with slash",
            revision: init?.method === "PUT" ? 2 : 1,
            skills: [installationFixture()],
          },
        });
      }),
    );
    const client = createNeoChatApiClient({
      env: {
        NEXT_PUBLIC_API_MODE: "server",
        NEXT_PUBLIC_API_BASE_URL: "/mm-api",
      },
    });

    await expect(
      client.skillStore.getConversationSelection("conversation/with slash"),
    ).resolves.toMatchObject({ revision: 1 });
    await expect(
      client.skillStore.replaceConversationSelection({
        conversationId: "conversation/with slash",
        revision: 1,
        installationIds: ["installation_1234567890abcdef"],
      }),
    ).resolves.toMatchObject({ revision: 2 });

    expect(requests).toEqual([
      {
        url: "/mm-api/v1/skills/conversations/conversation%2Fwith%20slash/selection",
        method: "GET",
        body: undefined,
      },
      {
        url: "/mm-api/v1/skills/conversations/conversation%2Fwith%20slash/selection",
        method: "PUT",
        body: JSON.stringify({
          revision: 1,
          installationIds: ["installation_1234567890abcdef"],
        }),
      },
    ]);
  });

  it("rejects a selection response bound to another conversation", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          selection: {
            conversationId: "conversation-b",
            revision: 0,
            skills: [],
          },
        }),
      ),
    );
    const client = createNeoChatApiClient({
      env: {
        NEXT_PUBLIC_API_MODE: "server",
        NEXT_PUBLIC_API_BASE_URL: "/mm-api",
      },
    });

    await expect(
      client.skillStore.getConversationSelection("conversation-a"),
    ).rejects.toMatchObject({ code: "INVALID_SERVER_RESPONSE" });
  });
});

function catalogSummaryFixture() {
  return {
    id: "demo-skill",
    name: "demo-skill",
    repository: "openai/skills",
    ref: "main",
    path: "skills/.curated/demo-skill",
    sourceUrl:
      "https://github.com/openai/skills/tree/main/skills/.curated/demo-skill",
    catalogSource: "openai/skills curated",
  };
}

function catalogItemFixture() {
  return {
    ...catalogSummaryFixture(),
    resolvedCommit: commit,
    packageFingerprint: fingerprint,
    version: "1.0.0",
    description: "Demo Skill",
    license: "MIT",
    compatibility: "Neo Chat",
    allowedTools: [],
    hasRuntime: false,
  };
}

function installationFixture() {
  return {
    id: "installation_1234567890abcdef",
    admissionId: "candidate_1234567890abcdef",
    packageFingerprint: fingerprint,
    name: "demo",
    version: "1.0.0",
    description: "Demo Skill",
    allowedTools: [],
    revision: 1,
    createdAt: "2026-08-17T00:00:00Z",
    updatedAt: "2026-08-17T00:00:00Z",
  };
}

function marketplaceSummaryFixture() {
  return {
    identifier: "owner-demo",
    name: "Demo Skill",
    description: "Marketplace demo",
    version: "1.2.3",
    category: "productivity",
    author: "Owner",
    repositoryUrl: "https://github.com/owner/demo",
    installCount: 7,
    rating: 4.5,
    official: false,
    validated: true,
    featured: true,
    resourceCount: 1,
  };
}

function marketplaceDetailFixture() {
  return {
    ...marketplaceSummaryFixture(),
    manifestName: "demo-skill",
    summary: "Demo summary",
    permissions: ["Read"],
    resources: [{ path: "references/demo.txt", sha256: "sha256:abc", size: 3 }],
    versions: [
      {
        version: "1.2.3",
        latest: true,
        validated: true,
        createdAt: "2026-08-28T00:00:00Z",
        versionRank: 1,
      },
    ],
    source: "lobehub",
    sourceUrl: "https://lobehub.com/skills/owner-demo",
    installed: false,
  };
}

function jsonResponse(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}
