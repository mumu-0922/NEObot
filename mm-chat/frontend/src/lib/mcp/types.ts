export type McpServerSource = "catalog" | "manifest" | "private";
export type McpTransport = "streamable_http" | "stdio";
export type McpAuthType = "none" | "header" | "oauth";
export type McpServerStatus =
  "draft" | "ready" | "needs_auth" | "unavailable" | "disabled";
export type McpToolClassification = "read" | "write" | "unknown";
export type McpSelectionMode = "inherit" | "custom";
export type McpCallStatus =
  | "queued"
  | "running"
  | "succeeded"
  | "failed"
  | "canceled"
  | "outcome_unknown";

export interface McpServerRef {
  source: McpServerSource;
  id: string;
}

export interface McpTool {
  serverRef: McpServerRef;
  name: string;
  alias: string;
  title?: string;
  description?: string;
  inputSchema: Record<string, unknown>;
  classification: McpToolClassification;
  supported: boolean;
  unsupportedReason?: string;
}

export interface McpGrant {
  scopeType: string;
  scopeId?: string;
  defaultEnabled: boolean;
}

export interface McpServer {
  ref: McpServerRef;
  name: string;
  description?: string;
  transport: McpTransport;
  endpointUrl?: string;
  authType: McpAuthType;
  status: McpServerStatus;
  hasCredential: boolean;
  toolCount: number;
  unsupportedToolCount: number;
  lastErrorCode?: string;
  validatedAt?: string;
  grants: McpGrant[];
  createdAt?: string;
  updatedAt?: string;
  tools: McpTool[];
}

export interface McpSelectionServer {
  ref: McpServerRef;
  disabledTools: string[];
}

export interface McpConversationSelection {
  conversationId: string;
  mode: McpSelectionMode;
  revision: number;
  servers: McpSelectionServer[];
}

export interface McpWorkspaceSelection {
  workspaceId: string;
  revision: number;
  servers: McpSelectionServer[];
}

export interface McpCallRecord {
  id: string;
  conversationId: string;
  messageId: string;
  runId: string;
  serverRef: McpServerRef;
  toolName: string;
  toolAlias: string;
  classification: McpToolClassification;
  status: McpCallStatus;
  round: number;
  call: number;
  argumentsSummary: Record<string, unknown>;
  resultSummary: string;
  errorCode: string;
  startedAt?: string;
  completedAt?: string;
  durationMillis: number;
}

export interface McpToolCallUpdate {
  executionId: string;
  callId: string;
  toolName: string;
  server?: string;
  classification?: McpToolClassification;
  processStatus:
    | "pending"
    | "running"
    | "completed"
    | "failed"
    | "cancelled"
    | "outcome_unknown";
  status: McpCallStatus;
  round: number;
  argumentsSummary: Record<string, unknown>;
  failureCategory?: string;
  durationMillis: number;
  mode: "mcp";
}

const SERVER_SOURCES = new Set<McpServerSource>([
  "catalog",
  "manifest",
  "private",
]);
const TRANSPORTS = new Set<McpTransport>(["streamable_http", "stdio"]);
const AUTH_TYPES = new Set<McpAuthType>(["none", "header", "oauth"]);
const SERVER_STATUSES = new Set<McpServerStatus>([
  "draft",
  "ready",
  "needs_auth",
  "unavailable",
  "disabled",
]);
const CLASSIFICATIONS = new Set<McpToolClassification>([
  "read",
  "write",
  "unknown",
]);
const SELECTION_MODES = new Set<McpSelectionMode>(["inherit", "custom"]);
const CALL_STATUSES = new Set<McpCallStatus>([
  "queued",
  "running",
  "succeeded",
  "failed",
  "canceled",
  "outcome_unknown",
]);
const PROCESS_STATUSES = new Set<McpToolCallUpdate["processStatus"]>([
  "pending",
  "running",
  "completed",
  "failed",
  "cancelled",
  "outcome_unknown",
]);

