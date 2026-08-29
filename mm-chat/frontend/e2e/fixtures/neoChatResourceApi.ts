import type { Route } from "@playwright/test";

import { json, NOW } from "./neoChatApiSupport";

const SKILL_FINGERPRINT = `sha256:${"b".repeat(64)}`;

export const TEST_SKILL = {
  id: "skill-installation-e2e",
  admissionId: "skill-admission-e2e",
  packageFingerprint: SKILL_FINGERPRINT,
  name: "e2e-report-skill",
  version: "1.0.0",
  description: "Creates a deterministic project report.",
  allowedTools: ["read", "write"],
  revision: 1,
  createdAt: NOW,
  updatedAt: NOW,
};

export const TEST_MCP_SERVER = {
  ref: { source: "catalog" as const, id: "e2e-issues" },
  name: "E2E Issues",
  description: "Reads deterministic issue metadata.",
  icon: "🧪",
  transport: "streamable_http" as const,
  endpointUrl: "https://mcp.example.test/endpoint",
  authType: "none" as const,
  status: "ready" as const,
  hasCredential: false,
  canManage: false,
  configurationFields: [],
  toolCount: 1,
  unsupportedToolCount: 0,
  grants: [{ scopeType: "user", defaultEnabled: false }],
  createdAt: NOW,
  updatedAt: NOW,
  tools: [
    {
      serverRef: { source: "catalog" as const, id: "e2e-issues" },
      name: "lookup_issue",
      alias: "lookup_issue",
      title: "Lookup issue",
      description: "Reads one deterministic issue.",
      inputSchema: { type: "object" },
      classification: "read" as const,
      supported: true,
    },
  ],
};

type SkillInstallation = typeof TEST_SKILL;
type McpServer = typeof TEST_MCP_SERVER;
type McpSelectionServer = {
  ref: { source: "catalog" | "manifest" | "private"; id: string };
  disabledTools: string[];
};

type SkillSelection = {
  revision: number;
  installationIds: string[];
};

type McpSelection = {
  mode: "inherit" | "custom";
  revision: number;
  servers: McpSelectionServer[];
};

export class NeoChatResourceApiFixture {
  readonly skillLibrary: SkillInstallation[] = [];
  readonly mcpServers: McpServer[] = [];
  readonly skillSelections = new Map<string, SkillSelection>();
  readonly mcpSelections = new Map<string, McpSelection>();
  mcpEnabled = false;
  lastInstalledSkillURL = "";

  enableMcp(): void {
    this.mcpEnabled = true;
  }

  seedSkill(): void {
    if (!this.skillLibrary.some((skill) => skill.id === TEST_SKILL.id)) {
      this.skillLibrary.push({ ...TEST_SKILL });
    }
  }

  seedMcpServer(): void {
    this.enableMcp();
    if (
      !this.mcpServers.some(
        (server) => server.ref.id === TEST_MCP_SERVER.ref.id,
      )
    ) {
      this.mcpServers.push(structuredClone(TEST_MCP_SERVER));
    }
  }

  getSkillSelection(conversationId: string): SkillSelection {
    return (
      this.skillSelections.get(conversationId) ?? {
        revision: 0,
        installationIds: [],
      }
    );
  }

  getMcpSelection(conversationId: string): McpSelection {
    return (
      this.mcpSelections.get(conversationId) ?? {
        mode: "inherit",
        revision: 0,
        servers: [],
      }
    );
  }

  async handle(route: Route, path: string, method: string): Promise<boolean> {
    const request = route.request();

    if (path === "/v1/skills/library" && method === "GET") {
      await json(route, { skills: this.skillLibrary });
      return true;
    }

    if (path === "/v1/skills/catalog" && method === "GET") {
      await json(route, {
        items: [],
        totalCount: 0,
        source: "openai/skills curated",
      });
      return true;
    }

    if (path === "/v1/skills/marketplace" && method === "GET") {
      await json(route, {
        items: [],
        categories: [],
        page: 1,
        pageSize: 20,
        totalCount: 0,
        totalPages: 0,
        source: "lobehub",
        sourceUrl: "https://lobehub.com/skills",
      });
      return true;
    }

    if (path === "/v1/skills/direct/install" && method === "POST") {
      const body = request.postDataJSON() as { url?: unknown };
      this.lastInstalledSkillURL = typeof body.url === "string" ? body.url : "";
      if (
        !/^https:\/\/(?:lobehub\.com\/skills\/|github\.com\/[^/]+\/[^/]+\/(?:tree|blob)\/)/.test(
          this.lastInstalledSkillURL,
        )
      ) {
        await json(
          route,
          {
            error: {
              code: "INVALID_SKILL_URL",
              message: "Invalid Skill URL",
            },
          },
          400,
        );
        return true;
      }
      this.seedSkill();
      await json(route, { skill: TEST_SKILL });
      return true;
    }

    const skillSelectionMatch = path.match(
      /^\/v1\/skills\/conversations\/([^/]+)\/selection$/,
    );
    if (skillSelectionMatch && (method === "GET" || method === "PUT")) {
      const conversationId = decodeURIComponent(skillSelectionMatch[1]);
      if (method === "PUT") {
        const current = this.getSkillSelection(conversationId);
        const body = request.postDataJSON() as {
          revision?: unknown;
          installationIds?: unknown;
        };
        if (
          body.revision !== current.revision ||
          !Array.isArray(body.installationIds) ||
          !body.installationIds.every(
            (id) =>
              typeof id === "string" &&
              this.skillLibrary.some((skill) => skill.id === id),
          )
        ) {
          await json(
            route,
            {
              error: { code: "REVISION_CONFLICT", message: "Stale selection" },
            },
            409,
          );
          return true;
        }
        const installationIds = body.installationIds as string[];
        this.skillSelections.set(conversationId, {
          revision: current.revision + 1,
          installationIds: [...installationIds],
        });
      }
      await json(route, {
        selection: this.skillSelectionDTO(conversationId),
      });
      return true;
    }

    if (path === "/v1/mcp/servers" && method === "GET") {
      await json(route, { servers: this.mcpServers, canManage: false });
      return true;
    }

    const mcpSelectionMatch = path.match(
      /^\/v1\/mcp\/conversations\/([^/]+)\/selection$/,
    );
    if (mcpSelectionMatch && (method === "GET" || method === "PUT")) {
      const conversationId = decodeURIComponent(mcpSelectionMatch[1]);
      if (method === "PUT") {
        const current = this.getMcpSelection(conversationId);
        const body = request.postDataJSON() as {
          mode?: unknown;
          revision?: unknown;
          servers?: unknown;
        };
        if (
          body.mode !== "custom" ||
          body.revision !== current.revision ||
          !Array.isArray(body.servers) ||
          !body.servers.every((server) => this.isAuthorizedMcpSelection(server))
        ) {
          await json(
            route,
            {
              error: { code: "REVISION_CONFLICT", message: "Stale selection" },
            },
            409,
          );
          return true;
        }
        this.mcpSelections.set(conversationId, {
          mode: "custom",
          revision: current.revision + 1,
          servers: structuredClone(body.servers) as McpSelectionServer[],
        });
      }
      await json(route, {
        selection: this.mcpSelectionDTO(conversationId),
      });
      return true;
    }

    return false;
  }

