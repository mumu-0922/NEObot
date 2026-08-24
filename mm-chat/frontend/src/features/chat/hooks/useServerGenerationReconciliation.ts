"use client";

import { useEffect } from "react";

import { logDevError } from "@/lib/utils/devLogger";

interface UseServerGenerationReconciliationOptions {
  enabled: boolean;
  activeGenerationCount: number;
  reconcile: () => Promise<boolean>;
  intervalMs?: number;
}

export function useServerGenerationReconciliation({
  enabled,
  activeGenerationCount,
  reconcile,
  intervalMs = 3_000,
}: UseServerGenerationReconciliationOptions) {
  useEffect(() => {
    if (!enabled || activeGenerationCount === 0) return;

    let disposed = false;
    let reconciling = false;
    let timer: ReturnType<typeof setTimeout> | null = null;

    const schedule = () => {
      if (disposed || document.visibilityState !== "visible") return;
      timer = setTimeout(() => {
        timer = null;
        void run();
      }, intervalMs);
    };

    const run = async () => {
      if (disposed || reconciling || document.visibilityState !== "visible") {
        return;
      }
      reconciling = true;
      try {
        await reconcile();
      } catch (error) {
        logDevError("Failed to reconcile active Conversation Runs", error);
      } finally {
        reconciling = false;
        schedule();
      }
    };

    const reconcileNow = () => {
      if (timer) {
        clearTimeout(timer);
        timer = null;
      }
      void run();
    };

    const handleVisibilityChange = () => {
      if (document.visibilityState === "visible") reconcileNow();
    };

    window.addEventListener("online", reconcileNow);
    document.addEventListener("visibilitychange", handleVisibilityChange);
    schedule();

    return () => {
      disposed = true;
      if (timer) clearTimeout(timer);
      window.removeEventListener("online", reconcileNow);
      document.removeEventListener("visibilitychange", handleVisibilityChange);
    };
  }, [activeGenerationCount, enabled, intervalMs, reconcile]);
}
