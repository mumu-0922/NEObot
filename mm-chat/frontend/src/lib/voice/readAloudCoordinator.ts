import type { DisposableAudioElement } from "@/lib/utils/disposableAudio";
import {
  createSpeechSynthesisPoller,
  type DisposablePoller,
} from "@/lib/utils/speechPolling";

export type ReadAloudPhase = "idle" | "loading" | "playing";

export interface ReadAloudError {
  messageId: string;
  message: string;
}

export interface ReadAloudSnapshot {
  messageId: string | null;
  phase: ReadAloudPhase;
  error: ReadAloudError | null;
}

export interface ReadAloudRequest {
  messageId: string;
  synthesize: (signal: AbortSignal) => Promise<DisposableAudioElement | void>;
  formatError: (error: unknown) => string;
  onError?: (error: unknown) => void;
}

interface ReadAloudCoordinatorOptions {
  cancelBrowserSpeech?: () => void;
  isBrowserSpeechSpeaking?: () => boolean;
  createPoller?: (options: {
    isSpeaking: () => boolean;
    onIdle: () => void;
  }) => DisposablePoller;
}

export interface ReadAloudMessageState {
  isActive: boolean;
  isLoading: boolean;
  isPlaying: boolean;
  error: string | null;
}

const idleSnapshot: ReadAloudSnapshot = Object.freeze({
  messageId: null,
  phase: "idle",
  error: null,
});

function browserSpeechSynthesis(): SpeechSynthesis | null {
  if (typeof window === "undefined" || !("speechSynthesis" in window)) {
    return null;
  }
  return window.speechSynthesis;
}

export class ReadAloudCoordinator {
  private snapshot: ReadAloudSnapshot = idleSnapshot;
  private readonly listeners = new Set<() => void>();
  private operationId = 0;
  private abortController: AbortController | null = null;
  private audio: DisposableAudioElement | null = null;
  private poller: DisposablePoller | null = null;
  private readonly cancelBrowserSpeech: () => void;
  private readonly isBrowserSpeechSpeaking: () => boolean;
  private readonly createPoller: NonNullable<
    ReadAloudCoordinatorOptions["createPoller"]
  >;

  constructor(options: ReadAloudCoordinatorOptions = {}) {
    this.cancelBrowserSpeech =
      options.cancelBrowserSpeech ??
      (() => {
        browserSpeechSynthesis()?.cancel();
      });
    this.isBrowserSpeechSpeaking =
      options.isBrowserSpeechSpeaking ??
      (() => browserSpeechSynthesis()?.speaking === true);
    this.createPoller =
      options.createPoller ??
      ((pollerOptions) => createSpeechSynthesisPoller(pollerOptions));
  }

  readonly getSnapshot = (): ReadAloudSnapshot => this.snapshot;

  readonly getServerSnapshot = (): ReadAloudSnapshot => idleSnapshot;

  readonly subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  async toggle(request: ReadAloudRequest): Promise<void> {
    const messageId = request.messageId.trim();
    if (!messageId) return;

    if (
      this.snapshot.messageId === messageId &&
      this.snapshot.phase !== "idle"
    ) {
      this.stop(messageId);
      return;
    }

    await this.start({ ...request, messageId });
  }

  stop(messageId?: string): void {
    const normalizedMessageId = messageId?.trim();
    if (
      normalizedMessageId &&
      this.snapshot.messageId !== normalizedMessageId
    ) {
      return;
    }

    this.operationId += 1;
    this.disposeOwnedResources();
    this.updateSnapshot(idleSnapshot);
  }

  release(messageId: string): void {
    const normalizedMessageId = messageId.trim();
    if (!normalizedMessageId) return;

    if (this.snapshot.messageId === normalizedMessageId) {
      this.stop(normalizedMessageId);
      return;
    }
    if (this.snapshot.error?.messageId === normalizedMessageId) {
      this.updateSnapshot(idleSnapshot);
    }
  }

  private async start(request: ReadAloudRequest): Promise<void> {
    this.operationId += 1;
    const operationId = this.operationId;
    this.disposeOwnedResources();

    const abortController = new AbortController();
    this.abortController = abortController;
    this.updateSnapshot({
      messageId: request.messageId,
      phase: "loading",
      error: null,
    });

    let audio: DisposableAudioElement | void = undefined;
    try {
      audio = await request.synthesize(abortController.signal);
      if (!this.isCurrent(operationId, abortController)) {
        audio?.dispose();
        return;
      }

      if (audio) {
        this.audio = audio;
        audio.onended = () => this.finish(operationId, abortController);
        audio.onerror = () => {
          this.fail(
            operationId,
            abortController,
            request,
            new Error("Audio playback failed"),
          );
        };

        await audio.play();
        if (!this.isCurrent(operationId, abortController)) {
          audio.dispose();
          return;
        }
        this.updateSnapshot({
          messageId: request.messageId,
          phase: "playing",
          error: null,
        });
        return;
      }

      this.updateSnapshot({
        messageId: request.messageId,
        phase: "playing",
        error: null,
      });
      this.poller = this.createPoller({
        isSpeaking: this.isBrowserSpeechSpeaking,
        onIdle: () => this.finish(operationId, abortController),
      });
    } catch (error) {
      audio?.dispose();
      if (!this.isCurrent(operationId, abortController)) return;
      this.fail(operationId, abortController, request, error);
    }
  }

  private finish(operationId: number, abortController: AbortController): void {
    if (!this.isCurrent(operationId, abortController)) return;
    this.operationId += 1;
    this.disposeOwnedResources();
    this.updateSnapshot(idleSnapshot);
  }

  private fail(
    operationId: number,
    abortController: AbortController,
    request: ReadAloudRequest,
    error: unknown,
  ): void {
    if (!this.isCurrent(operationId, abortController)) return;
    request.onError?.(error);
    const message = request.formatError(error);
    this.operationId += 1;
    this.disposeOwnedResources();
    this.updateSnapshot({
      messageId: null,
      phase: "idle",
      error: { messageId: request.messageId, message },
    });
  }

  private isCurrent(
    operationId: number,
    abortController: AbortController,
  ): boolean {
    return (
      this.operationId === operationId &&
      this.abortController === abortController &&
      !abortController.signal.aborted
    );
  }

  private disposeOwnedResources(): void {
    this.abortController?.abort();
    this.abortController = null;

    if (this.audio) {
      this.audio.onended = null;
      this.audio.onerror = null;
      this.audio.dispose();
      this.audio = null;
    }

    this.poller?.dispose();
    this.poller = null;
    this.cancelBrowserSpeech();
  }

  private updateSnapshot(snapshot: ReadAloudSnapshot): void {
    if (this.snapshot === snapshot) return;
    this.snapshot = snapshot;
    for (const listener of Array.from(this.listeners)) listener();
  }
}

export function selectReadAloudMessageState(
  snapshot: ReadAloudSnapshot,
  messageId: string,
): ReadAloudMessageState {
  const isActive =
    snapshot.messageId === messageId && snapshot.phase !== "idle";
  return {
    isActive,
    isLoading: isActive && snapshot.phase === "loading",
    isPlaying: isActive && snapshot.phase === "playing",
    error:
      snapshot.error?.messageId === messageId ? snapshot.error.message : null,
  };
}

export const readAloudCoordinator = new ReadAloudCoordinator();
