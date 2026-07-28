"use client";

import { Brain, ChevronDown, ChevronUp, Loader2, Undo2 } from "lucide-react";
import { useTranslations } from "next-intl";
import { useEffect, useMemo, useRef, useState } from "react";
import {
  areMemoryActivitiesTerminal,
  summarizeMemoryActivities,
} from "@/lib/memory/activity";
import type { MemoryActivity } from "@/lib/memory/types";
import { createNeoChatApiClient } from "@/services/api/client";

const POLL_INTERVAL_MS = 2_000;
const MAX_EMPTY_POLLS = 15;

interface MemoryActivityChipProps {
  assistantMessageId: string;
}

const MemoryActivityChip = ({
  assistantMessageId,
}: MemoryActivityChipProps) => {
  const t = useTranslations("Memory");
  const apiClient = useMemo(() => createNeoChatApiClient(), []);
  const enabled =
    apiClient.mode === "server" && apiClient.capabilities.memories;
  const rootRef = useRef<HTMLDivElement>(null);
  const [visible, setVisible] = useState(false);
  const [pageVisible, setPageVisible] = useState(true);
  const [activities, setActivities] = useState<MemoryActivity[]>([]);
  const [emptyPolls, setEmptyPolls] = useState(0);
  const [expanded, setExpanded] = useState(false);
  const [pollError, setPollError] = useState(false);
  const [undoingId, setUndoingId] = useState<string | null>(null);
  const [undoError, setUndoError] = useState(false);

  useEffect(() => {
    setActivities([]);
    setEmptyPolls(0);
    setExpanded(false);
    setPollError(false);
    setUndoError(false);
  }, [assistantMessageId]);

  useEffect(() => {
    setPageVisible(document.visibilityState === "visible");
    const handleVisibility = () =>
      setPageVisible(document.visibilityState === "visible");
    document.addEventListener("visibilitychange", handleVisibility);
    return () =>
      document.removeEventListener("visibilitychange", handleVisibility);
  }, []);

  useEffect(() => {
    const element = rootRef.current;
    if (!element) return;
    if (!("IntersectionObserver" in window)) {
      setVisible(true);
      return;
    }
    const observer = new IntersectionObserver(
      (entries) => setVisible(entries.some((entry) => entry.isIntersecting)),
      { rootMargin: "120px" },
    );
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  const terminal = areMemoryActivitiesTerminal(activities);
  const polling =
    enabled &&
    visible &&
    pageVisible &&
    !terminal &&
    emptyPolls < MAX_EMPTY_POLLS;

  useEffect(() => {
    if (!polling) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | null = null;
    const poll = async () => {
      try {
        const next = await apiClient.memories.listMessageMemoryActivities({
          assistantMessageId,
          limit: 20,
          signal: controller.signal,
        });
        if (controller.signal.aborted) return;
        setPollError(false);
        setActivities(next);
        if (next.length === 0) {
          setEmptyPolls((current) => current + 1);
        }
        if (!areMemoryActivitiesTerminal(next)) {
          timer = setTimeout(poll, POLL_INTERVAL_MS);
        }
      } catch {
        if (controller.signal.aborted) return;
        setPollError(true);
        setEmptyPolls((current) => current + 1);
        timer = setTimeout(poll, POLL_INTERVAL_MS);
      }
    };
    void poll();
    return () => {
      controller.abort();
      if (timer) clearTimeout(timer);
    };
  }, [apiClient, assistantMessageId, polling]);

  const undo = async (activity: MemoryActivity) => {
    if (!activity.subjectRevision || activity.undoStatus !== "available")
      return;
    setUndoingId(activity.id);
    setUndoError(false);
    try {
      const result = await apiClient.memories.undoMemoryActivity({
        activityId: activity.id,
        expectedRevision: activity.subjectRevision,
      });
      setActivities((current) =>
        current.map((item) =>
          item.id === activity.id
            ? {
                ...item,
                undoStatus:
                  result.status === "undone" ? "undone" : "review_required",
                memoryRevision: result.memoryRevision ?? item.memoryRevision,
              }
            : item,
        ),
      );
    } catch {
      setUndoError(true);
    } finally {
      setUndoingId(null);
    }
  };

  const summary = summarizeMemoryActivities(activities);
  const showChip = activities.length > 0;

  return (
    <div ref={rootRef} className="mt-1 min-h-px max-w-full">
      {showChip && (
        <div className="rounded-lg border border-cyan-200 bg-cyan-50/70 text-cyan-900 dark:border-cyan-900/60 dark:bg-cyan-950/20 dark:text-cyan-100">
          <button
            type="button"
            onClick={() => setExpanded((current) => !current)}
            aria-expanded={expanded}
            className="flex max-w-full items-center gap-1.5 px-2.5 py-1.5 text-[11px] font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
          >
            <Brain size={12} aria-hidden="true" />
            <span className="truncate">
              {t("activitySummary", {
                created: summary.created,
                corrected: summary.corrected,
                review: summary.review,
                failed: summary.failed,
              })}
            </span>
            {expanded ? (
              <ChevronUp size={12} aria-hidden="true" />
            ) : (
              <ChevronDown size={12} aria-hidden="true" />
            )}
          </button>
          {expanded && (
            <ul className="space-y-1 border-t border-cyan-200/70 p-2 dark:border-cyan-900/60">
              {activities.map((activity) => (
                <li
                  key={activity.id}
                  className="flex items-start justify-between gap-3 rounded-md bg-background/65 px-2 py-1.5 text-[11px]"
                >
                  <div className="min-w-0">
                    <div className="font-medium">
                      {t("activityItem", {
                        action: activity.action,
                        status: activity.status,
                      })}
                    </div>
                    <div className="mt-1 flex flex-wrap gap-1">
                      {activity.memoryType && (
                        <ActivityBadge>
                          {t(
                            `type${activity.memoryType[0].toUpperCase()}${activity.memoryType.slice(1)}`,
                          )}
                        </ActivityBadge>
                      )}
                      {activity.scopeType && (
                        <ActivityBadge>
                          {t(
                            `scope${activity.scopeType[0].toUpperCase()}${activity.scopeType.slice(1)}`,
                          )}
                        </ActivityBadge>
                      )}
                      <ActivityBadge>
                        {t(
                          activity.sourceKind === "direct_action"
                            ? "sourceDirectAction"
                            : activity.sourceKind === "review_suggestion"
                              ? "sourceReview"
                              : "sourceMemoryJob",
                        )}
                      </ActivityBadge>
                    </div>
                    <div className="mt-0.5 truncate text-muted-foreground">
                      {activity.memoryDeleted
                        ? t("memoryDeletedMarker")
                        : activity.memoryContent || activity.reasonCode}
                    </div>
                  </div>
                  {activity.undoStatus === "available" &&
                    activity.subjectRevision && (
                      <button
                        type="button"
                        onClick={() => void undo(activity)}
                        disabled={undoingId === activity.id}
                        aria-label={t("undoActivity")}
                        className="inline-flex shrink-0 items-center gap-1 rounded-md border border-border px-2 py-1 text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
                      >
                        {undoingId === activity.id ? (
                          <Loader2 size={11} className="animate-spin" />
                        ) : (
                          <Undo2 size={11} />
                        )}
                        {t("undo")}
                      </button>
                    )}
                </li>
              ))}
              {undoError && (
                <li
                  role="alert"
                  className="px-2 py-1 text-red-600 dark:text-red-300"
                >
                  {t("undoFailed")}
                </li>
              )}
            </ul>
          )}
        </div>
      )}
      {!showChip && pollError && emptyPolls >= MAX_EMPTY_POLLS && (
        <span className="sr-only" role="status">
          {t("activityUnavailable")}
        </span>
      )}
    </div>
  );
};

function ActivityBadge({ children }: { children: React.ReactNode }) {
  return (
    <span className="rounded bg-cyan-100/80 px-1.5 py-0.5 text-[10px] text-cyan-800 dark:bg-cyan-900/40 dark:text-cyan-100">
      {children}
    </span>
  );
}

export default MemoryActivityChip;
