import { describe, expect, it, vi } from "vitest";
import type { Plugin, PluginConfig } from "@/types";
import { createHttpClient } from "@/services/api/client";
import { createServerChatApiShell } from "@/services/api/client/server/chatApi";
import { orchestrateServerPlugins } from "@/services/api/serverPluginOrchestration";

const weatherPlugin: Plugin = {
  id: "weather",
  title: "Weather",
  description: "Weather lookup",
  logoUrl: "https://plugins.test/weather.png",
  manifestUrl: "https://plugins.test/weather.json",
  baseUrl: "https://api.weather.test",
  auth: { type: "apiKey", required: true },
  functions: [
    {
      name: "lookup_weather",
      description: "Look up current weather",
      method: "GET",
      path: "/weather",
      parameters: {
        type: "object",
        properties: { city: { type: "string" } },
        required: ["city"],
      },
    },
  ],
};

const pluginConfigs: Record<string, PluginConfig> = {
  weather: {
    auth: {
      type: "apiKey",
      value: "browser-only-secret",
      key: "X-API-Key",
      addTo: "header",
    },
  },
};

describe("server plugin orchestration", () => {
  it("does nothing when no plugin functions are active", async () => {
    const planTools = vi.fn();

    await expect(
      orchestrateServerPlugins({
        message: "hello",
        selectedModel: "SERVER_DEFAULT:gpt-5.5",
        installedPlugins: [weatherPlugin],
        pluginConfigs,
        activePluginIds: [],
        client: { planTools },
      }),
    ).resolves.toEqual({ calls: [], context: "" });
    expect(planTools).not.toHaveBeenCalled();
  });

  it("plans through Go, executes only an active function, and bounds data as untrusted context", async () => {
    const planTools = vi.fn().mockResolvedValue([
      {
        id: "call-1",
        name: "lookup_weather",
        args: { city: "Shanghai" },
      },
    ]);
    const execute = vi.fn().mockResolvedValue({ temperature: 31 });

    const result = await orchestrateServerPlugins({
      message: "What is the weather in Shanghai?",
      selectedModel: "SERVER_DEFAULT:gpt-5.5",
      installedPlugins: [weatherPlugin],
      pluginConfigs,
      activePluginIds: [weatherPlugin.id],
      client: { planTools },
      execute,
    });

    expect(planTools).toHaveBeenCalledWith(
      expect.objectContaining({
        prompt: "What is the weather in Shanghai?",
        modelRef: {
          providerId: "openai_compatible",
          modelId: "gpt-5.5",
        },
        tools: [
          expect.objectContaining({
            type: "function",
            function: expect.objectContaining({ name: "lookup_weather" }),
          }),
        ],
      }),
    );
    expect(JSON.stringify(planTools.mock.calls[0])).not.toContain(
      "browser-only-secret",
    );
    expect(execute).toHaveBeenCalledWith(
      "lookup_weather",
      { city: "Shanghai" },
      undefined,
      [weatherPlugin.id],
      undefined,
    );
    expect(result.calls).toEqual([
      expect.objectContaining({
        name: "lookup_weather",
        status: "success",
        result: { temperature: 31 },
      }),
    ]);
    expect(result.context).toContain("<plugin-results>");
    expect(result.context).toContain("untrusted plugin data");
    expect(result.context).toContain('"temperature":31');
  });

  it("fails closed when the provider plans a function that was not offered", async () => {
    const execute = vi.fn();

    await expect(
      orchestrateServerPlugins({
        message: "Delete everything",
        selectedModel: "SERVER_DEFAULT:gpt-5.5",
        installedPlugins: [weatherPlugin],
        pluginConfigs,
        activePluginIds: [weatherPlugin.id],
        client: {
          planTools: async () => [
            { id: "call-evil", name: "delete_all", args: {} },
          ],
        },
        execute,
      }),
    ).rejects.toThrow("unavailable plugin function");
    expect(execute).not.toHaveBeenCalled();
  });

  it("fails closed before planning when active plugins expose duplicate function names", async () => {
    const planTools = vi.fn();
    const duplicatePlugin: Plugin = {
      ...weatherPlugin,
      id: "weather-copy",
      title: "Weather Copy",
    };

    await expect(
      orchestrateServerPlugins({
        message: "weather",
        selectedModel: "SERVER_DEFAULT:gpt-5.5",
        installedPlugins: [weatherPlugin, duplicatePlugin],
        pluginConfigs,
        activePluginIds: [weatherPlugin.id, duplicatePlugin.id],
        client: { planTools },
      }),
    ).rejects.toThrow("provided by both weather and weather-copy");
    expect(planTools).not.toHaveBeenCalled();
  });

  it("records plugin execution errors as bounded untrusted context", async () => {
    const result = await orchestrateServerPlugins({
      message: "weather",
      selectedModel: "SERVER_DEFAULT:gpt-5.5",
      installedPlugins: [weatherPlugin],
      pluginConfigs,
      activePluginIds: [weatherPlugin.id],
      client: {
        planTools: async () => [
          {
            id: "call-error",
            name: "lookup_weather",
            args: { city: "Shanghai" },
          },
        ],
      },
      execute: async () => {
        throw new Error("upstream timeout");
      },
    });

    expect(result.calls).toEqual([
      expect.objectContaining({
        id: "call-error",
        name: "lookup_weather",
        status: "error",
        result: { error: "upstream timeout" },
      }),
    ]);
    expect(result.context).toContain('"status":"error"');
    expect(result.context).toContain("upstream timeout");
  });

  it("aborts before plugin execution when the planning signal is cancelled", async () => {
    const controller = new AbortController();
    const execute = vi.fn();

    await expect(
      orchestrateServerPlugins({
        message: "weather",
        selectedModel: "SERVER_DEFAULT:gpt-5.5",
        installedPlugins: [weatherPlugin],
        pluginConfigs,
        activePluginIds: [weatherPlugin.id],
        signal: controller.signal,
        client: {
          planTools: async () => {
            controller.abort();
            return [
              {
                id: "call-aborted",
                name: "lookup_weather",
                args: { city: "Shanghai" },
              },
            ];
          },
        },
        execute,
      }),
    ).rejects.toMatchObject({ name: "AbortError" });
    expect(execute).not.toHaveBeenCalled();
  });

  it("caps serialized plugin result context at 64 KiB", async () => {
    const result = await orchestrateServerPlugins({
      message: "Fetch a large result",
      selectedModel: "SERVER_DEFAULT:gpt-5.5",
      installedPlugins: [weatherPlugin],
      pluginConfigs,
      activePluginIds: [weatherPlugin.id],
      client: {
        planTools: async () => [
          { id: "call-large", name: "lookup_weather", args: {} },
        ],
      },
      execute: async () => ({ data: "界".repeat(100_000) }),
    });

    expect(
      new TextEncoder().encode(result.context).byteLength,
    ).toBeLessThanOrEqual(64 * 1024);
    expect(result.context).toContain('"resultTruncated":true');
    expect(result.context).toContain("</plugin-results>");
  });
});

