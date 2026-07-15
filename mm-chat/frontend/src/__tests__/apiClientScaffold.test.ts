import { afterEach, describe, expect, it, vi } from "vitest";
import {
  ApiClientError,
  createSseStreamParser,
  createHttpClient,
  createNeoChatApiClient,
  inferNetworkEdge,
  joinUrl,
  parseSseFrames,
  resolveApiClientConfig,
} from "../services/api/client";
import { createServerChatApiShell } from "../services/api/client/server/chatApi";
import {
  clearServerAuthSession,
  getServerAuthSession,
  getServerAuthToken,
  serverAuthSessionStorageKey,
  setServerAuthSession,
} from "../services/api/client/authSession";
import { isServerAuthGateEnabled } from "../lib/security/serverAuthMode";
import { createServerAgentApiShell } from "../services/api/client/server/agentApi";
import { createServerAuthApiShell } from "../services/api/client/server/authApi";
import { createServerByokApiShell } from "../services/api/client/server/byokApi";
import { createServerProviderApiShell } from "../services/api/client/server/providerApi";
import { createServerSettingsApiShell } from "../services/api/client/server/settingsApi";
import { createServerFileApiShell } from "../services/api/client/server/fileApi";
import { createServerPluginApiShell } from "../services/api/client/server/pluginApi";

describe("Phase 11.1B API mode resolver", () => {
  it("defaults invalid or missing modes to local", () => {
    expect(resolveApiClientConfig({ env: {} }).mode).toBe("local");
    expect(
      resolveApiClientConfig({ env: { NEXT_PUBLIC_API_MODE: "bogus" } }).mode,
    ).toBe("local");
    expect(
      resolveApiClientConfig({ env: { NEXT_PUBLIC_API_MODE: "bogus" } })
        .warnings[0],
    ).toContain("Unsupported API mode");
  });

  it("resolves server mode and base URL without performing network calls", () => {
    const resolved = resolveApiClientConfig({
      env: {
        NEXT_PUBLIC_API_MODE: "server",
        NEXT_PUBLIC_API_BASE_URL: "http://127.0.0.1:8080/",
      },
      frontendOrigin: "http://localhost:3000",
    });

    expect(resolved.mode).toBe("server");
    expect(resolved.baseUrl).toBe("http://127.0.0.1:8080");
    expect(resolved.serverConfigured).toBe(true);
    expect(resolved.networkEdge).toBe("direct-cors");
  });

  it("supports relative same-origin proxy base URLs", () => {
    const resolved = resolveApiClientConfig({
      env: {
        NEXT_PUBLIC_API_MODE: "server",
        NEXT_PUBLIC_API_BASE_URL: "/mm-api/",
      },
      frontendOrigin: "http://127.0.0.1:3000",
    });
    const client = createNeoChatApiClient({
      env: {
        NEXT_PUBLIC_API_MODE: "server",
        NEXT_PUBLIC_API_BASE_URL: "/mm-api/",
      },
    });

    expect(resolved.mode).toBe("server");
    expect(resolved.baseUrl).toBe("/mm-api");
    expect(resolved.networkEdge).toBe("same-origin-proxy");
    expect(client.mode).toBe("server");
    expect(client.capabilities).toMatchObject({
      chatCrud: true,
      chatStream: true,
      files: true,
      auth: true,
      voice: false,
      imageGeneration: false,
      codeExecution: false,
    });
  });

  it("reads bundled NEXT_PUBLIC env values by default", () => {
    const previousMode = process.env.NEXT_PUBLIC_API_MODE;
    const previousBaseUrl = process.env.NEXT_PUBLIC_API_BASE_URL;
    process.env.NEXT_PUBLIC_API_MODE = "server";
    process.env.NEXT_PUBLIC_API_BASE_URL = "/mm-api";

    try {
      const resolved = resolveApiClientConfig();

      expect(resolved.mode).toBe("server");
      expect(resolved.baseUrl).toBe("/mm-api");
      expect(resolved.networkEdge).toBe("same-origin-proxy");
    } finally {
      if (previousMode === undefined) {
        delete process.env.NEXT_PUBLIC_API_MODE;
      } else {
        process.env.NEXT_PUBLIC_API_MODE = previousMode;
      }
      if (previousBaseUrl === undefined) {
        delete process.env.NEXT_PUBLIC_API_BASE_URL;
      } else {
        process.env.NEXT_PUBLIC_API_BASE_URL = previousBaseUrl;
      }
    }
  });

  it("falls back to local when server mode lacks a base URL", () => {
    const client = createNeoChatApiClient({
      env: { NEXT_PUBLIC_API_MODE: "server" },
    });

    expect(client.mode).toBe("local");
    expect(client.config.serverConfigured).toBe(false);
    expect(client.config.requestedMode).toBe("server");
    expect(client.config.warnings).toContain(
      "Server mode requested without NEXT_PUBLIC_API_BASE_URL.",
    );
  });

  it("keeps default local scaffold capabilities disabled", () => {
    const client = createNeoChatApiClient();

    expect(client.mode).toBe("local");
    expect(client.capabilities).toEqual({
      chatCrud: false,
      chatStream: false,
      files: false,
      auth: false,
      imports: false,
      rag: false,
      plugins: false,
      providerSettings: false,
      agents: false,
      voice: false,
      imageGeneration: false,
      codeExecution: false,
    });
  });

  it("enables supported server capabilities for configured server mode", () => {
    const client = createNeoChatApiClient({
      env: {
        NEXT_PUBLIC_API_MODE: "server",
        NEXT_PUBLIC_API_BASE_URL: "http://127.0.0.1:8080",
      },
    });

    expect(client.mode).toBe("server");
    expect(client.capabilities).toMatchObject({
      chatCrud: true,
      chatStream: true,
      files: true,
      auth: true,
      voice: false,
      imageGeneration: false,
      codeExecution: false,
    });
  });

  it("classifies same-origin and direct-CORS network edges", () => {
    expect(inferNetworkEdge("/mm-api", "http://localhost:3000")).toBe(
      "same-origin-proxy",
    );
    expect(
      inferNetworkEdge("http://localhost:3000/mm-api", "http://localhost:3000"),
    ).toBe("same-origin-proxy");
    expect(
      inferNetworkEdge("http://127.0.0.1:8080", "http://localhost:3000"),
    ).toBe("direct-cors");
  });
});

describe("Phase 11.1B server HTTP scaffold", () => {
  it("joins base URLs and API paths predictably", () => {
    expect(joinUrl("http://127.0.0.1:8080", "/v1/version")).toBe(
      "http://127.0.0.1:8080/v1/version",
    );
    expect(joinUrl("/mm-api", "v1/version")).toBe("/mm-api/v1/version");
    expect(createHttpClient({ baseUrl: "" }).buildUrl("/ready")).toBe("/ready");
  });

  it("normalizes JSON error envelopes", async () => {
    const client = createHttpClient({
      baseUrl: "http://backend.test",
      fetchImpl: async () =>
        Response.json(
          {
            error: { code: "DATABASE_REQUIRED", message: "database required" },
          },
          { status: 503 },
        ),
    });

    await expect(client.requestJson("/ready")).rejects.toMatchObject({
      code: "DATABASE_REQUIRED",
      message: "database required",
      status: 503,
    });
  });

  it("rejects invalid successful JSON responses", async () => {
    const client = createHttpClient({
      baseUrl: "http://backend.test",
      fetchImpl: async () => new Response("<html></html>", { status: 200 }),
    });

    await expect(client.requestJson("/ready")).rejects.toMatchObject({
      code: "INVALID_SERVER_RESPONSE",
    });
  });

  it("normalizes network and CORS-style fetch failures", async () => {
    const client = createHttpClient({
      baseUrl: "http://backend.test",
      fetchImpl: async () => {
        throw new TypeError("Failed to fetch");
      },
    });

    await expect(client.requestJson("/ready")).rejects.toMatchObject({
      code: "NETWORK_ERROR",
      recoverable: true,
    });
  });
});