const MAX_SERVERS = 64;
const MAX_TOOLS_PER_SERVER = 256;
const MAX_CALLS = 256;
const MAX_STRING = 4096;

export function normalizeMcpServers(value: unknown): McpServer[] | null {
  if (!isRecord(value) || !Array.isArray(value.servers)) return null;
  const servers: McpServer[] = [];
  for (const candidate of value.servers.slice(0, MAX_SERVERS)) {
    const server = normalizeMcpServer(candidate);
    if (!server) return null;
    servers.push(server);
  }
  return servers;
}

export function normalizeMcpServerEnvelope(value: unknown): McpServer | null {
  if (!isRecord(value)) return null;
  return normalizeMcpServer(value.server);
}

export function normalizeMcpServer(value: unknown): McpServer | null {
  if (!isRecord(value)) return null;
  const ref = normalizeMcpServerRef(value.ref);
  const name = stringValue(value.name, 256);
  const transport = enumValue(value.transport, TRANSPORTS);
  const authType = enumValue(value.authType, AUTH_TYPES);
  const status = enumValue(value.status, SERVER_STATUSES);
  if (!ref || !name || !transport || !authType || !status) return null;
  if (typeof value.hasCredential !== "boolean" || !Array.isArray(value.tools)) {
    return null;
  }

  const tools: McpTool[] = [];
  for (const candidate of value.tools.slice(0, MAX_TOOLS_PER_SERVER)) {
    const tool = normalizeMcpTool(candidate);
    if (!tool || !sameServerRef(tool.serverRef, ref)) return null;
    tools.push(tool);
  }

  const grants = Array.isArray(value.grants)
    ? value.grants.slice(0, 64).map(normalizeMcpGrant)
    : [];
  if (grants.some((grant) => grant === null)) return null;

  return {
    ref,
    name,
    ...(stringValue(value.description, MAX_STRING)
      ? { description: stringValue(value.description, MAX_STRING) }
      : {}),
    transport,
    ...(stringValue(value.endpointUrl, MAX_STRING)
      ? { endpointUrl: stringValue(value.endpointUrl, MAX_STRING) }
      : {}),
    authType,
    status,
    hasCredential: value.hasCredential,
    toolCount: nonNegativeInteger(value.toolCount),
    unsupportedToolCount: nonNegativeInteger(value.unsupportedToolCount),
    ...(stringValue(value.lastErrorCode, 256)
      ? { lastErrorCode: stringValue(value.lastErrorCode, 256) }
      : {}),
    ...(dateString(value.validatedAt)
      ? { validatedAt: dateString(value.validatedAt) }
      : {}),
    grants: grants as McpGrant[],
    ...(dateString(value.createdAt)
      ? { createdAt: dateString(value.createdAt) }
      : {}),
    ...(dateString(value.updatedAt)
      ? { updatedAt: dateString(value.updatedAt) }
      : {}),
    tools,
  };
}

export function normalizeMcpConversationSelectionEnvelope(
  value: unknown,
): McpConversationSelection | null {
  if (!isRecord(value) || !isRecord(value.selection)) return null;
  const selection = value.selection;
  const conversationId = stringValue(selection.conversationId, 256);
  const mode = enumValue(selection.mode, SELECTION_MODES);
  const servers = normalizeSelectionServers(selection.servers);
  if (!conversationId || !mode || !servers) return null;
  return {
    conversationId,
    mode,
    revision: nonNegativeInteger(selection.revision),
    servers,
  };
}

export function normalizeMcpWorkspaceSelectionEnvelope(
  value: unknown,
): McpWorkspaceSelection | null {
  if (!isRecord(value) || !isRecord(value.selection)) return null;
  const selection = value.selection;
  const workspaceId = stringValue(selection.workspaceId, 256);
  const servers = normalizeSelectionServers(selection.servers);
  if (!workspaceId || !servers) return null;
  return {
    workspaceId,
    revision: nonNegativeInteger(selection.revision),
    servers,
  };
}

