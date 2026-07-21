import { describe, expect, it } from "vitest";
import { resolveEffectiveChatContext } from "../lib/chat/effectiveChatContext";

describe("effective chat context", () => {
  it("normalizes session plugins and skills and reports unavailable capabilities", () => {
    const context = resolveEffectiveChatContext({
      session: {
        id: "session-1",
        title: "New Chat",
        updatedAt: 1,
        model: "openai:gpt-test",
        messageCount: 0,
        systemInstruction: "Answer in project voice.",
        config: {
          activePlugins: ["needs-auth", "free-plugin"],
          activeSkills: ["session-skill", "session-skill", ""],
        },
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
        activeSkills: ["workspace-skill"],
      },
      systemPrompt: "Global system prompt.",
      now: new Date("2026-07-01T02:03:04.000Z"),
      selectedModel: "openai:gpt-test",
      provider: { type: "OpenAI Compatible" },
      modelMetadata: {},
      customModelMetadata: {},
      chatConfig: {
        useSearch: true,
        useReasoning: true,
        reasoningEffort: "high",
        temperature: 0.7,
      },
      search: {
        provider: "default",
        configs: { default: { serverAvailable: false } },
      },
      installedPlugins: [
        {
          id: "needs-auth",
          title: "Needs Auth",
          description: "",
          logoUrl: "",
          manifestUrl: "",
          functions: [],
          auth: { type: "apiKey" },
        },
        {
          id: "free-plugin",
          title: "Free Plugin",
          description: "",
          logoUrl: "",
          manifestUrl: "",
          functions: [],
          auth: { type: "none" },
        },
      ],
      pluginConfigs: {},
      activePlugins: [],
    });

    expect(context.workspaceFiles).toHaveLength(1);
    expect(context.systemInstruction).toContain("Global system prompt.");
    expect(context.systemInstruction).toContain("Answer in project voice.");
    expect(context.systemInstruction).toContain("Workspace context.");
    expect(context.systemInstruction).toContain("<diagram-rendering>");
    expect(context.systemInstruction).toContain("Current date and time");
    expect(context.systemInstruction).toContain("2026-07-01T02:03:04.000Z");
    expect(context.activePluginIds).toEqual(["free-plugin"]);
    expect(context.activeSkillIds).toEqual(["session-skill"]);
    expect(context.capabilityStatuses.map((status) => status.code)).toEqual(
      expect.arrayContaining(["search_unavailable", "plugin_auth_missing"]),
    );
  });

  it("uses workspace skills when the session does not override them", () => {
    const context = resolveEffectiveChatContext({
      session: {
        id: "session-1",
        title: "New Chat",
        updatedAt: 1,
        model: "openai:gpt-test",
        messageCount: 0,
      },
      workspace: {
        id: "workspace-1",
        name: "Workspace",
        color: "blue",
        createdAt: 1,
        files: [],
        activeSkills: ["workspace-skill", "workspace-skill"],
      },
      selectedModel: "openai:gpt-test",
      provider: { type: "OpenAI" },
      modelMetadata: {},
      customModelMetadata: {},
      chatConfig: {
        useSearch: false,
        useReasoning: false,
        reasoningEffort: "auto",
        temperature: 0.7,
      },
      search: {
        provider: "default",
        configs: { default: { serverAvailable: false } },
      },
      installedPlugins: [],
      pluginConfigs: {},
      activePlugins: [],
    });

    expect(context.activeSkillIds).toEqual(["workspace-skill"]);
  });

  it("uses browser-persisted plugin and skill selections for server sessions", () => {
    const context = resolveEffectiveChatContext({
      session: {
        id: "server-session",
        title: "Server session",
        updatedAt: 1,
        model: "SERVER_DEFAULT:gpt-server",
        messageCount: 0,
        config: {
          activePlugins: ["stale-plugin"],
          activeSkills: ["stale-skill"],
        },
      },
      selectedModel: "SERVER_DEFAULT:gpt-server",
      provider: { type: "OpenAI Compatible" },
      modelMetadata: {},
      customModelMetadata: {},
      chatConfig: {
        useSearch: false,
        useReasoning: false,
        reasoningEffort: "auto",
        temperature: 0.7,
      },
      search: {
        provider: "default",
        configs: { default: { serverAvailable: false } },
      },
      installedPlugins: [
        {
          id: "stale-plugin",
          title: "Stale Plugin",
          description: "",
          logoUrl: "",
          manifestUrl: "",
          functions: [],
          auth: { type: "none" },
        },
        {
          id: "server-plugin",
          title: "Server Plugin",
          description: "",
          logoUrl: "",
          manifestUrl: "",
          functions: [],
          auth: { type: "none" },
        },
      ],
      pluginConfigs: {},
      activePlugins: [],
      activePluginIdsOverride: ["server-plugin"],
      installedSkills: [
        {
          id: "server-skill",
          name: "server-skill",
          title: "Server Skill",
          description: "Applied to server chat.",
          category: "writing",
          tags: ["server"],
          audience: "general",
          language: "en",
          outputFormat: "text",
          risk: {
            level: "low",
            textOnly: true,
            scriptRequired: false,
            externalToolRequired: false,
            networkRequired: false,
            reviewRequiredForHighStakes: false,
          },
          activation: {
            embeddingText: "server skill",
            useWhen: ["selected"],
            avoidWhen: [],
            exampleQueries: [],
          },
          content: "Follow the server skill.",
        },
      ],
      activeSkillIds: ["missing-skill"],
      activeSkillIdsOverride: ["server-skill"],
    });

    expect(context.activePluginIds).toEqual(["server-plugin"]);
    expect(context.activeSkillIds).toEqual(["server-skill"]);
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
        useSearch: false,
        useReasoning: false,
        reasoningEffort: "auto",
        temperature: 0.7,
      },
      search: {
        provider: "default",
        configs: { default: { serverAvailable: false } },
      },
      installedPlugins: [],
      pluginConfigs: {},
      activePlugins: [],
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
        useSearch: false,
        useReasoning: false,
        reasoningEffort: "auto",
        temperature: 0.7,
      },
      search: {
        provider: "default",
        configs: { default: { serverAvailable: false } },
      },
      installedPlugins: [],
      pluginConfigs: {},
      activePlugins: [],
    });

    expect(context.systemInstruction).not.toContain("<html-visual>");
    expect(context.systemInstruction).toContain("<diagram-rendering>");
    expect(context.systemInstruction).not.toContain("<diagram-visual-polish>");
    expect(context.systemInstruction).not.toContain("<format_instructions");
  });
});
