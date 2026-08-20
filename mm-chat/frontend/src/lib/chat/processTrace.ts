import type {
  ChatAgentEvent,
  ChatAgentEventType,
  ProcessStep,
  ProcessStepKind,
  ProcessStepStatus,
  ProcessStepPresentation,
  ProcessTranscriptEntry,
  ProcessApprovalPresentation,
  ProcessRetryPresentation,
} from "./types";

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
  "outcome_unknown",
  "interrupted",
]);

const CHAT_AGENT_EVENT_TYPES = new Set<ChatAgentEventType>([
  "turn.started",
  "turn.ended",
  "step.started",
  "step.ended",
  "assistant.message",
  "tool.called",
  "tool.result",
  "goal.changed",
  "goal.round.started",
  "context.replaced",
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
  "server",
  "serverName",
  "classification",
  "callStatus",
  "callId",
  "retryOf",
  "argumentSummary",
  "round",
  "selectedCount",
  "truncated",
  "durability",
]);

const MAX_TERMINAL_COMMAND_BYTES = 4096;
const MAX_TERMINAL_CWD_BYTES = 1024;
const MAX_PRESENTATION_TEXT_BYTES = 64 * 1024;
const MAX_PRESENTATION_ITEMS = 64;

export type ProcessRoute = "direct" | "knowledge" | "web" | "both";

export type ProcessReasonCategory =
  | "knowledge_miss"
  | "web_miss"
  | "planner_failed"
  | "provider_degraded"
  | "memory_indexing"
  | "memory_unavailable"
  | "stream_gap";

