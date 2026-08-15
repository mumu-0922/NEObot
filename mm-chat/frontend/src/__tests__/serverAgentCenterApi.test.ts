import { afterEach, describe, expect, it, vi } from "vitest";

import { createNeoChatApiClient } from "../services/api/client";

const fingerprint = `sha256:${"a".repeat(64)}`;

describe("server Agent Center API", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("routes authenticated status and Run reads through the narrow facade", async () => {
    const requests: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        requests.push(url);
        if (url.endsWith("/status")) return jsonResponse(statusFixture());
        if (url.includes("/runs?")) {
          return jsonResponse({ runs: [runFixture()] });
        }
        if (url.endsWith("/runs/run_1234567890abcdef")) {
          return jsonResponse({
            run: runFixture(),
            steps: [],
            attempts: [],
            events: [],
            approvals: [],
            children: [],
            artifacts: [],
          });
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

    await expect(client.agentCenter.getStatus()).resolves.toMatchObject({
      runtime: { executable: false, reasonCode: "ISOLATION_UNAVAILABLE" },
    });
    await expect(client.agentCenter.listRuns()).resolves.toHaveLength(1);
    await expect(
      client.agentCenter.getRun("run_1234567890abcdef"),
    ).resolves.toMatchObject({ run: { state: "queued" } });
    expect(client.capabilities.agentCenter).toBe(true);
    expect(requests).toEqual([
      "/mm-api/v1/agent-center/status",
      "/mm-api/v1/agent-center/runs?limit=50",
      "/mm-api/v1/agent-center/runs/run_1234567890abcdef",
    ]);
  });

  it("fails closed on malformed server DTOs", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse({ runs: [{ id: "leak", body: "raw" }] })),
    );
    const client = createNeoChatApiClient({
      env: {
        NEXT_PUBLIC_API_MODE: "server",
        NEXT_PUBLIC_API_BASE_URL: "/mm-api",
      },
    });
    await expect(client.agentCenter.listRuns()).rejects.toMatchObject({
      code: "INVALID_SERVER_RESPONSE",
    });
  });

  it("submits only the current product-canary policy and generation", async () => {
    let captured: RequestInit | undefined;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
        captured = init;
        return jsonResponse({
          request: {
            id: "product_request_1234567890abcdef",
            activationId: "activation_1234567890abcdef",
            state: "queued",
            policyRevision: 4,
            optGeneration: 2,
            requestFingerprint: fingerprint,
            failureCount: 0,
            createdAt: "2026-08-15T00:00:00Z",
            updatedAt: "2026-08-15T00:00:00Z",
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
      client.agentCenter.enqueueRun({
        expectedPolicyRevision: 4,
        expectedGeneration: 2,
      }),
    ).resolves.toMatchObject({ state: "queued" });
    expect(JSON.parse(String(captured?.body))).toEqual({
      expectedPolicyRevision: 4,
      expectedGeneration: 2,
    });
  });
});

function statusFixture() {
  return {
    isAdministrator: false,
    runtime: {
      state: "held",
      reasonCode: "ISOLATION_UNAVAILABLE",
      executable: false,
      productCanary: false,
      scheduler: false,
      learningWorker: false,
    },
    shadow: {
      policy: {
        revision: 0,
        enabled: false,
        mode: "synthetic",
        cohortBasisPoints: 0,
        maxObservations: 1,
        maxErrors: 0,
        updatedAt: "1970-01-01T00:00:00Z",
      },
      optIn: {
        optedIn: false,
        generation: 0,
        policyRevision: 0,
        updatedAt: "1970-01-01T00:00:00Z",
      },
      cohortSelected: false,
      eligible: false,
      effective: false,
      heldReasonCode: "SHADOW_DISABLED",
      observationCount: 0,
      errorCount: 0,
      productCanary: { remainingRequests: 0 },
    },
  };
}

function runFixture() {
  return {
    id: "run_1234567890abcdef",
    state: "queued",
    snapshotFingerprint: fingerprint,
    requestFingerprint: fingerprint,
    createdAt: "2026-08-14T00:00:00Z",
    updatedAt: "2026-08-14T00:00:00Z",
  };
}

function jsonResponse(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}
