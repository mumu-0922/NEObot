import { describe, expect, it, vi } from "vitest";
import {
  retireBrowserLocalRAGState,
  SERVER_ONLY_RAG_MIGRATION_MARKER,
  SERVER_ONLY_RAG_MIGRATION_VERSION,
  stripRetiredLocalRAGState,
} from "../lib/settings/serverOnlyRagMigration";

class MemoryStorage implements Pick<Storage, "getItem" | "setItem"> {
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
  readonly setItemMock = vi.fn();

  async setItem<T>(key: string, value: T): Promise<T> {
    this.setItemMock(key, value);
    this.values.set(key, value);
    return value;
  }

  async getItem<T>(key: string): Promise<T | null> {
    return (this.values.get(key) as T | undefined) ?? null;
  }
}

const settingsKey = "neo-chat-settings";

describe("server-only RAG browser migration", () => {
  it("removes only top-level and persisted-state RAG fields", () => {
    const migrated = stripRetiredLocalRAGState({
      rag: { token: "retired" },
      state: {
        rag: { url: "https://retired.example" },
        theme: "dark",
        search: { provider: "default" },
        voice: { ttsProvider: "browser" },
        pluginConfigs: { demo: { enabled: true } },
      },
      version: 4,
    });

    expect(migrated).toEqual({
      changed: true,
      value: {
        state: {
          theme: "dark",
          search: { provider: "default" },
          voice: { ttsProvider: "browser" },
          pluginConfigs: { demo: { enabled: true } },
        },
        version: 4,
      },
    });
  });

  it("migrates localStorage and IndexedDB before writing the marker", async () => {
    const localStorageRef = new MemoryStorage();
    const indexedDbStore = new MemoryAsyncStore();
    localStorageRef.values.set(
      settingsKey,
      JSON.stringify({ state: { rag: { token: "local" }, theme: "light" } }),
    );
    indexedDbStore.values.set(settingsKey, {
      state: { rag: { token: "indexed" }, search: { provider: "default" } },
      version: 4,
    });

    await expect(
      retireBrowserLocalRAGState({
        localStorageRef,
        indexedDbStore,
        settingsKey,
      }),
    ).resolves.toBe(true);

    expect(JSON.parse(localStorageRef.getItem(settingsKey)!)).toEqual({
      state: { theme: "light" },
    });
    expect(indexedDbStore.values.get(settingsKey)).toEqual({
      state: { search: { provider: "default" } },
      version: 4,
    });
    expect(localStorageRef.getItem(SERVER_ONLY_RAG_MIGRATION_MARKER)).toBe(
      String(SERVER_ONLY_RAG_MIGRATION_VERSION),
    );
  });

  it("is idempotent after the migration marker is present", async () => {
    const localStorageRef = new MemoryStorage();
    const indexedDbStore = new MemoryAsyncStore();
    localStorageRef.values.set(
      SERVER_ONLY_RAG_MIGRATION_MARKER,
      String(SERVER_ONLY_RAG_MIGRATION_VERSION),
    );

    await expect(
      retireBrowserLocalRAGState({
        localStorageRef,
        indexedDbStore,
        settingsKey,
      }),
    ).resolves.toBe(false);
    expect(localStorageRef.setItem).not.toHaveBeenCalled();
    expect(indexedDbStore.setItemMock).not.toHaveBeenCalled();
  });

  it("does not overwrite either store or mark completion after invalid JSON", async () => {
    const localStorageRef = new MemoryStorage();
    const indexedDbStore = new MemoryAsyncStore();
    localStorageRef.values.set(settingsKey, "{invalid-json");
    indexedDbStore.values.set(settingsKey, {
      state: { rag: { token: "indexed" }, theme: "dark" },
    });

    await expect(
      retireBrowserLocalRAGState({
        localStorageRef,
        indexedDbStore,
        settingsKey,
      }),
    ).rejects.toThrow();
    expect(localStorageRef.getItem(settingsKey)).toBe("{invalid-json");
    expect(indexedDbStore.values.get(settingsKey)).toEqual({
      state: { rag: { token: "indexed" }, theme: "dark" },
    });
    expect(
      localStorageRef.getItem(SERVER_ONLY_RAG_MIGRATION_MARKER),
    ).toBeNull();
    expect(indexedDbStore.setItemMock).not.toHaveBeenCalled();
  });
});
