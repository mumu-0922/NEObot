import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { appDbMock } = vi.hoisted(() => ({
  appDbMock: {
    getItem: vi.fn(),
    setItem: vi.fn(),
    removeItem: vi.fn(),
  },
}));

vi.mock("localforage", () => ({
  default: {
    createInstance: vi.fn(() => appDbMock),
  },
}));

function installWindowStorage(): Storage {
  const values = new Map<string, string>();
  const storage = {
    get length() {
      return values.size;
    },
    clear: vi.fn(() => values.clear()),
    getItem: vi.fn((key: string) => values.get(key) ?? null),
    key: vi.fn((index: number) => [...values.keys()][index] ?? null),
    removeItem: vi.fn((key: string) => {
      values.delete(key);
    }),
    setItem: vi.fn((key: string, value: string) => {
      values.set(key, value);
    }),
  } satisfies Storage;

  vi.stubGlobal("window", {
    localStorage: storage,
  });

  return storage;
}

describe("browser local persistence authority", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("treats server mode as import-only browser-local authority", async () => {
    vi.stubEnv("NEXT_PUBLIC_API_MODE", "server");
    const localStorage = installWindowStorage();
    const {
      BrowserLocalIndexedDBAuthorityError,
      getAppDbStorage,
      getBrowserLocalPersistenceAuthority,
      getBrowserLocalStorage,
      getBrowserPreferenceStorage,
      removeRuntimeAppDbItem,
      setRuntimeAppDbItem,
    } = await import("../store/storage/storageConfig");

    expect(getBrowserLocalPersistenceAuthority()).toBe("import-only");

    await getAppDbStorage().setItem("neo-chat-storage", "{}");
    await getAppDbStorage().removeItem("neo-chat-storage");
    expect(await getAppDbStorage().getItem("neo-chat-storage")).toBe(null);
    expect(appDbMock.setItem).not.toHaveBeenCalled();
    expect(appDbMock.removeItem).not.toHaveBeenCalled();
    expect(appDbMock.getItem).not.toHaveBeenCalled();
    await expect(
      setRuntimeAppDbItem("session_messages_1", {}),
    ).rejects.toBeInstanceOf(BrowserLocalIndexedDBAuthorityError);
    await expect(
      removeRuntimeAppDbItem("session_messages_1"),
    ).rejects.toBeInstanceOf(BrowserLocalIndexedDBAuthorityError);
    expect(appDbMock.setItem).not.toHaveBeenCalled();
    expect(appDbMock.removeItem).not.toHaveBeenCalled();

    getBrowserLocalStorage().setItem("neo-chat-core-settings", "{}");
    getBrowserLocalStorage().removeItem("neo-chat-core-settings");
    expect(getBrowserLocalStorage().getItem("neo-chat-core-settings")).toBe(
      null,
    );
    expect(localStorage.setItem).not.toHaveBeenCalled();
    expect(localStorage.removeItem).not.toHaveBeenCalled();
    expect(localStorage.getItem).not.toHaveBeenCalled();

    getBrowserPreferenceStorage().setItem("neo-chat-core-settings", "{}");
    expect(
      getBrowserPreferenceStorage().getItem("neo-chat-core-settings"),
    ).toBe("{}");
    getBrowserPreferenceStorage().removeItem("neo-chat-core-settings");
    expect(
      getBrowserPreferenceStorage().getItem("neo-chat-core-settings"),
    ).toBe(null);
    expect(localStorage.setItem).toHaveBeenCalledWith(
      "neo-chat-core-settings",
      "{}",
    );
    expect(localStorage.removeItem).toHaveBeenCalledWith(
      "neo-chat-core-settings",
    );
  });

  it("keeps explicit local mode runtime persistence writable", async () => {
    vi.stubEnv("NEXT_PUBLIC_API_MODE", "local");
    const localStorage = installWindowStorage();
    appDbMock.setItem.mockResolvedValue("{}");
    appDbMock.removeItem.mockResolvedValue(undefined);
    const {
      getAppDbStorage,
      getBrowserLocalPersistenceAuthority,
      getBrowserLocalStorage,
      removeRuntimeAppDbItem,
      setRuntimeAppDbItem,
    } = await import("../store/storage/storageConfig");

    expect(getBrowserLocalPersistenceAuthority()).toBe("runtime");

    await getAppDbStorage().setItem("neo-chat-storage", "{}");
    await getAppDbStorage().removeItem("neo-chat-storage");
    await setRuntimeAppDbItem("session_messages_1", {});
    await removeRuntimeAppDbItem("session_messages_1");
    expect(appDbMock.setItem).toHaveBeenCalledWith("neo-chat-storage", "{}");
    expect(appDbMock.setItem).toHaveBeenCalledWith("session_messages_1", {});
    expect(appDbMock.removeItem).toHaveBeenCalledWith("neo-chat-storage");
    expect(appDbMock.removeItem).toHaveBeenCalledWith("session_messages_1");

    getBrowserLocalStorage().setItem("neo-chat-core-settings", "{}");
    getBrowserLocalStorage().removeItem("neo-chat-core-settings");
    expect(localStorage.setItem).toHaveBeenCalledWith(
      "neo-chat-core-settings",
      "{}",
    );
    expect(localStorage.removeItem).toHaveBeenCalledWith(
      "neo-chat-core-settings",
    );
  });
});
