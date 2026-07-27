import { describe, expect, it, vi } from "vitest";
import type { DisposableAudioElement } from "@/lib/utils/disposableAudio";
import {
  ReadAloudCoordinator,
  selectReadAloudMessageState,
  type ReadAloudRequest,
} from "@/lib/voice/readAloudCoordinator";

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

function createAudio(play: () => Promise<void> = async () => undefined) {
  const audio = {
    dispose: vi.fn(),
    onended: null,
    onerror: null,
    play: vi.fn(play),
  } as unknown as DisposableAudioElement & {
    dispose: ReturnType<typeof vi.fn>;
    play: ReturnType<typeof vi.fn>;
  };
  return audio;
}

function createRequest(
  messageId: string,
  synthesize: ReadAloudRequest["synthesize"],
) {
  return {
    messageId,
    synthesize,
    formatError: (error: unknown) =>
      `localized: ${error instanceof Error ? error.message : String(error)}`,
    onError: vi.fn(),
  } satisfies ReadAloudRequest;
}

describe("ReadAloudCoordinator", () => {
  it("treats a rapid second click as cancellation and discards stale audio", async () => {
    const pendingAudio = deferred<DisposableAudioElement>();
    let signal: AbortSignal | undefined;
    const request = createRequest("message-a", (currentSignal) => {
      signal = currentSignal;
      return pendingAudio.promise;
    });
    const coordinator = new ReadAloudCoordinator({
      cancelBrowserSpeech: vi.fn(),
    });

    const firstToggle = coordinator.toggle(request);
    expect(coordinator.getSnapshot()).toMatchObject({
      messageId: "message-a",
      phase: "loading",
    });

    await coordinator.toggle(request);
    expect(signal?.aborted).toBe(true);
    expect(coordinator.getSnapshot().phase).toBe("idle");

    const staleAudio = createAudio();
    pendingAudio.resolve(staleAudio);
    await firstToggle;

    expect(staleAudio.play).not.toHaveBeenCalled();
    expect(staleAudio.dispose).toHaveBeenCalledOnce();
    expect(request.onError).not.toHaveBeenCalled();
  });

  it("stops message A before message B becomes the only active playback", async () => {
    const coordinator = new ReadAloudCoordinator({
      cancelBrowserSpeech: vi.fn(),
    });
    const audioA = createAudio();
    const audioB = createAudio();
    const requestA = createRequest("message-a", async () => audioA);
    const requestB = createRequest("message-b", async () => audioB);

    await coordinator.toggle(requestA);
    await coordinator.toggle(requestB);

    expect(audioA.dispose).toHaveBeenCalled();
    expect(audioB.play).toHaveBeenCalledOnce();
    expect(coordinator.getSnapshot()).toEqual({
      messageId: "message-b",
      phase: "playing",
      error: null,
    });
    expect(
      selectReadAloudMessageState(coordinator.getSnapshot(), "message-a"),
    ).toMatchObject({ isActive: false, isPlaying: false });
    expect(
      selectReadAloudMessageState(coordinator.getSnapshot(), "message-b"),
    ).toMatchObject({ isActive: true, isPlaying: true });
  });

  it("suppresses an interrupted stale play rejection during replacement", async () => {
    const pendingPlay = deferred<void>();
    const coordinator = new ReadAloudCoordinator({
      cancelBrowserSpeech: vi.fn(),
    });
    const audioA = createAudio(() => pendingPlay.promise);
    const requestA = createRequest("message-a", async () => audioA);
    const requestB = createRequest("message-b", async () => createAudio());

    const firstToggle = coordinator.toggle(requestA);
    await vi.waitFor(() => expect(audioA.play).toHaveBeenCalledOnce());

    await coordinator.toggle(requestB);
    pendingPlay.reject(
      new DOMException(
        "The play() request was interrupted because the media was removed from the document",
        "AbortError",
      ),
    );
    await firstToggle;

    expect(requestA.onError).not.toHaveBeenCalled();
    expect(coordinator.getSnapshot()).toMatchObject({
      messageId: "message-b",
      phase: "playing",
      error: null,
    });
  });

  it("aborts and disposes playback when the active message is released", async () => {
    const coordinator = new ReadAloudCoordinator({
      cancelBrowserSpeech: vi.fn(),
    });
    const audio = createAudio();
    let signal: AbortSignal | undefined;
    const request = createRequest("message-a", async (currentSignal) => {
      signal = currentSignal;
      return audio;
    });

    await coordinator.toggle(request);
    coordinator.release("message-a");

    expect(signal?.aborted).toBe(true);
    expect(audio.dispose).toHaveBeenCalled();
    expect(coordinator.getSnapshot().phase).toBe("idle");
  });

  it("keeps a genuine synthesis failure visible", async () => {
    const coordinator = new ReadAloudCoordinator({
      cancelBrowserSpeech: vi.fn(),
    });
    const failure = new Error("provider unavailable");
    const request = createRequest("message-a", async () => {
      throw failure;
    });

    await coordinator.toggle(request);

    expect(request.onError).toHaveBeenCalledOnce();
    expect(request.onError).toHaveBeenCalledWith(failure);
    expect(coordinator.getSnapshot()).toEqual({
      messageId: null,
      phase: "idle",
      error: {
        messageId: "message-a",
        message: "localized: provider unavailable",
      },
    });
  });

  it("keeps a genuine audio playback failure visible", async () => {
    const coordinator = new ReadAloudCoordinator({
      cancelBrowserSpeech: vi.fn(),
    });
    const failure = new Error("autoplay denied");
    const audio = createAudio(async () => {
      throw failure;
    });
    const request = createRequest("message-a", async () => audio);

    await coordinator.toggle(request);

    expect(request.onError).toHaveBeenCalledWith(failure);
    expect(coordinator.getSnapshot()).toEqual({
      messageId: null,
      phase: "idle",
      error: {
        messageId: "message-a",
        message: "localized: autoplay denied",
      },
    });
  });

  it("cancels browser speech on replacement and returns to idle via its poller", async () => {
    const cancelBrowserSpeech = vi.fn();
    const pollerDisposals: Array<ReturnType<typeof vi.fn>> = [];
    const idleCallbacks: Array<() => void> = [];
    const coordinator = new ReadAloudCoordinator({
      cancelBrowserSpeech,
      isBrowserSpeechSpeaking: () => true,
      createPoller: ({ onIdle }) => {
        const dispose = vi.fn();
        idleCallbacks.push(onIdle);
        pollerDisposals.push(dispose);
        return { dispose };
      },
    });

    await coordinator.toggle(createRequest("message-a", async () => undefined));
    cancelBrowserSpeech.mockClear();

    await coordinator.toggle(createRequest("message-b", async () => undefined));

    expect(pollerDisposals[0]).toHaveBeenCalledOnce();
    expect(cancelBrowserSpeech).toHaveBeenCalledOnce();
    expect(coordinator.getSnapshot()).toMatchObject({
      messageId: "message-b",
      phase: "playing",
    });

    idleCallbacks[1]?.();
    expect(coordinator.getSnapshot().phase).toBe("idle");
    expect(pollerDisposals[1]).toHaveBeenCalledOnce();
  });
});
