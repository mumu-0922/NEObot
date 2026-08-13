import { describe, expect, it } from "vitest";

import { normalizeSession, normalizeWorkspace } from "../lib/chat/entities";
import { stripRetiredPluginFields } from "../store/storage/migrations";
import { STORAGE_VERSION } from "../store/storage/storageConfig";

describe("MCP cutover browser migration", () => {
  it("uses storage version 6 and removes retired Plugin state recursively", () => {
    expect(STORAGE_VERSION).toBe(6);
    expect(
      stripRetiredPluginFields({
        activePlugins: ["weather"],
        settings: {
          installedPlugins: ["legacy"],
          keep: true,
        },
        sessions: [{ id: "s1", config: { pluginConfigs: { secret: true } } }],
      }),
    ).toEqual({
      settings: { keep: true },
      sessions: [{ id: "s1", config: {} }],
    });
  });

  it("does not map retired Plugin selections into sessions or Workspaces", () => {
    const session = normalizeSession({
      id: "session-1",
      title: "Chat",
      createdAt: 1,
      updatedAt: 1,
      messageCount: 0,
      config: { activePlugins: ["weather"], useSearch: false },
    } as unknown as Parameters<typeof normalizeSession>[0]);
    const workspace = normalizeWorkspace({
      id: "workspace-1",
      name: "Workspace",
      createdAt: 1,
      activePlugins: ["weather"],
      files: [],
    } as unknown as Parameters<typeof normalizeWorkspace>[0]);
    expect(session.config).not.toHaveProperty("activePlugins");
    expect(workspace).not.toHaveProperty("activePlugins");
    expect(JSON.stringify({ session, workspace })).not.toContain("weather");
  });
});
