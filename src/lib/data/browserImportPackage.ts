import { zipSync } from "fflate";
import type {
  Message,
  Session,
  SessionMessageTree,
  Workspace,
} from "../../types";
import { normalizeSession, normalizeWorkspace } from "../chat/entities";
import {
  getAllMessagesFromTree,
  isSessionMessageTree,
  normalizeSessionMessageTree,
} from "../chat/messageTree";
import { isKnowledgeAttachment } from "../utils/knowledgeAttachments";
import {
  appDb,
  STORAGE_KEYS,
  STORAGE_VERSION,
} from "../../store/storage/storageConfig";
import {
  normalizeMessage,
  normalizeMessages,
} from "../../store/storage/migrations";
import {
  listOPFSDirectory,
  readBlobFromOPFSUrl,
  isOPFSUrl,
} from "../../utils/opfs";
import {
  APP_EXPORT_VERSION,
  type AppExportPayload,
  collectOrphanOpfsUrls,
  createBrowserAppExportPayload,
} from "./appExport";

export const BROWSER_IMPORT_FORMAT = "neo-chat-browser-import";
export const BROWSER_IMPORT_SCHEMA_VERSION = "mm-chat.browser-import.v2";
export const BROWSER_IMPORT_PACKAGE_NAME = "neo-chat-browser-import-v2.zip";

const SESSION_MESSAGES_PREFIX = "session_messages_";
const TEXT_ENCODER = new TextEncoder();
const OPFS_DIRECTORIES = ["chat", "images", "knowledge-base", "workspaces"];
const URL_SECRET_MARKERS = [
  "apikey",
  "accesstoken",
  "authorization",
  "bearertoken",
  "cookie",
  "credential",
  "password",
  "secret",
  "signature",
  "token",
] as const;

type ImportRole = "system" | "user" | "assistant" | "tool";
type ImportAttachmentSource = "file" | "remote" | "knowledge_ref";

export interface BrowserImportManifestV2 {
  format: typeof BROWSER_IMPORT_FORMAT;
  schemaVersion: typeof BROWSER_IMPORT_SCHEMA_VERSION;
  storageVersion: number;
  appExportVersion?: number;
  exportedAt?: string;
  generatedAt: string;
  idempotencyKey: string;
  source: {
    app: "neo-chat";
    origin?: string;
  };
  counts: {
    conversations: number;
    messages: number;
    files: number;
    bytes: number;
  };
  opfs: {
    referencedUrls: string[];
    missingUrls: string[];
    orphanUrls: string[];
  };
  options?: {
    onDuplicate?: "skip" | "copy";
    allowMissingFiles?: false;
  };
  conversations: ImportConversation[];
  messages: ImportMessage[];
  files: ImportFile[];
  workspaces?: ImportWorkspace[];
  deferred?: {
    knowledgeCollections?: number;
    memories?: number;
    providerSettings?: number;
  };
}

export interface ImportConversation {
  clientId: string;
  title: string;
  status?: "active" | "archived";
  modelRef?: { providerId: string; modelId: string };
  systemInstruction?: string;
  workspaceClientId?: string;
  pinned?: boolean;
  config?: Record<string, unknown>;
  createdAt?: string;
  updatedAt: string;
}

export interface ImportMessage {
  clientId: string;
  conversationClientId: string;
  parentClientId?: string;
  sequenceNo: number;
  role: ImportRole;
  status?: "completed" | "failed" | "cancelled";
  content: string;
  modelRef?: { providerId: string; modelId: string };
  attachments?: ImportAttachment[];
  outputBlocks?: unknown[];
  metadata?: Record<string, unknown>;
  createdAt: string;
  completedAt?: string;
}

export interface ImportAttachment {
  clientAttachmentId: string;
  source: ImportAttachmentSource;
  clientFileId?: string;
  fileName: string;
  mimeType: string;
  size?: number;
  sha256?: string;
  url?: string;
  purpose?: "input" | "output" | "image" | "knowledge_source";
}