export function normalizeMcpCalls(value: unknown): McpCallRecord[] | null {
  if (!isRecord(value) || !Array.isArray(value.calls)) return null;
  const calls: McpCallRecord[] = [];
  for (const candidate of value.calls.slice(0, MAX_CALLS)) {
    const call = normalizeMcpCall(candidate);
    if (!call) return null;
    calls.push(call);
  }
  return calls;
}

export function normalizeMcpToolCallUpdate(
  value: unknown,
): McpToolCallUpdate | null {
  if (!isRecord(value)) return null;
  const executionId = stringValue(value.executionId, 256);
  const callId = stringValue(value.callId, 256);
  const toolName = stringValue(value.toolName, 512);
  const processStatus = enumValue(value.processStatus, PROCESS_STATUSES);
  const status = enumValue(value.status, CALL_STATUSES);
  const classification = value.classification
    ? enumValue(value.classification, CLASSIFICATIONS)
    : undefined;
  if (
    !executionId ||
    !callId ||
    !toolName ||
    !processStatus ||
    !status ||
    value.mode !== "mcp" ||
    (value.classification !== undefined && !classification) ||
    (value.argumentsSummary !== undefined && !isRecord(value.argumentsSummary))
  ) {
    return null;
  }
  return {
    executionId,
    callId,
    toolName,
    ...(stringValue(value.server, 512)
      ? { server: stringValue(value.server, 512) }
      : {}),
    ...(classification ? { classification } : {}),
    processStatus,
    status,
    round: nonNegativeInteger(value.round),
    argumentsSummary: isRecord(value.argumentsSummary)
      ? boundedRecord(value.argumentsSummary)
      : {},
    ...(stringValue(value.failureCategory, 256)
      ? { failureCategory: stringValue(value.failureCategory, 256) }
      : {}),
    durationMillis: nonNegativeInteger(value.durationMillis),
    mode: "mcp",
  };
}

function normalizeMcpCall(value: unknown): McpCallRecord | null {
  if (!isRecord(value)) return null;
  const id = stringValue(value.ID ?? value.id, 256);
  const conversationId = stringValue(
    value.ConversationID ?? value.conversationId,
    256,
  );
  const runId = stringValue(value.RunID ?? value.runId, 256);
  const serverRef = normalizeMcpServerRef(value.ServerRef ?? value.serverRef);
  const toolName = stringValue(value.ToolName ?? value.toolName, 512);
  const toolAlias = stringValue(value.ToolAlias ?? value.toolAlias, 512);
  const classification = enumValue(
    value.Classification ?? value.classification,
    CLASSIFICATIONS,
  );
  const status = enumValue(value.Status ?? value.status, CALL_STATUSES);
  if (
    !id ||
    !conversationId ||
    !runId ||
    !serverRef ||
    !toolName ||
    !toolAlias ||
    !classification ||
    !status
  ) {
    return null;
  }
  const summary = value.ArgumentsSummary ?? value.argumentsSummary;
  return {
    id,
    conversationId,
    messageId: stringValue(value.MessageID ?? value.messageId, 256),
    runId,
    serverRef,
    toolName,
    toolAlias,
    classification,
    status,
    round: nonNegativeInteger(value.Round ?? value.round),
    call: nonNegativeInteger(value.Call ?? value.call),
    argumentsSummary: isRecord(summary) ? boundedRecord(summary) : {},
    resultSummary: stringValue(
      value.ResultSummary ?? value.resultSummary,
      MAX_STRING,
    ),
    errorCode: stringValue(value.ErrorCode ?? value.errorCode, 256),
    ...(dateString(value.StartedAt ?? value.startedAt)
      ? { startedAt: dateString(value.StartedAt ?? value.startedAt) }
      : {}),
    ...(dateString(value.CompletedAt ?? value.completedAt)
      ? { completedAt: dateString(value.CompletedAt ?? value.completedAt) }
      : {}),
    durationMillis: nonNegativeInteger(
      value.DurationMillis ?? value.durationMillis,
    ),
  };
}

