import { renderToStaticMarkup } from "react-dom/server";
import { NextIntlClientProvider } from "next-intl";
import { describe, expect, it } from "vitest";

import ProcessTracePanel from "../components/content/ProcessTracePanel";
import contentMessages from "../i18n/locales/zh/Content.json";
import {
  humanizeToolName,
  isProcessStepActive,
  normalizeChatAgentEvents,
  normalizeProcessStep,
  normalizeProcessTrace,
  processOutcomeForDisplay,
  processReasonCategoryForDisplay,
  processToolLabelForDisplay,
  processTraceFromChatAgentEvents,
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

  it("keeps redacted MCP timeline detail and outcome_unknown status", () => {
    expect(
      normalizeProcessStep({
        id: "tool-1",
        kind: "tool",
        status: "outcome_unknown",
        labelKey: "process.tool",
        detail: {
          server: "manifest:files",
          serverName: "Files",
          toolName: "write_file",
          classification: "write",
          callStatus: "outcome_unknown",
          durability: "process_local",
          argumentSummary: '{"path":"string"}',
          rawArguments: { path: "/private/value" },
        },
      }),
    ).toEqual({
      id: "tool-1",
      kind: "tool",
      status: "outcome_unknown",
      labelKey: "process.tool",
      detail: {
        server: "manifest:files",
        serverName: "Files",
        toolName: "write_file",
        classification: "write",
        callStatus: "outcome_unknown",
        durability: "process_local",
        argumentSummary: '{"path":"string"}',
      },
    });
  });

  it("strictly normalizes only authorized local Terminal presentations", () => {
    const terminal = normalizeProcessStep({
      id: "tool-terminal-1",
      kind: "tool",
      status: "completed",
      labelKey: "process.tool",
      detail: {
        toolName: "terminal",
        mode: "local_direct",
        classification: "execute",
      },
      presentation: {
        card: "terminal",
        command: "pnpm test processTrace.test.tsx",
        cwd: "$NEO_CHAT_WORKSPACE/frontend",
        exitCode: 0,
        timedOut: false,
        truncated: true,
        background: false,
      },
    });
    expect(terminal?.presentation).toEqual({
      card: "terminal",
      command: "pnpm test processTrace.test.tsx",
      cwd: "$NEO_CHAT_WORKSPACE/frontend",
      exitCode: 0,
      truncated: true,
    });

    for (const presentation of [
      { card: "unknown", command: "pwd" },
      { card: "terminal", command: "pwd", exitCode: "0" },
      { card: "terminal", command: "pwd", timedOut: "yes" },
      { card: "terminal", command: "" },
      { card: "terminal", command: "界".repeat(2000) },
    ]) {
      const normalized = normalizeProcessStep({
        id: "tool-terminal-malformed",
        kind: "tool",
        status: "running",
        labelKey: "process.tool",
        detail: { toolName: "terminal", mode: "local_direct" },
        presentation,
      });
      expect(normalized).not.toBeNull();
      expect(normalized?.presentation).toBeUndefined();
    }

    const mcp = normalizeProcessStep({
      id: "tool-terminal-mcp",
      kind: "tool",
      status: "running",
      labelKey: "process.tool",
      detail: { toolName: "terminal", mode: "mcp" },
      presentation: { card: "terminal", command: "cat /private/.env" },
    });
    expect(mcp).not.toBeNull();
    expect(mcp?.presentation).toBeUndefined();
  });

  it("builds generic human-readable MCP labels without exposing internal refs", () => {
    expect(humanizeToolName("ask_question")).toBe("Ask question");
    expect(humanizeToolName("resolve-library-id")).toBe("Resolve library id");
    expect(humanizeToolName("query.docs")).toBe("Query docs");
    expect(humanizeToolName("resolveLibraryID")).toBe("Resolve library id");
    expect(humanizeToolName("  ")).toBe("");
    expect(humanizeToolName(null)).toBe("");

    const step = normalizeProcessStep({
      id: "tool-1",
      kind: "tool",
      status: "completed",
      labelKey: "process.tool",
      detail: {
        server: "private:00000000-0000-4000-8000-000000000000",
        serverName: "DeepWiki",
        toolName: "ask_question",
        classification: "unknown",
        callStatus: "queued",
        argumentSummary: '{"question":"string"}',
      },
    });

    expect(processToolLabelForDisplay(step!)).toBe("DeepWiki · Ask question");
    expect(
      processToolLabelForDisplay({
        ...step!,
        detail: {
          ...step!.detail,
          serverName: "Context7",
          toolName: "resolve-library-id",
        },
      }),
    ).toBe("Context7 · Resolve library id");
    expect(processToolLabelForDisplay({ ...step!, kind: "web" })).toBe("");
  });

  it("renders only the readable MCP label and authoritative outer status", () => {
    const step = normalizeProcessStep({
      id: "tool-1",
      kind: "tool",
      status: "completed",
      labelKey: "process.tool",
      durationMs: 3600,
      detail: {
        server: "private:00000000-0000-4000-8000-000000000000",
        serverName: "DeepWiki",
        toolName: "ask_question",
        classification: "unknown",
        callStatus: "queued",
        argumentSummary: '{"question":"string"}',
      },
    });
    const html = renderToStaticMarkup(
      <NextIntlClientProvider
        locale="zh"
        messages={{ Content: contentMessages }}
        timeZone="UTC"
      >
        <ProcessTracePanel steps={[step!]} />
      </NextIntlClientProvider>,
    );

    expect(html).toContain("DeepWiki · Ask question");
    expect(html).toContain("成功 · 3.6s");
    expect(html).toContain("已调用 1 次工具");
    expect(html).not.toContain("private:");
    expect(html).not.toContain("unknown");
    expect(html).not.toContain("queued");
    expect(html).not.toContain("&quot;question&quot;");
  });

  it("renders a collapsible Terminal command card and result state", () => {
    const step = normalizeProcessStep({
      id: "tool-terminal-1",
      kind: "tool",
      status: "failed",
      labelKey: "process.tool",
      durationMs: 1250,
      detail: {
        toolName: "terminal",
        mode: "local_direct",
        classification: "execute",
      },
      presentation: {
        card: "terminal",
        command: "go test ./internal/chat\nprintf done",
        cwd: "$NEO_CHAT_WORKSPACE/backend",
        exitCode: 17,
        timedOut: true,
        truncated: true,
        background: true,
      },
    });
    const html = renderToStaticMarkup(
      <NextIntlClientProvider
        locale="zh"
        messages={{ Content: contentMessages }}
        timeZone="UTC"
      >
        <ProcessTracePanel steps={[step!]} />
      </NextIntlClientProvider>,
    );

    expect(html).toContain("go test ./internal/chat printf done");
    expect(html).toContain("$NEO_CHAT_WORKSPACE/backend");
    expect(html).toContain("退出码 17");
    expect(html).toContain("已超时");
    expect(html).toContain("输出已截断");
    expect(html).toContain("后台运行");
    expect(html).toContain("<details");
    expect(html).not.toContain("stdout");
    expect(html).not.toContain("stderr");
  });

  it("warns that process-local background Jobs disappear after restart", () => {
    const step = normalizeProcessStep({
      id: "tool-job-1",
      kind: "tool",
      status: "completed",
      labelKey: "process.tool",
      detail: {
        toolName: "terminal",
        mode: "local_direct",
        durability: "process_local",
      },
    });
    const html = renderToStaticMarkup(
      <NextIntlClientProvider
        locale="zh"
        messages={{ Content: contentMessages }}
        timeZone="UTC"
      >
        <ProcessTracePanel steps={[step!]} />
      </NextIntlClientProvider>,
    );

    expect(html).toContain(
      "后台任务仅在当前服务进程内有效，服务重启后无法恢复。",
    );
    expect(html).toContain('role="note"');
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

  it("prefers durable Agent events and interrupts active steps after restart", () => {
    const projected = processTraceFromChatAgentEvents(
      [
        {
          eventId: "event-1",
          turnId: "turn-1",
          conversationId: "conversation-1",
          messageId: "message-1",
          runId: "run-1",
          sequence: 2,
          type: "step.started",
          payload: {
            processStep: {
              id: "message-1:tool:1",
              kind: "tool",
              status: "running",
              labelKey: "process.tool",
              startedAt: "2026-08-16T12:00:00Z",
              detail: {
                toolName: "terminal",
                mode: "local_direct",
                round: 1,
              },
            },
          },
          occurredAt: "2026-08-16T12:00:00Z",
        },
        {
          eventId: "event-2",
          turnId: "turn-1",
          conversationId: "conversation-1",
          messageId: "message-1",
          runId: "run-1",
          sequence: 3,
          type: "turn.ended",
          payload: { status: "interrupted" },
          occurredAt: "2026-08-16T12:00:02Z",
        },
      ],
      [
        {
          id: "legacy-generation",
          kind: "generation",
          status: "completed",
          labelKey: "process.generation",
        },
      ],
    );

    expect(projected).toEqual([
      {
        id: "message-1:tool:1",
        kind: "tool",
        status: "interrupted",
        labelKey: "process.tool",
        startedAt: "2026-08-16T12:00:00Z",
        completedAt: "2026-08-16T12:00:02Z",
        durationMs: 2000,
        detail: {
          toolName: "terminal",
          mode: "local_direct",
          round: 1,
          failureCategory: "interrupted",
          outcome: "interrupted",
        },
      },
    ]);
    expect(projected?.some(isProcessStepActive)).toBe(false);
  });

  it("projects the same Terminal presentation from live and durable steps", () => {
    const rawStep = {
      id: "message-1:tool:1",
      kind: "tool",
      status: "completed",
      labelKey: "process.tool",
      detail: { toolName: "terminal", mode: "local_direct", round: 1 },
      presentation: {
        card: "terminal",
        command: "go test ./internal/chat",
        cwd: "$NEO_CHAT_WORKSPACE/backend",
        exitCode: 0,
      },
    };
    const live = normalizeProcessStep(rawStep);
    const durable = processTraceFromChatAgentEvents([
      {
        eventId: "event-terminal-1",
        turnId: "turn-1",
        conversationId: "conversation-1",
        messageId: "message-1",
        runId: "run-1",
        sequence: 1,
        type: "tool.result",
        payload: { processSteps: [rawStep] },
        occurredAt: "2026-08-20T12:00:00Z",
      },
    ]);
    expect(durable).toEqual([live]);
  });

  it("rejects malformed durable events without replacing legacy history", () => {
    expect(normalizeChatAgentEvents([{ sequence: 0, payload: [] }])).toEqual(
      [],
    );
    const legacy = [
      {
        id: "legacy-generation",
        kind: "generation" as const,
        status: "completed" as const,
        labelKey: "process.generation",
      },
    ];
    expect(processTraceFromChatAgentEvents([{ invalid: true }], legacy)).toBe(
      legacy,
    );
  });

  it("sorts durable events by sequence and drops repeated event IDs", () => {
    const base = {
      turnId: "turn-1",
      conversationId: "conversation-1",
      messageId: "message-1",
      runId: "run-1",
      type: "step.started" as const,
      payload: {},
      occurredAt: "2026-08-16T12:00:00Z",
    };
    expect(
      normalizeChatAgentEvents([
        { ...base, eventId: "event-2", sequence: 2 },
        { ...base, eventId: "event-1", sequence: 1 },
        { ...base, eventId: "event-2", sequence: 2 },
      ]).map((event) => event.eventId),
    ).toEqual(["event-1", "event-2"]);
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
      toolCalls: 0,
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
      {
        id: "tool-1",
        kind: "tool",
        status: "completed",
        labelKey: "process.tool",
        detail: { toolName: "ask_question" },
      },
    ]);
    expect(summarizeProcessRoute(steps)).toEqual({
      route: "both",
      knowledgeSources: 2,
      webSources: 3,
      toolCalls: 1,
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
    const memoryIndexing = normalizeProcessStep({
      id: "memory-1",
      kind: "tool",
      status: "failed",
      labelKey: "process.tool",
      detail: { failureCategory: "memory_indexing" },
    });
    const memoryUnavailable = normalizeProcessStep({
      id: "memory-2",
      kind: "tool",
      status: "failed",
      labelKey: "process.tool",
      detail: { failureCategory: "memory_status_unavailable" },
    });
    expect(processReasonCategoryForDisplay(miss!)).toBe("knowledge_miss");
    expect(processReasonCategoryForDisplay(memoryIndexing!)).toBe(
      "memory_indexing",
    );
    expect(processReasonCategoryForDisplay(memoryUnavailable!)).toBe(
      "memory_unavailable",
    );
    expect(processReasonCategoryForDisplay(raw!)).toBeUndefined();
    expect(processOutcomeForDisplay(raw!)).toBe("");
  });
});