export interface ImportFile {
  clientFileId: string;
  source: "opfs" | "inline";
  originalUrl?: `opfs://${string}`;
  sourceAttachmentIds: string[];
  fileName: string;
  mimeType: string;
  size: number;
  sha256: string;
  blobPath: `files/sha256/${string}`;
  purpose: "chat" | "workspace" | "knowledge" | "image" | "audio" | "export";
}

export interface ImportWorkspace {
  clientId: string;
  name: string;
  systemPrompt?: string;
  color?: string;
}

export interface BrowserImportPackageResult {
  fileName: typeof BROWSER_IMPORT_PACKAGE_NAME;
  blob: Blob;
  manifest: BrowserImportManifestV2;
  zipBytes: Uint8Array;
}

export interface BrowserImportSnapshot {
  appExport: AppExportPayload;
  appDbKeys?: string[];
  sessionMessagesById: Record<string, unknown>;
  existingOpfsUrls?: string[];
  origin?: string;
}

export interface BrowserImportPackageOptions {
  now?: Date;
  idempotencyKey?: string;
  readOpfsBlob?: (url: string) => Promise<Blob | null>;
}

interface FileCollectorInput {
  source: "opfs" | "inline";
  originalUrl?: `opfs://${string}`;
  attachmentId: string;
  fileName: string;
  mimeType: string;
  bytes: Uint8Array;
}

interface FileCollectorOutput {
  clientFileId: string;
  size: number;
  sha256: string;
}

interface MessageNodeRef {
  message: Message;
  parentMessageId?: string;
}

export class BrowserImportPackageError extends Error {
  readonly code: string;
  readonly missingOpfsUrls: string[];

  constructor(
    code: string,
    message: string,
    options: { missingOpfsUrls?: string[] } = {},
  ) {
    super(message);
    this.name = "BrowserImportPackageError";
    this.code = code;
    this.missingOpfsUrls = options.missingOpfsUrls ?? [];
  }
}

export async function createBrowserImportPackage(
  options: BrowserImportPackageOptions = {},
): Promise<BrowserImportPackageResult> {
  const [appExport, appDbKeys, existingOpfsUrls] = await Promise.all([
    createBrowserAppExportPayload(),
    appDb.keys(),
    listBrowserOwnedOpfsUrls(),
  ]);
  const sessionMessagesById = await readSessionMessageRecords(appDbKeys);

  return createBrowserImportPackageFromSnapshot(
    {
      appExport,
      appDbKeys,
      sessionMessagesById,
      existingOpfsUrls,
      origin:
        typeof window === "undefined" ? undefined : window.location.origin,
    },
    options,
  );
}