describe("G3.2 server auth session lifecycle", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("stores the Go bearer token in browser sessionStorage only", () => {
    const storage = new MemoryStorage();
    vi.stubGlobal("window", { sessionStorage: storage });

    setServerAuthSession({
      user: { id: "user-1", displayName: "Owner", role: "user" },
      token: "session-token",
      expiresAt: "2999-01-01T00:00:00Z",
    });

    expect(storage.getItem(serverAuthSessionStorageKey)).toContain(
      "session-token",
    );
    expect(getServerAuthToken()).toBe("session-token");
    expect(getServerAuthSession()).toMatchObject({
      user: { id: "user-1", displayName: "Owner", role: "user" },
    });

    clearServerAuthSession();
    expect(getServerAuthToken()).toBeNull();
  });

  it("drops expired or malformed stored sessions", () => {
    const storage = new MemoryStorage();
    vi.stubGlobal("window", { sessionStorage: storage });
    storage.setItem(
      serverAuthSessionStorageKey,
      JSON.stringify({
        token: "expired-token",
        user: { id: "user-1", displayName: "Owner", role: "user" },
        expiresAt: "2000-01-01T00:00:00Z",
      }),
    );

    expect(getServerAuthToken()).toBeNull();
    expect(storage.getItem(serverAuthSessionStorageKey)).toBeNull();

    storage.setItem(serverAuthSessionStorageKey, "not-json");
    expect(getServerAuthSession()).toBeNull();
  });

  it("injects the stored bearer token into server API requests", async () => {
    const requests: Array<{ url: string; auth: string | null }> = [];
    const client = createHttpClient({
      baseUrl: "/mm-api",
      getAuthToken: () => "session-token",
      fetchImpl: async (input, init) => {
        requests.push({
          url: String(input),
          auth: init?.headers
            ? new Headers(init.headers).get("authorization")
            : null,
        });
        return Response.json({ ok: true });
      },
    });

    await client.requestJson("/v1/me");

    expect(requests).toEqual([
      { url: "/mm-api/v1/me", auth: "Bearer session-token" },
    ]);
  });

  it("enables the client AuthGate only for server mode with required backend auth", () => {
    expect(
      isServerAuthGateEnabled({
        NEXT_PUBLIC_API_MODE: "server",
        AUTH_MODE: "required",
      }),
    ).toBe(true);
    expect(
      isServerAuthGateEnabled({
        NEXT_PUBLIC_API_MODE: "server",
        AUTH_MODE: "development",
      }),
    ).toBe(false);
    expect(
      isServerAuthGateEnabled({
        NEXT_PUBLIC_API_MODE: "local",
        AUTH_MODE: "required",
      }),
    ).toBe(false);
  });
});

class MemoryStorage implements Storage {
  private readonly data = new Map<string, string>();

  get length() {
    return this.data.size;
  }

  clear(): void {
    this.data.clear();
  }

  getItem(key: string): string | null {
    return this.data.get(key) ?? null;
  }

  key(index: number): string | null {
    return Array.from(this.data.keys())[index] ?? null;
  }

  removeItem(key: string): void {
    this.data.delete(key);
  }

  setItem(key: string, value: string): void {
    this.data.set(key, value);
  }
}

describe("G3.1 server runtime/auth API adapters", () => {
  it("routes auth lifecycle calls through Go auth endpoints", async () => {
    const requests: Array<{
      url: string;
      method?: string;
      auth?: string | null;
      body?: unknown;
    }> = [];
    const auth = createServerAuthApiShell(
      createHttpClient({
        baseUrl: "/mm-api",
        fetchImpl: async (input, init) => {
          requests.push({
            url: String(input),
            method: init?.method,
            auth: init?.headers
              ? new Headers(init.headers).get("authorization")
              : null,
            body: init?.body ? JSON.parse(String(init.body)) : undefined,
          });
          if (String(input).endsWith("/v1/auth/login")) {
            return Response.json({
              user: { id: "user-1", displayName: "Owner", role: "user" },
              token: "session-token",
              expiresAt: "2026-07-15T00:00:00Z",
            });
          }
          if (String(input).endsWith("/v1/me")) {
            return Response.json({
              id: "user-1",
              displayName: "Owner",
              role: "user",
            });
          }
          return new Response(null, { status: 204 });
        },
      }),
    );

    await expect(
      auth.login({ email: "owner@example.test", password: "secret" }),
    ).resolves.toMatchObject({ token: "session-token" });
    await expect(
      auth.getCurrentUser({ token: "session-token" }),
    ).resolves.toMatchObject({
      id: "user-1",
    });
    await expect(
      auth.logout({ token: "session-token" }),
    ).resolves.toBeUndefined();

    expect(requests).toEqual([
      {
        url: "/mm-api/v1/auth/login",
        method: "POST",
        auth: null,
        body: { email: "owner@example.test", password: "secret" },
      },
      {
        url: "/mm-api/v1/me",
        method: "GET",
        auth: "Bearer session-token",
        body: undefined,
      },
      {
        url: "/mm-api/v1/auth/logout",
        method: "POST",
        auth: "Bearer session-token",
        body: undefined,
      },
    ]);
  });

  it("routes runtime config, provider models, and BYOK public-key calls through Go endpoints", async () => {
    const requests: Array<{ url: string; method?: string; body?: unknown }> =
      [];
    const http = createHttpClient({
      baseUrl: "/mm-api",
      fetchImpl: async (input, init) => {
        requests.push({
          url: String(input),
          method: init?.method,
          body: init?.body ? JSON.parse(String(init.body)) : undefined,
        });
        if (String(input).endsWith("/v1/config")) {
          return Response.json({
            modelProvider: {
              available: true,
              id: "SERVER_DEFAULT",
              name: "Server Default",
              type: "OpenAI Compatible",
              models: ["gpt-5.5"],
              modelMetadata: {},
              defaultModels: {},
            },
            search: { available: false },
            rag: {
              vectorStoreAvailable: false,
              documentProcessingAvailable: false,
            },
            voice: {
              elevenLabsAvailable: false,
              mimoAvailable: false,
              defaultSttAvailable: false,
              defaultTtsAvailable: false,
            },
          });
        }
        if (String(input).endsWith("/v1/providers/models")) {
          return Response.json({ models: ["gpt-5.5"] });
        }
        return Response.json({
          kid: "kid",
          alg: "RSA-OAEP-256+A256GCM",
          publicKeyJwk: { kty: "RSA", n: "abc", e: "AQAB" },
        });
      },
    });

    await expect(
      createServerSettingsApiShell(http).getRuntimeConfig(),
    ).resolves.toMatchObject({
      modelProvider: { models: ["gpt-5.5"] },
    });
    await expect(
      createServerProviderApiShell(http).listModels({
        provider: { source: "server-default" },
      }),
    ).resolves.toEqual({ models: ["gpt-5.5"] });
    await expect(
      createServerByokApiShell(http).getPublicKey(),
    ).resolves.toMatchObject({
      kid: "kid",
    });

    expect(requests).toEqual([
      { url: "/mm-api/v1/config", method: "GET", body: undefined },
      {
        url: "/mm-api/v1/providers/models",
        method: "POST",
        body: { provider: { source: "server-default" } },
      },
      { url: "/mm-api/v1/byok/public-key", method: "GET", body: undefined },
    ]);
  });
});

