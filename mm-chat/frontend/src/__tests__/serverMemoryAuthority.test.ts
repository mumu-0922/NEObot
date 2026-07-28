import fs from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";

const root = process.cwd();

describe("server memory authority composition", () => {
  it("uses Go memory APIs in settings and blocks IndexedDB prompt injection in server mode", () => {
    const settings = fs.readFileSync(
      path.join(root, "src/components/settings/ServerMemoryGovernance.tsx"),
      "utf8",
    );
    const settingsRouter = fs.readFileSync(
      path.join(root, "src/components/settings/MemorySettings.tsx"),
      "utf8",
    );
    const chatApp = fs.readFileSync(
      path.join(root, "src/components/app/ChatApp.tsx"),
      "utf8",
    );

    expect(settings).toContain("apiClient.memories.getGovernance");
    expect(settings).toContain("apiClient.memories.createGovernanceMemory");
    expect(settings).toContain("apiClient.memories.updateGovernanceMemory");
    expect(settings).toContain("apiClient.memories.deleteGovernanceMemory");
    expect(settings).toContain("apiClient.memories.decideMemoryReview");
    expect(settingsRouter).toContain("<ServerMemoryGovernance");
    expect(settingsRouter).toContain("<LocalMemorySettings");
    expect(chatApp).toMatch(
      /const directMemoryContext =\s*!serverModeEnabled &&/,
    );
  });
});
