import { expect, type Page, type Route } from "@playwright/test";

import {
  isModelRef,
  json,
  message,
  NOW,
  providerConfigs,
  runtimeConfig,
  sseFrame,
  TEST_WORKSPACE_ID,
  toConversationDto,
  transcriptEvents,
  VERSION,
  workspace,
} from "./neoChatApiSupport";
import type {
  FixtureConversation,
  FixtureMessage,
  PendingRun,
  PendingRunResult,
} from "./neoChatApiTypes";
import { NeoChatKnowledgeApiFixture } from "./neoChatKnowledgeApi";
import {
  memorySearchTranscriptEvents,
  NeoChatMemoryApiFixture,
} from "./neoChatMemoryApi";
import {
  NeoChatResourceApiFixture,
  resourceTranscriptEvents,
} from "./neoChatResourceApi";

export type {
  FixtureConversation,
  FixtureMessage,
  FixtureModel,
} from "./neoChatApiTypes";

export const AUTH_SESSION_KEY = "mm-chat.server-auth-session.v1";
export const TEST_USER = {
  id: "00000000-0000-4000-8000-000000000001",
  displayName: "E2E Owner",
  role: "owner" as const,
};
export const TEST_TOKEN = "e2e-session-token";
export const TEST_RECOVERY_TOKEN = "e2e-recovery-token";
export { conversation, TEST_WORKSPACE_ID } from "./neoChatApiSupport";

export class NeoChatApiFixture {
  readonly conversations: FixtureConversation[];
  readonly messages = new Map<string, FixtureMessage[]>();
  readonly unhandledRequests: string[] = [];
  readonly knowledge = new NeoChatKnowledgeApiFixture();
  readonly memory = new NeoChatMemoryApiFixture();
  readonly resources = new NeoChatResourceApiFixture();
  private readonly pendingRuns = new Map<string, PendingRun>();
  private authValid = true;
  private password = "e2e-pass";
  private sequence = 0;

  constructor(conversations: FixtureConversation[]) {
    this.conversations = conversations.map((conversation) => ({
      ...conversation,
      modelRef: { ...conversation.modelRef },
      ...(conversation.config ? { config: { ...conversation.config } } : {}),
      ...(conversation.activeGeneration
        ? { activeGeneration: { ...conversation.activeGeneration } }
        : {}),
    }));
    for (const conversation of conversations) {
      this.messages.set(conversation.id, []);
    }
  }

  async install(page: Page): Promise<void> {
    await page.route("**/mm-api/**", (route) => this.handle(route));
  }

  async authenticate(page: Page): Promise<void> {
    await page.addInitScript(
      ({ key, token, user }) => {
        window.sessionStorage.setItem(
          key,
          JSON.stringify({ token, user, expiresAt: "2099-01-01T00:00:00Z" }),
        );
      },
      { key: AUTH_SESSION_KEY, token: TEST_TOKEN, user: TEST_USER },
    );
  }

  expireSession(): void {
    this.authValid = false;
  }

  seedMessages(conversationId: string, messages: FixtureMessage[]): void {
    this.messages.set(
      conversationId,
      messages.map((message) => ({ ...message })),
    );
  }

  seedAgentArtifact(conversationId: string): string {
    const userId = "30000000-0000-4000-8000-000000000001";
    const assistantId = "30000000-0000-4000-8000-000000000002";
    this.seedMessages(conversationId, [
      message({
        id: userId,
        conversationId,
        role: "user",
        content: "检查项目并生成报告",
        sequenceNo: 1,
      }),
      message({
        id: assistantId,
        conversationId,
        role: "assistant",
        content: "报告已经生成。",
        sequenceNo: 2,
        parentMessageId: userId,
        modelRef: { providerId: "SUB", modelId: "gpt-5.6-luna" },
        agentEvents: transcriptEvents(conversationId, assistantId),
        outputBlocks: [
          {
            id: "workspace-report",
            type: "workspace_file",
            workspaceId: TEST_WORKSPACE_ID,
            path: "reports/e2e-report.txt",
            fileName: "e2e-report.txt",
            mimeType: "text/plain",
            size: 22,
            version: VERSION,
          },
        ],
      }),
    ]);
    return assistantId;
  }

  seedTerminalAnswer(conversationId: string, content: string): string {
    const userId = "31000000-0000-4000-8000-000000000001";
    const assistantId = "31000000-0000-4000-8000-000000000002";
    const conversation = this.requireConversation(conversationId);
    this.seedMessages(conversationId, [
      message({
        id: userId,
        conversationId,
        role: "user",
        content: "恢复这个任务",
        sequenceNo: 1,
      }),
      message({
        id: assistantId,
        conversationId,
        role: "assistant",
        content,
        sequenceNo: 2,
        parentMessageId: userId,
        modelRef: conversation.modelRef,
      }),
    ]);
    return assistantId;
  }