  private skillSelectionDTO(conversationId: string) {
    const selection = this.getSkillSelection(conversationId);
    return {
      conversationId,
      revision: selection.revision,
      skills: selection.installationIds.flatMap((id) => {
        const skill = this.skillLibrary.find(
          (candidate) => candidate.id === id,
        );
        return skill ? [skill] : [];
      }),
    };
  }

  private mcpSelectionDTO(conversationId: string) {
    const selection = this.getMcpSelection(conversationId);
    return {
      conversationId,
      mode: selection.mode,
      revision: selection.revision,
      servers: selection.servers,
    };
  }

  private isAuthorizedMcpSelection(value: unknown): boolean {
    if (!value || typeof value !== "object" || Array.isArray(value)) {
      return false;
    }
    const selection = value as Record<string, unknown>;
    const ref = selection.ref;
    if (!ref || typeof ref !== "object" || Array.isArray(ref)) return false;
    const candidate = ref as Record<string, unknown>;
    return (
      typeof candidate.source === "string" &&
      typeof candidate.id === "string" &&
      Array.isArray(selection.disabledTools) &&
      selection.disabledTools.every((tool) => typeof tool === "string") &&
      this.mcpServers.some(
        (server) =>
          server.ref.source === candidate.source &&
          server.ref.id === candidate.id,
      )
    );
  }
}

export function resourceTranscriptEvents(
  conversationId: string,
  messageId: string,
  mcpStatus: "completed" | "failed" = "completed",
) {
  const base = {
    turnId: `turn-resource-${messageId}`,
    conversationId,
    messageId,
    runId: `run-resource-${conversationId}`,
    occurredAt: NOW,
  };
  const event = (
    sequence: number,
    type: string,
    payload: Record<string, unknown>,
  ) => ({
    ...base,
    eventId: `event-resource-${messageId}-${sequence}`,
    sequence,
    type,
    payload,
  });
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
      content: "先运行已选择的 Skill。",
    }),
    event(4, "assistant.block.completed", {
      blockType: "narration",
      blockIndex: 1,
    }),
    event(5, "tool.result", {
      processSteps: [skillStep()],
    }),
    event(6, "assistant.chunk", {
      chunkType: "block-start",
      blockType: "narration",
      blockIndex: 2,
    }),
    event(7, "assistant.chunk", {
      chunkType: "narration-delta",
      blockType: "narration",
      blockIndex: 2,
      content: "再调用已选择的 MCP。",
    }),
    event(8, "assistant.block.completed", {
      blockType: "narration",
      blockIndex: 2,
    }),
    event(9, "tool.result", {
      processSteps: [mcpStep(mcpStatus)],
    }),
    event(10, "turn.ended", {
      status: mcpStatus === "failed" ? "failed" : "completed",
    }),
  ];
}

function skillStep() {
  return {
    id: "resource-skill-e2e",
    kind: "tool",
    status: "completed",
    labelKey: "process.tool",
    durationMs: 12,
    detail: { toolName: "skill", mode: "local_direct", round: 1 },
    presentation: {
      version: 1,
      card: "skill",
      title: "e2e-report-skill",
      summary: "Skill instructions loaded for this Run.",
    },
  };
}

function mcpStep(status: "completed" | "failed") {
  return {
    id: "resource-mcp-e2e",
    kind: "tool",
    status,
    labelKey: "process.tool",
    durationMs: 34,
    detail: {
      toolName: "lookup_issue",
      serverName: TEST_MCP_SERVER.name,
      mode: "mcp",
      round: 2,
      ...(status === "failed" ? { failureCategory: "unavailable" } : {}),
    },
    presentation: {
      version: 1,
      card: "mcp",
      title: "Lookup issue",
      summary:
        status === "failed"
          ? "The MCP Server returned a bounded failure."
          : "Issue E2E-42 was loaded.",
    },
  };
}