describe("G2 server agent API adapter", () => {
  it("routes agent catalog requests through the Go API", async () => {
    const requests: Array<{ url: string; method?: string }> = [];
    const agents = createServerAgentApiShell(
      createHttpClient({
        baseUrl: "/mm-api",
        fetchImpl: async (input, init) => {
          requests.push({ url: String(input), method: init?.method });
          if (String(input).includes("/v1/agents/")) {
            return Response.json({
              identifier: "agent-1",
              meta: { title: "Agent One" },
            });
          }
          return Response.json({
            agents: [
              {
                identifier: "agent-1",
                meta: { title: "Agent One" },
              },
            ],
          });
        },
      }),
    );

    await expect(agents.listAgents({ locale: "zh" })).resolves.toMatchObject({
      agents: [expect.objectContaining({ identifier: "agent-1" })],
    });
    await expect(
      agents.getAgentDetail({ identifier: "agent/1", locale: "ja" }),
    ).resolves.toMatchObject({ identifier: "agent-1" });

    expect(requests).toEqual([
      { url: "/mm-api/v1/agents?locale=zh", method: "GET" },
      { url: "/mm-api/v1/agents/agent%2F1?locale=ja", method: "GET" },
    ]);
  });
});

describe("G4.2 server plugin registry API adapter", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("routes plugin list requests through the Go API", async () => {
    const requests: Array<{ url: string; method?: string }> = [];
    const plugins = createServerPluginApiShell(
      createHttpClient({
        baseUrl: "/mm-api",
        fetchImpl: async (input, init) => {
          requests.push({ url: String(input), method: init?.method });
          return Response.json({
            plugins: [
              {
                id: "example.com:weather",
                title: "Weather",
                description: "Weather plugin",
                manifestUrl: "https://example.com/weather.json",
                functions: [],
              },
            ],
          });
        },
      }),
    );

    await expect(plugins.listAvailable()).resolves.toMatchObject({
      plugins: [expect.objectContaining({ id: "example.com:weather" })],
    });
    expect(requests).toEqual([{ url: "/mm-api/v1/plugins", method: "GET" }]);
  });

  it("routes plugin install requests through the Go API", async () => {
    const requests: Array<{ url: string; method?: string; body?: unknown }> =
      [];
    const plugins = createServerPluginApiShell(
      createHttpClient({
        baseUrl: "/mm-api",
        fetchImpl: async (input, init) => {
          requests.push({
            url: String(input),
            method: init?.method,
            body: init?.body ? JSON.parse(String(init.body)) : undefined,
          });
          return Response.json({
            plugin: {
              id: "example.com:weather",
              title: "Weather",
              description: "Weather plugin",
              logoUrl: "",
              manifestUrl: "https://example.com/weather.json",
              functions: [],
            },
          });
        },
      }),
    );

    await expect(
      plugins.install({
        plugin: {
          id: "example.com:weather",
          title: "Weather",
          description: "Weather plugin",
          logoUrl: "",
          manifestUrl: "https://example.com/weather.json",
          functions: [],
        },
      }),
    ).resolves.toMatchObject({
      plugin: { id: "example.com:weather" },
    });
    await expect(
      plugins.install({ customInput: "https://example.com/custom.json" }),
    ).resolves.toMatchObject({
      plugin: { id: "example.com:weather" },
    });

    expect(requests).toEqual([
      {
        url: "/mm-api/v1/plugins/install",
        method: "POST",
        body: {
          plugin: expect.objectContaining({ id: "example.com:weather" }),
        },
      },
      {
        url: "/mm-api/v1/plugins/install",
        method: "POST",
        body: { customInput: "https://example.com/custom.json" },
      },
    ]);
  });

  it("treats missing plugin install routes as explicitly unavailable", async () => {
    const plugins = createServerPluginApiShell(
      createHttpClient({
        baseUrl: "/mm-api",
        fetchImpl: async () =>
          Response.json(
            { error: { code: "NOT_FOUND", message: "route not found" } },
            { status: 404 },
          ),
      }),
    );

    await expect(
      plugins.install({ customInput: "https://example.com/custom.json" }),
    ).rejects.toMatchObject({
      code: "PLUGIN_INSTALL_UNAVAILABLE",
      recoverable: true,
    });
  });

  it("routes full plugin execution payloads to Go and maps execution errors", async () => {
    const requests: Array<{ url: string; method?: string; body?: unknown }> =
      [];
    const plugins = createServerPluginApiShell(
      createHttpClient({
        baseUrl: "/mm-api",
        fetchImpl: async (input, init) => {
          requests.push({
            url: String(input),
            method: init?.method,
            body: init?.body ? JSON.parse(String(init.body)) : undefined,
          });
          return Response.json(
            {
              error: {
                code: "PLUGIN_URL_BLOCKED",
                message: "plugin URL is blocked by policy",
              },
            },
            { status: 403 },
          );
        },
      }),
    );

    const response = await plugins.execute({
      payload: {
        plugin: {
          id: "example.com:weather",
          title: "Weather",
          description: "Weather plugin",
          logoUrl: "",
          manifestUrl: "https://example.com/weather.json",
          baseUrl: "https://api.example.com",
          functions: [
            {
              name: "lookup_weather",
              description: "Lookup weather",
              path: "/weather",
              method: "GET",
              parameters: { type: "object" },
            },
          ],
          auth: { type: "none" },
        },
        functionDef: {
          name: "lookup_weather",
          description: "Lookup weather",
          path: "/weather",
          method: "GET",
          parameters: { type: "object" },
        },
        args: { city: "Shanghai" },
      },
    });

    expect(response.status).toBe(403);
    await expect(response.json()).resolves.toMatchObject({
      code: "PLUGIN_URL_BLOCKED",
      error: "plugin URL is blocked by policy",
    });

    expect(requests).toEqual([
      {
        url: "/mm-api/v1/plugins/execute",
        method: "POST",
        body: {
          plugin: expect.objectContaining({ id: "example.com:weather" }),
          functionDef: expect.objectContaining({ name: "lookup_weather" }),
          args: { city: "Shanghai" },
        },
      },
    ]);
  });

  it("treats missing plugin registry routes as explicitly unavailable", async () => {
    const plugins = createServerPluginApiShell(
      createHttpClient({
        baseUrl: "/mm-api",
        fetchImpl: async () =>
          Response.json(
            { error: { code: "NOT_FOUND", message: "route not found" } },
            { status: 404 },
          ),
      }),
    );

    await expect(plugins.listAvailable()).resolves.toEqual({
      plugins: [],
      unavailable: true,
    });
  });

  it("rejects malformed successful plugin registry responses", async () => {
    const plugins = createServerPluginApiShell(
      createHttpClient({
        baseUrl: "/mm-api",
        fetchImpl: async () => Response.json({ plugins: "bad" }),
      }),
    );

    await expect(plugins.listAvailable()).rejects.toMatchObject({
      code: "INVALID_SERVER_RESPONSE",
    });
  });
});

