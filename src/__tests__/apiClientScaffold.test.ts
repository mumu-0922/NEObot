import { describe, expect, it } from "vitest";
import {
  ApiClientError,
  createHttpClient,
  createNeoChatApiClient,
  inferNetworkEdge,
  joinUrl,
  parseSseFrames,
  resolveApiClientConfig,
} from "../services/api/client";
import { createServerChatApiShell } from "../services/api/client/server/chatApi";

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
    });
  });

  it("enables only chat CRUD capability for configured server mode", () => {
    const client = createNeoChatApiClient({
      env: {
        NEXT_PUBLIC_API_MODE: "server",
        NEXT_PUBLIC_API_BASE_URL: "http://127.0.0.1:8080",
      },
    });

    expect(client.mode).toBe("server");
    expect(client.capabilities).toMatchObject({
      chatCrud: true,
      chatStream: false,
      files: false,
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

  it("fails closed when event and data.type disagree", () => {
    expect(() =>
      parseSseFrames(
        'event: message.delta\ndata: {"type":"message.started"}\n\n',
      ),
    ).toThrow(ApiClientError);
  });
});
