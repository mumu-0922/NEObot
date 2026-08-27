import type { ChatAgentEvent, ProcessStep } from "./types";
import {
  normalizeChatAgentEvents,
  normalizeProcessStep,
  projectProcessStepsForDisplay,
} from "./processTrace";

const CONTEXT_SOURCES = new Set([
  "system-prompt",
  "skill-catalog",
  "skill-instruction",
  "runtime-context",
]);
const MAX_CONTEXT_BYTES = 64 * 1024;
const MAX_REASONING_BYTES = 1024 * 1024;
const MAX_NARRATION_BYTES = 1024 * 1024;

export interface AgentTranscriptContextNode {
  id: string;
  type: "context";
  sequence: number;
  source: string;
  label: string;
  content: string;
  truncated: boolean;
}

export interface AgentTranscriptReasoningNode {
  id: string;
  type: "reasoning";
  sequence: number;
  blockIndex: number;
  round?: number;
  content: string;
  status: "running" | "completed" | "interrupted";
}

export interface AgentTranscriptNarrationNode {
  id: string;
  type: "narration";
  sequence: number;
  blockIndex: number;
  round?: number;
  content: string;
  status: "running" | "completed" | "interrupted";
}

export interface AgentTranscriptToolNode {
  id: string;
  type: "tool";
  sequence: number;
  step: ProcessStep;
}

export type AgentTranscriptNode =
  | AgentTranscriptContextNode
  | AgentTranscriptReasoningNode
  | AgentTranscriptNarrationNode
  | AgentTranscriptToolNode;

export function projectAgentTranscript(value: unknown): AgentTranscriptNode[] {
  const events = normalizeChatAgentEvents(value);
  if (!isTranscriptV2(events)) return [];
  const nodes: AgentTranscriptNode[] = [];
  const reasoningIndexes = new Map<number, number>();
  const narrationIndexes = new Map<number, number>();
  const toolIndexes = new Map<string, number>();
  let reasoningBytes = 0;
  let narrationBytes = 0;
  let turnEnded = false;

  for (const event of events) {
    if (event.type === "context.injected") {
      const node = contextNodeFromEvent(event);
      if (node) nodes.push(node);
      continue;
    }
    if (event.type === "assistant.chunk") {
      applyAssistantChunk(
        nodes,
        reasoningIndexes,
        narrationIndexes,
        event,
        (blockType, bytes) => {
          if (blockType === "reasoning") {
            const remaining = Math.max(MAX_REASONING_BYTES - reasoningBytes, 0);
            const accepted = Math.min(bytes, remaining);
            reasoningBytes += accepted;
            return accepted;
          }
          const remaining = Math.max(MAX_NARRATION_BYTES - narrationBytes, 0);
          const accepted = Math.min(bytes, remaining);
          narrationBytes += accepted;
          return accepted;
        },
      );
      continue;
    }
    if (event.type === "assistant.block.completed") {
      const blockType = assistantBlockType(event.payload.blockType);
      const blockIndex = positiveInteger(event.payload.blockIndex);
      if (!blockType || blockIndex === undefined) continue;
      const nodeIndex =
        blockType === "reasoning"
          ? reasoningIndexes.get(blockIndex)
          : narrationIndexes.get(blockIndex);
      if (nodeIndex === undefined) continue;
      const node = nodes[nodeIndex];
      if (node?.type === blockType) {
        nodes[nodeIndex] = { ...node, status: "completed" };
      }
      continue;
    }
    if (
      event.type === "tool.called" ||
      event.type === "tool.result" ||
      event.type === "step.started" ||
      event.type === "step.ended"
    ) {
      for (const step of processStepsFromEvent(event)) {
        if (step.kind === "reasoning" || step.kind === "generation") continue;
        const existingIndex = toolIndexes.get(step.id);
        if (existingIndex === undefined) {
          toolIndexes.set(step.id, nodes.length);
          nodes.push({
            id: `tool:${step.id}`,
            type: "tool",
            sequence: event.sequence,
            step,
          });
        } else {
          const existing = nodes[existingIndex];
          if (existing?.type === "tool") {
            nodes[existingIndex] = { ...existing, step };
          }
        }
      }
      continue;
    }
    if (event.type === "turn.ended") turnEnded = true;
  }

  const visibleSteps = projectProcessStepsForDisplay(
    nodes.flatMap((node) => (node.type === "tool" ? [node.step] : [])),
  );
  const visibleById = new Map(visibleSteps.map((step) => [step.id, step]));
  const projected: AgentTranscriptNode[] = [];
  for (const node of nodes) {
    if (node.type === "tool") {
      const step = visibleById.get(node.step.id);
      if (step) projected.push({ ...node, step });
      continue;
    }
    if (
      (node.type === "reasoning" || node.type === "narration") &&
      turnEnded &&
      node.status === "running"
    ) {
      projected.push({ ...node, status: "interrupted" });
      continue;
    }
    projected.push(node);
  }
  return projected;
}