function normalizeMcpTool(value: unknown): McpTool | null {
  if (!isRecord(value)) return null;
  const serverRef = normalizeMcpServerRef(value.serverRef);
  const name = stringValue(value.name, 512);
  const alias = stringValue(value.alias, 512);
  const classification = enumValue(value.classification, CLASSIFICATIONS);
  if (
    !serverRef ||
    !name ||
    !alias ||
    !classification ||
    typeof value.supported !== "boolean" ||
    !isRecord(value.inputSchema)
  ) {
    return null;
  }
  return {
    serverRef,
    name,
    alias,
    ...(stringValue(value.title, 512)
      ? { title: stringValue(value.title, 512) }
      : {}),
    ...(stringValue(value.description, MAX_STRING)
      ? { description: stringValue(value.description, MAX_STRING) }
      : {}),
    inputSchema: boundedRecord(value.inputSchema),
    classification,
    supported: value.supported,
    ...(stringValue(value.unsupportedReason, 512)
      ? { unsupportedReason: stringValue(value.unsupportedReason, 512) }
      : {}),
  };
}

function normalizeSelectionServers(
  value: unknown,
): McpSelectionServer[] | null {
  if (!Array.isArray(value) || value.length > 8) return null;
  const servers: McpSelectionServer[] = [];
  const seen = new Set<string>();
  for (const candidate of value) {
    if (!isRecord(candidate)) return null;
    const ref = normalizeMcpServerRef(candidate.ref);
    if (!ref) return null;
    const key = `${ref.source}:${ref.id}`;
    if (seen.has(key)) return null;
    seen.add(key);
    const disabledTools = stringArray(candidate.disabledTools, 256, 512);
    if (!disabledTools) return null;
    servers.push({ ref, disabledTools });
  }
  return servers;
}

function normalizeMcpServerRef(value: unknown): McpServerRef | null {
  if (!isRecord(value)) return null;
  const source = enumValue(value.source, SERVER_SOURCES);
  const id = stringValue(value.id, 256);
  return source && id ? { source, id } : null;
}

function normalizeMcpGrant(value: unknown): McpGrant | null {
  if (!isRecord(value) || typeof value.defaultEnabled !== "boolean") {
    return null;
  }
  const scopeType = stringValue(value.scopeType, 64);
  if (!scopeType) return null;
  return {
    scopeType,
    ...(stringValue(value.scopeId, 256)
      ? { scopeId: stringValue(value.scopeId, 256) }
      : {}),
    defaultEnabled: value.defaultEnabled,
  };
}

function sameServerRef(left: McpServerRef, right: McpServerRef): boolean {
  return left.source === right.source && left.id === right.id;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}

function stringValue(value: unknown, limit: number): string {
  return typeof value === "string" ? value.trim().slice(0, limit) : "";
}

function enumValue<T extends string>(
  value: unknown,
  allowed: ReadonlySet<T>,
): T | null {
  return typeof value === "string" && allowed.has(value as T)
    ? (value as T)
    : null;
}

function nonNegativeInteger(value: unknown): number {
  const number = Number(value);
  return Number.isFinite(number) && number >= 0 ? Math.floor(number) : 0;
}

function dateString(value: unknown): string {
  if (typeof value !== "string" || !value.trim()) return "";
  return Number.isFinite(Date.parse(value)) ? value : "";
}

function stringArray(
  value: unknown,
  maxItems: number,
  maxLength: number,
): string[] | null {
  if (!Array.isArray(value) || value.length > maxItems) return null;
  const result: string[] = [];
  const seen = new Set<string>();
  for (const candidate of value) {
    const item = stringValue(candidate, maxLength);
    if (!item || seen.has(item)) continue;
    seen.add(item);
    result.push(item);
  }
  return result;
}

function boundedRecord(
  value: Record<string, unknown>,
): Record<string, unknown> {
  return Object.fromEntries(Object.entries(value).slice(0, 128));
}
