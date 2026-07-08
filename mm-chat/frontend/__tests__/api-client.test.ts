import { describe, expect, it } from "vitest";
import {
  ApiClientError,
  createHttpClient,
  createNeoChatApiClient,
  inferNetworkEdge,
  joinUrl,
  parseSseFrames,
  resolveApiClientConfig,
} from "../src/api-client";

describe("Phase 11.1 API mode resolver", () => {
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

describe("Phase 11.1 server HTTP scaffold", () => {
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

describe("Phase 11.1 Go SSE scaffold", () => {
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