export async function createBrowserImportPackageFromSnapshot(
  snapshot: BrowserImportSnapshot,
  options: BrowserImportPackageOptions = {},
): Promise<BrowserImportPackageResult> {
  const now = options.now ?? new Date();
  const generatedAt = toUtcIso(now.getTime());
  const readOpfsBlob = options.readOpfsBlob ?? readBlobFromOPFSUrl;
  const chatState = readPersistedState(snapshot.appExport.data.chat);
  const sessions = readSessions(chatState);
  const workspaces = readWorkspaces(chatState);
  const workspaceIds = new Set(workspaces.map((workspace) => workspace.id));
  const fileCollector = new ImportFileCollector();
  const referencedOpfsUrls = new Set<string>();
  const missingOpfsUrls = new Set<string>();
  const importMessages: ImportMessage[] = [];

  for (const session of sessions) {
    const tree = readSessionMessageTree(session, snapshot.sessionMessagesById);
    const messageNodes = getImportableMessageNodes(tree);
    const localToImportMessageIds = new Map<string, string>();
    let sequenceNo = 0;

    for (const node of messageNodes) {
      const message = normalizeMessage(node.message);
      const clientId = importMessageClientId(session.id, message.id);
      const attachments = await normalizeImportAttachments({
        sessionId: session.id,
        message,
        fileCollector,
        readOpfsBlob,
        referencedOpfsUrls,
        missingOpfsUrls,
      });
      const outputBlocks = sanitizeOutputBlocks(message.outputBlocks);
      const content =
        typeof message.content === "string" ? message.content : "";

      if (
        !content.trim() &&
        attachments.length === 0 &&
        outputBlocks.length === 0
      ) {
        continue;
      }

      const parentClientId = node.parentMessageId
        ? localToImportMessageIds.get(node.parentMessageId)
        : undefined;
      const importMessage: ImportMessage = removeUndefined({
        clientId,
        conversationClientId: session.id,
        parentClientId,
        sequenceNo,
        role: normalizeImportRole(message.role),
        status: normalizeImportMessageStatus(message),
        content,
        modelRef: parseModelRef(message.model || session.model),
        attachments: attachments.length > 0 ? attachments : undefined,
        outputBlocks: outputBlocks.length > 0 ? outputBlocks : undefined,
        metadata: safeMessageMetadata(message),
        createdAt: toUtcIso(message.timestamp),
        completedAt:
          message.role === "model" ? toUtcIso(message.timestamp) : undefined,
      });

      importMessages.push(importMessage);
      localToImportMessageIds.set(message.id, clientId);
      sequenceNo += 1;
    }
  }

  if (missingOpfsUrls.size > 0) {
    const missing = [...missingOpfsUrls].sort();
    throw new BrowserImportPackageError(
      "MISSING_OPFS_FILE",
      `Cannot build server import package; missing OPFS files: ${missing.join(", ")}`,
      { missingOpfsUrls: missing },
    );
  }

  const importFiles = fileCollector.files();
  const existingOpfsUrls = snapshot.existingOpfsUrls ?? [];
  const manifest: BrowserImportManifestV2 = {
    format: BROWSER_IMPORT_FORMAT,
    schemaVersion: BROWSER_IMPORT_SCHEMA_VERSION,
    storageVersion: STORAGE_VERSION,
    appExportVersion: APP_EXPORT_VERSION,
    exportedAt: snapshot.appExport.exportedAt,
    generatedAt,
    idempotencyKey: options.idempotencyKey ?? createIdempotencyKey(),
    source: {
      app: "neo-chat",
      origin: snapshot.origin,
    },
    counts: {
      conversations: sessions.length,
      messages: importMessages.length,
      files: importFiles.length,
      bytes: importFiles.reduce((total, file) => total + file.size, 0),
    },
    opfs: {
      referencedUrls: [...referencedOpfsUrls].sort(),
      missingUrls: [],
      orphanUrls: collectOrphanOpfsUrls({
        existingUrls: existingOpfsUrls,
        referencedUrls: referencedOpfsUrls,
      }),
    },
    options: { onDuplicate: "skip", allowMissingFiles: false },
    conversations: sessions.map((session) =>
      toImportConversation(session, workspaceIds, importMessages),
    ),
    messages: importMessages,
    files: importFiles,
    workspaces:
      workspaces.length > 0 ? workspaces.map(toImportWorkspace) : undefined,
    deferred: summarizeDeferred(snapshot.appExport, snapshot.appDbKeys),
  };

  const zipBytes = zipSync({
    "manifest.json": TEXT_ENCODER.encode(JSON.stringify(manifest)),
    ...fileCollector.zipEntries(),
  });

  return {
    fileName: BROWSER_IMPORT_PACKAGE_NAME,
    blob: new Blob([zipBytes], { type: "application/zip" }),
    manifest,
    zipBytes,
  };
}

async function readSessionMessageRecords(
  appDbKeys: string[],
): Promise<Record<string, unknown>> {
  const entries: Record<string, unknown> = {};
  await Promise.all(
    appDbKeys
      .filter((key) => key.startsWith(SESSION_MESSAGES_PREFIX))
      .map(async (key) => {
        const sessionId = key.slice(SESSION_MESSAGES_PREFIX.length);
        entries[sessionId] = parseStoredValue(
          await appDb.getItem<unknown>(key),
        );
      }),
  );
  return entries;
}

async function listBrowserOwnedOpfsUrls(): Promise<string[]> {
  const paths = await Promise.all(
    OPFS_DIRECTORIES.map(async (directory) => listOPFSDirectory(directory)),
  );
  return paths
    .flat()
    .map((path) => `opfs://${path}`)
    .sort();
}

