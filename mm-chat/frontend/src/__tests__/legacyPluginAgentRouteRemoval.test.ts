import { existsSync, readFileSync } from "node:fs";
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
  it("removes the retired frontend plugin runtime", () => {
    const retiredPluginSources = [
      "src/services/api/client/local/pluginApi.ts",
      "src/services/api/client/server/pluginApi.ts",
      "src/services/api/pluginService.ts",
      "src/services/api/serverPluginOrchestration.ts",
      "src/utils/pluginUtils.ts",
      "src/components/plugin/PluginMarket.tsx",
      "src/lib/plugin",
      "src/config/plugins.ts",
    ];

    for (const source of retiredPluginSources) {
      expect(existsSync(resolve(process.cwd(), source))).toBe(false);
    }
  });

  it("keeps active frontend sources from calling deleted Next routes", () => {
    const checkedSources = [
      "src/config/api.ts",
      "src/services/api/client/local/agentApi.ts",
      "src/services/api/client/server/agentApi.ts",
      "src/services/api/agentService.ts",
      "src/components/assistant/AssistantHub.tsx",
    ].map(readSource);

    for (const source of checkedSources) {
      for (const route of removedRoutes) {
        expect(source).not.toContain(route);
      }
    }
  });

  it("makes the local agent adapter fail closed without fetch", async () => {
    const { createLocalAgentApiShell } =
      await import("../services/api/client/local/agentApi");
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
    expect(fetchSpy).not.toHaveBeenCalled();
  });
});
