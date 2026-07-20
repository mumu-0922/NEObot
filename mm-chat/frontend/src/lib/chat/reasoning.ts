import type { ReasoningEffort } from "./types";

const BASE_REASONING_EFFORTS = [
  "auto",
  "low",
  "medium",
  "high",
] as const satisfies readonly ReasoningEffort[];

const REASONING_EFFORT_SET = new Set<ReasoningEffort>([
  ...BASE_REASONING_EFFORTS,
  "xhigh",
  "max",
]);

export function isReasoningEffort(value: unknown): value is ReasoningEffort {
  if (typeof value !== "string") return false;
  return (
    value === value.trim().toLowerCase() &&
    REASONING_EFFORT_SET.has(value as ReasoningEffort)
  );
}

export function normalizeReasoningEffort(
  value: unknown,
  fallback: ReasoningEffort = "auto",
): ReasoningEffort {
  if (typeof value !== "string") return fallback;
  const normalized = value.trim().toLowerCase() as ReasoningEffort;
  return REASONING_EFFORT_SET.has(normalized) ? normalized : fallback;
}

export function getReasoningEffortOptions(
  modelId: string,
): readonly ReasoningEffort[] {
  const efforts: ReasoningEffort[] = [...BASE_REASONING_EFFORTS];
  if (supportsXHighReasoning(modelId)) efforts.push("xhigh");
  if (supportsMaxReasoning(modelId)) efforts.push("max");
  return efforts;
}

export function resolveOpenAIReasoningEffort(
  modelId: string,
  requested: ReasoningEffort,
): Exclude<ReasoningEffort, "auto"> | undefined {
  const effort = normalizeReasoningEffort(requested);
  if (effort === "auto") return undefined;
  if (effort === "max" && !supportsMaxReasoning(modelId)) {
    return supportsXHighReasoning(modelId) ? "xhigh" : "high";
  }
  if (effort === "xhigh" && !supportsXHighReasoning(modelId)) return "high";
  return effort;
}

function supportsXHighReasoning(modelId: string): boolean {
  const model = modelId.trim().toLowerCase();
  return (
    model.includes("deepseek") ||
    isModelFamily(model, "gpt-5.2") ||
    isModelFamily(model, "gpt-5.3") ||
    isModelFamily(model, "gpt-5.4") ||
    isModelFamily(model, "gpt-5.5") ||
    isModelFamily(model, "gpt-5.6")
  );
}

function supportsMaxReasoning(modelId: string): boolean {
  return isModelFamily(modelId.trim().toLowerCase(), "gpt-5.6");
}

function isModelFamily(modelId: string, family: string): boolean {
  return (
    modelId === family ||
    modelId.startsWith(`${family}-`) ||
    modelId.startsWith(`${family}.`)
  );
}