function readPersistedState(value: unknown): Record<string, unknown> {
  const parsed = parseStoredValue(value);
  if (!isRecord(parsed)) return {};
  if (isRecord(parsed.state)) return parsed.state;
  return parsed;
}

function parseStoredValue(value: unknown): unknown {
  if (typeof value !== "string") return value;
  try {
    return JSON.parse(value);
  } catch {
    return value;
  }
}

function readSessions(chatState: Record<string, unknown>): Session[] {
  const rawSessions = Array.isArray(chatState.sessions)
    ? chatState.sessions
    : [];
  return rawSessions
    .map((value) => normalizeImportableSession(value))
    .filter((session): session is Session => Boolean(session));
}

function normalizeImportableSession(value: unknown): Session | null {
  if (!isRecord(value) || typeof value.id !== "string" || !value.id.trim()) {
    return null;
  }

  return normalizeSession({
    id: value.id.trim(),
    title: typeof value.title === "string" ? value.title : "New Chat",
    messageCount: readFiniteNumber(value.messageCount, 0),
    updatedAt: readFiniteNumber(value.updatedAt, Date.now()),
    model: typeof value.model === "string" ? value.model : "",
    systemInstruction:
      typeof value.systemInstruction === "string"
        ? value.systemInstruction
        : undefined,
    pinned: value.pinned === true,
    workspaceId:
      typeof value.workspaceId === "string" ? value.workspaceId : undefined,
    config: isRecord(value.config) ? value.config : undefined,
    compression: isRecord(value.compression)
      ? (value.compression as Session["compression"])
      : undefined,
    memoryContext: isRecord(value.memoryContext)
      ? (value.memoryContext as Session["memoryContext"])
      : undefined,
  });
}

function readWorkspaces(chatState: Record<string, unknown>): Workspace[] {
  const rawWorkspaces = Array.isArray(chatState.workspaces)
    ? chatState.workspaces
    : [];
  return rawWorkspaces
    .map((value) => normalizeImportableWorkspace(value))
    .filter((workspace): workspace is Workspace => Boolean(workspace));
}

function normalizeImportableWorkspace(value: unknown): Workspace | null {
  if (!isRecord(value) || typeof value.id !== "string" || !value.id.trim()) {
    return null;
  }

  return normalizeWorkspace({
    id: value.id.trim(),
    name: typeof value.name === "string" ? value.name : "Workspace",
    systemPrompt:
      typeof value.systemPrompt === "string" ? value.systemPrompt : undefined,
    knowledgeCollectionIds: Array.isArray(value.knowledgeCollectionIds)
      ? value.knowledgeCollectionIds.filter(
          (id): id is string => typeof id === "string",
        )
      : [],
    files: [],
    color: typeof value.color === "string" ? value.color : undefined,
    enableSearch: value.enableSearch === true,
    enableReasoning: value.enableReasoning === true,
    activePlugins: Array.isArray(value.activePlugins)
      ? value.activePlugins.filter((id): id is string => typeof id === "string")
      : [],
    activeSkills: Array.isArray(value.activeSkills)
      ? value.activeSkills.filter((id): id is string => typeof id === "string")
      : [],
    createdAt: readFiniteNumber(value.createdAt, Date.now()),
  });
}

function readSessionMessageTree(
  session: Session,
  sessionMessagesById: Record<string, unknown>,
): SessionMessageTree {
  const stored = parseStoredValue(sessionMessagesById[session.id]);
  if (stored !== undefined && stored !== null) {
    return normalizeSessionMessageTree(
      stored as Message[] | SessionMessageTree,
    );
  }
  return normalizeSessionMessageTree(normalizeMessages(session.messages));
}

function getImportableMessageNodes(tree: SessionMessageTree): MessageNodeRef[] {
  if (!isSessionMessageTree(tree)) return [];
  const orderedMessages = getAllMessagesFromTree(tree);
  return orderedMessages.map((message) => ({
    message,
    parentMessageId: tree.nodesById[message.id]?.parentMessageId,
  }));
}

