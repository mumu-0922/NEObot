import type { Route } from "@playwright/test";

import type {
  FixtureConversation,
  FixtureMessage,
  FixtureModel,
} from "./neoChatApiTypes";

export const TEST_WORKSPACE_ID = "10000000-0000-4000-8000-000000000001";
export const NOW = "2026-08-29T08:00:00.000Z";
export const VERSION = `sha256:${"a".repeat(64)}`;

export function conversation(
  id: string,
  title: string,
  modelId: string,
  options: Partial<FixtureConversation> = {},
): FixtureConversation {
  return {
    id,
    title,
    modelRef: { providerId: "SUB", modelId },
    workspaceId: TEST_WORKSPACE_ID,
    ...options,
  };
}

export function runtimeConfig() {
  return {
    modelProvider: {
      available: true,
      id: "SERVER_DEFAULT",
      name: "Server Default",
      type: "OpenAI Compatible",
      models: [],
      modelMetadata: {},
      defaultModels: {
        titleGeneration: "gpt-5.6-luna",
        relatedQuestions: "gpt-5.6-luna",
        compression: "gpt-5.6-luna",
        polish: "gpt-5.6-luna",
        ragQuery: "gpt-5.6-luna",
        memory: "gpt-5.6-luna",
      },
      defaultModelsConfigured: true,
    },
    search: { available: false },
    mcp: { enabled: false, remoteEnabled: false, stdioEnabled: false },
    voice: {
      elevenLabsAvailable: false,
      mimoAvailable: false,
      defaultSttAvailable: false,
      defaultTtsAvailable: false,
    },
  };
}

export function providerConfigs() {
  return [
    {
      id: "SUB",
      name: "Sub",
      type: "OpenAI Compatible",
      baseUrl: "https://provider.invalid/v1",
      models: ["gpt-5.6-luna", "gpt-5.6-terra", "gpt-5.5"],
      enabled: true,
      hasApiKey: true,
      source: "server-stored",
      connectionTestValid: true,
      toolCapabilityDefault: "enabled",
    },
  ];
}

export function workspace() {
  return {
    id: TEST_WORKSPACE_ID,
    name: "E2E Workspace",
    systemPrompt: "",
    files: [],
    color: "#2563eb",
    enableSearch: false,
    enableReasoning: true,
    revision: 1,
    bindingStatus: "bound",
    runnerId: "e2e-runner",
    canonicalPath: "/workspace/e2e",
    displayPath: "/workspace/e2e",
    pathKind: "wsl",
    directoryFingerprint: "e2e-fingerprint",
    boundAt: NOW,
    createdAt: NOW,
    updatedAt: NOW,
  };
}

export function toConversationDto(conversation: FixtureConversation) {
  return {
    id: conversation.id,
    title: conversation.title,
    status: "active",
    modelRef: conversation.modelRef,
    messageCount: 0,
    config: { toolMode: "agent", reasoningEffort: "auto" },
    workspaceId: conversation.workspaceId,
    permissionMode: "workspace-write",
    createdAt: NOW,
    updatedAt: NOW,
    ...(conversation.activeGeneration
      ? { activeGeneration: conversation.activeGeneration }
      : {}),
  };
}

export function message(input: {
  id: string;
  conversationId: string;
  role: "user" | "assistant";
  content: string;
  sequenceNo: number;
  status?: FixtureMessage["status"];
  parentMessageId?: string;
  modelRef?: FixtureModel;
  completedAt?: string;
  agentEvents?: unknown[];
  outputBlocks?: unknown[];
}): FixtureMessage {
  return {
    id: input.id,
    conversationId: input.conversationId,
    role: input.role,
    status: input.status ?? "completed",
    content: input.content,
    sequenceNo: input.sequenceNo,
    attachments: [],
    outputBlocks: input.outputBlocks ?? [],
    metadata: {},
    createdAt: NOW,
    updatedAt: NOW,
    completedAt: input.completedAt ?? NOW,
    ...(input.parentMessageId
      ? { parentMessageId: input.parentMessageId }
      : {}),
    ...(input.modelRef ? { modelRef: input.modelRef } : {}),
    ...(input.agentEvents ? { agentEvents: input.agentEvents } : {}),
  };
}

export function transcriptEvents(conversationId: string, messageId: string) {
  const base = {
    turnId: "turn-e2e",
    conversationId,
    messageId,
    runId: "run-e2e",
    occurredAt: NOW,
  };
  const event = (
    sequence: number,
    type: string,
    payload: Record<string, unknown>,
  ) => ({ ...base, eventId: `event-e2e-${sequence}`, sequence, type, payload });
  return [
    event(1, "turn.started", { status: "running", transcriptVersion: 2 }),
    event(2, "assistant.chunk", {
      chunkType: "block-start",
      blockType: "narration",
      blockIndex: 1,
    }),
    event(3, "assistant.chunk", {
      chunkType: "narration-delta",
      blockType: "narration",
      blockIndex: 1,
      content: "先读取项目状态。",
    }),
    event(4, "assistant.block.completed", {
      blockType: "narration",
      blockIndex: 1,
    }),
    event(5, "tool.called", { processSteps: [terminalStep("running")] }),
    event(6, "tool.result", { processSteps: [terminalStep("completed")] }),
    event(7, "assistant.chunk", {
      chunkType: "block-start",
      blockType: "narration",
      blockIndex: 2,
    }),
    event(8, "assistant.chunk", {
      chunkType: "narration-delta",
      blockType: "narration",
      blockIndex: 2,
      content: "文件已经写入工作区。",
    }),
    event(9, "assistant.block.completed", {
      blockType: "narration",
      blockIndex: 2,
    }),
    event(10, "turn.ended", { status: "completed" }),
  ];
}

function terminalStep(status: "running" | "completed") {
  return {
    id: "agent-terminal-e2e",
    kind: "tool",
    status,
    labelKey: "process.tool",
    detail: { toolName: "bash", mode: "local_direct", round: 1 },
    presentation: {
      version: 1,
      card: "terminal",
      command: "pwd",
      cwd: "$NEO_CHAT_WORKSPACE",
      ...(status === "completed"
        ? {
            transcript: [
              { sequence: 1, stream: "stdout", content: "/workspace/e2e\n" },
            ],
            exitCode: 0,
          }
        : {}),
    },
  };
}

export function isModelRef(value: unknown): value is FixtureModel {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const candidate = value as Record<string, unknown>;
  return (
    typeof candidate.providerId === "string" &&
    typeof candidate.modelId === "string"
  );
}

export function sseFrame(event: Record<string, unknown>): string {
  return `event: ${String(event.type)}\ndata: ${JSON.stringify(event)}\n\n`;
}

export async function json(
  route: Route,
  body: unknown,
  status = 200,
): Promise<void> {
  await route.fulfill({
    status,
    contentType: "application/json",
    body: JSON.stringify(body),
  });
}
