import { existsSync, readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { getSkillIconImageURL } from "../components/skills/SkillIcon";
import { MARKETPLACE_CATEGORIES } from "../components/skills/SkillMarketplacePrimitives";
import en from "../i18n/locales/en";
import ja from "../i18n/locales/ja";
import zh from "../i18n/locales/zh";

describe("Skill Store product boundary", () => {
  const store = [
    "src/components/skills/SkillStore.tsx",
    "src/components/skills/SkillDetailDialog.tsx",
    "src/components/skills/SkillIcon.tsx",
    "src/components/skills/SkillMarketplacePrimitives.tsx",
    "src/components/skills/SkillStorePrimitives.tsx",
    "src/components/skills/useSkillStore.ts",
  ]
    .map((path) => readFileSync(path, "utf8"))
    .join("\n");
  const chatApp = readFileSync("src/components/app/ChatApp.tsx", "utf8");
  const nextConfig = readFileSync("next.config.ts", "utf8");

  it("is a separate URL-addressable top-level surface", () => {
    expect(chatApp).toContain("navigateSkillStore(skillId)");
    expect(chatApp).toContain('viewMode === "skill-store"');
    expect(chatApp).toContain("<SkillStore");
    expect(store).toContain('id="skill-store-content"');
  });

  it("keeps install, list, uninstall, modal drill-in, and live feedback", () => {
    expect(store).toContain("client.skillStore.listCatalog()");
    expect(store).toContain(".getCatalogSkill(selectedKey.id");
    expect(store).toContain(".getMarketplaceSkill(selectedKey.id");
    expect(store).toContain("client.skillStore.installMarketplaceSkill");
    expect(store).toContain("client.skillStore.installSkillLink");
    expect(store).toContain("client.skillStore.listPackageLibrary()");
    expect(store).toContain("client.skillStore.installCatalogSkill");
    expect(store).toContain("client.skillStore.uninstallPackageSkill");
    expect(store).toContain("<Dialog");
    expect(store).toContain("open={Boolean(selectedKey)}");
    expect(store).toContain("<MarketplaceCategoryNav");
    expect(store).toContain('className="grid gap-3 md:grid-cols-2"');
    expect(store).not.toContain("<SplitShell");
    expect(store).toContain("restoreFocus.current?.focus");
    expect(store).toContain('aria-live="polite"');
    expect(store).not.toContain("dangerouslySetInnerHTML");
  });

  it("separates the installed library from the curated catalog with tabs", () => {
    expect(store).toContain('type SkillStoreTab = "installed" | "store"');
    expect(store).toContain('role="tablist"');
    expect(store).toContain('active={tab === "installed"}');
    expect(store).toContain('active={tab === "store"}');
    expect(store).toContain("<InstalledSkills");
    expect(store).toContain("const loadMarketplace = useCallback");
    expect(store).toContain("locale: marketplaceLocale");
    expect(store).toContain("setMarketTotalCount(result.totalCount)");
    expect(store).toContain("setMarketSourceURL(result.sourceUrl)");
    expect(store).toContain('value="installCount"');
    expect(store).toContain("const loadCatalog = useCallback");
    expect(store).toContain("const loadInstalled = useCallback");
    expect(store).toContain('onNavigate(null, "replace")');
    expect(store).toContain('setTab("installed")');
    expect(zh.SkillStore.installedTab).toBe("已安装");
    expect(zh.Sidebar.skillStore).toBe("Skill");
    expect(en.Sidebar.skillStore).toBe("Skill");
    expect(ja.Sidebar.skillStore).toBe("Skill");
    expect(zh.SkillStore.title).toBe("Skill");
    expect(zh.SkillStore.storeTab).toBe("Skill");
    expect(en.SkillStore.installedTab).toBe("Installed");
    expect(en.SkillStore.storeTab).toBe("Skill");
    expect(ja.SkillStore.storeTab).toBe("Skill");
    expect(zh.SkillStore.lobehubSource).toBe("LobeHub");
    expect(en.SkillStore.openaiSource).toBe("OpenAI Curated");
    expect(zh.SkillStore.emptyStore).not.toContain("准入");
    expect(en.SkillStore.emptyStore).not.toContain("admitted");
  });

  it("removes the old Agent control surface", () => {
    expect(existsSync("src/components/agent/AgentCenter.tsx")).toBe(false);
    expect(chatApp).not.toContain("AgentCenter");
    expect(store).not.toContain("Shadow");
    expect(store).not.toContain("Schedules");
    expect(store).not.toContain("Learning");
    expect(store).not.toContain("Canary");
  });

  it("renders bounded Skill icons through the same-origin image optimizer", () => {
    expect(store).toContain("<SkillIcon icon={item.icon}");
    expect(store).toContain("<SkillIcon label={entry.name}");
    expect(store).toContain("label.trim().slice(0, 1).toUpperCase()");
    expect(store).toContain('referrerPolicy="no-referrer"');
    expect(store).not.toContain("unoptimized");
    expect(nextConfig).toContain('hostname: "github.com"');
    expect(nextConfig).toContain('pathname: "/*.png"');
    expect(getSkillIconImageURL("https://github.com/openclaw.png")).toBe(
      "https://github.com/openclaw.png",
    );
    expect(getSkillIconImageURL("https://evil.example/openclaw.png")).toBe("");
    expect(
      getSkillIconImageURL("https://github.com/openclaw.png?size=88"),
    ).toBe("");
  });

  it("renders a localized, ordered category rail with semantic icons", () => {
    expect(MARKETPLACE_CATEGORIES).toHaveLength(21);
    expect(
      new Set(MARKETPLACE_CATEGORIES.map((category) => category.icon)).size,
    ).toBe(MARKETPLACE_CATEGORIES.length);
    expect(MARKETPLACE_CATEGORIES.map((category) => category.id)).not.toContain(
      "_meta",
    );
    for (const { id } of MARKETPLACE_CATEGORIES) {
      expect(zh.SkillStore.marketplaceCategories[id]).toBeTruthy();
      expect(en.SkillStore.marketplaceCategories[id]).toBeTruthy();
      expect(ja.SkillStore.marketplaceCategories[id]).toBeTruthy();
    }
    expect(zh.SkillStore.marketplaceCategories["coding-agents-ides"]).toBe(
      "编程 Agent 与 IDE",
    );
    expect(store).toContain("marketplaceCategories.${category.id}");
    expect(store).not.toContain("label={item.category}");
    expect(store).not.toContain("<Folder");
  });
});
