import { existsSync, readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import en from "../i18n/locales/en";
import zh from "../i18n/locales/zh";

describe("Skill Store product boundary", () => {
  const store = readFileSync("src/components/skills/SkillStore.tsx", "utf8");
  const chatApp = readFileSync("src/components/app/ChatApp.tsx", "utf8");

  it("is a separate URL-addressable top-level surface", () => {
    expect(chatApp).toContain("navigateSkillStore(skillId)");
    expect(chatApp).toContain('viewMode === "skill-store"');
    expect(chatApp).toContain("<SkillStore");
    expect(store).toContain('id="skill-store-content"');
  });

  it("keeps install, list, uninstall, drill-in, and live feedback", () => {
    expect(store).toContain("client.skillStore.listPackageStore()");
    expect(store).toContain("client.skillStore.listPackageLibrary()");
    expect(store).toContain("client.skillStore.installPackageSkill");
    expect(store).toContain("client.skillStore.uninstallPackageSkill");
    expect(store).toContain('selected ? "hidden md:block"');
    expect(store).toContain("restoreFocus.current?.focus");
    expect(store).toContain('aria-live="polite"');
    expect(store).not.toContain("dangerouslySetInnerHTML");
  });

  it("separates the installed library from the admitted store with tabs", () => {
    expect(store).toContain('type SkillStoreTab = "installed" | "store"');
    expect(store).toContain('role="tablist"');
    expect(store).toContain('active={tab === "installed"}');
    expect(store).toContain('active={tab === "store"}');
    expect(store).toContain("<InstalledSkills");
    expect(store).toContain("const loadStore = useCallback");
    expect(store).toContain("const loadInstalled = useCallback");
    expect(store).toContain('onNavigate(null, "replace")');
    expect(store).toContain('setTab("installed")');
    expect(zh.SkillStore.installedTab).toBe("已安装");
    expect(zh.SkillStore.storeTab).toBe("技能商店");
    expect(en.SkillStore.installedTab).toBe("Installed");
    expect(en.SkillStore.storeTab).toBe("Skill Store");
  });

  it("removes the old Agent control surface", () => {
    expect(existsSync("src/components/agent/AgentCenter.tsx")).toBe(false);
    expect(chatApp).not.toContain("AgentCenter");
    expect(store).not.toContain("Shadow");
    expect(store).not.toContain("Schedules");
    expect(store).not.toContain("Learning");
    expect(store).not.toContain("Canary");
  });
});
