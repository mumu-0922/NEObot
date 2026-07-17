import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

describe("settings UI primitives", () => {
  it("uses shadcn-style semantic tokens for select and switch controls", () => {
    const settingsUi = readFileSync(
      resolve(process.cwd(), "src/components/settings/SettingsUI.tsx"),
      "utf8",
    );

    expect(settingsUi).toContain("bg-background");
    expect(settingsUi).toContain("border-input");
    expect(settingsUi).toContain("focus-visible:ring-ring");
    expect(settingsUi).toContain("data-[state=checked]");
  });

  it("exposes memory management as a settings tab", () => {
    const settingsPage = readFileSync(
      resolve(process.cwd(), "src/components/settings/SettingsPage.tsx"),
      "utf8",
    );

    expect(settingsPage).toContain('id: "memory"');
    expect(settingsPage).toContain("tabMemory");
  });

  it("keeps the standalone settings UI single-user without Team controls", () => {
    const settingsPage = readFileSync(
      resolve(process.cwd(), "src/components/settings/SettingsPage.tsx"),
      "utf8",
    );
    const panelUrlState = readFileSync(
      resolve(process.cwd(), "src/lib/chat/panelUrlState.ts"),
      "utf8",
    );

    expect(settingsPage).not.toContain("TeamSettings");
    expect(settingsPage).not.toContain('id: "teams"');
    expect(settingsPage).not.toContain("tabTeams");
    expect(panelUrlState).not.toContain('"teams"');
  });
  it("wires Server Default provider settings to backend admin config", () => {
    const providerSettings = readFileSync(
      resolve(process.cwd(), "src/components/settings/ProviderSettings.tsx"),
      "utf8",
    );

    expect(providerSettings).toContain("listAdminProviderConfigs");
    expect(providerSettings).toContain("updateServerDefaultConfig");
    expect(providerSettings).toContain("updateAdminProviderConfig");
    expect(providerSettings).toContain("deleteAdminProviderConfig");
    expect(providerSettings).toContain("encryptSecret");
    expect(providerSettings).toContain("BYOK_CONTEXTS");
    expect(providerSettings).toContain("provider(");
    expect(providerSettings).toContain("keyStoredOnServer");
  });
});
