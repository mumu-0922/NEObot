import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it, vi } from "vitest";

const removedRoutes = [
  "/api/byok/public-key",
  "/api/config",
  "/api/providers/models",
] as const;

function readSource(path: string): string {
  return readFileSync(resolve(process.cwd(), path), "utf8");
}

describe("G9.3 config/provider/BYOK route removal", () => {
  it("keeps frontend API clients and callers from calling deleted Next routes", () => {
    const checkedSources = [
      "src/services/api/client/local/settingsApi.ts",
      "src/services/api/client/local/providerApi.ts",
      "src/services/api/client/local/byokApi.ts",
      "src/services/api/client/server/settingsApi.ts",
      "src/services/api/client/server/providerApi.ts",
      "src/services/api/client/server/byokApi.ts",
      "src/lib/byok/client.ts",
      "src/components/app/ChatApp.tsx",
      "src/components/settings/ProviderSettings.tsx",
    ].map(readSource);

    for (const source of checkedSources) {
      for (const route of removedRoutes) {
        expect(source).not.toContain(route);
      }
    }
  });

  it("makes local config/provider/BYOK adapters fail closed without fetch", async () => {
    const { createLocalSettingsApiShell } =
      await import("../services/api/client/local/settingsApi");
    const { createLocalProviderApiShell } =
      await import("../services/api/client/local/providerApi");
    const { createLocalByokApiShell } =
      await import("../services/api/client/local/byokApi");
    const fetchSpy = vi.spyOn(globalThis, "fetch");

    await expect(
      createLocalSettingsApiShell().getRuntimeConfig(),
    ).rejects.toMatchObject({ code: "FEATURE_NOT_IMPLEMENTED" });
    await expect(
      createLocalProviderApiShell().listModels({
        provider: { type: "OpenAI", source: "server-default" },
      }),
    ).rejects.toMatchObject({ code: "FEATURE_NOT_IMPLEMENTED" });
    await expect(
      createLocalByokApiShell().getPublicKey(),
    ).rejects.toMatchObject({ code: "FEATURE_NOT_IMPLEMENTED" });

    expect(fetchSpy).not.toHaveBeenCalled();
  });
});
