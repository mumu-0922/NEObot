import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

describe("Agent Center product boundary", () => {
  const center = readFileSync("src/components/agent/AgentCenter.tsx", "utf8");
  const legacy = readFileSync(
    "src/components/agent/LegacySkillCutoverCard.tsx",
    "utf8",
  );
  const chatApp = readFileSync("src/components/app/ChatApp.tsx", "utf8");

  it("is a separate URL-addressable top-level surface", () => {
    expect(chatApp).toContain("navigateAgentCenter(agentTab, agentId)");
    expect(chatApp).toContain('viewMode === "agent-center"');
    expect(chatApp).toContain("<AgentCenter");
    expect(center).toContain('role="tablist"');
    expect(center).toContain("aria-selected={activeTab === tab.id}");
  });

  it("keeps mobile drill-in, focus restoration, live status, and held truth", () => {
    expect(center).toContain('selected ? "hidden md:block"');
    expect(center).toContain("restoreFocus.current?.focus");
    expect(center).not.toContain("ref={selectedId ===");
    expect(center).toContain("detailError && selectedId");
    expect(center).toContain("retry={() => void loadDetail()}");
    expect(center).toContain('aria-live="polite"');
    expect(center).toContain("status.runtime.reasonCode");
    expect(center).not.toContain("dangerouslySetInnerHTML");
  });

  it("offers only local backup and dry-run for legacy Skills", () => {
    expect(legacy).toContain("createLegacySkillInventory");
    expect(legacy).toContain("createLegacySkillDeletionDryRun");
    expect(legacy).toContain("createLegacySkillBackup");
    expect(legacy).not.toContain("removeRuntimeAppDbItem");
    expect(legacy).not.toContain("clearAllData");
  });
});