export interface ProcessRouteSummary {
  route: ProcessRoute;
  knowledgeSources: number;
  webSources: number;
  toolCalls: number;
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
  const presentation = normalizeProcessStepPresentation(
    value.presentation,
    kind,
    detail,
  );
  if (status !== "failed" && presentation && "retry" in presentation) {
    delete presentation.retry;
  }
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
    ...(presentation ? { presentation } : {}),
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

export function normalizeChatAgentEvent(value: unknown): ChatAgentEvent | null {
  if (!isRecord(value)) return null;
  const eventId = stringValue(value.eventId);
  const turnId = stringValue(value.turnId);
  const conversationId = stringValue(value.conversationId);
  const messageId = stringValue(value.messageId);
  const runId = stringValue(value.runId);
  const type = stringValue(value.type) as ChatAgentEventType;
  const sequence = positiveInteger(value.sequence);
  const stepSequence = optionalPositiveInteger(value.stepSequence);
  const occurredAt = stringValue(value.occurredAt);
  if (
    !eventId ||
    !turnId ||
    !conversationId ||
    !messageId ||
    !runId ||
    !CHAT_AGENT_EVENT_TYPES.has(type) ||
    sequence === undefined ||
    (value.stepSequence !== undefined && stepSequence === undefined) ||
    !occurredAt ||
    !Number.isFinite(Date.parse(occurredAt)) ||
    !isRecord(value.payload)
  ) {
    return null;
  }
  return {
    eventId,
    turnId,
    conversationId,
    messageId,
    runId,
    sequence,
    type,
    ...(stepSequence !== undefined ? { stepSequence } : {}),
    payload: { ...value.payload },
    occurredAt,
  };
}

export function normalizeChatAgentEvents(value: unknown): ChatAgentEvent[] {
  if (!Array.isArray(value)) return [];
  const events: ChatAgentEvent[] = [];
  const eventIds = new Set<string>();
  for (const candidate of value) {
    const event = normalizeChatAgentEvent(candidate);
    if (!event || eventIds.has(event.eventId)) continue;
    eventIds.add(event.eventId);
    events.push(event);
  }
  return events.sort(
    (left, right) =>
      left.sequence - right.sequence ||
      left.eventId.localeCompare(right.eventId),
  );
}

export function processTraceFromChatAgentEvents(
  value: unknown,
  legacy: ProcessStep[] | undefined = undefined,
): ProcessStep[] | undefined {
  const events = normalizeChatAgentEvents(value);
  if (events.length === 0) return legacy;

  let steps: ProcessStep[] = [];
  let interruptedAt = "";
  for (const event of events) {
    const processStep = normalizeProcessStep(event.payload.processStep);
    if (processStep) {
      steps = upsertProcessStep(steps, processStep);
    }
    if (Array.isArray(event.payload.processSteps)) {
      for (const candidate of event.payload.processSteps) {
        const step = normalizeProcessStep(candidate);
        if (step) steps = upsertProcessStep(steps, step);
      }
    }
    if (
      event.type === "turn.ended" &&
      stringValue(event.payload.status) === "interrupted"
    ) {
      interruptedAt = event.occurredAt;
    }
  }
  if (steps.length === 0) return legacy;
  if (interruptedAt) {
    steps = steps.map((step) => {
      if (isUnresolvedProcessLocalJob(step)) {
        return interruptProcessStep(step, interruptedAt, true);
      }
      return isProcessStepActive(step)
        ? interruptProcessStep(step, interruptedAt)
        : step;
    });
  }
  return steps;
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
  const visible = steps.filter((step) => {
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
  return mergeJobLifecycleSteps(visible);
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
  const toolCalls = steps.filter((step) => step.kind === "tool").length;
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
    toolCalls,
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
  if (failure === "stream_gap") {
    return "stream_gap";
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

export function humanizeToolName(value: unknown): string {
  if (typeof value !== "string") return "";
  const normalized = value
    .trim()
    .replace(/([a-z0-9])([A-Z])/g, "$1 $2")
    .replace(/[._-]+/g, " ")
    .replace(/\s+/g, " ")
    .trim();
  if (!normalized) return "";
  const [first = "", ...rest] = Array.from(normalized);
  return `${first.toUpperCase()}${rest.join("").toLowerCase()}`;
}

export function processToolLabelForDisplay(step: ProcessStep): string {
  if (step.kind !== "tool") return "";
  const serverName = processStepStringDetail(step, "serverName");
  const toolName = humanizeToolName(processStepStringDetail(step, "toolName"));
  if (serverName && toolName) return `${serverName} · ${toolName}`;
  return serverName || toolName;
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function nonNegativeNumber(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) && value >= 0
    ? value
    : undefined;
}

function positiveInteger(value: unknown): number | undefined {
  return typeof value === "number" && Number.isInteger(value) && value >= 1
    ? value
    : undefined;
}

function optionalPositiveInteger(value: unknown): number | undefined {
  return value === undefined ? undefined : positiveInteger(value);
}

function interruptProcessStep(
  step: ProcessStep,
  interruptedAt: string,
  interruptJob = false,
): ProcessStep {
  const startedAt = step.startedAt ? Date.parse(step.startedAt) : Number.NaN;
  const completedAt = Date.parse(interruptedAt);
  const durationMs =
    Number.isFinite(startedAt) && Number.isFinite(completedAt)
      ? Math.max(0, completedAt - startedAt)
      : step.durationMs;
  return {
    ...step,
    status: "interrupted",
    completedAt: interruptedAt,
    ...(durationMs !== undefined ? { durationMs } : {}),
    detail: {
      ...(step.detail ?? {}),
      failureCategory: "interrupted",
      outcome: "interrupted",
    },
    ...(interruptJob && step.presentation?.card === "job"
      ? {
          presentation: {
            ...step.presentation,
            jobStatus: "interrupted",
          },
        }
      : {}),
  };
}

function isUnresolvedProcessLocalJob(step: ProcessStep): boolean {
  return (
    step.detail?.durability === "process_local" &&
    step.presentation?.card === "job" &&
    (step.presentation.jobStatus === "running" ||
      step.presentation.jobStatus === "stopping")
  );
}

function mergeJobLifecycleSteps(steps: ProcessStep[]): ProcessStep[] {
  const merged: ProcessStep[] = [];
  const lifecycleIndex = new Map<string, number>();
  for (const step of steps) {
    const presentation = step.presentation;
    if (presentation?.card !== "job" || !presentation.jobId) {
      merged.push(step);
      continue;
    }
    const index = lifecycleIndex.get(presentation.jobId);
    if (index === undefined) {
      lifecycleIndex.set(presentation.jobId, merged.length);
      merged.push(projectJobLifecycleStep(step));
      continue;
    }
    merged[index] = mergeJobLifecycleStep(merged[index], step);
  }
  return merged;
}

function projectJobLifecycleStep(step: ProcessStep): ProcessStep {
  const presentation = step.presentation;
  if (presentation?.card !== "job") return step;
  const status = processStatusForJobLifecycle(
    presentation.jobStatus,
    step.status,
  );
  const projected: ProcessStep = {
    ...step,
    status,
    startedAt: presentation.jobStartedAt ?? step.startedAt,
    detail: {
      ...(step.detail ?? {}),
      toolName: "job_lifecycle",
    },
    presentation: { ...presentation },
  };
  if (status === "running" || status === "pending") {
    delete projected.completedAt;
    delete projected.durationMs;
  } else {
    projected.completedAt = presentation.jobCompletedAt ?? step.completedAt;
    projected.durationMs = presentation.jobDurationMs ?? step.durationMs;
  }
  return projected;
}

function mergeJobLifecycleStep(
  current: ProcessStep,
  incoming: ProcessStep,
): ProcessStep {
  const currentPresentation = current.presentation;
  const incomingPresentation = incoming.presentation;
  if (
    currentPresentation?.card !== "job" ||
    incomingPresentation?.card !== "job"
  ) {
    return current;
  }
  const presentation = {
    ...currentPresentation,
    ...incomingPresentation,
    operation: "lifecycle",
    command: currentPresentation.command ?? incomingPresentation.command,
    cwd: currentPresentation.cwd ?? incomingPresentation.cwd,
    jobStartedAt:
      currentPresentation.jobStartedAt ?? incomingPresentation.jobStartedAt,
    transcript:
      incomingPresentation.transcript ?? currentPresentation.transcript,
  };
  const status = processStatusForJobLifecycle(
    presentation.jobStatus,
    incoming.status,
  );
  const merged: ProcessStep = {
    ...current,
    status,
    detail: {
      ...(incoming.detail ?? {}),
      ...(current.detail ?? {}),
      toolName: "job_lifecycle",
    },
    presentation,
  };
  if (status === "running" || status === "pending") {
    delete merged.completedAt;
    delete merged.durationMs;
  } else {
    merged.completedAt =
      presentation.jobCompletedAt ??
      incoming.completedAt ??
      current.completedAt;
    merged.durationMs =
      presentation.jobDurationMs ?? incoming.durationMs ?? current.durationMs;
  }
  return merged;
}

function processStatusForJobLifecycle(
  jobStatus: string | undefined,
  fallback: ProcessStepStatus,
): ProcessStepStatus {
  switch (jobStatus) {
    case "running":
    case "stopping":
      return "running";
    case "completed":
      return "completed";
    case "failed":
      return "failed";
    case "killed":
      return "cancelled";
    case "interrupted":
    case "unknown":
      return "interrupted";
    default:
      return fallback;
  }
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

function normalizeProcessStepPresentation(
  value: unknown,
  kind: ProcessStepKind,
  detail: Record<string, unknown> | undefined,
): ProcessStepPresentation | undefined {
  if (!isRecord(value)) return undefined;
  const version = value.version === undefined ? 1 : value.version;
  if (version !== 1 || typeof value.card !== "string") return undefined;
  const card = value.card;
  const toolName = detail?.toolName;
  const mode = detail?.mode;
  const valid =
    (card === "terminal" &&
      kind === "tool" &&
      toolName === "terminal" &&
      mode === "local_direct") ||
    (card === "search" &&
      (kind === "web" ||
        kind === "knowledge" ||
        toolName === "search_memory")) ||
    (card === "file" &&
      kind === "tool" &&
      mode === "local_direct" &&
      typeof toolName === "string" &&
      (toolName.startsWith("file_") || toolName === "publish_file")) ||
    (card === "job" &&
      kind === "tool" &&
      mode === "local_direct" &&
      typeof toolName === "string" &&
      (toolName.startsWith("job_") ||
        (toolName === "terminal" && value.background === true))) ||
    (card === "skill" &&
      kind === "tool" &&
      mode === "local_direct" &&
      toolName === "skill") ||
    (card === "goal" && kind === "tool" && mode === "goal") ||
    ((card === "mcp" || card === "browser") &&
      kind === "tool" &&
      mode === "mcp");
  if (!valid) return undefined;

  if (card !== "terminal") {
    const text = (key: string, maxBytes = 2048) =>
      value[key] === undefined
        ? undefined
        : boundedPresentationString(value[key], maxBytes) || null;
    const title = text("title");
    const summary = text("summary");
    const provider = text("provider", 256);
    const query = text("query");
    const operation = text("operation", 128);
    const path = text("path", 4096);
    const content = text("content", MAX_PRESENTATION_TEXT_BYTES);
    const diff = text("diff", MAX_PRESENTATION_TEXT_BYTES);
    const jobId = text("jobId", 128);
    const jobStatus = text("jobStatus", 64);
    const jobStartedAt = text("jobStartedAt", 128);
    const jobCompletedAt = text("jobCompletedAt", 128);
    const command = text("command", MAX_TERMINAL_COMMAND_BYTES);
    const cwd = text("cwd", MAX_TERMINAL_CWD_BYTES);
    if (
      [
        title,
        summary,
        provider,
        query,
        operation,
        path,
        content,
        diff,
        jobId,
        jobStatus,
        jobStartedAt,
        jobCompletedAt,
        command,
        cwd,
      ].includes(null)
    ) {
      return undefined;
    }
    const count = optionalNonNegativeInteger(value.count);
    const size = optionalNonNegativeInteger(value.size);
    const offset = optionalNonNegativeInteger(value.offset);
    const nextOffset = optionalNonNegativeInteger(value.nextOffset);
    const jobDurationMs = optionalNonNegativeInteger(value.jobDurationMs);
    const exitCode = optionalExitCode(value.exitCode);
    const items = normalizePresentationItems(value.items);
    const transcript = normalizePresentationTranscript(value.transcript);
    const retry = normalizeProcessRetryPresentation(
      value.retry,
      card,
      kind,
      mode,
      toolName,
      operation,
    );
    if (
      count === null ||
      size === null ||
      offset === null ||
      nextOffset === null ||
      jobDurationMs === null ||
      exitCode === null ||
      items === null ||
      transcript === null ||
      !optionalBoolean(value.timedOut) ||
      !optionalBoolean(value.truncated) ||
      !optionalBoolean(value.background) ||
      (card === "job" && !validJobStatus(jobStatus)) ||
      (typeof jobStartedAt === "string" &&
        !validPresentationTimestamp(jobStartedAt)) ||
      (typeof jobCompletedAt === "string" &&
        !validPresentationTimestamp(jobCompletedAt)) ||
      (card === "job" && toolName === "terminal" && !command)
    )
      return undefined;
    return {
      version: 1,
      card,
      ...(title ? { title } : {}),
      ...(summary ? { summary } : {}),
      ...(provider ? { provider } : {}),
      ...(query ? { query } : {}),
      ...(operation ? { operation } : {}),
      ...(path ? { path } : {}),
      ...(content ? { content } : {}),
      ...(diff ? { diff } : {}),
      ...(jobId ? { jobId } : {}),
      ...(jobStatus ? { jobStatus } : {}),
      ...(jobStartedAt ? { jobStartedAt } : {}),
      ...(jobCompletedAt ? { jobCompletedAt } : {}),
      ...(jobDurationMs !== undefined ? { jobDurationMs } : {}),
      ...(command ? { command } : {}),
      ...(cwd ? { cwd } : {}),
      ...(count !== undefined ? { count } : {}),
      ...(size !== undefined ? { size } : {}),
      ...(offset !== undefined ? { offset } : {}),
      ...(nextOffset !== undefined ? { nextOffset } : {}),
      ...(items?.length ? { items } : {}),
      ...(transcript?.length ? { transcript } : {}),
      ...(exitCode !== undefined ? { exitCode } : {}),
      ...(value.timedOut === true ? { timedOut: true } : {}),
      ...(value.truncated === true ? { truncated: true } : {}),
      ...(value.background === true ? { background: true } : {}),
      ...(retry ? { retry } : {}),
    } as ProcessStepPresentation;
  }
  const command = boundedPresentationString(
    value.command,
    MAX_TERMINAL_COMMAND_BYTES,
  );
  if (!command) return undefined;
  const cwd =
    value.cwd === undefined
      ? undefined
      : boundedPresentationString(value.cwd, MAX_TERMINAL_CWD_BYTES);
  const exitCode =
    value.exitCode === undefined
      ? undefined
      : typeof value.exitCode === "number" &&
          Number.isInteger(value.exitCode) &&
          value.exitCode >= -1 &&
          value.exitCode <= 255
        ? value.exitCode
        : null;
  if (
    (value.cwd !== undefined && !cwd) ||
    exitCode === null ||
    !optionalBoolean(value.timedOut) ||
    !optionalBoolean(value.truncated) ||
    !optionalBoolean(value.background) ||
    normalizePresentationTranscript(value.transcript) === null
  ) {
    return undefined;
  }
  const approval = normalizeProcessApprovalPresentation(value.approval);
  return {
    version: 1,
    card: "terminal",
    command,
    ...(cwd ? { cwd } : {}),
    ...(exitCode !== undefined ? { exitCode } : {}),
    ...(value.timedOut === true ? { timedOut: true } : {}),
    ...(value.truncated === true ? { truncated: true } : {}),
    ...(value.background === true ? { background: true } : {}),
    ...(normalizePresentationTranscript(value.transcript)?.length
      ? { transcript: normalizePresentationTranscript(value.transcript)! }
      : {}),
    ...(approval ? { approval } : {}),
  };
}

function normalizeProcessRetryPresentation(
  value: unknown,
  card: string,
  kind: ProcessStepKind,
  mode: unknown,
  toolName: unknown,
  operation: string | null | undefined,
): ProcessRetryPresentation | undefined {
  if (
    !isRecord(value) ||
    card !== "file" ||
    kind !== "tool" ||
    mode !== "local_direct" ||
    toolName !== "file_read" ||
    operation !== "read"
  ) {
    return undefined;
  }
  const eventId = stringValue(value.eventId);
  const retryOf = boundedPresentationString(value.retryOf, 256);
  if (
    !/^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(
      eventId,
    ) ||
    !retryOf
  ) {
    return undefined;
  }
  return { eventId, retryOf };
}

function normalizeProcessApprovalPresentation(
  value: unknown,
): ProcessApprovalPresentation | undefined {
  if (!isRecord(value)) return undefined;
  const id = stringValue(value.id);
  const revision = positiveInteger(value.revision);
  const status = stringValue(value.status);
  const decision = stringValue(value.decision);
  const expiresAt = stringValue(value.expiresAt);
  if (
    !/^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(
      id,
    ) ||
    revision === undefined ||
    !["pending", "allowed", "denied", "expired"].includes(status) ||
    !Number.isFinite(Date.parse(expiresAt)) ||
    typeof value.allowConversation !== "boolean" ||
    (decision !== "" &&
      ![
        "allow_once",
        "allow_conversation",
        "deny",
        "expired",
        "restart_denied",
      ].includes(decision))
  ) {
    return undefined;
  }
  return {
    id,
    revision,
    status: status as ProcessApprovalPresentation["status"],
    ...(decision
      ? {
          decision: decision as NonNullable<
            ProcessApprovalPresentation["decision"]
          >,
        }
      : {}),
    expiresAt,
    allowConversation: value.allowConversation,
  };
}

function normalizePresentationItems(
  value: unknown,
): { label: string; detail?: string }[] | undefined | null {
  if (value === undefined) return undefined;
  if (!Array.isArray(value) || value.length > MAX_PRESENTATION_ITEMS)
    return null;
  const result: { label: string; detail?: string }[] = [];
  for (const candidate of value) {
    if (!isRecord(candidate)) return null;
    const label = boundedPresentationString(candidate.label, 1024);
    const detail =
      candidate.detail === undefined
        ? undefined
        : boundedPresentationString(candidate.detail, 2048);
    if (!label || (candidate.detail !== undefined && !detail)) return null;
    result.push({ label, ...(detail ? { detail } : {}) });
  }
  return result;
}

function normalizePresentationTranscript(
  value: unknown,
): ProcessTranscriptEntry[] | undefined | null {
  if (value === undefined) return undefined;
  if (!Array.isArray(value) || value.length > 128) return null;
  let totalBytes = 0;
  const result: ProcessTranscriptEntry[] = [];
  for (const candidate of value) {
    if (
      !isRecord(candidate) ||
      (candidate.stream !== "stdout" && candidate.stream !== "stderr") ||
      !Number.isInteger(candidate.sequence) ||
      Number(candidate.sequence) < 1
    )
      return null;
    const content = boundedPresentationString(
      candidate.content,
      MAX_PRESENTATION_TEXT_BYTES,
    );
    if (!content) return null;
    totalBytes += new TextEncoder().encode(content).byteLength;
    if (totalBytes > MAX_PRESENTATION_TEXT_BYTES) return null;
    result.push({
      sequence: Number(candidate.sequence),
      stream: candidate.stream,
      content,
    });
  }
  return result.sort((left, right) => left.sequence - right.sequence);
}

function optionalNonNegativeInteger(value: unknown): number | undefined | null {
  if (value === undefined) return undefined;
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0
    ? value
    : null;
}

function optionalExitCode(value: unknown): number | undefined | null {
  if (value === undefined) return undefined;
  return typeof value === "number" &&
    Number.isSafeInteger(value) &&
    value >= -1 &&
    value <= 255
    ? value
    : null;
}

function validJobStatus(value: string | undefined | null): boolean {
  return (
    value === undefined ||
    value === "running" ||
    value === "stopping" ||
    value === "completed" ||
    value === "killed" ||
    value === "failed" ||
    value === "interrupted" ||
    value === "unknown"
  );
}

function validPresentationTimestamp(value: string): boolean {
  return Number.isFinite(Date.parse(value));
}

function boundedPresentationString(value: unknown, maxBytes: number): string {
  if (typeof value !== "string") return "";
  const normalized = value.trim();
  return normalized &&
    new TextEncoder().encode(normalized).byteLength <= maxBytes
    ? normalized
    : "";
}

function optionalBoolean(value: unknown): boolean {
  return value === undefined || typeof value === "boolean";
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