  async waitForPendingRun(conversationId: string): Promise<void> {
    await expect
      .poll(() => this.pendingRuns.has(conversationId), { timeout: 10_000 })
      .toBe(true);
  }

  completeRun(
    conversationId: string,
    content: string,
    agentEvents?: unknown[],
  ): void {
    const run = this.pendingRuns.get(conversationId);
    if (!run) throw new Error(`No pending run for ${conversationId}`);
    run.resolve({ status: "completed", content, agentEvents });
  }

  completeKnowledgeRun(
    conversationId: string,
    content: string,
    knowledge: Record<string, unknown>,
  ): void {
    const run = this.pendingRuns.get(conversationId);
    if (!run) throw new Error(`No pending run for ${conversationId}`);
    run.resolve({
      status: "completed",
      content,
      metadata: { knowledge },
    });
  }

  completeMemoryRecallRun(
    conversationId: string,
    content: string,
    memoryContent: string,
  ): void {
    const run = this.requirePendingRun(conversationId);
    run.resolve({
      status: "completed",
      content,
      agentEvents: memorySearchTranscriptEvents(
        conversationId,
        run.messageId,
        memoryContent,
      ),
    });
  }

  completeMemoryActionRun(
    conversationId: string,
    content: string,
    memoryContent: string,
  ): string {
    const run = this.requirePendingRun(conversationId);
    const activity = this.memory.seedDirectActionActivity(
      run.messageId,
      memoryContent,
    );
    run.resolve({ status: "completed", content });
    return activity.id;
  }

  completeResourceRun(
    conversationId: string,
    content: string,
    mcpStatus: "completed" | "failed" = "completed",
  ): string {
    const run = this.requirePendingRun(conversationId);
    run.resolve({
      status: "completed",
      content,
      agentEvents: resourceTranscriptEvents(
        conversationId,
        run.messageId,
        mcpStatus,
      ),
    });
    return run.messageId;
  }

  failRun(conversationId: string, messageText = "Fixture run failed"): void {
    const run = this.pendingRuns.get(conversationId);
    if (!run) throw new Error(`No pending run for ${conversationId}`);
    run.resolve({
      status: "failed",
      code: "PROVIDER_ERROR",
      message: messageText,
    });
  }

  private async handle(route: Route): Promise<void> {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^\/mm-api/, "");
    const method = request.method();

    if (await this.knowledge.handle(route, path, method)) return;
    if (await this.memory.handle(route, path, method)) return;
    if (await this.resources.handle(route, path, method)) return;

    if (path === "/v1/auth/login" && method === "POST") {
      const body = request.postDataJSON() as {
        email?: string;
        password?: string;
      };
      if (
        body.email !== "owner@example.test" ||
        body.password !== this.password
      ) {
        await json(
          route,
          { error: { code: "UNAUTHORIZED", message: "Invalid login" } },
          401,
        );
        return;
      }
      this.authValid = true;
      await json(route, {
        token: TEST_TOKEN,
        user: TEST_USER,
        expiresAt: "2099-01-01T00:00:00Z",
      });
      return;
    }

    if (path === "/v1/auth/recovery/request" && method === "POST") {
      await json(route, { status: "accepted" }, 202);
      return;
    }

    if (path === "/v1/auth/recovery/complete" && method === "POST") {
      const body = request.postDataJSON() as {
        token?: string;
        newPassword?: string;
      };
      if (
        body.token !== TEST_RECOVERY_TOKEN ||
        typeof body.newPassword !== "string"
      ) {
        await json(
          route,
          { error: { code: "INVALID_CREDENTIAL", message: "Invalid token" } },
          401,
        );
        return;
      }
      this.password = body.newPassword;
      this.authValid = false;
      await route.fulfill({ status: 204 });
      return;
    }

    if (path === "/v1/me" && method === "GET") {
      if (
        !this.authValid ||
        request.headers().authorization !== `Bearer ${TEST_TOKEN}`
      ) {
        await json(
          route,
          { error: { code: "UNAUTHORIZED", message: "Session expired" } },
          401,
        );
        return;
      }
      await json(route, TEST_USER);
      return;
    }

    if (path === "/v1/auth/logout" && method === "POST") {
      this.authValid = false;
      await route.fulfill({ status: 204 });
      return;
    }

    if (path === "/v1/me/password" && method === "POST") {
      const body = request.postDataJSON() as {
        currentPassword?: string;
        newPassword?: string;
      };
      if (
        !this.authValid ||
        request.headers().authorization !== `Bearer ${TEST_TOKEN}` ||
        body.currentPassword !== this.password ||
        typeof body.newPassword !== "string"
      ) {
        await json(
          route,
          {
            error: {
              code: "INVALID_CREDENTIAL",
              message: "Invalid credential",
            },
          },
          401,
        );
        return;
      }
      this.password = body.newPassword;
      this.authValid = false;
      await route.fulfill({ status: 204 });
      return;
    }

