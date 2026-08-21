import { renderToStaticMarkup } from "react-dom/server";
import { NextIntlClientProvider } from "next-intl";
import { describe, expect, it } from "vitest";

import AgentTranscript from "../components/content/AgentTranscript";
import contentMessages from "../i18n/locales/zh/Content.json";
import { projectAgentTranscript } from "../lib/chat/agentTranscript";
import type { ChatAgentEvent } from "../lib/chat/types";

const baseEvent = {
  turnId: "turn-1",
  conversationId: "conversation-1",
  messageId: "message-1",
  runId: "run-1",
  occurredAt: "2026-08-21T00:00:00Z",
} satisfies Omit<ChatAgentEvent, "eventId" | "sequence" | "type" | "payload">;

function event(
  sequence: number,
  type: ChatAgentEvent["type"],
  payload: Record<string, unknown>,
  stepSequence?: number,
): ChatAgentEvent {
  return {
    ...baseEvent,
    eventId: `event-${sequence}`,
    sequence,
    type,
    payload,
    ...(stepSequence ? { stepSequence } : {}),
  };
}

describe("Harness Transcript v2", () => {
  it("projects context, separate Think blocks, and Tool rows in durable order", () => {
    const events: ChatAgentEvent[] = [
      event(1, "turn.started", { status: "running" }),
      event(2, "context.injected", {
        source: "system-prompt",
        label: "System prompt",
        content: "Be precise.",
        truncated: false,
      }),
      event(
        3,
        "assistant.chunk",
        { chunkType: "block-start", blockType: "reasoning", blockIndex: 1 },
        1,
      ),
      event(
        4,
        "assistant.chunk",
        {
          chunkType: "reasoning-delta",
          blockType: "reasoning",
          blockIndex: 1,
          content: "First inspect ",
        },
        1,
      ),
      event(
        5,
        "assistant.chunk",
        {
          chunkType: "reasoning-delta",
          blockType: "reasoning",
          blockIndex: 1,
          content: "the workspace.",
        },
        1,
      ),
      event(6, "assistant.block.completed", {
        blockType: "reasoning",
        blockIndex: 1,
      }),
      event(7, "tool.called", {
        processSteps: [
          {
            id: "message-1:tool:1",
            kind: "tool",
            status: "running",
            labelKey: "process.tool",
            detail: { toolName: "terminal", mode: "local_direct", round: 1 },
            presentation: {
              version: 1,
              card: "terminal",
              command: "pwd",
              cwd: "$NEO_CHAT_WORKSPACE",
            },
          },
        ],
      }),
      event(8, "tool.result", {
        processSteps: [
          {
            id: "message-1:tool:1",
            kind: "tool",
            status: "completed",
            labelKey: "process.tool",
            detail: { toolName: "terminal", mode: "local_direct", round: 1 },
            presentation: {
              version: 1,
              card: "terminal",
              command: "pwd",
              cwd: "$NEO_CHAT_WORKSPACE",
              transcript: [
                { sequence: 1, stream: "stdout", content: "/workspace\n" },
              ],
              exitCode: 0,
            },
          },
        ],
      }),
      event(
        9,
        "assistant.chunk",
        { chunkType: "block-start", blockType: "reasoning", blockIndex: 2 },
        2,
      ),
      event(
        10,
        "assistant.chunk",
        {
          chunkType: "reasoning-delta",
          blockType: "reasoning",
          blockIndex: 2,
          content: "The path is correct; now answer.",
        },
        2,
      ),
      event(11, "assistant.block.completed", {
        blockType: "reasoning",
        blockIndex: 2,
      }),
      event(12, "turn.ended", { status: "completed" }),
    ];

    const nodes = projectAgentTranscript(events);
    expect(nodes.map((node) => node.type)).toEqual([
      "context",
      "reasoning",
      "tool",
      "reasoning",
    ]);
    expect(nodes[1]).toMatchObject({
      type: "reasoning",
      round: 1,
      content: "First inspect the workspace.",
      status: "completed",
    });
    expect(nodes[2]).toMatchObject({
      type: "tool",
      step: { status: "completed" },
    });
    expect(nodes[3]).toMatchObject({
      type: "reasoning",
      round: 2,
      content: "The path is correct; now answer.",
    });

    const html = renderToStaticMarkup(
      <NextIntlClientProvider
        locale="zh"
        messages={{ Content: contentMessages }}
        timeZone="UTC"
      >
        <AgentTranscript events={events} />
      </NextIntlClientProvider>,
    );
    expect(html).toContain("上下文注入");
    expect(html.match(/思考/g)).toHaveLength(2);
    expect(html).toContain("Terminal");
    expect(html).toContain("First inspect the workspace.");
    expect(html).not.toContain("已调用 1 次工具");
  });

  it("fails closed on malformed blocks and keeps unfinished reasoning explicit", () => {
    const nodes = projectAgentTranscript([
      event(1, "assistant.chunk", {
        chunkType: "reasoning-delta",
        blockType: "reasoning",
        blockIndex: 1,
        content: "orphan",
      }),
      event(2, "assistant.chunk", {
        chunkType: "block-start",
        blockType: "reasoning",
        blockIndex: 2,
      }),
      event(3, "assistant.chunk", {
        chunkType: "reasoning-delta",
        blockType: "reasoning",
        blockIndex: 2,
        content: "visible",
      }),
      event(4, "turn.ended", { status: "interrupted" }),
    ]);
    expect(nodes).toEqual([
      expect.objectContaining({
        type: "reasoning",
        blockIndex: 2,
        content: "visible",
        status: "interrupted",
      }),
    ]);
  });

  it("uses the flat transcript for authoritative Tool-only turns", () => {
    const nodes = projectAgentTranscript([
      event(1, "turn.started", {
        status: "running",
        transcriptVersion: 2,
      }),
      event(2, "tool.result", {
        processSteps: [
          {
            id: "message-1:tool:1",
            kind: "tool",
            status: "completed",
            labelKey: "process.tool",
            detail: { toolName: "file_read", mode: "local_direct", round: 1 },
            presentation: {
              version: 1,
              card: "file",
              operation: "read",
              path: "README.md",
              content: "fixture",
            },
          },
        ],
      }),
      event(3, "turn.ended", { status: "completed" }),
    ]);
    expect(nodes).toEqual([
      expect.objectContaining({
        type: "tool",
        step: expect.objectContaining({ id: "message-1:tool:1" }),
      }),
    ]);
  });

  it("keeps legacy Tool-only turns on the legacy renderer", () => {
    const nodes = projectAgentTranscript([
      event(1, "turn.started", { status: "running" }),
      event(2, "tool.result", {
        processSteps: [
          {
            id: "message-1:tool:1",
            kind: "tool",
            status: "completed",
            labelKey: "process.tool",
            detail: { toolName: "file_read", mode: "local_direct", round: 1 },
            presentation: {
              version: 1,
              card: "file",
              operation: "read",
              path: "README.md",
              content: "legacy fixture",
            },
          },
        ],
      }),
      event(3, "turn.ended", { status: "completed" }),
    ]);
    expect(nodes).toEqual([]);
  });

  it("recognizes v2 reasoning events when the start marker is unavailable", () => {
    const nodes = projectAgentTranscript([
      event(2, "assistant.chunk", {
        chunkType: "block-start",
        blockType: "reasoning",
        blockIndex: 1,
      }),
      event(3, "assistant.chunk", {
        chunkType: "reasoning-delta",
        blockType: "reasoning",
        blockIndex: 1,
        content: "Recovered v2 fixture",
      }),
      event(4, "assistant.block.completed", {
        blockType: "reasoning",
        blockIndex: 1,
      }),
    ]);
    expect(nodes).toEqual([
      expect.objectContaining({
        type: "reasoning",
        content: "Recovered v2 fixture",
        status: "completed",
      }),
    ]);
  });
});
