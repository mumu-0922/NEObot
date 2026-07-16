import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it, vi } from "vitest";

const removedRoutes = [
  "/api/agents",
  "/api/plugins/execute",
  "/api/plugins/install",
  "/api/plugins/list",
] as const;

function readSource(path: string): string {
  return readFileSync(resolve(process.cwd(), path), "utf8");
}

describe("G9.4 plugin/agent route removal", () => {
  it("keeps frontend API clients and services from calling deleted Next routes", () => {
    const checkedSources = [
      "src/config/api.ts",
      "src/services/api/client/local/agentApi.ts",
      "src/services/api/client/local/pluginApi.ts",
      "src/services/api/client/server/agentApi.ts",
      "src/services/api/client/server/pluginApi.ts",
      "src/services/api/agentService.ts",
      "src/services/api/pluginService.ts",
      "src/utils/pluginUtils.ts",
      "src/components/assistant/AssistantHub.tsx",
      "src/components/plugin/PluginMarket.tsx",
    ].map(readSource);

    for (const source of checkedSources) {
      for (const route of removedRoutes) {
        expect(source).not.toContain(route);
      }
    }
  });

  it("makes local plugin and agent adapters fail closed without fetch", async () => {
    const { createLocalAgentApiShell } =
      await import("../services/api/client/local/agentApi");
    const { createLocalPluginApiShell } =
      await import("../services/api/client/local/pluginApi");
    const fetchSpy = vi.spyOn(globalThis, "fetch");

    await expect(
      createLocalAgentApiShell().listAgents({ locale: "en" }),
    ).rejects.toMatchObject({ code: "FEATURE_NOT_IMPLEMENTED" });
    await expect(
      createLocalAgentApiShell().getAgentDetail({
        identifier: "agent-1",
        locale: "en",
      }),
    ).rejects.toMatchObject({ code: "FEATURE_NOT_IMPLEMENTED" });
    await expect(
      createLocalPluginApiShell().listAvailable(),
    ).rejects.toMatchObject({ code: "FEATURE_NOT_IMPLEMENTED" });
    await expect(
      createLocalPluginApiShell().install({ customInput: "{}" }),
    ).rejects.toMatchObject({ code: "FEATURE_NOT_IMPLEMENTED" });
    await expect(
      createLocalPluginApiShell().execute({
        payload: { pluginId: "plugin-1", functionName: "lookup", args: {} },
      }),
    ).rejects.toMatchObject({ code: "FEATURE_NOT_IMPLEMENTED" });

    expect(fetchSpy).not.toHaveBeenCalled();
  });
});
