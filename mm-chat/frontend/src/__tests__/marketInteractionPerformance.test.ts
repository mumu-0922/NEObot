import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const readSource = (path: string) =>
  readFileSync(resolve(process.cwd(), path), "utf8");

describe("marketplace interaction performance", () => {
  it("keeps marketplace scrolling off full-surface backdrop filters", () => {
    const markets = [
      "src/components/assistant/AssistantHub.tsx",
      "src/components/skill/SkillMarket.tsx",
      "src/components/plugin/PluginMarket.tsx",
    ];

    for (const market of markets) {
      const source = readSource(market);
      expect(source).not.toMatch(/fixed inset-0[^\n]*backdrop-blur/);
      expect(source).not.toContain("backdrop-blur-md");
      expect(source).not.toContain("backdrop-blur-xl");
      expect(source).not.toContain("animate-in fade-in duration-300");
      expect(source).toContain("overscroll-contain");
      expect(source).toContain("[scrollbar-gutter:stable]");
      expect(source).toContain("[contain:paint]");
    }
  });

  it("keeps shared full-screen dialogs free of page-wide blur", () => {
    const dialogs = [
      "src/components/ui/primitives.tsx",
      "src/components/layout/WorkspaceSettingsModal.tsx",
      "src/components/modals/RemoteFileModal.tsx",
      "src/components/knowledge/KnowledgeSelectionModal.tsx",
      "src/components/knowledge/AddToKnowledgeModal.tsx",
      "src/components/knowledge/ServerKnowledgeBase.tsx",
      "src/components/settings/ModelEditor.tsx",
    ];

    for (const dialog of dialogs) {
      expect(readSource(dialog)).not.toMatch(
        /fixed inset-0[^\n]*backdrop-blur/,
      );
    }
  });

  it("renders settings sections without deliberate 300ms entrance motion", () => {
    const sections = [
      "src/components/settings/SettingsPage.tsx",
      "src/components/settings/SearchSettings.tsx",
      "src/components/settings/VoiceSettings.tsx",
      "src/components/settings/SystemSettings.tsx",
      "src/components/settings/DefaultModelSettings.tsx",
      "src/components/settings/MemorySettings.tsx",
      "src/components/settings/ProviderSettings.tsx",
    ];

    for (const section of sections) {
      expect(readSource(section)).not.toContain(
        "slide-in-from-bottom-2 duration-300",
      );
    }
  });
});