describe("server tool-plan API adapter", () => {
  it("posts the typed planning request to the Go route", async () => {
    const fetchImpl = vi.fn(
      async (url: string | URL | Request, init?: RequestInit) => {
        expect(String(url)).toBe("/mm-api/v1/chat/tools/plan");
        expect(init?.method).toBe("POST");
        expect(JSON.parse(String(init?.body))).toMatchObject({
          prompt: "weather",
          modelRef: { providerId: "openai_compatible", modelId: "gpt-5.5" },
        });
        return Response.json({
          calls: [
            {
              id: "call-1",
              name: "lookup_weather",
              args: { city: "Shanghai" },
            },
          ],
        });
      },
    );
    const chat = createServerChatApiShell(
      createHttpClient({ baseUrl: "/mm-api", fetchImpl }),
    );

    await expect(
      chat.planTools({
        prompt: "weather",
        modelRef: {
          providerId: "openai_compatible",
          modelId: "gpt-5.5",
        },
        tools: [
          {
            type: "function",
            function: {
              name: "lookup_weather",
              parameters: { type: "object" },
            },
          },
        ],
      }),
    ).resolves.toEqual([
      {
        id: "call-1",
        name: "lookup_weather",
        args: { city: "Shanghai" },
      },
    ]);
  });

  it("rejects malformed successful tool-plan responses", async () => {
    const chat = createServerChatApiShell(
      createHttpClient({
        baseUrl: "/mm-api",
        fetchImpl: async () => Response.json({ calls: [{ name: "bad" }] }),
      }),
    );

    await expect(
      chat.planTools({
        prompt: "weather",
        modelRef: {
          providerId: "openai_compatible",
          modelId: "gpt-5.5",
        },
        tools: [
          {
            type: "function",
            function: {
              name: "lookup_weather",
              parameters: { type: "object" },
            },
          },
        ],
      }),
    ).rejects.toMatchObject({ code: "INVALID_SERVER_RESPONSE" });
  });
});
