import { describe, expect, it } from "vitest";

import {
  isProcessStepActive,
  normalizeProcessStep,
  normalizeProcessTrace,
  processTraceFromMessageMetadata,
  reasoningFromMessageMetadata,
  resolveProcessPanelExpanded,
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
      detail: { query: "weather shanghai", sourceCount: 2 },
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
});