async function normalizeImportAttachments({
  sessionId,
  message,
  fileCollector,
  readOpfsBlob,
  referencedOpfsUrls,
  missingOpfsUrls,
}: {
  sessionId: string;
  message: Message;
  fileCollector: ImportFileCollector;
  readOpfsBlob: (url: string) => Promise<Blob | null>;
  referencedOpfsUrls: Set<string>;
  missingOpfsUrls: Set<string>;
}): Promise<ImportAttachment[]> {
  const attachments = message.attachments ?? [];
  const normalized: ImportAttachment[] = [];

  for (let index = 0; index < attachments.length; index += 1) {
    const attachment = attachments[index];
    const clientAttachmentId = importAttachmentClientId(
      sessionId,
      message.id,
      attachment.id || String(index),
    );
    const fileName = normalizeFileName(attachment.fileName);
    const mimeType = normalizeMimeType(attachment.mimeType);

    if (isKnowledgeAttachment(attachment)) {
      normalized.push({
        clientAttachmentId,
        source: "knowledge_ref",
        fileName,
        mimeType,
        purpose: "knowledge_source",
      });
      continue;
    }

    if (attachment.url && isOPFSUrl(attachment.url)) {
      referencedOpfsUrls.add(attachment.url);
      const blob = await readOpfsBlob(attachment.url);
      if (!blob) {
        missingOpfsUrls.add(attachment.url);
        continue;
      }
      const bytes = new Uint8Array(await blob.arrayBuffer());
      const file = await fileCollector.add({
        source: "opfs",
        originalUrl: attachment.url as `opfs://${string}`,
        attachmentId: clientAttachmentId,
        fileName,
        mimeType: normalizeMimeType(blob.type || mimeType),
        bytes,
      });
      normalized.push({
        clientAttachmentId,
        source: "file",
        clientFileId: file.clientFileId,
        fileName,
        mimeType,
        size: file.size,
        sha256: file.sha256,
        purpose: attachmentPurpose(mimeType),
      });
      continue;
    }

    if (attachment.data) {
      const bytes = decodeBase64Bytes(attachment.data);
      const file = await fileCollector.add({
        source: "inline",
        attachmentId: clientAttachmentId,
        fileName,
        mimeType,
        bytes,
      });
      normalized.push({
        clientAttachmentId,
        source: "file",
        clientFileId: file.clientFileId,
        fileName,
        mimeType,
        size: file.size,
        sha256: file.sha256,
        purpose: attachmentPurpose(mimeType),
      });
      continue;
    }

    if (attachment.url) {
      const safeUrl = sanitizeRemoteAttachmentUrl(attachment.url);
      const remoteAttachment: ImportAttachment = removeUndefined({
        clientAttachmentId,
        source: "remote" as const,
        fileName,
        mimeType,
        url: safeUrl,
        purpose: attachmentPurpose(mimeType),
      });
      normalized.push(remoteAttachment);
    }
  }

  return normalized;
}

function sanitizeRemoteAttachmentUrl(url: string): string | undefined {
  const trimmed = url.trim();
  if (!trimmed) return undefined;

  let parsed: URL;
  try {
    parsed = new URL(trimmed, "https://neo-chat.local");
  } catch {
    return undefined;
  }

  const hasScheme = /^[a-z][a-z0-9+.-]*:/i.test(trimmed);
  if (
    hasScheme &&
    parsed.protocol !== "http:" &&
    parsed.protocol !== "https:"
  ) {
    return undefined;
  }
  if (parsed.username || parsed.password) return undefined;
  if (hasSecretMarker(parsed.hash)) return undefined;

  for (const [key, value] of parsed.searchParams.entries()) {
    if (hasSecretMarker(key) || hasSecretMarker(value)) {
      return undefined;
    }
  }

  for (const segment of parsed.pathname.split("/")) {
    if (hasSecretMarker(segment)) {
      return undefined;
    }
  }

  return trimmed;
}

function hasSecretMarker(value: string): boolean {
  const normalized = value.toLowerCase().replace(/[^a-z0-9]/g, "");
  return URL_SECRET_MARKERS.some((marker) => normalized.includes(marker));
}

class ImportFileCollector {
  private readonly collected = new Map<
    string,
    { file: ImportFile; bytes: Uint8Array }
  >();

