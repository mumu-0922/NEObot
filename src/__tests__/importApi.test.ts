import { describe, expect, it } from "vitest";
import { createNeoChatApiClient } from "../services/api/client";
import { createServerImportApiShell } from "../services/api/client/server/importApi";
import { createHttpClient } from "../services/api/client/server/httpClient";

describe("browser import API client", () => {
  it("enables import capability only for configured server mode", async () => {
    const local = createNeoChatApiClient();
    expect(local.capabilities.imports).toBe(false);
    await expect(
      local.imports?.previewBrowserImport({ package: new Blob(["zip"]) }),
    ).rejects.toMatchObject({ code: "FEATURE_NOT_IMPLEMENTED" });

    const server = createNeoChatApiClient({
      env: {
        NEXT_PUBLIC_API_MODE: "server",
        NEXT_PUBLIC_API_BASE_URL: "/mm-api",
      },
    });
    expect(server.capabilities.imports).toBe(true);
    expect(server.imports).toBeTruthy();
  });

  it("posts browser import packages as multipart form data without content-type", async () => {
    const requests: Array<{
      url: string;
      method?: string;
      contentType?: string | null;
      fileName: string;
      fileText: string;
    }> = [];
    const imports = createServerImportApiShell(
      createHttpClient({
        baseUrl: "http://backend.test",
        fetchImpl: async (input, init) => {
          const headers = new Headers(init?.headers);
          const body = init?.body;
          if (!(body instanceof FormData)) {
            throw new Error("Expected FormData body.");
          }
          const pkg = body.get("package") as File;
          requests.push({
            url: String(input),
            method: init?.method,
            contentType: headers.get("content-type"),
            fileName: pkg.name,
            fileText: await pkg.text(),
          });
          return Response.json({
            summary: {
              conversations: 1,
              messages: 2,
              files: 0,
              bytes: 0,
              skippedDuplicates: 0,
            },
            warnings: [],
            errors: [],
            commitAllowed: true,
          });
        },
      }),
    );

    await expect(
      imports.previewBrowserImport({
        package: new Blob(["zip-bytes"], { type: "application/zip" }),
        fileName: "neo-chat-browser-import-v2.zip",
      }),
    ).resolves.toMatchObject({ commitAllowed: true });
    expect(requests).toEqual([
      {
        url: "http://backend.test/v1/import/browser/preview",
        method: "POST",
        contentType: null,
        fileName: "neo-chat-browser-import-v2.zip",
        fileText: "zip-bytes",
      },
    ]);
  });

  it("commits, reads, and rolls back import batches through the Go endpoints", async () => {
    const requests: Array<{ url: string; method?: string }> = [];
    const imports = createServerImportApiShell(
      createHttpClient({
        baseUrl: "/mm-api",
        fetchImpl: async (input, init) => {
          requests.push({ url: String(input), method: init?.method });
          if (init?.method === "DELETE") {
            return new Response(null, { status: 204 });
          }
          if (String(input).endsWith("/batch-1")) {
            return Response.json({
              batchId: "batch-1",
              status: "completed",
              createdAt: "2026-07-09T00:00:00Z",
            });
          }
          return Response.json(
            {
              batchId: "batch-1",
              status: "completed",
              created: {
                conversations: 1,
                messages: 2,
                files: 0,
                attachments: 0,
              },
              mappings: { conversations: {}, messages: {}, files: {} },
              warnings: [],
            },
            { status: 201 },
          );
        },
      }),
    );

    await expect(
      imports.commitBrowserImport({ package: new Blob(["zip"]) }),
    ).resolves.toMatchObject({ batchId: "batch-1" });
    await expect(
      imports.getBrowserImportBatch("batch-1"),
    ).resolves.toMatchObject({
      status: "completed",
    });
    await expect(
      imports.rollbackBrowserImportBatch("batch-1"),
    ).resolves.toBeUndefined();
    expect(requests).toEqual([
      { url: "/mm-api/v1/import/browser", method: "POST" },
      { url: "/mm-api/v1/import/browser/batch-1", method: "GET" },
      { url: "/mm-api/v1/import/browser/batch-1", method: "DELETE" },
    ]);
  });
});
