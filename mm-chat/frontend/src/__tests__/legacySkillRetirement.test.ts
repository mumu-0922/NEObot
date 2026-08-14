import { describe, expect, it, vi } from "vitest";

import { MessageSchema } from "../lib/api/schemas";
import { useChatStore } from "../store/core/chatStore";
import { useSettingsStore } from "../store/core/settingsStore";
import { normalizeMessage } from "../store/storage/migrations";
import { STORAGE_VERSION } from "../store/storage/storageConfig";
import {
  LEGACY_SKILL_RETIREMENT_MARKER,
  LEGACY_SKILL_RETIREMENT_VERSION,
  RETIRED_LEGACY_SKILL_SETTINGS_FIELDS,
  retireBrowserLegacySkillState,
  stripRetiredLegacySkillChat,
  stripRetiredLegacySkillSettings,
} from "../store/storage/legacySkillRetirement";

class MemoryLocalStorage implements Pick<Storage, "getItem" | "setItem"> {
  readonly values = new Map<string, string>();
  readonly setItem = vi.fn((key: string, value: string) => {
    this.values.set(key, value);
  });

  getItem(key: string): string | null {
    return this.values.get(key) ?? null;
  }
}

class MemoryAsyncStore {
  readonly values = new Map<string, unknown>();
  failKey = "";
  writeCount = 0;

  async getItem<T>(key: string): Promise<T | null> {
    return (this.values.get(key) as T | undefined) ?? null;
  }

  async setItem<T>(key: string, value: T): Promise<T> {
    this.writeCount += 1;
    if (key === this.failKey) throw new Error("write failed");
    this.values.set(key, value);
    return value;
  }
}

const settingsKey = "neo-chat-settings";
const chatKey = "neo-chat-storage";

