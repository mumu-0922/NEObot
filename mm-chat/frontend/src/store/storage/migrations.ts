import { Message, MessageOutputBlock, ToolCall } from "@/types";
import { normalizeSearchSettings } from "../../lib/settings/search";

const RETIRED_PLUGIN_KEYS = new Set([
  "activePlugins",
  "installedPlugins",
  "pluginConfigs",
  "marketPlugins",
  "marketPluginsTimestamp",
]);

export function stripRetiredPluginFields(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(stripRetiredPluginFields);
  if (!value || typeof value !== "object") return value;

  return Object.fromEntries(
    Object.entries(value as Record<string, unknown>)
      .filter(([key]) => !RETIRED_PLUGIN_KEYS.has(key))
      .map(([key, field]) => [key, stripRetiredPluginFields(field)]),
  );
}

export function normalizeToolCall(toolCall: Partial<ToolCall>): ToolCall {
  let status = toolCall.status;
  if (!status) {
    if (toolCall.isError) {
      status = "error";
    } else if (toolCall.result !== undefined) {
      status = "success";
    } else {
      status = "pending";
    }
  }

  return {
    id: toolCall.id || `tool_${Date.now()}`,
    name: toolCall.name || "unknown_tool",
    args: toolCall.args ?? {},
    status,
    result: toolCall.result,
    isError: toolCall.isError,
    auth: toolCall.auth,
  };
}

export function normalizeMessage(message: Message): Message {
  const legacyMessage = message as Message & { skillInvocations?: unknown };
  const { skillInvocations, ...retainedMessage } = legacyMessage;
  const legacySkillRetired =
    message.legacySkillRetired === true ||
    (Array.isArray(skillInvocations) && skillInvocations.length > 0);
  const normalizedBlocks = message.outputBlocks?.map((block) => {
    if (block.type !== "tool_group") return block;
    return {
      ...block,
      toolCalls: block.toolCalls.map((toolCall) => normalizeToolCall(toolCall)),
    } satisfies MessageOutputBlock;
  });

  if (
    skillInvocations === undefined &&
    !legacySkillRetired &&
    !message.toolCalls?.length &&
    !normalizedBlocks
  ) {
    return message;
  }

  return {
    ...retainedMessage,
    ...(legacySkillRetired ? { legacySkillRetired: true as const } : {}),
    ...(message.toolCalls?.length
      ? {
          toolCalls: message.toolCalls.map((toolCall) =>
            normalizeToolCall(toolCall),
          ),
        }
      : {}),
    ...(normalizedBlocks ? { outputBlocks: normalizedBlocks } : {}),
  };
}

export function normalizeMessages(messages: Message[] | null | undefined) {
  return (messages || []).map((message) => normalizeMessage(message));
}

export function migrateSearchSettings(search: any) {
  return normalizeSearchSettings(search);
}