export function hasAgentTranscript(value: unknown): boolean {
  return projectAgentTranscript(value).length > 0;
}

function isTranscriptV2(events: ChatAgentEvent[]): boolean {
  return events.some(
    (event) =>
      (event.type === "turn.started" &&
        event.payload.transcriptVersion === 2) ||
      event.type === "context.injected" ||
      event.type === "assistant.chunk" ||
      event.type === "assistant.block.completed",
  );
}

function contextNodeFromEvent(
  event: ChatAgentEvent,
): AgentTranscriptContextNode | null {
  const source = boundedString(event.payload.source, 64);
  const label = boundedString(event.payload.label, 256);
  const content = boundedContentString(
    event.payload.content,
    MAX_CONTEXT_BYTES,
  );
  if (!source || !CONTEXT_SOURCES.has(source) || !label || !content)
    return null;
  if (
    event.payload.truncated !== undefined &&
    typeof event.payload.truncated !== "boolean"
  ) {
    return null;
  }
  return {
    id: `context:${event.eventId}`,
    type: "context",
    sequence: event.sequence,
    source,
    label,
    content,
    truncated:
      event.payload.truncated === true ||
      byteLength(content) >= MAX_CONTEXT_BYTES,
  };
}

function applyAssistantChunk(
  nodes: AgentTranscriptNode[],
  reasoningIndexes: Map<number, number>,
  narrationIndexes: Map<number, number>,
  event: ChatAgentEvent,
  reserveBytes: (blockType: "reasoning" | "narration", bytes: number) => number,
): void {
  const chunkType = stringValue(event.payload.chunkType);
  const blockType = assistantBlockType(event.payload.blockType);
  const blockIndex = positiveInteger(event.payload.blockIndex);
  if (!blockType || blockIndex === undefined) return;
  const indexes =
    blockType === "reasoning" ? reasoningIndexes : narrationIndexes;

  if (chunkType === "block-start") {
    if (indexes.has(blockIndex)) return;
    indexes.set(blockIndex, nodes.length);
    nodes.push({
      id: `${blockType}:${event.turnId}:${blockIndex}`,
      type: blockType,
      sequence: event.sequence,
      blockIndex,
      ...(event.stepSequence ? { round: event.stepSequence } : {}),
      content: "",
      status: "running",
    });
    return;
  }

  if (chunkType !== `${blockType}-delta`) return;
  const nodeIndex = indexes.get(blockIndex);
  if (nodeIndex === undefined) return;
  const node = nodes[nodeIndex];
  const content = boundedContentString(event.payload.content, 64 * 1024);
  if (node?.type !== blockType || !content) return;
  const acceptedBytes = reserveBytes(blockType, byteLength(content));
  if (acceptedBytes <= 0) return;
  nodes[nodeIndex] = {
    ...node,
    content: `${node.content}${truncateUtf8(content, acceptedBytes)}`,
  };
}

function assistantBlockType(
  value: unknown,
): "reasoning" | "narration" | undefined {
  return value === "reasoning" || value === "narration" ? value : undefined;
}

function processStepsFromEvent(event: ChatAgentEvent): ProcessStep[] {
  const steps: ProcessStep[] = [];
  const single = normalizeProcessStep(event.payload.processStep);
  if (single) steps.push(single);
  if (Array.isArray(event.payload.processSteps)) {
    for (const value of event.payload.processSteps) {
      const step = normalizeProcessStep(value);
      if (step) steps.push(step);
    }
  }
  return steps;
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function boundedString(value: unknown, maxBytes: number): string {
  return truncateUtf8(stringValue(value), maxBytes);
}

function boundedContentString(value: unknown, maxBytes: number): string {
  return typeof value === "string" ? truncateUtf8(value, maxBytes) : "";
}

function positiveInteger(value: unknown): number | undefined {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0
    ? value
    : undefined;
}

function byteLength(value: string): number {
  return new TextEncoder().encode(value).length;
}

function truncateUtf8(value: string, maxBytes: number): string {
  const bytes = new TextEncoder().encode(value);
  if (bytes.length <= maxBytes) return value;
  return new TextDecoder().decode(bytes.slice(0, maxBytes));
}
