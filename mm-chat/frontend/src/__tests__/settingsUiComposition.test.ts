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
    expect(providerSettings).toContain("testAdminProviderConnection");
    expect(providerSettings).toContain("activateAdminProvider");
    expect(providerSettings).toContain("queueServerProviderPersist");
    expect(providerSettings).toContain("providerPersistQueueRef");
    expect(providerSettings).toContain("toolCapabilityDefault");
    expect(providerSettings).toContain("toolCapabilityModelOverrides");
    expect(providerSettings).toContain("selectedToolCapabilityModelOverrides");
    expect(providerSettings).toContain(
      "handleToolCapabilityModelOverrideChange",
    );
    expect(providerSettings).toContain("newModels.includes(candidate)");
    expect(providerSettings).toContain('<option value="inherit">');
    expect(providerSettings).toContain("currentProvider.isServerManaged");
    expect(providerSettings).toContain("onBlur={() =>");
    expect(providerSettings).not.toContain("models: response.models");
    expect(providerSettings).toContain("encryptSecret");
    expect(providerSettings).toContain("BYOK_CONTEXTS");
    expect(providerSettings).toContain("provider(");
    expect(providerSettings).toContain("keyStoredOnServer");
    expect(providerSettings).toContain('<option value="Anthropic">');
    expect(providerSettings).not.toContain('<option value="DeepSeek">');
    expect(providerSettings).not.toContain('<option value="OpenRouter">');
  });

  it("wires Search provider settings to backend admin config and BYOK", () => {
    const searchSettings = readFileSync(
      resolve(process.cwd(), "src/components/settings/SearchSettings.tsx"),
      "utf8",
    );

    expect(searchSettings).toContain("listAdminSearchProviderConfigs");
    expect(searchSettings).toContain("updateAdminSearchProviderConfig");
    expect(searchSettings).toContain("testAdminSearchProviderConnection");
    expect(searchSettings).toContain("activateAdminSearchProvider");
    expect(searchSettings).toContain("deleteAdminSearchProviderConfig");
    expect(searchSettings).toContain("encryptSecret");
    expect(searchSettings).toContain("BYOK_CONTEXTS.searchProvider");
    expect(searchSettings).toContain("SecretInput");
    expect(searchSettings).toContain("keyStoredOnServer");
  });

  it("wires RAG provider settings to backend admin config and BYOK", () => {
    const ragProviderAdmin = readFileSync(
      resolve(process.cwd(), "src/components/settings/RAGProviderAdmin.tsx"),
      "utf8",
    );
    const ragSettings = readFileSync(
      resolve(process.cwd(), "src/components/settings/RAGSettings.tsx"),
      "utf8",
    );

    expect(ragProviderAdmin).toContain("listAdminRAGProviderConfigs");
    expect(ragProviderAdmin).toContain("getRAGProviderStatus");
    expect(ragProviderAdmin).toContain("configureAdminRAGProvider");
    expect(ragProviderAdmin).toContain("deleteAdminRAGProviderConfig");
    expect(ragProviderAdmin).toContain("encryptSecret");
    expect(ragProviderAdmin).toContain("BYOK_CONTEXTS.ragProvider");
    expect(ragProviderAdmin).toContain("data-rag-provider={provider.id}");
    expect(ragProviderAdmin).not.toContain("updateAdminRAGProviderConfig");
    expect(ragProviderAdmin).not.toContain("testAdminRAGProviderConnection");
    expect(ragProviderAdmin).not.toContain("activateAdminRAGProvider");
    expect(ragProviderAdmin).not.toContain("SecretInput");
    expect(ragProviderAdmin).not.toContain("apiKey:");
    expect(ragProviderAdmin).toContain("providerKeyStoredOnServer");
    expect(ragSettings).not.toContain("LlamaParse");
    expect(ragSettings).not.toContain("Upstash");
    expect(ragSettings).not.toContain("rag.enabled");
    expect(ragSettings).not.toContain("chunkSize");
    expect(ragSettings).not.toContain("topK");
    expect(ragSettings).not.toContain("SegmentedControl");
    expect(ragSettings).not.toContain("SimpleSwitch");
  });
});
