"use client";

import { useCallback, useRef, useState } from "react";

import {
  createActiveGenerationSyncSnapshot,
  getNextGenerationRunId,
  isCurrentGenerationRun,
  type ActiveGenerationSyncSnapshot,
} from "@/lib/chat/generationLifecycle";
import { useChatStore } from "@/store/core/chatStore";

export interface ActiveGenerationRun {
  runId: number;
  sessionId: string;
  controller: AbortController;
}

interface UseChatGenerationControllerOptions {
  persistStoppedGeneration?: (
    snapshot: ActiveGenerationSyncSnapshot,
  ) => Promise<void>;
}

export function useChatGenerationController({
  persistStoppedGeneration,
}: UseChatGenerationControllerOptions = {}) {
  const [activeSessionIds, setActiveSessionIds] = useState<string[]>([]);
  const [unreadSessionIds, setUnreadSessionIds] = useState<string[]>([]);
  const activeRunsRef = useRef(new Map<string, ActiveGenerationRun>());
  const generationRunRef = useRef(0);

  const beginActiveGeneration = useCallback((sessionId: string) => {
    const existing = activeRunsRef.current.get(sessionId);
    if (existing) return existing;
    const runId = getNextGenerationRunId(generationRunRef.current);
    const controller = new AbortController();
    const run = { runId, sessionId, controller };
    generationRunRef.current = runId;
    activeRunsRef.current.set(sessionId, run);
    setActiveSessionIds(Array.from(activeRunsRef.current.keys()));
    setUnreadSessionIds((current) =>
      current.filter((candidate) => candidate !== sessionId),
    );

    return run;
  }, []);

  const isGenerationRunActive = useCallback(
    ({ runId, sessionId, controller }: ActiveGenerationRun) => {
      const current = activeRunsRef.current.get(sessionId);
      return (
        current !== undefined &&
        isCurrentGenerationRun({
          currentRunId: current.runId,
          runId,
          currentController: current.controller,
          controller,
        })
      );
    },
    [],
  );

  const finishActiveGeneration = useCallback(
    (run: ActiveGenerationRun, selectedSessionId?: string | null) => {
      if (!isGenerationRunActive(run)) return;

      activeRunsRef.current.delete(run.sessionId);
      setActiveSessionIds(Array.from(activeRunsRef.current.keys()));
      if (selectedSessionId !== run.sessionId) {
        setUnreadSessionIds((current) =>
          current.includes(run.sessionId)
            ? current
            : [...current, run.sessionId],
        );
      }
    },
    [isGenerationRunActive],
  );

  const abortActiveGeneration = useCallback((sessionId: string) => {
    const active = activeRunsRef.current.get(sessionId);
    if (!active) return false;
    activeRunsRef.current.delete(sessionId);
    active.controller.abort();
    setActiveSessionIds(Array.from(activeRunsRef.current.keys()));
    return true;
  }, []);

  const stopActiveGeneration = useCallback(
    async (sessionId: string) => {
      const state = useChatStore.getState();
      const syncSnapshot = createActiveGenerationSyncSnapshot({
        currentSessionId:
          state.currentSessionId === sessionId ? state.currentSessionId : null,
        activeMessages: state.activeMessages,
      });

      abortActiveGeneration(sessionId);

      if (!syncSnapshot) return;

      if (persistStoppedGeneration) {
        await persistStoppedGeneration(syncSnapshot);
        return;
      }

      await state.syncActiveSession(
        syncSnapshot.sessionId,
        syncSnapshot.messages,
      );
    },
    [abortActiveGeneration, persistStoppedGeneration],
  );

  const clearGenerationUnread = useCallback((sessionId: string) => {
    setUnreadSessionIds((current) =>
      current.filter((candidate) => candidate !== sessionId),
    );
  }, []);

  const isSessionGenerating = (sessionId: string | null | undefined) =>
    Boolean(sessionId && activeSessionIds.includes(sessionId));

  return {
    activeSessionIds,
    unreadSessionIds,
    isSessionGenerating,
    beginActiveGeneration,
    isGenerationRunActive,
    finishActiveGeneration,
    abortActiveGeneration,
    stopActiveGeneration,
    clearGenerationUnread,
  };
}
