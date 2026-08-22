import { beforeEach, describe, expect, it, vi } from "vitest";

const { preferenceStorage } = vi.hoisted(() => {
  const values = new Map<string, string>();
  const preferenceStorage = {
    getItem: vi.fn((key: string) => values.get(key) ?? null),
    setItem: vi.fn((key: string, value: string) => {
      values.set(key, value);
    }),
    removeItem: vi.fn((key: string) => {
      values.delete(key);
    }),
  };
  return { preferenceStorage };
});

vi.mock("../store/storage/storageConfig", () => ({
  getBrowserPreferenceStorage: () => preferenceStorage,
  STORAGE_KEYS: { CORE_SETTINGS: "neo-chat-core-settings" },
  STORAGE_VERSION: 7,
}));

const { useCoreSettingsStore } =
  await import("../store/core/coreSettingsStore");

describe("chat model browser preference", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useCoreSettingsStore.setState({ selectedChatModel: "" });
  });

  it("persists the complete Provider and model identity", () => {
    useCoreSettingsStore
      .getState()
      .setSelectedChatModel("NEW_PROVIDER:gpt-5.6-terra");

    const partialize = useCoreSettingsStore.persist.getOptions().partialize;
    const persisted = partialize?.(useCoreSettingsStore.getState());

    expect(useCoreSettingsStore.getState().selectedChatModel).toBe(
      "NEW_PROVIDER:gpt-5.6-terra",
    );
    expect(persisted).toMatchObject({
      selectedChatModel: "NEW_PROVIDER:gpt-5.6-terra",
    });
    expect(preferenceStorage.setItem).toHaveBeenCalledWith(
      "neo-chat-core-settings",
      expect.any(String),
    );
    const stored = JSON.parse(
      preferenceStorage.setItem.mock.calls.at(-1)?.[1] ?? "{}",
    );
    expect(stored.state.selectedChatModel).toBe("NEW_PROVIDER:gpt-5.6-terra");
  });
});
