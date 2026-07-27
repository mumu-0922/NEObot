import { describe, expect, it, vi } from "vitest";
import {
  attachObjectUrlDisposal,
  createDisposableAudioFromBlob,
} from "../lib/utils/disposableAudio";

function createFakeAudio() {
  const listeners = new Map<string, EventListener>();
  const callOrder: string[] = [];
  return {
    paused: false,
    pause: vi.fn(() => callOrder.push("pause")),
    load: vi.fn(() => callOrder.push("load")),
    remove: vi.fn(() => callOrder.push("remove")),
    removeAttribute: vi.fn(() => callOrder.push("removeAttribute")),
    addEventListener: vi.fn((event: string, listener: EventListener) => {
      listeners.set(event, listener);
    }),
    removeEventListener: vi.fn((event: string) => {
      listeners.delete(event);
    }),
    emit(event: string) {
      listeners.get(event)?.(new Event(event));
    },
    callOrder,
  } as unknown as HTMLAudioElement & {
    emit: (event: string) => void;
    callOrder: string[];
  };
}

describe("disposable audio helpers", () => {
  it("revokes object URLs once when disposed", () => {
    const audio = createFakeAudio();
    const revoke = vi.fn(() => audio.callOrder.push("revoke"));
    const disposable = attachObjectUrlDisposal(audio, "blob:test", revoke);

    disposable.dispose();
    disposable.dispose();

    expect(audio.pause).toHaveBeenCalledTimes(1);
    expect(audio.removeAttribute).toHaveBeenCalledWith("src");
    expect(audio.load).toHaveBeenCalledTimes(1);
    expect(audio.remove).toHaveBeenCalledTimes(1);
    expect(revoke).toHaveBeenCalledTimes(1);
    expect(revoke).toHaveBeenCalledWith("blob:test");
    expect(audio.callOrder).toEqual([
      "pause",
      "removeAttribute",
      "load",
      "remove",
      "revoke",
    ]);
  });

  it("revokes object URLs when playback ends", () => {
    const audio = createFakeAudio();
    const revoke = vi.fn();
    attachObjectUrlDisposal(audio, "blob:ended", revoke);

    audio.emit("ended");

    expect(revoke).toHaveBeenCalledWith("blob:ended");
  });

  it("creates disposable audio from a blob", () => {
    const audio = createFakeAudio();
    const createObjectUrl = vi.fn(() => "blob:created");
    const revoke = vi.fn();

    const disposable = createDisposableAudioFromBlob(
      new Blob(["audio"]),
      () => audio,
      createObjectUrl,
      revoke,
    );

    disposable.dispose();

    expect(createObjectUrl).toHaveBeenCalledTimes(1);
    expect(revoke).toHaveBeenCalledWith("blob:created");
  });

  it("attaches the default audio element before assigning its blob URL", () => {
    const audio = createFakeAudio();
    let connected = false;
    let assignedWhileConnected = false;
    let assignedSrc = "";
    Object.defineProperties(audio, {
      isConnected: {
        get: () => connected,
      },
      src: {
        get: () => assignedSrc,
        set: (value: string) => {
          assignedSrc = value;
          assignedWhileConnected = connected;
        },
      },
    });
    Object.assign(audio, {
      hidden: false,
      preload: "",
      setAttribute: vi.fn(),
      remove: vi.fn(() => {
        connected = false;
      }),
    });
    const append = vi.fn(() => {
      connected = true;
    });
    vi.stubGlobal("document", {
      body: { append },
      createElement: vi.fn(() => audio),
    });
    const createObjectUrl = vi.fn(() => "blob:attached");
    const revoke = vi.fn();

    try {
      const disposable = createDisposableAudioFromBlob(
        new Blob(["audio"]),
        undefined,
        createObjectUrl,
        revoke,
      );

      expect(append).toHaveBeenCalledWith(audio);
      expect(assignedSrc).toBe("blob:attached");
      expect(assignedWhileConnected).toBe(true);
      expect(audio.hidden).toBe(true);
      expect(audio.setAttribute).toHaveBeenCalledWith("aria-hidden", "true");

      disposable.dispose();
      expect(audio.remove).toHaveBeenCalledTimes(1);
      expect(connected).toBe(false);
    } finally {
      vi.unstubAllGlobals();
    }
  });
});
