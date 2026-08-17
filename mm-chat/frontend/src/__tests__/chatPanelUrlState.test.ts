import { describe, expect, it } from "vitest";
import {
  parseChatPanelUrlState,
  setChatPanelUrlState,
} from "../lib/chat/panelUrlState";

describe("chat panel URL state", () => {
  it("parses valid panel and settings tab query params", () => {
    const state = parseChatPanelUrlState(
      new URLSearchParams("panel=settings&settingsTab=memory&keep=1"),
    );

    expect(state.panel).toBe("settings");
    expect(state.settingsTab).toBe("memory");
    expect(state.needsReplace).toBe(false);
  });

  it("normalizes invalid panel and tab params away", () => {
    const state = parseChatPanelUrlState(
      new URLSearchParams("panel=missing&settingsTab=wrong&keep=1"),
    );

    expect(state.panel).toBe("chat");
    expect(state.settingsTab).toBeNull();
    expect(state.needsReplace).toBe(true);
    expect(state.normalizedSearchParams.get("keep")).toBe("1");
    expect(state.normalizedSearchParams.has("panel")).toBe(false);
    expect(state.normalizedSearchParams.has("settingsTab")).toBe(false);
  });

  it("serializes panel state while preserving unrelated query params", () => {
    const params = setChatPanelUrlState(new URLSearchParams("keep=1"), {
      panel: "settings",
      settingsTab: "health",
    });

    expect(params.get("keep")).toBe("1");
    expect(params.get("panel")).toBe("settings");
    expect(params.get("settingsTab")).toBe("health");
  });

  it("round-trips the Tools panel without settings params", () => {
    const params = setChatPanelUrlState(new URLSearchParams("keep=1"), {
      panel: "tools",
    });
    const state = parseChatPanelUrlState(params);

    expect(params.get("panel")).toBe("tools");
    expect(params.has("settingsTab")).toBe(false);
    expect(state.panel).toBe("tools");
    expect(state.settingsTab).toBeNull();
    expect(state.needsReplace).toBe(false);
  });

  it("round-trips Skill Store and its selected package", () => {
    const params = setChatPanelUrlState(new URLSearchParams("keep=1"), {
      panel: "skill-store",
      skillId: "candidate_1234567890abcdef",
    });
    const state = parseChatPanelUrlState(params);

    expect(state).toMatchObject({
      panel: "skill-store",
      skillId: "candidate_1234567890abcdef",
      needsReplace: false,
    });
    expect(params.get("keep")).toBe("1");
  });

  it("migrates old Agent Center Skills URLs and drops control-plane state", () => {
    const migrated = parseChatPanelUrlState(
      "panel=agent-center&agentTab=skills&agentId=candidate_1234567890abcdef",
    );
    expect(migrated).toMatchObject({
      panel: "skill-store",
      skillId: "candidate_1234567890abcdef",
      needsReplace: true,
    });
    expect(migrated.normalizedSearchParams.get("panel")).toBe("skill-store");
    expect(migrated.normalizedSearchParams.get("skillId")).toBe(
      "candidate_1234567890abcdef",
    );
    expect(migrated.normalizedSearchParams.has("agentTab")).toBe(false);

    const invalid = parseChatPanelUrlState(
      "panel=skill-store&skillId=../secret",
    );
    expect(invalid.skillId).toBeNull();
    expect(invalid.needsReplace).toBe(true);

    const chat = parseChatPanelUrlState(
      "panel=chat&skillId=candidate_1234567890abcdef&agentTab=runs",
    );
    expect(chat.panel).toBe("chat");
    expect(chat.skillId).toBeNull();
    expect(chat.normalizedSearchParams.has("agentTab")).toBe(false);
  });

  it("removes panel params when returning to chat", () => {
    const params = setChatPanelUrlState(
      new URLSearchParams("panel=settings&settingsTab=voice&keep=1"),
      { panel: "chat" },
    );

    expect(params.get("keep")).toBe("1");
    expect(params.has("panel")).toBe(false);
    expect(params.has("settingsTab")).toBe(false);
  });
});
