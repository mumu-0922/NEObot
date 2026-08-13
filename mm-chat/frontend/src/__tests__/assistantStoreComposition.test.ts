import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

describe("assistant store composition", () => {
  const source = readFileSync(
    resolve(process.cwd(), "src/components/assistant/AssistantHub.tsx"),
    "utf8",
  );

  it("separates My Assistants and Assistant Store", () => {
    expect(source).toContain('"library" | "market"');
    expect(source).toContain('t("myAssistants")');
    expect(source).toContain('t("assistantStore")');
    expect(source).toContain("IntersectionObserver");
  });

  it("does not read legacy browser assistant authorities", () => {
    expect(source).not.toContain("customAgents");
    expect(source).not.toContain("usedAgents");
    expect(source).not.toContain("agentOverrides");
    expect(source).not.toContain("useSettingsStore");
  });

  it("shows the bounded compatibility warning and never starts Store cards", () => {
    expect(source).toContain('t("compatibilityNotice")');
    expect(source).toContain("installMarket!");
    expect(source).toContain("onStart={() => onStart(entry)}");
  });

  it("confirms manual Store updates and exposes CAS recovery actions", () => {
    expect(source).toContain('t("admitUpdate")');
    expect(source).toContain('t("updateAssistantTitle")');
    expect(source).toContain('t("confirmUpdate")');
    expect(source).toContain('t("reloadLatest")');
    expect(source).toContain('t("saveAsCopy")');
    expect(source).toContain("getLibraryEntry!");
  });

  it("preserves declared Tool metadata and links to separately-authoritative Tools", () => {
    expect(source).toContain("entry.requiredTools");
    expect(source).toContain("agent.requiredTools");
    expect(source).toContain("onOpenTools");
    expect(source).toContain('t("openTools")');
  });

  it("uses the shared keyboard and focus-safe modal primitive", () => {
    expect(source).toContain(
      'import { Dialog } from "@/components/ui/primitives"',
    );
    expect(source).toContain('role="alertdialog"');
    expect(source).not.toContain('className="fixed inset-0 z-10000');
  });
});
