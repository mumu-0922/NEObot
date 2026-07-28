import fs from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";

const root = process.cwd();

describe("server memory governance composition", () => {
  it("keeps local Memory hidden behind the mode router and exposes accessible governance controls", () => {
    const router = fs.readFileSync(
      path.join(root, "src/components/settings/MemorySettings.tsx"),
      "utf8",
    );
    const governance = fs.readFileSync(
      path.join(root, "src/components/settings/ServerMemoryGovernance.tsx"),
      "utf8",
    );

    expect(router).toMatch(
      /apiClient\.mode === "server" && apiClient\.capabilities\.memories/,
    );
    expect(router).toContain("<ServerMemoryGovernance");
    expect(router).toContain("<LocalMemorySettings");
    expect(governance).toContain('aria-label={t("globalPolicy")}');
    expect(governance).toContain('aria-label={t("governanceSections")}');
    expect(governance).toContain("aria-current=");
    expect(governance).toContain("aria-expanded=");
    expect(governance).toContain('role="alert"');
    expect(governance).toContain('role="status"');
    expect(governance).toContain("exportMemoryPackage");
    expect(governance).toContain("dryRunMemoryImport");
    expect(governance).toContain("confirmMemoryImport");
    expect(governance).toContain("settingsNeverApplied");
    expect(governance).toContain("planStale");
    expect(governance).not.toContain("localStorage");
    expect(governance).not.toContain("sessionStorage");
    expect(governance).not.toContain("useChatStore");
  });

  it("polls Activity only for visible server messages and keeps undo revision-fenced", () => {
    const chip = fs.readFileSync(
      path.join(root, "src/components/chat/MemoryActivityChip.tsx"),
      "utf8",
    );
    const message = fs.readFileSync(
      path.join(root, "src/components/chat/MessageItem.tsx"),
      "utf8",
    );

    expect(chip).toContain("IntersectionObserver");
    expect(chip).toContain("document.visibilityState");
    expect(chip).toContain("MAX_EMPTY_POLLS");
    expect(chip).toContain("areMemoryActivitiesTerminal");
    expect(chip).toContain("expectedRevision: activity.subjectRevision");
    expect(chip).not.toContain("expectedRevision: activity.memoryRevision");
    expect(chip).toContain("activity.sourceKind");
    expect(chip).toContain("activity.scopeType");
    expect(chip).toContain('role="alert"');
    expect(chip).not.toContain("localStorage");
    expect(chip).not.toContain("sessionStorage");
    expect(message).toMatch(
      /message\.role === "model" && !isTyping && \(\s*<MemoryActivityChip/,
    );
  });
});
