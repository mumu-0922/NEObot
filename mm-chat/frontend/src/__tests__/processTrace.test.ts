import { describe, expect, it } from "vitest";

import {
  isProcessStepActive,
  normalizeProcessStep,
  normalizeProcessTrace,
  processOutcomeForDisplay,
  processReasonCategoryForDisplay,
  processTraceFromMessageMetadata,
  projectProcessStepsForDisplay,
  reasoningFromMessageMetadata,
  resolveProcessPanelExpanded,
  summarizeProcessRoute,
  upsertProcessStep,
} from "../lib/chat/processTrace";

describe("durable process trace", () => {
  it("normalizes ordered steps and replaces duplicate step ids in place", () => {
    const trace = normalizeProcessTrace([
      {
        id: "generation-1",
        kind: "generation",
        status: "running",
        labelKey: "process.generation",
      },
      {
        id: "generation-1",
        kind: "generation",
        status: "completed",
        labelKey: "process.generation",
        durationMs: 1250,
      },
      {
        id: "web-1",
        kind: "web",
        status: "failed",
        labelKey: "process.web",
      },
    ]);

    expect(trace).toMatchObject([
      { id: "generation-1", status: "completed", durationMs: 1250 },
      { id: "web-1", status: "failed" },
    ]);
    expect(trace.some(isProcessStepActive)).toBe(false);
  });

  it("rejects invalid steps and drops non-public detail fields", () => {
    expect(
      normalizeProcessStep({
        id: "tool-1",
        kind: "tool",
        status: "running",
        labelKey: "process.tool",
        detail: {
          query: "weather shanghai",
          sourceCount: 2,
          authorization: "Bearer fixture-secret",
          rawPayload: { hidden: true },
        },
      }),
    ).toEqual({
      id: "tool-1",
      kind: "tool",
      status: "running",
      labelKey: "process.tool",
      detail: { sourceCount: 2 },
    });
    expect(
      normalizeProcessStep({
        id: "bad",
        kind: "database",
        status: "running",
        labelKey: "process.database",
      }),
    ).toBeNull();
  });

  it("hydrates reasoning and process steps from server message metadata", () => {
    const metadata = {
      reasoning: "Provider summary",
      processTrace: [
        {
          id: "reasoning-1",
          kind: "reasoning",
          status: "completed",
          labelKey: "process.reasoning",
        },
      ],
    };
    expect(reasoningFromMessageMetadata(metadata)).toBe("Provider summary");
    expect(processTraceFromMessageMetadata(metadata)).toHaveLength(1);
  });

  it("upserts live step transitions without reordering other steps", () => {
    const running = normalizeProcessStep({
      id: "generation-1",
      kind: "generation",
      status: "running",
      labelKey: "process.generation",
    });
    const completed = normalizeProcessStep({
      id: "generation-1",
      kind: "generation",
      status: "completed",
      labelKey: "process.generation",
    });
    expect(running).not.toBeNull();
    expect(completed).not.toBeNull();
    expect(upsertProcessStep(running ? [running] : [], completed!)).toEqual([
      completed,
    ]);
  });

  it("auto-expands active work, collapses completion, and preserves manual choice", () => {
    expect(resolveProcessPanelExpanded(true, null)).toBe(true);
    expect(resolveProcessPanelExpanded(false, null)).toBe(false);
    expect(resolveProcessPanelExpanded(false, true)).toBe(true);
    expect(resolveProcessPanelExpanded(true, false)).toBe(false);
  });

  it("projects specialized search tools once while retaining backend diagnostics", () => {
    const steps = normalizeProcessTrace([
      {
        id: "tool-1",
        kind: "tool",
        status: "completed",
        labelKey: "process.tool",
        detail: {
          toolName: "search_web",
          round: 1,
          query: "Tokyo weather",
        },
      },
      {
        id: "web-1",
        kind: "web",
        status: "completed",
        labelKey: "process.web",
        detail: {
          toolName: "search_web",
          round: 1,
          query: "Tokyo weather",
        },
      },
      {
        id: "tool-2",
        kind: "tool",
        status: "failed",
        labelKey: "process.tool",
        detail: {
          toolName: "search_web",
          round: 2,
          query: "Tokyo forecast",
        },
      },
      {
        id: "tool-3",
        kind: "tool",
        status: "completed",
        labelKey: "process.tool",
        detail: { toolName: "search_knowledge", round: 3 },
      },
      {
        id: "knowledge-1",
        kind: "knowledge",
        status: "completed",
        labelKey: "process.knowledge",
        detail: { toolName: "search_knowledge", round: 3 },
      },
      {
        id: "tool-4",
        kind: "tool",
        status: "completed",
        labelKey: "process.tool",
        detail: { toolName: "custom_tool", round: 4 },
      },
    ]);

    expect(projectProcessStepsForDisplay(steps).map((step) => step.id)).toEqual(
      ["web-1", "tool-2", "knowledge-1", "tool-4"],
    );
    expect(steps.map((step) => step.id)).toEqual([
      "tool-1",
      "web-1",
      "tool-2",
      "tool-3",
      "knowledge-1",
      "tool-4",
    ]);
  });

  it("does not repeat lifecycle words already expressed by step status", () => {
    const streaming = normalizeProcessStep({
      id: "reasoning-1",
      kind: "reasoning",
      status: "completed",
      labelKey: "process.reasoning",
      detail: { outcome: "streaming" },
    });
    const degraded = normalizeProcessStep({
      id: "web-1",
      kind: "web",
      status: "failed",
      labelKey: "process.web",
      detail: { outcome: "degraded" },
    });

    expect(processOutcomeForDisplay(streaming!)).toBe("");
    expect(processOutcomeForDisplay(degraded!)).toBe("degraded");
  });

  it("summarizes Direct, Knowledge, Web, and Both without exposing queries", () => {
    expect(summarizeProcessRoute([])).toEqual({
      route: "direct",
      knowledgeSources: 0,
      webSources: 0,
    });
    const steps = normalizeProcessTrace([
      {
        id: "knowledge-1",
        kind: "knowledge",
        status: "completed",
        labelKey: "process.knowledge",
        detail: { hitCount: 2, query: "private exact query" },
      },
      {
        id: "web-1",
        kind: "web",
        status: "completed",
        labelKey: "process.web",
        detail: { sourceCount: 3, rawPayload: "forbidden" },
      },
    ]);
    expect(summarizeProcessRoute(steps)).toEqual({
      route: "both",
      knowledgeSources: 2,
      webSources: 3,
    });
    expect(JSON.stringify(steps)).not.toContain("private exact query");
    expect(JSON.stringify(steps)).not.toContain("forbidden");
  });

  it("maps only allowlisted reason categories for display", () => {
    const miss = normalizeProcessStep({
      id: "knowledge-1",
      kind: "knowledge",
      status: "completed",
      labelKey: "process.knowledge",
      detail: { outcome: "no_evidence" },
    });
    const raw = normalizeProcessStep({
      id: "web-1",
      kind: "web",
      status: "failed",
      labelKey: "process.web",
      detail: { failureCategory: "raw upstream stack trace" },
    });
    expect(processReasonCategoryForDisplay(miss!)).toBe("knowledge_miss");
    expect(processReasonCategoryForDisplay(raw!)).toBeUndefined();
    expect(processOutcomeForDisplay(raw!)).toBe("");
  });
});