describe("Phase 11.4A server file API adapter", () => {
  const fileId = "00000000-0000-4000-8000-000000000001";
  const fileRecord = {
    id: fileId,
    fileName: "notes.txt",
    mimeType: "text/plain",
    size: 11,
    sha256: "64ec88ca00b268e5ba1a35678a1b5316d212f4f366b2477232534a8aeca37f3c",
    purpose: "chat",
    createdAt: "2026-07-08T00:00:00Z",
    downloadUrl: `/v1/files/${fileId}/content`,
  };

  it("keeps local file adapter calls fail-closed", async () => {
    const client = createNeoChatApiClient();

    await expect(
      client.files.uploadFile({
        file: new Blob(["hello"], { type: "text/plain" }),
        fileName: "hello.txt",
        purpose: "chat",
      }),
    ).rejects.toMatchObject({ code: "FEATURE_NOT_IMPLEMENTED" });
    expect(client.capabilities.files).toBe(false);
  });

  it("uploads files through multipart form data without manual content-type", async () => {
    const requests: Array<{
      url: string;
      method?: string;
      contentType?: string | null;
      accept?: string | null;
      fields: Record<string, unknown>;
    }> = [];
    const files = createServerFileApiShell(
      createHttpClient({
        baseUrl: "http://backend.test",
        fetchImpl: async (input, init) => {
          const headers = new Headers(init?.headers);
          const body = init?.body;
          if (!(body instanceof FormData)) {
            throw new Error("Expected multipart FormData body.");
          }
          const uploadedFile = body.get("file") as File;
          requests.push({
            url: String(input),
            method: init?.method,
            contentType: headers.get("content-type"),
            accept: headers.get("accept"),
            fields: {
              fileName: uploadedFile.name,
              fileType: uploadedFile.type,
              purpose: body.get("purpose"),
              conversationId: body.get("conversationId"),
              workspaceId: body.get("workspaceId"),
              knowledgeCollectionId: body.get("knowledgeCollectionId"),
              clientFileId: body.get("clientFileId"),
            },
          });
          return Response.json(
            {
              ...fileRecord,
              objectKey: "users/dev/files/file-1",
              bucket: "neo-chat-files",
            },
            { status: 201 },
          );
        },
      }),
    );

    await expect(
      files.uploadFile({
        file: new Blob(["hello world"], { type: "text/plain" }),
        fileName: "notes.txt",
        purpose: "chat",
        conversationId: "conversation-1",
        workspaceId: "workspace-1",
        knowledgeCollectionId: "knowledge-1",
        clientFileId: "client-file-1",
      }),
    ).resolves.toEqual(fileRecord);
    expect(requests).toEqual([
      {
        url: "http://backend.test/v1/files",
        method: "POST",
        contentType: null,
        accept: "application/json",
        fields: {
          fileName: "notes.txt",
          fileType: "text/plain",
          purpose: "chat",
          conversationId: "conversation-1",
          workspaceId: "workspace-1",
          knowledgeCollectionId: "knowledge-1",
          clientFileId: "client-file-1",
        },
      },
    ]);
  });

  it("gets metadata and rejects unsafe file metadata", async () => {
    const okFiles = createServerFileApiShell(
      createHttpClient({
        baseUrl: "http://backend.test",
        fetchImpl: async (input, init) => {
          expect(String(input)).toBe(`http://backend.test/v1/files/${fileId}`);
          expect(init?.method).toBe("GET");
          return Response.json(fileRecord);
        },
      }),
    );
    await expect(okFiles.getFile(fileId)).resolves.toEqual(fileRecord);

    for (const unsafeRecord of [
      {
        ...fileRecord,
        downloadUrl: "http://minio:9000/neo-chat-files/object-key",
      },
      {
        ...fileRecord,
        downloadUrl: "/v1/files/users/dev/files/file-1/content",
      },
      { ...fileRecord, downloadUrl: "/v1/files/../file-1/content" },
      {
        ...fileRecord,
        downloadUrl: "/v1/files/00000000-0000-4000-8000-000000000002/content",
      },
      {
        ...fileRecord,
        id: "file/with-slash",
        downloadUrl: "/v1/files/file%2Fwith-slash/content",
      },
      { ...fileRecord, purpose: "object-store" },
    ]) {
      const unsafeFiles = createServerFileApiShell(
        createHttpClient({
          baseUrl: "http://backend.test",
          fetchImpl: async () => Response.json(unsafeRecord),
        }),
      );
      await expect(unsafeFiles.getFile("file-1")).rejects.toMatchObject({
        code: "INVALID_SERVER_RESPONSE",
      });
    }
  });

  it("downloads file content through the backend gateway", async () => {
    const requests: Array<{
      url: string;
      method?: string;
      accept?: string | null;
    }> = [];
    const files = createServerFileApiShell(
      createHttpClient({
        baseUrl: "http://backend.test",
        fetchImpl: async (input, init) => {
          const headers = new Headers(init?.headers);
          requests.push({
            url: String(input),
            method: init?.method,
            accept: headers.get("accept"),
          });
          return new Response("hello world", {
            headers: {
              "content-type": "text/plain",
              "content-length": "11",
            },
          });
        },
      }),
    );

    const downloaded = await files.downloadFileContent({
      fileId: "file/with slash",
      disposition: "attachment",
    });

    await expect(downloaded.blob.text()).resolves.toBe("hello world");
    expect(downloaded.contentType).toBe("text/plain");
    expect(downloaded.size).toBe(11);
    expect(requests).toEqual([
      {
        url: "http://backend.test/v1/files/file%2Fwith%20slash/content?disposition=attachment",
        method: "GET",
        accept: "application/octet-stream, */*",
      },
    ]);
  });

  it("normalizes binary download errors and deletes files", async () => {
    const requests: Array<{ url: string; method?: string }> = [];
    const files = createServerFileApiShell(
      createHttpClient({
        baseUrl: "http://backend.test",
        fetchImpl: async (input, init) => {
          requests.push({ url: String(input), method: init?.method });
          if (init?.method === "DELETE") {
            return new Response(null, { status: 204 });
          }
          return Response.json(
            { error: { code: "FILE_NOT_FOUND", message: "file not found" } },
            { status: 404 },
          );
        },
      }),
    );

    await expect(
      files.downloadFileContent({ fileId: "missing-file" }),
    ).rejects.toMatchObject({
      code: "FILE_NOT_FOUND",
      status: 404,
    });
    await expect(files.deleteFile("file/with slash")).resolves.toBeUndefined();
    expect(requests).toEqual([
      {
        url: "http://backend.test/v1/files/missing-file/content",
        method: "GET",
      },
      {
        url: "http://backend.test/v1/files/file%2Fwith%20slash",
        method: "DELETE",
      },
    ]);
  });
});

