import { existsSync, readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

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

  it("removes the old Agent control surface", () => {
    expect(existsSync("src/components/agent/AgentCenter.tsx")).toBe(false);
    expect(chatApp).not.toContain("AgentCenter");
    expect(store).not.toContain("Shadow");
    expect(store).not.toContain("Schedules");
    expect(store).not.toContain("Learning");
    expect(store).not.toContain("Canary");
  });
});