    if (path === "/v1/me/sessions" && method === "DELETE") {
      if (
        !this.authValid ||
        request.headers().authorization !== `Bearer ${TEST_TOKEN}`
      ) {
        await json(
          route,
          { error: { code: "UNAUTHORIZED", message: "Session expired" } },
          401,
        );
        return;
      }
      this.authValid = false;
      await route.fulfill({ status: 204 });
      return;
    }

    if (path === "/v1/config" && method === "GET") {
      await json(
        route,
        runtimeConfig({ mcpEnabled: this.resources.mcpEnabled }),
      );
      return;
    }

    if (path === "/v1/admin/providers" && method === "GET") {
      await json(route, { providers: providerConfigs() });
      return;
    }

    if (path === "/v1/providers/models" && method === "POST") {
      await json(route, {
        models: ["gpt-5.6-luna", "gpt-5.6-terra", "gpt-5.5"],
      });
      return;
    }

    if (path === "/v1/workspaces/host-status" && method === "GET") {
      await json(route, {
        enabled: true,
        status: "ready",
        runnerId: "e2e-runner",
        platform: "linux",
        architecture: "amd64",
        features: {
          workspaceResolve: true,
          directoryBrowse: true,
          nativeDirectoryPicker: false,
          windowsPathInterop: true,
          execution: true,
          permissionModes: [
            "read-only",
            "workspace-write",
            "danger-full-access",
          ],
        },
      });
      return;
    }

    if (path === "/v1/workspaces" && method === "GET") {
      await json(route, { workspaces: [workspace()] });
      return;
    }

    if (path === "/v1/chat/conversations" && method === "GET") {
      await json(route, { items: this.conversations.map(toConversationDto) });
      return;
    }

    if (path === "/v1/chat/conversations" && method === "POST") {
      const body = request.postDataJSON() as Record<string, unknown>;
      const id = `20000000-0000-4000-8000-${String(this.conversations.length + 1).padStart(12, "0")}`;
      const modelRef = isModelRef(body.modelRef)
        ? body.modelRef
        : { providerId: "SUB", modelId: "gpt-5.6-luna" };
      const conversation = {
        id,
        title: typeof body.title === "string" ? body.title : "New Chat",
        modelRef,
        ...(isRecord(body.config) ? { config: { ...body.config } } : {}),
      } satisfies FixtureConversation;
      this.conversations.unshift(conversation);
      this.messages.set(id, []);
      await json(route, toConversationDto(conversation), 201);
      return;
    }

    const conversationMatch = path.match(
      /^\/v1\/chat\/conversations\/([^/]+)$/,
    );
    if (conversationMatch && method === "PATCH") {
      const id = decodeURIComponent(conversationMatch[1]);
      const conversation = this.requireConversation(id);
      const body = request.postDataJSON() as Record<string, unknown>;
      if (isModelRef(body.modelRef)) conversation.modelRef = body.modelRef;
      if (typeof body.title === "string") conversation.title = body.title;
      if (isRecord(body.config)) {
        conversation.config = {
          ...(conversation.config ?? {}),
          ...body.config,
        };
      }
      await json(route, toConversationDto(conversation));
      return;
    }

    const messagesMatch = path.match(
      /^\/v1\/chat\/conversations\/([^/]+)\/messages$/,
    );
    if (messagesMatch && method === "GET") {
      const id = decodeURIComponent(messagesMatch[1]);
      await json(route, { items: this.messages.get(id) ?? [] });
      return;
    }

    if (messagesMatch && method === "POST") {
      const conversationId = decodeURIComponent(messagesMatch[1]);
      const body = request.postDataJSON() as Record<string, unknown>;
      const items = this.messages.get(conversationId) ?? [];
      const user = message({
        id: `40000000-0000-4000-8000-${String(++this.sequence).padStart(12, "0")}`,
        conversationId,
        role: "user",
        content: typeof body.content === "string" ? body.content : "",
        sequenceNo: items.length + 1,
        ...(typeof body.parentMessageId === "string"
          ? { parentMessageId: body.parentMessageId }
          : {}),
      });
      items.push(user);
      this.messages.set(conversationId, items);
      await json(route, user, 201);
      return;
    }

    const streamMatch = path.match(
      /^\/v1\/chat\/conversations\/([^/]+)\/stream$/,
    );
    if (streamMatch && method === "POST") {
      const conversationId = decodeURIComponent(streamMatch[1]);
      await this.stream(
        route,
        conversationId,
        request.postDataJSON() as Record<string, unknown>,
      );
      return;
    }

