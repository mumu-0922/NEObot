import type { MemoryActivity } from "./types";

export interface MemoryActivitySummary {
  created: number;
  corrected: number;
  review: number;
  failed: number;
  total: number;
}

const pendingStatuses = new Set(["pending", "queued", "running"]);

export function summarizeMemoryActivities(
  activities: MemoryActivity[],
): MemoryActivitySummary {
  const summary: MemoryActivitySummary = {
    created: 0,
    corrected: 0,
    review: 0,
    failed: 0,
    total: activities.length,
  };
  for (const activity of activities) {
    if (activity.action === "created") summary.created += 1;
    if (activity.action === "corrected") summary.corrected += 1;
    if (
      activity.action === "review_required" ||
      activity.status === "review_required"
    ) {
      summary.review += 1;
    }
    if (activity.status === "failed" || activity.status === "dead_letter") {
      summary.failed += 1;
    }
  }
  return summary;
}

export function areMemoryActivitiesTerminal(
  activities: MemoryActivity[],
): boolean {
  return (
    activities.length > 0 &&
    activities.every((activity) => !pendingStatuses.has(activity.status))
  );
}
