import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { dirMock, fileMock, removeMock, writeMock } = vi.hoisted(() => {
  const removeMock = vi.fn(() => Promise.resolve());
  return {
    dirMock: vi.fn(() => ({
      exists: vi.fn(() => Promise.resolve(true)),
      remove: removeMock,
    })),
    fileMock: vi.fn(() => ({
      exists: vi.fn(() => Promise.resolve(true)),
      remove: removeMock,
    })),
    removeMock,
    writeMock: vi.fn(() => Promise.resolve()),
  };
});

vi.mock("opfs-tools", () => ({
  dir: dirMock,
  file: fileMock,
  write: writeMock,
}));

vi.mock("uuid", () => ({
  v7: () => "018f0d7a-0000-7000-8000-000000000000",
}));

describe("OPFS browser-local authority", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    vi.unstubAllEnvs();
  });

  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it("blocks OPFS writes and deletes in server mode", async () => {
    vi.stubEnv("NEXT_PUBLIC_API_MODE", "server");
    const {
      BrowserLocalOPFSAuthorityError,
      deleteFromOPFS,
      deleteOPFSDirectory,
      saveToOPFS,
      writeToOPFS,
    } = await import("../utils/opfs");

    await expect(
      saveToOPFS(new File(["x"], "note.txt"), "chat/session-1"),
    ).rejects.toBeInstanceOf(BrowserLocalOPFSAuthorityError);
    await expect(
      writeToOPFS("opfs://chat/session-1/note.txt", "updated"),
    ).rejects.toBeInstanceOf(BrowserLocalOPFSAuthorityError);
    await expect(
      deleteFromOPFS("opfs://chat/session-1/note.txt"),
    ).rejects.toBeInstanceOf(BrowserLocalOPFSAuthorityError);
    await expect(deleteOPFSDirectory("chat")).rejects.toBeInstanceOf(
      BrowserLocalOPFSAuthorityError,
    );

    expect(writeMock).not.toHaveBeenCalled();
    expect(fileMock).not.toHaveBeenCalled();
    expect(dirMock).not.toHaveBeenCalled();
    expect(removeMock).not.toHaveBeenCalled();
  });

  it("keeps OPFS writes available in explicit local mode", async () => {
    vi.stubEnv("NEXT_PUBLIC_API_MODE", "local");
    const { deleteFromOPFS, deleteOPFSDirectory, saveToOPFS, writeToOPFS } =
      await import("../utils/opfs");

    await expect(
      saveToOPFS(new File(["x"], "note.txt"), "chat/session-1"),
    ).resolves.toBe(
      "opfs://chat/session-1/018f0d7a-0000-7000-8000-000000000000.txt",
    );
    await writeToOPFS("opfs://chat/session-1/note.txt", "updated");
    await deleteFromOPFS("opfs://chat/session-1/note.txt");
    await deleteOPFSDirectory("chat");

    expect(writeMock).toHaveBeenCalledTimes(2);
    expect(fileMock).toHaveBeenCalledWith("chat/session-1/note.txt");
    expect(dirMock).toHaveBeenCalledWith("chat");
    expect(removeMock).toHaveBeenCalledTimes(2);
  });
});