describe("G20.9 legacy Skill retirement", () => {
  it("advances persistence and defensively strips both persisted stores", async () => {
    expect(STORAGE_VERSION).toBe(7);
    const settingsOptions = useSettingsStore.persist.getOptions();
    const chatOptions = useChatStore.persist.getOptions();
    const migratedSettings = (await settingsOptions.migrate!(
      {
        installedSkills: [{ id: "retired" }],
        customSkills: [{ id: "retired" }],
        activeSkillIds: ["retired"],
        skillAutoSelect: true,
        skillDefinitions: { retired: { content: "private" } },
        system: useSettingsStore.getState().system,
        search: useSettingsStore.getState().search,
        voice: useSettingsStore.getState().voice,
      },
      6,
    )) as Record<string, unknown>;
    const migratedChat = chatOptions.migrate!(
      {
        sessions: [
          {
            id: "s",
            title: "Chat",
            model: "model",
            messageCount: 0,
            updatedAt: 1,
            config: { activeSkills: ["retired"], useReasoning: true },
          },
        ],
        workspaces: [
          {
            id: "w",
            name: "Workspace",
            files: [],
            activeSkills: ["retired"],
            createdAt: 1,
          },
        ],
      },
      6,
    ) as Record<string, unknown>;

    for (const field of RETIRED_LEGACY_SKILL_SETTINGS_FIELDS) {
      expect(migratedSettings).not.toHaveProperty(field);
    }
    expect(migratedChat.sessions).toEqual([
      expect.objectContaining({ config: { useReasoning: true } }),
    ]);
    expect(migratedChat.workspaces).toEqual([
      expect.not.objectContaining({ activeSkills: expect.anything() }),
    ]);

    const persistedSettings = settingsOptions.partialize!(
      useSettingsStore.getState(),
    ) as Record<string, unknown>;
    for (const field of RETIRED_LEGACY_SKILL_SETTINGS_FIELDS) {
      expect(persistedSettings).not.toHaveProperty(field);
    }

    const persistedChat = chatOptions.partialize!({
      ...useChatStore.getState(),
      sessions: [
        {
          id: "poisoned-session",
          title: "Chat",
          model: "model",
          messageCount: 0,
          updatedAt: 1,
          config: { activeSkills: ["retired"], useReasoning: true },
        } as never,
      ],
      workspaces: [
        {
          id: "poisoned-workspace",
          name: "Workspace",
          files: [],
          activeSkills: ["retired"],
          createdAt: 1,
        } as never,
      ],
    }) as Record<string, unknown>;
    expect(persistedChat.sessions).toEqual([
      expect.objectContaining({ config: { useReasoning: true } }),
    ]);
    expect(persistedChat.workspaces).toEqual([
      expect.not.objectContaining({ activeSkills: expect.anything() }),
    ]);
  });

  it("strips exactly the retired settings fields from top-level and state", () => {
    const retired = Object.fromEntries(
      RETIRED_LEGACY_SKILL_SETTINGS_FIELDS.map((field) => [field, field]),
    );
    const result = stripRetiredLegacySkillSettings({
      ...retired,
      theme: "dark",
      state: {
        ...retired,
        customAgents: [{ id: "assistant-kept" }],
        selectedKnowledgeCollectionIds: ["knowledge-kept"],
      },
      version: 6,
    });

    expect(result.changed).toBe(true);
    expect(result.value).toEqual({
      theme: "dark",
      state: {
        customAgents: [{ id: "assistant-kept" }],
        selectedKnowledgeCollectionIds: ["knowledge-kept"],
      },
      version: 6,
    });
    expect(stripRetiredLegacySkillSettings(result.value)).toEqual({
      changed: false,
      value: result.value,
    });
  });

  it("removes only Session and Workspace legacy selections", () => {
    const result = stripRetiredLegacySkillChat({
      state: {
        sessions: [
          {
            id: "chat-kept",
            config: {
              activeSkills: ["retired"],
              useReasoning: true,
              selectedKnowledgeCollectionIds: ["knowledge-kept"],
            },
          },
        ],
        workspaces: [
          {
            id: "workspace-kept",
            activeSkills: ["retired"],
            files: [{ id: "file-kept" }],
          },
        ],
        currentSessionId: "chat-kept",
      },
      version: 6,
    });

    expect(result.value).toEqual({
      state: {
        sessions: [
          {
            id: "chat-kept",
            config: {
              useReasoning: true,
              selectedKnowledgeCollectionIds: ["knowledge-kept"],
            },
          },
        ],
        workspaces: [
          {
            id: "workspace-kept",
            files: [{ id: "file-kept" }],
          },
        ],
        currentSessionId: "chat-kept",
      },
      version: 6,
    });
  });

  it("migrates local and IndexedDB records before writing the marker", async () => {
    const localStorageRef = new MemoryLocalStorage();
    const indexedDbStore = new MemoryAsyncStore();
    localStorageRef.values.set(
      settingsKey,
      JSON.stringify({ state: { activeSkillIds: ["retired"], theme: "dark" } }),
    );
    indexedDbStore.values.set(chatKey, {
      state: {
        sessions: [{ id: "chat-kept", config: { activeSkills: ["retired"] } }],
      },
      version: 6,
    });

    await expect(
      retireBrowserLegacySkillState({
        localStorageRef,
        indexedDbStore,
        settingsKey,
        chatKey,
      }),
    ).resolves.toBe(true);

    expect(JSON.parse(localStorageRef.getItem(settingsKey)!)).toEqual({
      state: { theme: "dark" },
    });
    expect(indexedDbStore.values.get(chatKey)).toEqual({
      state: { sessions: [{ id: "chat-kept", config: {} }] },
      version: 6,
    });
    expect(localStorageRef.getItem(LEGACY_SKILL_RETIREMENT_MARKER)).toBe(
      String(LEGACY_SKILL_RETIREMENT_VERSION),
    );

    const localWrites = localStorageRef.setItem.mock.calls.length;
    const indexedWrites = indexedDbStore.writeCount;
    await expect(
      retireBrowserLegacySkillState({
        localStorageRef,
        indexedDbStore,
        settingsKey,
        chatKey,
      }),
    ).resolves.toBe(false);
    expect(localStorageRef.setItem).toHaveBeenCalledTimes(localWrites);
    expect(indexedDbStore.writeCount).toBe(indexedWrites);
  });

  it("rolls back earlier writes and leaves no marker after a later failure", async () => {
    const localStorageRef = new MemoryLocalStorage();
    const indexedDbStore = new MemoryAsyncStore();
    const originalLocal = JSON.stringify({ activeSkillIds: ["retired"] });
    const originalIndexed = { state: { workspaces: [] }, installedSkills: [] };
    localStorageRef.values.set(settingsKey, originalLocal);
    indexedDbStore.values.set(settingsKey, originalIndexed);
    indexedDbStore.values.set(chatKey, {
      state: { workspaces: [{ id: "w", activeSkills: ["retired"] }] },
    });
    indexedDbStore.failKey = chatKey;

    await expect(
      retireBrowserLegacySkillState({
        localStorageRef,
        indexedDbStore,
        settingsKey,
        chatKey,
      }),
    ).rejects.toThrow("write failed");

    expect(localStorageRef.getItem(settingsKey)).toBe(originalLocal);
    expect(indexedDbStore.values.get(settingsKey)).toEqual(originalIndexed);
    expect(localStorageRef.getItem(LEGACY_SKILL_RETIREMENT_MARKER)).toBeNull();
  });

  it("collapses historical invocation details to one retirement fact", () => {
    const legacy = {
      id: "message-1",
      role: "model" as const,
      content: "Historical answer remains.",
      timestamp: 1,
      skillInvocations: [
        {
          id: "private-id",
          title: "Private title",
          description: "Private description",
          category: "writing",
          mode: "manual",
        },
      ],
    };

    const normalized = normalizeMessage(legacy as never);
    const parsed = MessageSchema.parse(legacy);

    for (const result of [normalized, parsed]) {
      expect(result).toMatchObject({
        content: "Historical answer remains.",
        legacySkillRetired: true,
      });
      expect(result).not.toHaveProperty("skillInvocations");
      expect(JSON.stringify(result)).not.toContain("Private description");
      expect(JSON.stringify(result)).not.toContain("private-id");
    }
  });
});
