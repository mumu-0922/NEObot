import { beforeEach, describe, expect, it, vi } from "vitest";

const { appDbMock, dirMock, localforageClearMock, removeMock } = vi.hoisted(
  () => {
    const removeMock = vi.fn(() => Promise.resolve());
    return {
      appDbMock: {
        clear: vi.fn(() => Promise.resolve()),
      },
      dirMock: vi.fn(() => ({
        exists: vi.fn(() => Promise.resolve(true)),
        remove: removeMock,
      })),
      localforageClearMock: vi.fn(() => Promise.resolve()),
      removeMock,
    };
  },
);

vi.mock("localforage", () => ({
  default: {
    clear: localforageClearMock,
  },
}));

vi.mock("opfs-tools", () => ({
  dir: dirMock,
  file: vi.fn(),
  write: vi.fn(),
}));

vi.mock("../store/storage/storageConfig", () => ({
  appDb: appDbMock,
  canUseBrowserLocalRuntimePersistence: () => true,
  STORAGE_KEYS: {
    CORE_SETTINGS: "neo-chat-core-settings",
    SETTINGS: "neo-chat-settings",
    CHAT: "neo-chat-storage",
    KNOWLEDGE: "knowledge-storage",
    MEMORY: "neo-chat-memory",
  },
}));

const { clearBrowserAppData } = await import("../lib/data/clearAppData");
const { deleteOPFSDirectory } = await import("../utils/opfs");

describe("clear app data", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal("fetch", vi.fn());
  });

  it("cleans local OPFS and browser stores without calling removed RAG routes", async () => {
    await clearBrowserAppData();

    expect(fetch).not.toHaveBeenCalled();
    expect(dirMock).toHaveBeenCalledWith("knowledge-base");
    expect(dirMock).toHaveBeenCalledWith("workspaces");
    expect(dirMock).toHaveBeenCalledWith("images");
    expect(dirMock).toHaveBeenCalledWith("chat");
    expect(removeMock).toHaveBeenCalledWith({ force: true });

    expect(localforageClearMock).toHaveBeenCalled();
    expect(appDbMock.clear).toHaveBeenCalled();
  });

  it("continues local cleanup when the removed RAG endpoint would fail", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() => Promise.reject(new Error("RAG endpoint removed"))),
    );

    await clearBrowserAppData();

    expect(fetch).not.toHaveBeenCalled();
    expect(dirMock).toHaveBeenCalledWith("knowledge-base");
    expect(localforageClearMock).toHaveBeenCalled();
    expect(appDbMock.clear).toHaveBeenCalled();
  });

  it("rejects unsafe OPFS directory paths", async () => {
    await deleteOPFSDirectory("../secret");

    expect(dirMock).not.toHaveBeenCalled();
  });
});
