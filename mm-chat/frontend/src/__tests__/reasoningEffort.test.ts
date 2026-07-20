import { describe, expect, it } from "vitest";
import {
  getReasoningEffortOptions,
  isReasoningEffort,
  normalizeReasoningEffort,
  resolveOpenAIReasoningEffort,
} from "../lib/chat/reasoning";

describe("reasoning effort", () => {
  it("normalizes only the supported semantic levels", () => {
    expect(isReasoningEffort("high")).toBe(true);
    expect(isReasoningEffort("unbounded")).toBe(false);
    expect(normalizeReasoningEffort(" XHIGH ")).toBe("xhigh");
    expect(normalizeReasoningEffort("unknown")).toBe("auto");
  });

  it("shows extended levels only for compatible model families", () => {
    expect(getReasoningEffortOptions("reasoning-model")).toEqual([
      "auto",
      "low",
      "medium",
      "high",
    ]);
    expect(getReasoningEffortOptions("gpt-5.4")).toContain("xhigh");
    expect(getReasoningEffortOptions("gpt-5.4")).not.toContain("max");
    expect(getReasoningEffortOptions("gpt-5.6-sol")).toEqual([
      "auto",
      "low",
      "medium",
      "high",
      "xhigh",
      "max",
    ]);
  });

  it("clamps unsupported OpenAI-compatible levels safely", () => {
    expect(resolveOpenAIReasoningEffort("gpt-5.6", "max")).toBe("max");
    expect(resolveOpenAIReasoningEffort("gpt-5.4", "max")).toBe("xhigh");
    expect(resolveOpenAIReasoningEffort("custom-model", "xhigh")).toBe("high");
    expect(resolveOpenAIReasoningEffort("custom-model", "auto")).toBe(
      undefined,
    );
  });
});
