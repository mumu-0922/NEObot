export type SlashCommandGroup = "builtin" | "skill" | "mcp" | "session";

export interface SlashCommandDefinition {
  command: string;
  description: string;
  group: SlashCommandGroup;
  argumentHint?: string;
  skillName?: string;
}

export type ParsedSlashCommand =
  | {
      kind: "skill-invoke";
      name: string;
      arguments: string;
      raw: string;
    }
  | {
      kind: "resource-command";
      resource: "skill" | "mcp";
      action: string;
      arguments: string;
      raw: string;
    }
  | { kind: "builtin"; command: "resources" | "reload"; raw: string }
  | { kind: "unknown"; raw: string };

const SKILL_NAME_PATTERN = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
const GROUP_ORDER: Record<SlashCommandGroup, number> = {
  builtin: 0,
  skill: 1,
  mcp: 2,
  session: 3,
};

export const BUILTIN_SLASH_COMMANDS: readonly SlashCommandDefinition[] = [
  {
    command: "/resources",
    description: "查看当前会话可用的 Skill 与 MCP",
    group: "builtin",
  },
  {
    command: "/reload",
    description: "重新读取已安装资源；不会改动运行中的任务",
    group: "session",
  },
  {
    command: "/skill",
    description: "查看已安装 Skill",
    group: "skill",
  },
  {
    command: "/skill search",
    description: "搜索已准入的 Skill",
    group: "skill",
    argumentHint: "<关键词>",
  },
  {
    command: "/skill info",
    description: "查看 Skill 的来源、版本与权限",
    group: "skill",
    argumentHint: "<名称或 ID>",
  },
  {
    command: "/skill install",
    description: "安装已准入的 Skill",
    group: "skill",
    argumentHint: "<候选 ID>",
  },
  {
    command: "/skill remove",
    description: "卸载当前用户的 Skill",
    group: "skill",
    argumentHint: "<名称或安装 ID>",
  },
  {
    command: "/mcp",
    description: "查看当前会话的 MCP Server",
    group: "mcp",
  },
  {
    command: "/mcp search",
    description: "搜索 MCP Marketplace",
    group: "mcp",
    argumentHint: "<关键词>",
  },
  {
    command: "/mcp info",
    description: "查看 MCP 的部署、认证与 Tool",
    group: "mcp",
    argumentHint: "<identifier>",
  },
  {
    command: "/mcp status",
    description: "查看当前会话 MCP selection 与健康状态",
    group: "mcp",
  },
  {
    command: "/mcp install",
    description: "安装并启用无需凭据的 MCP",
    group: "mcp",
    argumentHint: "<identifier>",
  },
  {
    command: "/mcp enable",
    description: "为当前会话启用 MCP",
    group: "mcp",
    argumentHint: "<名称或 source:id>",
  },
  {
    command: "/mcp disable",
    description: "为当前会话停用 MCP",
    group: "mcp",
    argumentHint: "<名称或 source:id>",
  },
  {
    command: "/mcp remove",
    description: "删除有管理权限的 MCP Server",
    group: "mcp",
    argumentHint: "<名称或 source:id>",
  },
];

export function buildSlashCommands(
  installedSkillNames: readonly string[],
): SlashCommandDefinition[] {
  const dynamic = Array.from(
    new Set(
      installedSkillNames
        .map((name) => name.trim().toLowerCase())
        .filter((name) => SKILL_NAME_PATTERN.test(name)),
    ),
  )
    .sort((left, right) => left.localeCompare(right))
    .map((name) => ({
      command: `/skill:${name}`,
      description: `调用已安装 Skill：${name}`,
      group: "skill" as const,
      argumentHint: "[参数]",
      skillName: name,
    }));
  return [...BUILTIN_SLASH_COMMANDS, ...dynamic].sort(
    (left, right) => GROUP_ORDER[left.group] - GROUP_ORDER[right.group],
  );
}

export function filterSlashCommands(
  commands: readonly SlashCommandDefinition[],
  value: string,
): SlashCommandDefinition[] {
  const normalized = value.trimStart().toLowerCase();
  if (!normalized.startsWith("/") || normalized.includes("\n")) return [];
  const terms = normalized.slice(1).split(/\s+/).filter(Boolean);
  return commands.filter((item) => {
    const searchable =
      `${item.command} ${item.description} ${item.argumentHint ?? ""}`.toLowerCase();
    return terms.every((term) => searchable.includes(term));
  });
}

export function slashCommandInsertion(command: SlashCommandDefinition): string {
  return command.argumentHint ? `${command.command} ` : command.command;
}

export function parseSlashCommand(value: string): ParsedSlashCommand | null {
  const raw = value.trim();
  if (!raw.startsWith("/") || raw.includes("\n")) return null;

  const skillInvocation = raw.match(
    /^\/skill:([a-z0-9]+(?:-[a-z0-9]+)*)(?:\s+([\s\S]*))?$/i,
  );
  if (skillInvocation) {
    return {
      kind: "skill-invoke",
      name: skillInvocation[1].toLowerCase(),
      arguments: (skillInvocation[2] ?? "").trim(),
      raw,
    };
  }

  const resource = raw.match(
    /^\/(skill|mcp)(?:\s+([^\s]+))?(?:\s+([\s\S]*))?$/i,
  );
  if (resource) {
    return {
      kind: "resource-command",
      resource: resource[1].toLowerCase() as "skill" | "mcp",
      action: (resource[2] ?? "list").toLowerCase(),
      arguments: (resource[3] ?? "").trim(),
      raw,
    };
  }

  if (raw === "/resources" || raw === "/reload") {
    return {
      kind: "builtin",
      command: raw.slice(1) as "resources" | "reload",
      raw,
    };
  }
  return { kind: "unknown", raw };
}
