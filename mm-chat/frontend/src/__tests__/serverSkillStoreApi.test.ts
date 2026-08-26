import { afterEach, describe, expect, it, vi } from "vitest";

import { createNeoChatApiClient } from "../services/api/client";

const fingerprint = `sha256:${"a".repeat(64)}`;

describe("server Skill Store API", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("lists, installs, and uninstalls only through /v1/skills", async () => {
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
        if (url.includes("/v1/skills/store?")) {
          return jsonResponse({
            items: [candidateFixture()],
            page: 1,
            pageSize: 20,
            totalCount: 1,
            totalPages: 1,
          });
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

    await expect(client.skillStore.listPackageStore()).resolves.toMatchObject({
      totalCount: 1,
    });
    await expect(client.skillStore.listPackageLibrary()).resolves.toHaveLength(
      1,
    );
    await expect(
      client.skillStore.installPackageSkill({
        candidateId: "candidate_1234567890abcdef",
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
      "/mm-api/v1/skills/store?page=1&pageSize=20",
      "/mm-api/v1/skills/library",
      "/mm-api/v1/skills/store/items/candidate_1234567890abcdef/install",
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
    await expect(client.skillStore.listPackageStore()).rejects.toMatchObject({
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

function candidateFixture() {
  return {
    id: "candidate_1234567890abcdef",
    sourceType: "official",
    sourceRef: "skills/demo",
    sourceArtifactSha256: fingerprint,
    package: {
      packageFingerprint: fingerprint,
      sbomFingerprint: fingerprint,
      name: "demo",
      version: "1.0.0",
      description: "Demo Skill",
      allowedTools: [],
      capabilityRequests: [],
      hasRuntime: true,
      fileCount: 1,
      packageBytes: 10,
      expandedBytes: 20,
      createdAt: "2026-08-17T00:00:00Z",
    },
    status: "admitted",
    admissionEligible: true,
    validationSummary: "valid",
    revision: 1,
    createdAt: "2026-08-17T00:00:00Z",
    updatedAt: "2026-08-17T00:00:00Z",
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

function jsonResponse(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}
