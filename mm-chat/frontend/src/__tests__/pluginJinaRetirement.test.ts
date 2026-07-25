import { describe, expect, it, vi } from "vitest";
import { executePluginFunctionRequest } from "../lib/plugin/pluginExecutionExecutor";
import type { Plugin, PluginFunction } from "../types";

vi.mock("server-only", () => ({}));

const functionDef: PluginFunction = {
  name: "read_webpage",
  description: "Read a page",
  method: "GET",
  path: "/{url}",
  parameters: {
    type: "object",
    properties: { url: { type: "string" } },
    required: ["url"],
  },
};

function plugin(id: string, baseUrl: string): Plugin {
  return {
    id,
    title: id,
    description: "",
    logoUrl: "",
    manifestUrl: "",
    baseUrl,
    functions: [functionDef],
    auth: { type: "none" },
  };
}

describe("retired Jina plugin execution fence", () => {
  it.each([
    plugin("jina-web-reader", "https://reader.example.com"),
    plugin("custom-reader", "https://r.jina.ai"),
    plugin("custom-reader", "https://API.JINA.AI./v1"),
  ])("rejects $id before credential resolution or fetch", async (retired) => {
    const decryptSecret = vi.fn();
    const fetchText = vi.fn();

    const response = await executePluginFunctionRequest({
      plugin: retired,
      functionDef,
      args: { url: "https://example.com" },
      decryptSecret,
      fetchText,
    });

    expect(response.status).toBe(410);
    await expect(response.json()).resolves.toEqual({
      error: "Jina-backed plugins are permanently retired",
    });
    expect(decryptSecret).not.toHaveBeenCalled();
    expect(fetchText).not.toHaveBeenCalled();
  });
});
