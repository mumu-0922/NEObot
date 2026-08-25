import { describe, expect, it } from "vitest";

import {
  buildSlashCommands,
  filterSlashCommands,
  parseSlashCommand,
  slashCommandInsertion,
} from "../lib/chat/slashCommands";

describe("slash commands", () => {
  it("builds deterministic dynamic Skill commands and rejects unsafe names", () => {
    const commands = buildSlashCommands([
      "office-xlsx",
      "office-xlsx",
      "../../escape",
      "UPPER-SKILL",
    ]);
    expect(commands.filter((item) => item.skillName)).toEqual([
      expect.objectContaining({ command: "/skill:office-xlsx" }),
      expect.objectContaining({ command: "/skill:upper-skill" }),
    ]);
    expect(commands.map((item) => item.group)).toEqual(
      [...commands.map((item) => item.group)].sort(
        (left, right) =>
          ["builtin", "skill", "mcp", "session"].indexOf(left) -
          ["builtin", "skill", "mcp", "session"].indexOf(right),
      ),
    );
  });

  it("parses Pi-style Skill invocation without treating arguments as a command", () => {
    expect(parseSlashCommand(" /skill:office-xlsx 创建报表 ")).toEqual({
      kind: "skill-invoke",
      name: "office-xlsx",
      arguments: "创建报表",
      raw: "/skill:office-xlsx 创建报表",
    });
  });

  it("parses resource mutations and built-ins deterministically", () => {
    expect(parseSlashCommand("/skill install candidate-id")).toEqual(
      expect.objectContaining({
        kind: "resource-command",
        resource: "skill",
        action: "install",
        arguments: "candidate-id",
      }),
    );
    expect(parseSlashCommand("/resources")).toEqual({
      kind: "builtin",
      command: "resources",
      raw: "/resources",
    });
    expect(parseSlashCommand("hello")).toBeNull();
  });

  it("filters and inserts commands with argument hints", () => {
    const commands = buildSlashCommands(["office-xlsx"]);
    const root = filterSlashCommands(commands, "/");
    expect(root.map((item) => item.command)).toEqual(
      expect.arrayContaining([
        "/resources",
        "/reload",
        "/skill",
        "/mcp",
        "/skill:office-xlsx",
      ]),
    );
    const matches = filterSlashCommands(commands, "/xlsx");
    expect(matches.map((item) => item.command)).toContain("/skill:office-xlsx");
    const install = commands.find((item) => item.command === "/skill install");
    expect(install && slashCommandInsertion(install)).toBe("/skill install ");
  });
});