  async add(input: FileCollectorInput): Promise<FileCollectorOutput> {
    const sha256 = await sha256Hex(input.bytes);
    const key = input.originalUrl ?? `inline:${sha256}:${input.fileName}`;
    const existing = this.collected.get(key);
    if (existing) {
      if (!existing.file.sourceAttachmentIds.includes(input.attachmentId)) {
        existing.file.sourceAttachmentIds.push(input.attachmentId);
      }
      return {
        clientFileId: existing.file.clientFileId,
        size: existing.file.size,
        sha256: existing.file.sha256,
      };
    }

    const file: ImportFile = removeUndefined({
      clientFileId: importFileClientId(
        input.source,
        sha256,
        this.collected.size,
      ),
      source: input.source,
      originalUrl: input.originalUrl,
      sourceAttachmentIds: [input.attachmentId],
      fileName: input.fileName,
      mimeType: input.mimeType,
      size: input.bytes.byteLength,
      sha256,
      blobPath: `files/sha256/${sha256}` as const,
      purpose: filePurpose(input.mimeType),
    });

    this.collected.set(key, { file, bytes: input.bytes });
    return { clientFileId: file.clientFileId, size: file.size, sha256 };
  }

  files(): ImportFile[] {
    return [...this.collected.values()].map(({ file }) => ({
      ...file,
      sourceAttachmentIds: [...file.sourceAttachmentIds],
    }));
  }

  zipEntries(): Record<string, Uint8Array> {
    const entries: Record<string, Uint8Array> = {};
    for (const { file, bytes } of this.collected.values()) {
      entries[file.blobPath] = bytes;
    }
    return entries;
  }
}

function toImportConversation(
  session: Session,
  workspaceIds: Set<string>,
  messages: ImportMessage[],
): ImportConversation {
  const sessionMessages = messages.filter(
    (message) => message.conversationClientId === session.id,
  );
  const firstMessage = sessionMessages[0];

  return removeUndefined({
    clientId: session.id,
    title: session.title || "New Chat",
    status: "active" as const,
    modelRef: parseModelRef(session.model),
    systemInstruction: session.systemInstruction,
    workspaceClientId:
      session.workspaceId && workspaceIds.has(session.workspaceId)
        ? session.workspaceId
        : undefined,
    pinned: session.pinned === true ? true : undefined,
    config: safeSessionConfig(session.config),
    createdAt: firstMessage?.createdAt ?? toUtcIso(session.updatedAt),
    updatedAt: toUtcIso(session.updatedAt),
  });
}

function toImportWorkspace(workspace: Workspace): ImportWorkspace {
  return removeUndefined({
    clientId: workspace.id,
    name: workspace.name,
    systemPrompt: workspace.systemPrompt,
    color: workspace.color,
  });
}

function safeSessionConfig(
  config: Session["config"],
): Record<string, unknown> | undefined {
  if (!config) return undefined;
  return removeUndefined({
    useSearch: config.useSearch,
    useReasoning: config.useReasoning,
    activePlugins: config.activePlugins?.length
      ? config.activePlugins
      : undefined,
    activeSkills: config.activeSkills?.length ? config.activeSkills : undefined,
  });
}

function safeMessageMetadata(
  message: Message,
): Record<string, unknown> | undefined {
  const metadata = removeUndefined({
    legacyRole: message.role,
    hasReasoning: Boolean(message.reasoning),
  });
  return Object.keys(metadata).length > 0 ? metadata : undefined;
}

function sanitizeOutputBlocks(blocks: Message["outputBlocks"]): unknown[] {
  if (!blocks?.length) return [];
  return blocks
    .filter((block) => block.type === "text" || block.type === "reasoning")
    .map((block) => ({
      id: block.id,
      type: block.type,
      content: block.content,
    }));
}

function summarizeDeferred(
  appExport: AppExportPayload,
  appDbKeys: string[] | undefined,
): BrowserImportManifestV2["deferred"] | undefined {
  const deferred = removeUndefined({
    knowledgeCollections: countMaybeArray(
      readPersistedState(appExport.data.knowledge).collections,
    ),
    memories: countMaybeArray(
      readPersistedState(appExport.data.memory).memories,
    ),
    providerSettings: appDbKeys?.includes(STORAGE_KEYS.SETTINGS)
      ? 1
      : undefined,
  });
  return Object.keys(deferred).length > 0 ? deferred : undefined;
}

