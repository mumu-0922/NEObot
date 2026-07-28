import { describe, expect, it } from "vitest";
import {
  areMemoryActivitiesTerminal,
  summarizeMemoryActivities,
} from "../lib/memory/activity";
import type { MemoryActivity } from "../lib/memory/types";

describe("memory activity polling contract", () => {
  it("summarizes bounded user-visible outcomes without copying message data", () => {
    expect(
      summarizeMemoryActivities([
        activity("created", "completed"),
        activity("corrected", "completed"),
        activity("review_required", "review_required"),
        activity("capture", "failed"),
      ]),
    ).toEqual({ created: 1, corrected: 1, review: 1, failed: 1, total: 4 });
  });

  it("stops only after at least one activity exists and all are terminal", () => {
    expect(areMemoryActivitiesTerminal([])).toBe(false);
    expect(areMemoryActivitiesTerminal([activity("capture", "pending")])).toBe(
      false,
    );
    expect(
      areMemoryActivitiesTerminal([activity("created", "completed")]),
    ).toBe(true);
  });
});

function activity(action: string, status: string): MemoryActivity {
  return {
    id: `${action}-${status}`,
    assistantMessageId: "assistant-1",
    ordinal: 1,
    subjectType: "memory",
    subjectId: "memory-1",
    subjectRevision: 1,
    action,
    status,
    reasonCode: "BOUNDED_CODE",
    undoKind: "none",
    undoStatus: "unavailable",
    sourceKind: "memory_job",
    scopeType: "global",
    createdAt: 1,
    updatedAt: 1,
  };
}
