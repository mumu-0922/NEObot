import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import en from "../i18n/locales/en";
import zh from "../i18n/locales/zh";
import { describe, expect, it } from "vitest";

describe("settings deployment health panel", () => {
  it("is reachable from settings navigation with bilingual copy", () => {
    const settingsPage = readFileSync(
      resolve(process.cwd(), "src/components/settings/SettingsPage.tsx"),
      "utf8",
    );

    expect(settingsPage).toContain("DeploymentHealth");
    expect(settingsPage).toContain("tabHealth");
    expect(en.SettingsPage.tabHealth).toBeTruthy();
    expect(zh.SettingsPage.tabHealth).toBeTruthy();
    expect(en.DeploymentHealth.title).toBeTruthy();
    expect(zh.DeploymentHealth.title).toBeTruthy();
  });

  it("supports controlled settings tabs for deep links", () => {
    const settingsPage = readFileSync(
      resolve(process.cwd(), "src/components/settings/SettingsPage.tsx"),
      "utf8",
    );

    expect(settingsPage).toContain("activeTab?: SettingsTabId");
    expect(settingsPage).toContain(
      "onTabChange?: (tab: SettingsTabId) => void",
    );
    expect(settingsPage).toContain("resolvedActiveTab");
  });

  it("derives Knowledge health from server provider status only", () => {
    const deploymentHealth = readFileSync(
      resolve(process.cwd(), "src/components/settings/DeploymentHealth.tsx"),
      "utf8",
    );

    expect(deploymentHealth).toContain("getRAGProviderStatus");
    expect(deploymentHealth).toContain("ragHealthState");
    expect(deploymentHealth).toContain('t("ragPartial")');
    expect(deploymentHealth).toContain('t("ragUnavailable")');
    expect(deploymentHealth).not.toContain("serverConfig?.rag");
    expect(deploymentHealth).not.toContain("rag.url");
    expect(deploymentHealth).not.toContain("llamaParseApiKey");
  });

  it("keeps the existing settings layout while tightening navigation and content width", () => {
    const settingsPage = readFileSync(
      resolve(process.cwd(), "src/components/settings/SettingsPage.tsx"),
      "utf8",
    );

    expect(settingsPage).toContain("md:w-60");
    expect(settingsPage).toContain("max-w-5xl");
    expect(settingsPage).toContain("bg-background/95");
    expect(settingsPage).not.toContain("backdrop-blur-md");
    expect(settingsPage).toContain("border-l-2");
    expect(settingsPage).not.toContain("bg-blue-50 text-blue-600 shadow-sm");
  });
});
