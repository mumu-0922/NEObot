import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const picker = readFileSync(
  resolve(process.cwd(), "src/components/chat/ConversationResourcePickers.tsx"),
  "utf8",
);

describe("Conversation resource pickers", () => {
  it("keeps inventory management outside the two compact composer pickers", () => {
    expect(picker).toContain("<Blocks");
    expect(picker).toContain("<Cable");
    expect(picker).toContain("<SelectionCount");
    expect(picker).toContain("onOpenSkillStore");
    expect(picker).toContain("onOpenMcpTools");
    expect(picker).not.toContain("installPackageSkill");
    expect(picker).not.toContain("installMarketplaceItem");
    expect(picker).not.toContain("uninstallPackageSkill");
    expect(picker).not.toContain("setCredential");
  });

  it("loads and writes exact conversation selections with CAS authority", () => {
    expect(picker).toContain("getConversationSelection(targetConversationId");
    expect(picker).toContain("replaceConversationSelection({");
    expect(picker).toContain("revision: skillSelection.revision");
    expect(picker).toContain("revision: mcpSelection.revision");
    expect(picker).toContain('mode: "custom"');
    expect(picker).toContain('server.status !== "ready"');
  });

  it("fences stale requests and selection badges when conversations switch", () => {
    expect(picker).toContain("currentConversationIdRef");
    expect(picker).toContain(
      "currentConversationIdRef.current !== targetConversationId",
    );
    expect(picker).toContain(
      "skillSelection.conversationId !== conversationId",
    );
    expect(picker).toContain("mcpSelection.conversationId !== conversationId");
  });

  it("labels active-run edits as next-run changes", () => {
    expect(picker).toContain("runActive ? (");
    expect(picker).toContain('t("resourceSelectionNextRun")');
  });
});
