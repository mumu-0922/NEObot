import { describe, expect, it } from "vitest";
import { resolveEffectiveChatContext } from "../lib/chat/effectiveChatContext";

describe("effective chat context", () => {
  it("combines prompt scopes and reports unavailable capabilities", () => {
    const context = resolveEffectiveChatContext({
      session: {
        id: "session-1",
        title: "New Chat",
        updatedAt: 1,
        model: "openai:gpt-test",
        messageCount: 0,
        systemInstruction: "Answer in project voice.",
      },
      workspace: {
        id: "workspace-1",
        name: "Workspace",
        color: "blue",
        systemPrompt: "Workspace context.",
        createdAt: 1,
        files: [
          { id: "file-1", fileName: "brief.txt", mimeType: "text/plain" },
        ],
      },
      systemPrompt: "Global system prompt.",
      now: new Date("2026-07-01T02:03:04.000Z"),
      selectedModel: "openai:gpt-test",
      provider: { type: "OpenAI Compatible" },
      modelMetadata: {},
      customModelMetadata: {},
      chatConfig: {
        searchMode: "external",
        useSearch: true,
        useReasoning: true,
        reasoningEffort: "high",
        temperature: 0.7,
      },
      search: {
        provider: "default",
        configs: { default: { serverAvailable: false } },
      },
    });

    expect(context.workspaceFiles).toHaveLength(1);
    expect(context.systemInstruction).toContain("Global system prompt.");
    expect(context.systemInstruction).toContain("Answer in project voice.");
    expect(context.systemInstruction).toContain("Workspace context.");
    expect(context.systemInstruction).toContain("<diagram-rendering>");
    expect(context.systemInstruction).toContain("Current date and time");
    expect(context.systemInstruction).toContain("2026-07-01T02:03:04.000Z");
    expect(context.capabilityStatuses.map((status) => status.code)).toEqual(
      expect.arrayContaining(["search_unavailable"]),
    );
  });

  it("appends safe inline HTML guidance when the visual prompt setting is enabled", () => {
    const context = resolveEffectiveChatContext({
      systemPrompt: "Global system prompt.",
      enableHtmlVisualPrompt: true,
      selectedModel: "openai:gpt-test",
      provider: { type: "OpenAI" },
      modelMetadata: {},
      customModelMetadata: {},
      chatConfig: {
        searchMode: "off",
        useSearch: false,
        useReasoning: false,
        reasoningEffort: "auto",
        temperature: 0.7,
      },
      search: {
        provider: "default",
        configs: { default: { serverAvailable: false } },
      },
    });

    expect(context.systemInstruction).toContain("Global system prompt.");
    expect(context.systemInstruction).toContain("<format");
    expect(context.systemInstruction).toContain("<html-visual>");
    expect(context.systemInstruction).toContain(
      "actively use safe inline HTML",
    );
    expect(context.systemInstruction).toContain("raw HTML");
    expect(context.systemInstruction).toContain("<diagram-visual-polish>");
    expect(context.systemInstruction).toContain(
      "Do not wrap HTML visual fragments in code fences",
    );
    expect(context.systemInstruction).toContain("Do not use class attributes");
    expect(context.systemInstruction).toContain(
      "Do not output full HTML documents",
    );
    expect(context.systemInstruction).toContain(
      "Use light or pale backgrounds with dark, readable foreground text",
    );
    expect(context.systemInstruction).toContain(
      "Aim for at least a 4.5:1 foreground/background contrast ratio",
    );
    expect(context.systemInstruction).toContain(
      "Never use surface, border, pastel, or translucent color variables as text color",
    );
  });

  it("does not inject HTML visual guidance when the setting is disabled", () => {
    const context = resolveEffectiveChatContext({
      systemPrompt: "Global system prompt.",
      enableHtmlVisualPrompt: false,
      selectedModel: "openai:gpt-test",
      provider: { type: "OpenAI" },
      modelMetadata: {},
      customModelMetadata: {},
      chatConfig: {
        searchMode: "off",
        useSearch: false,
        useReasoning: false,
        reasoningEffort: "auto",
        temperature: 0.7,
      },
      search: {
        provider: "default",
        configs: { default: { serverAvailable: false } },
      },
    });

    expect(context.systemInstruction).not.toContain("<html-visual>");
    expect(context.systemInstruction).toContain("<diagram-rendering>");
    expect(context.systemInstruction).not.toContain("<diagram-visual-polish>");
    expect(context.systemInstruction).not.toContain("<format_instructions");
  });
});
