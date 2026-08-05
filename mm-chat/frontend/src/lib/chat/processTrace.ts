import type { ProcessStep, ProcessStepKind, ProcessStepStatus } from "./types";

const PROCESS_STEP_KINDS = new Set<ProcessStepKind>([
  "reasoning",
  "knowledge",
  "web",
  "tool",
  "generation",
]);

const PROCESS_STEP_STATUSES = new Set<ProcessStepStatus>([
  "pending",
  "running",
  "awaiting_approval",
  "completed",
  "failed",
  "skipped",
  "cancelled",
]);

const SPECIALIZED_TOOL_KINDS: Readonly<Record<string, ProcessStepKind>> = {
  search_knowledge: "knowledge",
  search_web: "web",
};

const REDUNDANT_OUTCOME_DETAILS = new Set([
  "cancelled",
  "completed",
  "running",
  "streaming",
]);

const SAFE_OUTCOME_DETAILS = new Set([
  "answer_governance_required",
  "completed_unreferenced",
  "degraded",
  "dependency_unavailable",
  "no_evidence",
  "no_results",
]);

const PROCESS_DETAIL_KEYS = new Set([
  "hitCount",
  "sourceCount",
  "citationMarkers",
  "provider",
  "mode",
  "outcome",
  "failureCategory",
  "queryRewritten",
  "toolName",
  "round",
  "selectedCount",
  "truncated",
]);

export type ProcessRoute = "direct" | "knowledge" | "web" | "both";

export type ProcessReasonCategory =
  | "knowledge_miss"
  | "web_miss"
  | "planner_failed"
  | "provider_degraded"
  | "memory_indexing"
  | "memory_unavailable";

export interface ProcessRouteSummary {
  route: ProcessRoute;
  knowledgeSources: number;
  webSources: number;
}

export function normalizeProcessTrace(value: unknown): ProcessStep[] {
  if (!Array.isArray(value)) return [];

  const steps: ProcessStep[] = [];
  const indexes = new Map<string, number>();
  for (const candidate of value) {
    const step = normalizeProcessStep(candidate);
    if (!step) continue;
    const existingIndex = indexes.get(step.id);
    if (existingIndex === undefined) {
      indexes.set(step.id, steps.length);
      steps.push(step);
    } else {
      steps[existingIndex] = step;
    }
  }
  return steps;
}

export function normalizeProcessStep(value: unknown): ProcessStep | null {
  if (!isRecord(value)) return null;
  const id = stringValue(value.id);
  const kind = stringValue(value.kind) as ProcessStepKind;
  const status = stringValue(value.status) as ProcessStepStatus;
  const labelKey = stringValue(value.labelKey);
  if (
    !id ||
    !PROCESS_STEP_KINDS.has(kind) ||
    !PROCESS_STEP_STATUSES.has(status) ||
    labelKey !== `process.${kind}`
  ) {
    return null;
  }

  const durationMs = nonNegativeNumber(value.durationMs);
  const detail = normalizeProcessDetail(value.detail);
  return {
    id,
    kind,
    status,
    labelKey,
    ...(stringValue(value.startedAt)
      ? { startedAt: stringValue(value.startedAt) }
      : {}),
    ...(stringValue(value.completedAt)
      ? { completedAt: stringValue(value.completedAt) }
      : {}),
    ...(durationMs !== undefined ? { durationMs } : {}),
    ...(detail ? { detail } : {}),
  };
}

export function reasoningFromMessageMetadata(
  metadata: Record<string, unknown>,
): string | undefined {
  const reasoning = metadata.reasoning;
  return typeof reasoning === "string" && reasoning.length > 0
    ? reasoning
    : undefined;
}

export function processTraceFromMessageMetadata(
  metadata: Record<string, unknown>,
): ProcessStep[] | undefined {
  const trace = normalizeProcessTrace(metadata.processTrace);
  return trace.length > 0 ? trace : undefined;
}

export function upsertProcessStep(
  steps: ProcessStep[] | undefined,
  incoming: ProcessStep,
): ProcessStep[] {
  const next = [...(steps ?? [])];
  const index = next.findIndex((step) => step.id === incoming.id);
  if (index === -1) {
    next.push(incoming);
  } else {
    next[index] = incoming;
  }
  return next;
}

export function isProcessStepActive(step: ProcessStep): boolean {
  return (
    step.status === "pending" ||
    step.status === "running" ||
    step.status === "awaiting_approval"
  );
}