    const cancelMatch = path.match(/^\/v1\/chat\/runs\/([^/]+)\/cancel$/);
    if (cancelMatch && method === "POST") {
      const runId = decodeURIComponent(cancelMatch[1]);
      const conversation = this.conversations.find(
        (item) => item.activeGeneration?.runId === runId,
      );
      if (!conversation) {
        await json(
          route,
          { error: { code: "RUN_NOT_FOUND", message: "Run not found" } },
          404,
        );
        return;
      }
      const assistant = message({
        id:
          conversation.activeGeneration?.messageId ??
          `50000000-0000-4000-8000-${String(++this.sequence).padStart(12, "0")}`,
        conversationId: conversation.id,
        role: "assistant",
        status: "cancelled",
        content: "任务已停止。",
        sequenceNo: (this.messages.get(conversation.id)?.length ?? 0) + 1,
        completedAt: NOW,
        modelRef: conversation.modelRef,
      });
      this.messages.set(conversation.id, [
        ...(this.messages.get(conversation.id) ?? []).filter(
          (item) => item.id !== assistant.id,
        ),
        assistant,
      ]);
      delete conversation.activeGeneration;
      await json(route, { runId, status: "cancelled", message: assistant });
      return;
    }

    const previewMatch = path.match(
      /^\/v1\/workspaces\/([^/]+)\/files\/preview$/,
    );
    if (previewMatch && method === "GET") {
      await json(route, {
        preview: {
          kind: "text",
          fileName: "e2e-report.txt",
          mimeType: "text/plain",
          size: 22,
          version: VERSION,
          truncated: false,
          text: "deterministic report\n",
        },
      });
      return;
    }

    if (
      /^\/v1\/chat\/conversations\/[^/]+\/title$/.test(path) &&
      method === "POST"
    ) {
      await json(route, { title: "E2E conversation" });
      return;
    }

    this.unhandledRequests.push(`${method} ${path}`);
    await json(route, {});
  }

  private async stream(
    route: Route,
    conversationId: string,
    body: Record<string, unknown>,
  ): Promise<void> {
    const runId = `run-${conversationId}`;
    const messageId = `60000000-0000-4000-8000-${String(++this.sequence).padStart(12, "0")}`;
    let resolve!: (result: PendingRunResult) => void;
    const result = new Promise<PendingRunResult>((next) => {
      resolve = next;
    });
    const pending: PendingRun = {
      runId,
      messageId,
      userMessageId: String(body.userMessageId ?? ""),
      resolve,
      result,
    };
    this.pendingRuns.set(conversationId, pending);
    const outcome = await result;
    this.pendingRuns.delete(conversationId);

    const frames: Record<string, unknown>[] = [
      {
        type: "message.started",
        runId,
        conversationId,
        messageId,
        sequence: 1,
        createdAt: NOW,
        role: "assistant",
      },
    ];

    if (outcome.status === "failed") {
      frames.push({
        type: "message.error",
        runId,
        conversationId,
        messageId,
        sequence: 2,
        error: {
          code: outcome.code ?? "PROVIDER_ERROR",
          message: outcome.message ?? "Fixture run failed",
          recoverable: true,
        },
      });
    } else {
      const conversation = this.requireConversation(conversationId);
      const items = this.messages.get(conversationId) ?? [];
      const assistant = message({
        id: messageId,
        conversationId,
        role: "assistant",
        content: outcome.content,
        sequenceNo: items.length + 1,
        parentMessageId: pending.userMessageId,
        modelRef: conversation.modelRef,
        ...(outcome.agentEvents ? { agentEvents: outcome.agentEvents } : {}),
        ...(outcome.metadata ? { metadata: outcome.metadata } : {}),
      });
      items.push(assistant);
      this.messages.set(conversationId, items);
      frames.push({
        type: "message.delta",
        runId,
        conversationId,
        messageId,
        sequence: 2,
        delta: outcome.content,
      });
      frames.push({
        type: "message.completed",
        runId,
        conversationId,
        messageId,
        sequence: 3,
        message: assistant,
      });
    }

    await route.fulfill({
      status: 200,
      headers: {
        "Content-Type": "text/event-stream; charset=utf-8",
        "Cache-Control": "no-cache",
      },
      body: frames.map(sseFrame).join(""),
    });
  }

  private requireConversation(id: string): FixtureConversation {
    const conversation = this.conversations.find((item) => item.id === id);
    if (!conversation) throw new Error(`Unknown conversation ${id}`);
    return conversation;
  }

  private requirePendingRun(conversationId: string): PendingRun {
    const run = this.pendingRuns.get(conversationId);
    if (!run) throw new Error(`No pending run for ${conversationId}`);
    return run;
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}