function normalizeImportRole(role: Message["role"]): ImportRole {
  return role === "model" ? "assistant" : "user";
}

function normalizeImportMessageStatus(
  message: Message,
): ImportMessage["status"] {
  if (message.generationError) return "failed";
  return "completed";
}

function parseModelRef(
  model: string | undefined,
): ImportConversation["modelRef"] {
  const trimmed = model?.trim() ?? "";
  if (!trimmed) return undefined;
  const separator = trimmed.indexOf(":");
  if (separator > 0 && separator < trimmed.length - 1) {
    return {
      providerId: trimmed.slice(0, separator),
      modelId: trimmed.slice(separator + 1),
    };
  }
  return { providerId: "openai_compatible", modelId: trimmed };
}

function attachmentPurpose(mimeType: string): ImportAttachment["purpose"] {
  if (mimeType.startsWith("image/")) return "image";
  return "input";
}

function filePurpose(mimeType: string): ImportFile["purpose"] {
  if (mimeType.startsWith("image/")) return "image";
  if (mimeType.startsWith("audio/")) return "audio";
  return "chat";
}

function normalizeFileName(value: string | undefined): string {
  const normalized = value?.replace(/\\/g, "/").split("/").pop()?.trim() ?? "";
  return normalized || "upload.bin";
}

function normalizeMimeType(value: string | undefined): string {
  const normalized = value?.trim().toLowerCase() ?? "";
  return normalized || "application/octet-stream";
}

function decodeBase64Bytes(value: string): Uint8Array {
  const commaIndex = value.indexOf(",");
  const base64 =
    value.startsWith("data:") && commaIndex >= 0
      ? value.slice(commaIndex + 1)
      : value;
  const binary = atob(base64);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index);
  }
  return bytes;
}

async function sha256Hex(bytes: Uint8Array): Promise<string> {
  if (!globalThis.crypto?.subtle) {
    throw new BrowserImportPackageError(
      "CRYPTO_UNAVAILABLE",
      "Browser crypto is required to build the import package.",
    );
  }
  const buffer = new ArrayBuffer(bytes.byteLength);
  new Uint8Array(buffer).set(bytes);
  const digest = await globalThis.crypto.subtle.digest("SHA-256", buffer);
  return [...new Uint8Array(digest)]
    .map((byte) => byte.toString(16).padStart(2, "0"))
    .join("");
}

function toUtcIso(timestamp: number): string {
  const numeric = Number(timestamp);
  const date = new Date(Number.isFinite(numeric) ? numeric : Date.now());
  return date.toISOString();
}

function createIdempotencyKey(): string {
  if (globalThis.crypto?.randomUUID) {
    return `browser-import:${globalThis.crypto.randomUUID()}`;
  }
  return `browser-import:${Date.now()}:${Math.random().toString(36).slice(2)}`;
}

function importMessageClientId(sessionId: string, messageId: string): string {
  return `message:${sessionId}:${messageId}`;
}

function importAttachmentClientId(
  sessionId: string,
  messageId: string,
  attachmentId: string,
): string {
  return `attachment:${sessionId}:${messageId}:${attachmentId}`;
}

function importFileClientId(
  source: "opfs" | "inline",
  sha256: string,
  index: number,
): string {
  return `file:${source}:${index}:${sha256.slice(0, 16)}`;
}

function readFiniteNumber(value: unknown, fallback: number): number {
  const numberValue = Number(value);
  return Number.isFinite(numberValue) ? numberValue : fallback;
}

function countMaybeArray(value: unknown): number | undefined {
  return Array.isArray(value) && value.length > 0 ? value.length : undefined;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}

function removeUndefined<T extends Record<string, unknown>>(value: T): T {
  return Object.fromEntries(
    Object.entries(value).filter(([, field]) => field !== undefined),
  ) as T;
}