describe("Phase 11.2A server chat CRUD adapter", () => {
  it("creates conversations through the Go chat endpoint", async () => {
    const requests: Array<{ url: string; body: unknown; method?: string }> = [];
    const chat = createServerChatApiShell(
      createHttpClient({
        baseUrl: "http://backend.test",
        fetchImpl: async (input, init) => {
          requests.push({
            url: String(input),
            method: init?.method,
            body: JSON.parse(String(init?.body)),
          });
          return Response.json(
            {
              id: "c1",
              title: "Hello",
              status: "active",
              modelRef: { providerId: "openai", modelId: "gpt-5.5" },
              messageCount: 0,
              config: { temperature: 0.2 },
              createdAt: "2026-07-08T00:00:00Z",
              updatedAt: "2026-07-08T00:00:00Z",
            },
            { status: 201 },
          );
        },
      }),
    );

    const conversation = await chat.createConversation({
      title: "Hello",
      modelRef: { providerId: "openai", modelId: "gpt-5.5" },
      config: { temperature: 0.2 },
      idempotencyKey: "conversation-key",
    });

    expect(conversation.id).toBe("c1");
    expect(requests).toEqual([
      {
        url: "http://backend.test/v1/chat/conversations",
        method: "POST",
        body: {
          title: "Hello",
          modelRef: { providerId: "openai", modelId: "gpt-5.5" },
          config: { temperature: 0.2 },
          idempotencyKey: "conversation-key",
        },
      },
    ]);
  });

  it("updates, deletes, duplicates, titles, and related questions conversations through Go endpoints", async () => {
    const requests: Array<{ url: string; body?: unknown; method?: string }> =
      [];
    const chat = createServerChatApiShell(
      createHttpClient({
        baseUrl: "http://backend.test",
        fetchImpl: async (input, init) => {
          requests.push({
            url: String(input),
            method: init?.method,
            body: init?.body ? JSON.parse(String(init.body)) : undefined,
          });
          if (init?.method === "DELETE") {
            return new Response(null, { status: 204 });
          }
          if (String(input).endsWith("/duplicate")) {
            return Response.json(
              {
                id: "copy-1",
                title: "Renamed (Copy)",
                status: "active",
                messageCount: 1,
                pinned: false,
                config: { pinned: false },
                createdAt: "2026-07-08T00:02:00Z",
                updatedAt: "2026-07-08T00:02:00Z",
              },
              { status: 201 },
            );
          }
          if (String(input).endsWith("/title")) {
            return Response.json({ title: "Generated Server Title" });
          }
          if (String(input).endsWith("/related-questions")) {
            return Response.json({ questions: ["Next?", 7, "Caveats?"] });
          }
          return Response.json({
            id: "conversation/with slash",
            title: "Renamed",
            status: "active",
            messageCount: 1,
            systemInstruction: "be precise",
            pinned: true,
            config: { pinned: true },
            createdAt: "2026-07-08T00:00:00Z",
            updatedAt: "2026-07-08T00:01:00Z",
          });
        },
      }),
    );

    await expect(
      chat.updateConversation({
        conversationId: "conversation/with slash",
        title: "Renamed",
        systemInstruction: "be precise",
        pinned: true,
      }),
    ).resolves.toMatchObject({
      id: "conversation/with slash",
      title: "Renamed",
      pinned: true,
    });
    await expect(
      chat.deleteConversation("conversation/with slash"),
    ).resolves.toBeUndefined();
    await expect(
      chat.duplicateConversation({
        conversationId: "conversation/with slash",
        idempotencyKey: "duplicate-key",
      }),
    ).resolves.toMatchObject({
      id: "copy-1",
      title: "Renamed (Copy)",
      pinned: false,
    });
    await expect(
      chat.generateConversationTitle({
        conversationId: "conversation/with slash",
        modelRef: { providerId: "openai", modelId: "gpt-title" },
      }),
    ).resolves.toEqual({ title: "Generated Server Title" });
    await expect(
      chat.generateRelatedQuestions({
        conversationId: "conversation/with slash",
        modelRef: { providerId: "openai", modelId: "gpt-related" },
      }),
    ).resolves.toEqual({ questions: ["Next?", "Caveats?"] });

    expect(requests).toEqual([
      {
        url: "http://backend.test/v1/chat/conversations/conversation%2Fwith%20slash",
        method: "PATCH",
        body: {
          title: "Renamed",
          systemInstruction: "be precise",
          pinned: true,
        },
      },
      {
        url: "http://backend.test/v1/chat/conversations/conversation%2Fwith%20slash",
        method: "DELETE",
        body: undefined,
      },
      {
        url: "http://backend.test/v1/chat/conversations/conversation%2Fwith%20slash/duplicate",
        method: "POST",
        body: {
          idempotencyKey: "duplicate-key",
        },
      },
      {
        url: "http://backend.test/v1/chat/conversations/conversation%2Fwith%20slash/title",
        method: "POST",
        body: {
          modelRef: { providerId: "openai", modelId: "gpt-title" },
        },
      },
      {
        url: "http://backend.test/v1/chat/conversations/conversation%2Fwith%20slash/related-questions",
        method: "POST",
        body: {
          modelRef: { providerId: "openai", modelId: "gpt-related" },
        },
      },
    ]);
  });

  it("lists conversations from the Go page envelope", async () => {
    const chat = createServerChatApiShell(
      createHttpClient({
        baseUrl: "http://backend.test",
        fetchImpl: async (input, init) => {
          expect(String(input)).toBe(
            "http://backend.test/v1/chat/conversations",
          );
          expect(init?.method).toBe("GET");
          return Response.json({
            items: [
              {
                id: "c1",
                title: "Hello",
                status: "active",
                messageCount: 1,
                config: {},
                createdAt: "2026-07-08T00:00:00Z",
                updatedAt: "2026-07-08T00:00:00Z",
              },
            ],
          });
        },
      }),
    );

    await expect(chat.listConversations()).resolves.toHaveLength(1);
  });

  it("deletes single and subsequent messages through the Go message endpoint", async () => {
    const requests: Array<{ url: string; method?: string }> = [];
    const chat = createServerChatApiShell(
      createHttpClient({
        baseUrl: "http://backend.test",
        fetchImpl: async (input, init) => {
          requests.push({ url: String(input), method: init?.method });
          return new Response(null, { status: 204 });
        },
      }),
    );

    await expect(
      chat.deleteMessage({
        conversationId: "conversation/with slash",
        messageId: "message/with slash",
      }),
    ).resolves.toBeUndefined();
    await expect(
      chat.deleteMessage({
        conversationId: "conversation/with slash",
        messageId: "message/with slash",
        scope: "subsequent",
      }),
    ).resolves.toBeUndefined();

    expect(requests).toEqual([
      {
        url: "http://backend.test/v1/chat/conversations/conversation%2Fwith%20slash/messages/message%2Fwith%20slash",
        method: "DELETE",
      },
      {
        url: "http://backend.test/v1/chat/conversations/conversation%2Fwith%20slash/messages/message%2Fwith%20slash?scope=subsequent",
        method: "DELETE",
      },
    ]);
  });

  it("updates message content through the Go message endpoint", async () => {
    const requests: Array<{ url: string; method?: string; body: unknown }> = [];
    const chat = createServerChatApiShell(
      createHttpClient({
        baseUrl: "http://backend.test",
        fetchImpl: async (input, init) => {
          requests.push({
            url: String(input),
            method: init?.method,
            body: JSON.parse(String(init?.body)),
          });
          return Response.json({
            id: "message/with slash",
            conversationId: "conversation/with slash",
            role: "assistant",
            status: "completed",
            content: "edited",
            sequenceNo: 2,
            attachments: [],
            outputBlocks: [],
            metadata: {},
            parentMessageId: "m1",
            createdAt: "2026-07-08T00:00:00Z",
            updatedAt: "2026-07-08T00:00:01Z",
          });
        },
      }),
    );

    await expect(
      chat.updateMessage({
        conversationId: "conversation/with slash",
        messageId: "message/with slash",
        content: "edited",
      }),
    ).resolves.toMatchObject({
      id: "message/with slash",
      content: "edited",
      parentMessageId: "m1",
    });

    expect(requests).toEqual([
      {
        url: "http://backend.test/v1/chat/conversations/conversation%2Fwith%20slash/messages/message%2Fwith%20slash",
        method: "PATCH",
        body: { content: "edited" },
      },
    ]);
  });

  it("appends completed user messages without server-managed fields", async () => {
    const requests: Array<{ url: string; body: unknown; method?: string }> = [];
    const chat = createServerChatApiShell(
      createHttpClient({
        baseUrl: "http://backend.test",
        fetchImpl: async (input, init) => {
          requests.push({
            url: String(input),
            method: init?.method,
            body: JSON.parse(String(init?.body)),
          });
          return Response.json(
            {
              id: "m1",
              conversationId: "conversation/with slash",
              role: "user",
              status: "completed",
              content: "hello",
              sequenceNo: 1,
              attachments: [],
              metadata: {},
              createdAt: "2026-07-08T00:00:00Z",
              updatedAt: "2026-07-08T00:00:00Z",
              completedAt: "2026-07-08T00:00:00Z",
            },
            { status: 201 },
          );
        },
      }),
    );

    await chat.appendUserMessage({
      conversationId: "conversation/with slash",
      content: "hello",
      attachments: [{ fileId: "file-1", purpose: "chat" }],
      metadata: { source: "test" },
      idempotencyKey: "message-key",
    });

    expect(requests).toEqual([
      {
        url: "http://backend.test/v1/chat/conversations/conversation%2Fwith%20slash/messages",
        method: "POST",
        body: {
          content: "hello",
          attachments: [
            { source: "server", fileId: "file-1", purpose: "chat" },
          ],
          metadata: { source: "test" },
          idempotencyKey: "message-key",
        },
      },
    ]);
  });

  it("rejects blank user messages before sending network requests", async () => {
    let called = false;
    const chat = createServerChatApiShell(
      createHttpClient({
        baseUrl: "http://backend.test",
        fetchImpl: async () => {
          called = true;
          return Response.json({});
        },
      }),
    );

    await expect(
      chat.appendUserMessage({ conversationId: "c1", content: "   " }),
    ).rejects.toMatchObject({ code: "EMPTY_CONTENT" });
    expect(called).toBe(false);
  });

  it("lists messages from the Go page envelope and rejects invalid pages", async () => {
    const okChat = createServerChatApiShell(
      createHttpClient({
        baseUrl: "http://backend.test",
        fetchImpl: async (input) => {
          expect(String(input)).toBe(
            "http://backend.test/v1/chat/conversations/c1/messages",
          );
          return Response.json({
            items: [
              {
                id: "m1",
                conversationId: "c1",
                role: "assistant",
                status: "completed",
                content: "hello",
                sequenceNo: 2,
                attachments: [],
                metadata: {},
                createdAt: "2026-07-08T00:00:00Z",
                updatedAt: "2026-07-08T00:00:00Z",
              },
            ],
          });
        },
      }),
    );
    await expect(okChat.listMessages("c1")).resolves.toHaveLength(1);

    const badChat = createServerChatApiShell(
      createHttpClient({
        baseUrl: "http://backend.test",
        fetchImpl: async () => Response.json({ items: {} }),
      }),
    );
    await expect(badChat.listMessages("c1")).rejects.toMatchObject({
      code: "INVALID_SERVER_RESPONSE",
    });
  });

  it("streams assistant messages from the Go SSE endpoint", async () => {
    const requests: Array<{
      url: string;
      body: unknown;
      method?: string;
      accept?: string | null;
      contentType?: string | null;
    }> = [];
    const chat = createServerChatApiShell(
      createHttpClient({
        baseUrl: "http://backend.test",
        fetchImpl: async (input, init) => {
          const headers = new Headers(init?.headers);
          requests.push({
            url: String(input),
            method: init?.method,
            body: JSON.parse(String(init?.body)),
            accept: headers.get("accept"),
            contentType: headers.get("content-type"),
          });
          return new Response(
            [
              "event: message.started",
              'data: {"type":"message.started","runId":"run-1","conversationId":"c1","messageId":"m2","sequence":1,"createdAt":"2026-07-08T00:00:00Z","role":"assistant"}',
              "",
              "event: message.delta",
              'data: {"type":"message.delta","runId":"run-1","conversationId":"c1","messageId":"m2","sequence":2,"createdAt":"2026-07-08T00:00:01Z","delta":"hel"}',
              "",
              "event: usage.updated",
              'data: {"type":"usage.updated","runId":"run-1","conversationId":"c1","messageId":"m2","sequence":3,"usage":{"total_tokens":3}}',
              "",
              "event: message.completed",
              'data: {"type":"message.completed","runId":"run-1","conversationId":"c1","messageId":"m2","sequence":4,"message":{"id":"m2","conversationId":"c1","role":"assistant","status":"completed","content":"hello","sequenceNo":2,"attachments":[],"outputBlocks":[],"metadata":{},"createdAt":"2026-07-08T00:00:02Z","updatedAt":"2026-07-08T00:00:02Z"}}',
              "",
            ].join("\n"),
            {
              headers: { "content-type": "text/event-stream; charset=utf-8" },
            },
          );
        },
      }),
    );
    const events: string[] = [];

    const result = await chat.streamAssistantMessage(
      {
        conversationId: "c1",
        userMessageId: "m1",
        modelRef: { providerId: "openai", modelId: "gpt-5.5" },
        config: { useSearch: true },
        metadata: { source: "test" },
        idempotencyKey: "stream-key",
      },
      {
        onStarted: (event) => events.push(event.type),
        onDelta: (event) => events.push(`${event.type}:${event.delta}`),
        onUsage: (event) => events.push(event.type),
        onCompleted: (event) => events.push(`${event.type}:${event.messageId}`),
      },
    );

    expect(result).toMatchObject({
      status: "completed",
      message: { id: "m2", role: "assistant", content: "hello" },
    });
    expect(events).toEqual([
      "message.started",
      "message.delta:hel",
      "usage.updated",
      "message.completed:m2",
    ]);
    expect(requests).toEqual([
      {
        url: "http://backend.test/v1/chat/conversations/c1/stream",
        method: "POST",
        accept: "text/event-stream",
        contentType: "application/json",
        body: {
          userMessageId: "m1",
          modelRef: { providerId: "openai", modelId: "gpt-5.5" },
          config: { useSearch: true },
          metadata: { source: "test" },
          idempotencyKey: "stream-key",
        },
      },
    ]);
  });

  it("maps stream error frames and pre-SSE JSON errors to failed run results", async () => {
    const errorEvents: string[] = [];
    const streamErrorChat = createServerChatApiShell(
      createHttpClient({
        baseUrl: "http://backend.test",
        fetchImpl: async () =>
          new Response(
            [
              "event: message.error",
              'data: {"type":"message.error","runId":"run-1","error":{"code":"PROVIDER_TIMEOUT","message":"Provider timed out.","recoverable":true}}',
              "",
            ].join("\n"),
            { headers: { "content-type": "text/event-stream" } },
          ),
      }),
    );

    await expect(
      streamErrorChat.streamAssistantMessage(
        {
          conversationId: "c1",
          userMessageId: "m1",
          modelRef: { providerId: "openai", modelId: "gpt-5.5" },
          idempotencyKey: "stream-key",
        },
        {
          onError: (event) => errorEvents.push(event.type),
        },
      ),
    ).resolves.toMatchObject({
      status: "failed",
      error: { code: "PROVIDER_TIMEOUT", recoverable: true },
    });
    expect(errorEvents).toEqual(["message.error"]);

    const jsonErrorChat = createServerChatApiShell(
      createHttpClient({
        baseUrl: "http://backend.test",
        fetchImpl: async () =>
          Response.json(
            {
              error: {
                code: "IDEMPOTENCY_KEY_REQUIRED",
                message: "idempotency required",
              },
            },
            { status: 400 },
          ),
      }),
    );

    await expect(
      jsonErrorChat.streamAssistantMessage({
        conversationId: "c1",
        userMessageId: "m1",
        modelRef: { providerId: "openai", modelId: "gpt-5.5" },
        idempotencyKey: "stream-key",
      }),
    ).resolves.toMatchObject({
      status: "failed",
      error: { code: "IDEMPOTENCY_KEY_REQUIRED" },
    });
  });

  it("maps stream cancelled frames and EOF interruptions", async () => {
    const cancelEvents: string[] = [];
    const cancelledChat = createServerChatApiShell(
      createHttpClient({
        baseUrl: "http://backend.test",
        fetchImpl: async () =>
          new Response(
            [
              "event: message.cancelled",
              'data: {"type":"message.cancelled","runId":"run-1","conversationId":"c1","messageId":"m2","sequence":1,"message":{"id":"m2","conversationId":"c1","role":"assistant","status":"cancelled","content":"","sequenceNo":2,"attachments":[],"outputBlocks":[],"metadata":{},"createdAt":"2026-07-08T00:00:00Z","updatedAt":"2026-07-08T00:00:00Z"}}',
              "",
            ].join("\n"),
            { headers: { "content-type": "text/event-stream" } },
          ),
      }),
    );

    await expect(
      cancelledChat.streamAssistantMessage(
        {
          conversationId: "c1",
          userMessageId: "m1",
          modelRef: { providerId: "openai", modelId: "gpt-5.5" },
          idempotencyKey: "stream-key",
        },
        {
          onCancelled: (event) => cancelEvents.push(event.type),
        },
      ),
    ).resolves.toMatchObject({
      status: "cancelled",
      message: { id: "m2", status: "cancelled" },
    });
    expect(cancelEvents).toEqual(["message.cancelled"]);

    const eofChat = createServerChatApiShell(
      createHttpClient({
        baseUrl: "http://backend.test",
        fetchImpl: async () =>
          new Response(
            [
              "event: message.started",
              'data: {"type":"message.started","runId":"run-1","conversationId":"c1","messageId":"m2","sequence":1}',
              "",
            ].join("\n"),
            { headers: { "content-type": "text/event-stream" } },
          ),
      }),
    );

    await expect(
      eofChat.streamAssistantMessage({
        conversationId: "c1",
        userMessageId: "m1",
        modelRef: { providerId: "openai", modelId: "gpt-5.5" },
        idempotencyKey: "stream-key",
      }),
    ).resolves.toMatchObject({
      status: "failed",
      error: { code: "STREAM_INTERRUPTED", recoverable: true },
    });
  });

  it("ignores duplicate stream sequences and fails on sequence gaps", async () => {
    const duplicateChat = createServerChatApiShell(
      createHttpClient({
        baseUrl: "http://backend.test",
        fetchImpl: async () =>
          new Response(
            [
              "event: message.started",
              'data: {"type":"message.started","runId":"run-1","conversationId":"c1","messageId":"m2","sequence":1}',
              "",
              "event: message.delta",
              'data: {"type":"message.delta","runId":"run-1","conversationId":"c1","messageId":"m2","sequence":2,"delta":"first"}',
              "",
              "event: message.delta",
              'data: {"type":"message.delta","runId":"run-1","conversationId":"c1","messageId":"m2","sequence":2,"delta":"duplicate"}',
              "",
              "event: message.completed",
              'data: {"type":"message.completed","runId":"run-1","conversationId":"c1","messageId":"m2","sequence":3,"message":{"id":"m2","conversationId":"c1","role":"assistant","status":"completed","content":"first","sequenceNo":2,"attachments":[],"outputBlocks":[],"metadata":{},"createdAt":"2026-07-08T00:00:02Z","updatedAt":"2026-07-08T00:00:02Z"}}',
              "",
            ].join("\n"),
            { headers: { "content-type": "text/event-stream" } },
          ),
      }),
    );
    const duplicateDeltas: string[] = [];

    await expect(
      duplicateChat.streamAssistantMessage(
        {
          conversationId: "c1",
          userMessageId: "m1",
          modelRef: { providerId: "openai", modelId: "gpt-5.5" },
          idempotencyKey: "stream-key",
        },
        {
          onDelta: (event) => duplicateDeltas.push(String(event.delta)),
        },
      ),
    ).resolves.toMatchObject({ status: "completed" });
    expect(duplicateDeltas).toEqual(["first"]);

    const gapChat = createServerChatApiShell(
      createHttpClient({
        baseUrl: "http://backend.test",
        fetchImpl: async () =>
          new Response(
            [
              "event: message.started",
              'data: {"type":"message.started","runId":"run-1","conversationId":"c1","messageId":"m2","sequence":1}',
              "",
              "event: message.delta",
              'data: {"type":"message.delta","runId":"run-1","conversationId":"c1","messageId":"m2","sequence":3,"delta":"gap"}',
              "",
            ].join("\n"),
            { headers: { "content-type": "text/event-stream" } },
          ),
      }),
    );

    await expect(
      gapChat.streamAssistantMessage({
        conversationId: "c1",
        userMessageId: "m1",
        modelRef: { providerId: "openai", modelId: "gpt-5.5" },
        idempotencyKey: "stream-key",
      }),
    ).resolves.toMatchObject({
      status: "failed",
      error: { code: "STREAM_INTERRUPTED", recoverable: true },
    });
  });

  it("cancels a server stream when aborted after run start", async () => {
    const abortController = new AbortController();
    const encoder = new TextEncoder();
    const requests: Array<{ url: string; method?: string }> = [];
    let streamChunkSent = false;
    const chat = createServerChatApiShell(
      createHttpClient({
        baseUrl: "http://backend.test",
        fetchImpl: async (input, init) => {
          requests.push({ url: String(input), method: init?.method });
          if (String(input).endsWith("/cancel")) {
            return Response.json({
              runId: "run-1",
              status: "cancelled",
              message: {
                id: "m2",
                conversationId: "c1",
                role: "assistant",
                status: "cancelled",
                content: "",
                sequenceNo: 2,
                attachments: [],
                outputBlocks: [],
                metadata: {},
                createdAt: "2026-07-08T00:00:00Z",
                updatedAt: "2026-07-08T00:00:00Z",
              },
            });
          }

          return new Response(
            new ReadableStream<Uint8Array>({
              pull(controller) {
                if (!streamChunkSent) {
                  streamChunkSent = true;
                  controller.enqueue(
                    encoder.encode(
                      [
                        "event: message.started",
                        'data: {"type":"message.started","runId":"run-1","conversationId":"c1","messageId":"m2","sequence":1}',
                        "",
                        "",
                      ].join("\n"),
                    ),
                  );
                  abortController.abort();
                  return;
                }
                controller.error(
                  new DOMException("Request was aborted.", "AbortError"),
                );
              },
            }),
            { headers: { "content-type": "text/event-stream" } },
          );
        },
      }),
    );

    await expect(
      chat.streamAssistantMessage({
        conversationId: "c1",
        userMessageId: "m1",
        modelRef: { providerId: "openai", modelId: "gpt-5.5" },
        idempotencyKey: "stream-key",
        signal: abortController.signal,
      }),
    ).resolves.toMatchObject({
      status: "cancelled",
      message: { id: "m2", status: "cancelled" },
    });
    expect(requests).toEqual([
      {
        url: "http://backend.test/v1/chat/conversations/c1/stream",
        method: "POST",
      },
      {
        url: "http://backend.test/v1/chat/runs/run-1/cancel",
        method: "POST",
      },
    ]);
  });

  it("cancels buffered streams when the caller aborts from started", async () => {
    const abortController = new AbortController();
    const requests: Array<{ url: string; method?: string }> = [];
    const events: string[] = [];
    const chat = createServerChatApiShell(
      createHttpClient({
        baseUrl: "http://backend.test",
        fetchImpl: async (input, init) => {
          requests.push({ url: String(input), method: init?.method });
          if (String(input).endsWith("/cancel")) {
            return Response.json({
              runId: "run-1",
              status: "cancelled",
              message: {
                id: "m2",
                conversationId: "c1",
                role: "assistant",
                status: "cancelled",
                content: "",
                sequenceNo: 2,
                attachments: [],
                outputBlocks: [],
                metadata: {},
                createdAt: "2026-07-08T00:00:00Z",
                updatedAt: "2026-07-08T00:00:00Z",
              },
            });
          }

          return new Response(
            [
              "event: message.started",
              'data: {"type":"message.started","runId":"run-1","conversationId":"c1","messageId":"m2","sequence":1}',
              "",
              "event: message.completed",
              'data: {"type":"message.completed","runId":"run-1","conversationId":"c1","messageId":"m2","sequence":2,"message":{"id":"m2","conversationId":"c1","role":"assistant","status":"completed","content":"done","sequenceNo":2,"attachments":[],"outputBlocks":[],"metadata":{},"createdAt":"2026-07-08T00:00:00Z","updatedAt":"2026-07-08T00:00:00Z"}}',
              "",
            ].join("\n"),
            { headers: { "content-type": "text/event-stream" } },
          );
        },
      }),
    );

    await expect(
      chat.streamAssistantMessage(
        {
          conversationId: "c1",
          userMessageId: "m1",
          modelRef: { providerId: "openai", modelId: "gpt-5.5" },
          idempotencyKey: "stream-key",
          signal: abortController.signal,
        },
        {
          onStarted: (event) => {
            events.push(event.type);
            abortController.abort();
          },
          onCompleted: (event) => events.push(event.type),
        },
      ),
    ).resolves.toMatchObject({
      status: "cancelled",
      message: { id: "m2", status: "cancelled" },
    });
    expect(events).toEqual(["message.started"]);
    expect(requests).toEqual([
      {
        url: "http://backend.test/v1/chat/conversations/c1/stream",
        method: "POST",
      },
      {
        url: "http://backend.test/v1/chat/runs/run-1/cancel",
        method: "POST",
      },
    ]);
  });

  it("cancels server runs through the Go cancel endpoint", async () => {
    const requests: Array<{ url: string; method?: string }> = [];
    const chat = createServerChatApiShell(
      createHttpClient({
        baseUrl: "http://backend.test",
        fetchImpl: async (input, init) => {
          requests.push({ url: String(input), method: init?.method });
          return Response.json({
            runId: "run/with slash",
            status: "cancelled",
            message: {
              id: "m2",
              conversationId: "c1",
              role: "assistant",
              status: "cancelled",
              content: "",
              sequenceNo: 2,
              attachments: [],
              outputBlocks: [],
              metadata: {},
              createdAt: "2026-07-08T00:00:00Z",
              updatedAt: "2026-07-08T00:00:00Z",
            },
          });
        },
      }),
    );

    await expect(chat.cancelRun("run/with slash")).resolves.toMatchObject({
      status: "cancelled",
      message: { id: "m2", status: "cancelled" },
    });
    expect(requests).toEqual([
      {
        url: "http://backend.test/v1/chat/runs/run%2Fwith%20slash/cancel",
        method: "POST",
      },
    ]);
  });
});