export function projectProcessStepsForDisplay(
  steps: ProcessStep[],
): ProcessStep[] {
  return steps.filter((step) => {
    if (step.kind !== "tool") return true;
    const toolName = processStepStringDetail(step, "toolName");
    const specializedKind = SPECIALIZED_TOOL_KINDS[toolName];
    if (!specializedKind) return true;
    return !steps.some(
      (candidate) =>
        candidate.kind === specializedKind &&
        representsSameToolExecution(step, candidate, toolName),
    );
  });
}

export function processOutcomeForDisplay(step: ProcessStep): string {
  const outcome = processStepStringDetail(step, "outcome");
  return REDUNDANT_OUTCOME_DETAILS.has(outcome) ||
    !SAFE_OUTCOME_DETAILS.has(outcome)
    ? ""
    : outcome;
}

export function summarizeProcessRoute(
  steps: readonly ProcessStep[],
): ProcessRouteSummary {
  const knowledgeSteps = steps.filter((step) => step.kind === "knowledge");
  const webSteps = steps.filter((step) => step.kind === "web");
  const knowledgeSources = knowledgeSteps.reduce(
    (total, step) => total + (processStepNumberDetail(step, "hitCount") ?? 0),
    0,
  );
  const webSources = webSteps.reduce(
    (total, step) =>
      total + (processStepNumberDetail(step, "sourceCount") ?? 0),
    0,
  );
  return {
    route:
      knowledgeSteps.length > 0 && webSteps.length > 0
        ? "both"
        : knowledgeSteps.length > 0
          ? "knowledge"
          : webSteps.length > 0
            ? "web"
            : "direct",
    knowledgeSources,
    webSources,
  };
}

export function processReasonCategoryForDisplay(
  step: ProcessStep,
): ProcessReasonCategory | undefined {
  const outcome = processStepStringDetail(step, "outcome");
  if (step.kind === "knowledge" && outcome === "no_evidence") {
    return "knowledge_miss";
  }
  if (step.kind === "web" && outcome === "no_results") {
    return "web_miss";
  }
  const failure = processStepStringDetail(step, "failureCategory");
  if (failure === "memory_indexing") {
    return "memory_indexing";
  }
  if (
    failure === "memory_service_unavailable" ||
    failure === "memory_status_unavailable" ||
    failure === "memory_disabled"
  ) {
    return "memory_unavailable";
  }
  if (failure === "planner_failed") {
    return "planner_failed";
  }
  if (
    failure === "provider_failed" ||
    failure === "dependency_unavailable" ||
    failure === "unavailable" ||
    failure === "answer_governance_required"
  ) {
    return "provider_degraded";
  }
  return undefined;
}

export function resolveProcessPanelExpanded(
  hasActiveStep: boolean,
  manualExpanded: boolean | null,
): boolean {
  return manualExpanded ?? hasActiveStep;
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function nonNegativeNumber(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) && value >= 0
    ? value
    : undefined;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function normalizeProcessDetail(
  value: unknown,
): Record<string, unknown> | undefined {
  if (!isRecord(value)) return undefined;
  const detail: Record<string, unknown> = {};
  for (const [key, candidate] of Object.entries(value)) {
    if (!PROCESS_DETAIL_KEYS.has(key)) continue;
    if (
      typeof candidate === "string" ||
      typeof candidate === "boolean" ||
      (typeof candidate === "number" &&
        Number.isFinite(candidate) &&
        candidate >= 0) ||
      (Array.isArray(candidate) &&
        candidate.every((item) => typeof item === "string"))
    ) {
      detail[key] = Array.isArray(candidate) ? [...candidate] : candidate;
    }
  }
  return Object.keys(detail).length > 0 ? detail : undefined;
}

function representsSameToolExecution(
  tool: ProcessStep,
  specialized: ProcessStep,
  toolName: string,
): boolean {
  if (processStepStringDetail(specialized, "toolName") !== toolName) {
    return false;
  }
  const toolRound = processStepNumberDetail(tool, "round");
  const specializedRound = processStepNumberDetail(specialized, "round");
  if (toolRound !== undefined || specializedRound !== undefined) {
    return toolRound === specializedRound;
  }
  const toolQuery = processStepStringDetail(tool, "query");
  const specializedQuery = processStepStringDetail(specialized, "query");
  return (
    toolQuery === "" ||
    specializedQuery === "" ||
    toolQuery === specializedQuery
  );
}

function processStepStringDetail(step: ProcessStep, key: string): string {
  const value = step.detail?.[key];
  return typeof value === "string" ? value : "";
}

function processStepNumberDetail(
  step: ProcessStep,
  key: string,
): number | undefined {
  const value = step.detail?.[key];
  return typeof value === "number" && Number.isFinite(value)
    ? value
    : undefined;
}
