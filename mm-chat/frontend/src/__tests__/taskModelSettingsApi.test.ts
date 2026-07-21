import { afterEach, describe, expect, it, vi } from "vitest";
import { createNeoChatApiClient } from "../services/api/client";

const models = {
  titleGeneration: "CUSTOM:gpt-title",
  relatedQuestions: "CUSTOM:gpt-related",
  contextCompression: "CUSTOM:gpt-compress",
  promptOptimization: "CUSTOM:gpt-polish",
  ragQuery: "CUSTOM:gpt-rag",
  memory: "CUSTOM:gpt-memory",
};

describe("server task model settings API", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("loads and patches the Go-owned task model settings", async () => {
    const requests: Array<{ url: string; method: string; body?: unknown }> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        requests.push({
          url: String(input),
          method: init?.method ?? "GET",
          body: init?.body ? JSON.parse(String(init.body)) : undefined,
        });
        return Response.json({
          models,
          configured: true,
          updatedAt: "2026-07-21T05:00:00Z",
        });
      }),
    );
    const client = createNeoChatApiClient({
      env: {
        NEXT_PUBLIC_API_MODE: "server",
        NEXT_PUBLIC_API_BASE_URL: "/mm-api",
      },
    });

    await expect(client.settings.getTaskModels()).resolves.toMatchObject({
      configured: true,
      models,
    });
    await expect(
      client.settings.updateTaskModels({ memory: "CUSTOM:gpt-memory" }),
    ).resolves.toMatchObject({ models });

    expect(requests).toEqual([
      {
        url: "/mm-api/v1/admin/task-models",
        method: "GET",
        body: undefined,
      },
      {
        url: "/mm-api/v1/admin/task-models",
        method: "PATCH",
        body: { memory: "CUSTOM:gpt-memory" },
      },
    ]);
  });

  it("does not provide a fake browser-local adapter", async () => {
    const client = createNeoChatApiClient({
      env: { NEXT_PUBLIC_API_MODE: "local", NEXT_PUBLIC_API_BASE_URL: "" },
    });

    await expect(client.settings.getTaskModels()).rejects.toMatchObject({
      code: "FEATURE_NOT_IMPLEMENTED",
    });
  });
});