describe("Phase 11.1B Go SSE scaffold", () => {
  it("parses named SSE events with multi-line data", () => {
    const frames = parseSseFrames(
      [
        "event: message.delta",
        'data: {"type":"message.delta",',
        'data: "delta":"hel"}',
        "",
        "event: message.completed",
        'data: {"type":"message.completed","message":{"id":"m1","conversationId":"c1","role":"assistant","status":"completed","content":"hello","sequenceNo":2,"createdAt":"2026-07-08T00:00:00Z","updatedAt":"2026-07-08T00:00:00Z"}}',
        "",
      ].join("\n"),
    );

    expect(frames).toHaveLength(2);
    expect(frames[0]?.event).toBe("message.delta");
    expect(frames[0]?.data.delta).toBe("hel");
    expect(frames[1]?.data.message?.role).toBe("assistant");
  });

  it("preserves CRLF line endings split across chunks", () => {
    const parser = createSseStreamParser();

    expect(parser.push("event: message.delta\r")).toEqual([]);
    expect(
      parser.push(
        '\ndata: {"type":"message.delta","runId":"run-1","sequence":1,"delta":"hel"}\r',
      ),
    ).toEqual([]);

    const frames = parser.push("\n\r\n");

    expect(frames).toHaveLength(1);
    expect(frames[0]?.event).toBe("message.delta");
    expect(frames[0]?.data.delta).toBe("hel");
    expect(parser.flush()).toEqual([]);
  });

  it("fails closed when event and data.type disagree", () => {
    expect(() =>
      parseSseFrames(
        'event: message.delta\ndata: {"type":"message.started"}\n\n',
      ),
    ).toThrow(ApiClientError);
  });
});
