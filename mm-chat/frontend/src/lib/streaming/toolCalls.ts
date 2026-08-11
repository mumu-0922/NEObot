import { TOOL_EXECUTION_LIMITS } from "../../config/limits";
import type { ToolCall } from "../../types";

export interface PendingOpenAIToolCall {
  id: string;
  name: string;
  argsText: string;
  error?: string;
}

export interface OpenAIToolCallAccumulator {
  calls: PendingOpenAIToolCall[];
  indexToPosition: Map<number, number>;
}

const UNKNOWN_TOOL_NAME = "unknown_tool";

function trimText(value: unknown, maxChars: number): string {
  if (typeof value !== "string") return "";
  return value.trim().slice(0, maxChars);
}

function createFallbackToolCallId(position: number): string {
  return `call_${Date.now()}_${position}`;
}

function createToolCallError(
  id: string,
  name: string,
  message: string,
): ToolCall {
  return {
    id,
    name: name || UNKNOWN_TOOL_NAME,
    args: {},
    status: "error",
    isError: true,
    result: message,
  };
}

export function createOpenAIToolCallAccumulator(): OpenAIToolCallAccumulator {
  return {
    calls: [],
    indexToPosition: new Map(),
  };
}

export function appendOpenAIToolCallDelta(
  accumulator: OpenAIToolCallAccumulator,
  delta: any,
) {
  const rawIndex = delta?.index;
  const providerIndex =
    Number.isSafeInteger(rawIndex) && rawIndex >= 0
      ? rawIndex
      : accumulator.calls.length;

  let position = accumulator.indexToPosition.get(providerIndex);
  if (position === undefined) {
    if (
      accumulator.calls.length >= TOOL_EXECUTION_LIMITS.maxStreamedToolCalls
    ) {
      return;
    }

    position = accumulator.calls.length;
    accumulator.indexToPosition.set(providerIndex, position);
    accumulator.calls.push({
      id: "",
      name: "",
      argsText: "",
    });
  }

  const pending = accumulator.calls[position];
  const id = trimText(delta?.id, TOOL_EXECUTION_LIMITS.maxToolCallIdChars);
  const name = trimText(
    delta?.function?.name,
    TOOL_EXECUTION_LIMITS.maxFunctionNameChars,
  );
  const argsFragment =
    typeof delta?.function?.arguments === "string"
      ? delta.function.arguments
      : "";

  if (id) pending.id = id;
  if (name) pending.name = name;

  if (!argsFragment || pending.error) return;

  const nextLength = pending.argsText.length + argsFragment.length;
  if (nextLength > TOOL_EXECUTION_LIMITS.maxArgsJsonChars) {
    pending.error = "Tool call arguments are too large to process safely.";
    pending.argsText = pending.argsText.slice(
      0,
      TOOL_EXECUTION_LIMITS.maxArgsJsonChars,
    );
    return;
  }

  pending.argsText += argsFragment;
}

export function finalizeStreamedToolCall(
  input: {
    id?: unknown;
    name?: unknown;
    args?: unknown;
    argsText?: string;
    error?: string;
  },
  position = 0,
): ToolCall | null {
  const id =
    trimText(input.id, TOOL_EXECUTION_LIMITS.maxToolCallIdChars) ||
    createFallbackToolCallId(position);
  const name = trimText(input.name, TOOL_EXECUTION_LIMITS.maxFunctionNameChars);

  if (
    !name &&
    !input.error &&
    input.args === undefined &&
    input.argsText === undefined
  ) {
    return null;
  }

  const nameError = getToolCallFunctionNameError(name);
  if (input.error || nameError) {
    return createToolCallError(
      id,
      name,
      input.error || nameError || "Tool call is invalid.",
    );
  }

  let args = input.args;
  if (input.argsText !== undefined) {
    const text = input.argsText.trim();
    if (!text) {
      args = {};
    } else {
      try {
        args = JSON.parse(text);
      } catch {
        return createToolCallError(
          id,
          name,
          "Tool call arguments were not valid JSON.",
        );
      }
    }
  }

  const argsError = getToolCallArgumentsError(args);
  if (argsError) {
    return createToolCallError(id, name, argsError);
  }

  return {
    id,
    name,
    args: args as Record<string, unknown>,
    status: "pending",
  };
}

function getToolCallFunctionNameError(name: unknown): string | null {
  if (typeof name !== "string" || !name.trim()) {
    return "Tool function name is missing.";
  }
  if (name.length > TOOL_EXECUTION_LIMITS.maxFunctionNameChars) {
    return "Tool function name is too long.";
  }
  return null;
}

function getToolCallArgumentsError(args: unknown): string | null {
  if (!isRecord(args)) return "Tool arguments must be a JSON object.";
  const seen = new WeakSet<object>();
  const stack: Array<{ value: unknown; depth: number }> = [
    { value: args, depth: 0 },
  ];
  let entries = 0;

  while (stack.length > 0) {
    const current = stack.pop();
    if (!current || current.value === null) continue;
    const value = current.value;
    if (typeof value === "string" || typeof value === "boolean") continue;
    if (typeof value === "number") {
      if (!Number.isFinite(value))
        return "Tool arguments contain an invalid number.";
      continue;
    }
    if (typeof value !== "object") {
      return "Tool arguments contain values that are not JSON-compatible.";
    }
    if (current.depth >= TOOL_EXECUTION_LIMITS.maxArgDepth) {
      return "Tool arguments are too deeply nested.";
    }
    if (seen.has(value)) return "Tool arguments contain a circular reference.";
    seen.add(value);
    if (!Array.isArray(value) && !isPlainObject(value)) {
      return "Tool arguments must contain only JSON objects and arrays.";
    }
    const children = Array.isArray(value)
      ? value
      : Object.values(value as Record<string, unknown>);
    entries += children.length;
    if (entries > TOOL_EXECUTION_LIMITS.maxArgEntries) {
      return "Tool arguments contain too many values.";
    }
    for (const child of children) {
      stack.push({ value: child, depth: current.depth + 1 });
    }
  }

  if (JSON.stringify(args).length > TOOL_EXECUTION_LIMITS.maxArgsJsonChars) {
    return "Tool arguments are too large.";
  }
  return null;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}

function isPlainObject(value: object): boolean {
  const prototype = Object.getPrototypeOf(value);
  return prototype === Object.prototype || prototype === null;
}

export function finalizeOpenAIToolCalls(
  accumulator: OpenAIToolCallAccumulator,
): ToolCall[] {
  const finalized: ToolCall[] = [];
  for (let index = 0; index < accumulator.calls.length; index += 1) {
    const pending = accumulator.calls[index];
    const toolCall = finalizeStreamedToolCall(
      {
        id: pending.id,
        name: pending.name,
        argsText: pending.argsText,
        error: pending.error,
      },
      index,
    );
    if (toolCall) finalized.push(toolCall);
  }
  return finalized;
}
