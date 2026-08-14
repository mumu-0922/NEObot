import { describe, expect, it } from "vitest";

import { useSettingsStore } from "../store/core/settingsStore";
import { STORAGE_VERSION } from "../store/storage/storageConfig";

describe("Assistant Store browser authority migration", () => {
  it("advances persistence and hard-deletes every legacy Assistant authority", async () => {
    expect(STORAGE_VERSION).toBe(7);
    const migrate = (
      useSettingsStore as unknown as {
        persist: {
          getOptions(): {
            migrate?: (state: unknown, version: number) => Promise<unknown>;
          };
        };
      }
    ).persist.getOptions().migrate;
    expect(migrate).toBeTypeOf("function");

    const migrated = (await migrate!(
      {
        customAgents: [{ identifier: "custom-writer" }],
        usedAgents: [{ identifier: "used-writer" }],
        agentOverrides: { "store-writer": { meta: { title: "Override" } } },
        marketAgents: [{ identifier: "cached-writer" }],
        marketAgentsTimestamp: 123,
        marketAgentsLocale: "zh",
      },
      5,
    )) as Record<string, unknown>;

    expect(migrated).toMatchObject({
      customAgents: [],
      usedAgents: [],
      agentOverrides: {},
      marketAgents: [],
      marketAgentsTimestamp: 0,
      marketAgentsLocale: "",
    });
  });

  it("never persists legacy Assistant values again", () => {
    const partialize = (
      useSettingsStore as unknown as {
        persist: {
          getOptions(): {
            partialize?: (state: unknown) => Record<string, unknown>;
          };
        };
      }
    ).persist.getOptions().partialize;
    const persisted = partialize!(useSettingsStore.getState());

    expect(persisted).not.toHaveProperty("customAgents");
    expect(persisted).not.toHaveProperty("usedAgents");
    expect(persisted).not.toHaveProperty("agentOverrides");
    expect(persisted).not.toHaveProperty("marketAgents");
  });
});
