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
