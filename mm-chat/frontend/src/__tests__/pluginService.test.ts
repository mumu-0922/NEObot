import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Plugin } from "../types";

const storeMock = vi.hoisted(() => ({
  state: {} as {
    marketPlugins: Plugin[];
    marketPluginsTimestamp: number;
    setMarketPlugins: ReturnType<typeof vi.fn>;
  },
}));

vi.mock("@/store/core/settingsStore", () => ({
  useSettingsStore: {
    getState: () => storeMock.state,
  },
}));

vi.mock("../lib/utils/devLogger", () => ({
  logDevError: vi.fn(),
  logDevInfo: vi.fn(),
  logDevWarn: vi.fn(),
}));

const pluginA: Plugin = {
  id: "example.com:alpha",
  title: "Alpha",
  description: "Alpha plugin",
  logoUrl: "",
  manifestUrl: "https://example.com/alpha.json",
  functions: [],
};

const pluginB: Plugin = {
  id: "example.com:beta",
  title: "Beta",
  description: "Beta plugin",
  logoUrl: "",
  manifestUrl: "https://example.com/beta.json",
  functions: [],
};

const jsonResponse = (body: unknown, init?: ResponseInit) =>
  new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
    ...init,
  });

describe("plugin market service cache", () => {
  let previousApiMode: string | undefined;
  let previousApiBaseUrl: string | undefined;

  beforeEach(() => {
    vi.resetModules();
    previousApiMode = process.env.NEXT_PUBLIC_API_MODE;
    previousApiBaseUrl = process.env.NEXT_PUBLIC_API_BASE_URL;
    delete process.env.NEXT_PUBLIC_API_MODE;
    delete process.env.NEXT_PUBLIC_API_BASE_URL;
    storeMock.state = {
      marketPlugins: [],
      marketPluginsTimestamp: 0,
      setMarketPlugins: vi.fn((plugins: Plugin[]) => {
        storeMock.state.marketPlugins = plugins;
        storeMock.state.marketPluginsTimestamp = Date.now();
      }),
    };
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    if (previousApiMode === undefined) {
      delete process.env.NEXT_PUBLIC_API_MODE;
    } else {
      process.env.NEXT_PUBLIC_API_MODE = previousApiMode;
    }
    if (previousApiBaseUrl === undefined) {
      delete process.env.NEXT_PUBLIC_API_BASE_URL;
    } else {
      process.env.NEXT_PUBLIC_API_BASE_URL = previousApiBaseUrl;
    }
  });

  it("returns valid cached plugins without fetching", async () => {
    storeMock.state.marketPlugins = [pluginA];
    storeMock.state.marketPluginsTimestamp = Date.now();
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    const { fetchApiGuruList } = await import("../services/api/pluginService");
    const plugins = await fetchApiGuruList();

    expect(plugins).toHaveLength(1);
    expect(plugins[0]).toMatchObject(pluginA);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("keeps cached plugins fresh for 72 hours", async () => {
    storeMock.state.marketPlugins = [pluginA];
    storeMock.state.marketPluginsTimestamp = Date.now() - 48 * 60 * 60 * 1000;
    const fetchMock = vi.fn(async () => jsonResponse({ plugins: [pluginB] }));
    vi.stubGlobal("fetch", fetchMock);

    const { fetchApiGuruList } = await import("../services/api/pluginService");
    const plugins = await fetchApiGuruList();

    expect(plugins).toHaveLength(1);
    expect(plugins[0]).toMatchObject(pluginA);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("refreshes cached plugins after 72 hours", async () => {
    storeMock.state.marketPlugins = [pluginA];
    storeMock.state.marketPluginsTimestamp = Date.now() - 73 * 60 * 60 * 1000;
    const fetchMock = vi.fn(async () => jsonResponse({ plugins: [pluginB] }));
    vi.stubGlobal("fetch", fetchMock);

    const { fetchApiGuruList } = await import("../services/api/pluginService");
    const plugins = await fetchApiGuruList();

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/plugins/list",
      expect.objectContaining({ method: "GET", cache: "no-store" }),
    );
    expect(plugins).toHaveLength(1);
    expect(plugins[0]).toMatchObject(pluginB);
  });

  it("force refresh bypasses cache and stores fresh plugins", async () => {
    storeMock.state.marketPlugins = [pluginA];
    storeMock.state.marketPluginsTimestamp = Date.now();
    const fetchMock = vi.fn(async () => jsonResponse({ plugins: [pluginB] }));
    vi.stubGlobal("fetch", fetchMock);

    const { fetchApiGuruList } = await import("../services/api/pluginService");
    const plugins = await fetchApiGuruList(true);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/plugins/list",
      expect.objectContaining({ method: "GET", cache: "no-store" }),
    );
    expect(plugins).toHaveLength(1);
    expect(plugins[0]).toMatchObject(pluginB);
    expect(storeMock.state.setMarketPlugins).toHaveBeenCalledWith([
      expect.objectContaining(pluginB),
    ]);
  });

  it("falls back to stale cache when refreshing fails", async () => {
    storeMock.state.marketPlugins = [pluginA];
    storeMock.state.marketPluginsTimestamp = Date.now() - 25 * 60 * 60 * 1000;
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse({ error: "failed" }, { status: 500 })),
    );

    const { fetchApiGuruList } = await import("../services/api/pluginService");
    const plugins = await fetchApiGuruList();

    expect(plugins).toHaveLength(1);
    expect(plugins[0]).toMatchObject(pluginA);
  });

  it("reuses an in-flight plugin list request", async () => {
    let resolveFetch: ((response: Response) => void) | undefined;
    const fetchMock = vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          resolveFetch = resolve;
        }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const { fetchApiGuruList } = await import("../services/api/pluginService");
    const first = fetchApiGuruList();
    const second = fetchApiGuruList();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    resolveFetch?.(jsonResponse({ plugins: [pluginA] }));

    await expect(first).resolves.toEqual([expect.objectContaining(pluginA)]);
    await expect(second).resolves.toEqual([expect.objectContaining(pluginA)]);
  });

  it("routes server-mode plugin lists through the Go adapter and degrades when registry is unavailable", async () => {
    process.env.NEXT_PUBLIC_API_MODE = "server";
    process.env.NEXT_PUBLIC_API_BASE_URL = "/mm-api";
    const fetchMock = vi.fn(async () =>
      jsonResponse(
        { error: { code: "NOT_FOUND", message: "route not found" } },
        { status: 404 },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const { fetchApiGuruList } = await import("../services/api/pluginService");
    const plugins = await fetchApiGuruList(true);

    expect(fetchMock).toHaveBeenCalledWith(
      "/mm-api/v1/plugins",
      expect.objectContaining({ method: "GET" }),
    );
    expect(plugins).toEqual([]);
    expect(storeMock.state.setMarketPlugins).toHaveBeenCalledWith([]);
  });

  it("installs marketplace and custom plugins through the local API adapter", async () => {
    const fetchMock = vi.fn(async (_url: string, init?: RequestInit) => {
      const body = JSON.parse(String(init?.body));
      return jsonResponse({
        plugin: body.plugin ?? {
          ...pluginB,
          manifestUrl: body.customInput,
        },
      });
    });
    vi.stubGlobal("fetch", fetchMock);

    const { installPlugin, installCustomPlugin } =
      await import("../services/api/pluginService");

    await expect(installPlugin(pluginA)).resolves.toMatchObject(pluginA);
    await expect(
      installCustomPlugin("https://example.com/custom.json"),
    ).resolves.toMatchObject({
      manifestUrl: "https://example.com/custom.json",
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/plugins/install",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ plugin: pluginA }),
      }),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/plugins/install",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({
          customInput: "https://example.com/custom.json",
        }),
      }),
    );
  });

  it("does not fall back to the Next install route when server-mode install is unavailable", async () => {
    process.env.NEXT_PUBLIC_API_MODE = "server";
    process.env.NEXT_PUBLIC_API_BASE_URL = "/mm-api";
    const fetchMock = vi.fn(async () =>
      jsonResponse(
        { error: { code: "NOT_FOUND", message: "route not found" } },
        { status: 404 },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const { installCustomPlugin } =
      await import("../services/api/pluginService");

    await expect(
      installCustomPlugin("https://example.com/custom.json"),
    ).rejects.toMatchObject({ code: "PLUGIN_INSTALL_UNAVAILABLE" });
    expect(fetchMock).toHaveBeenCalledWith(
      "/mm-api/v1/plugins/install",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({
          customInput: "https://example.com/custom.json",
        }),
      }),
    );
    const calledUrls = fetchMock.mock.calls.map((call) =>
      String((call as unknown[])[0]),
    );
    expect(calledUrls).not.toContain("/api/plugins/install");
  });
});
