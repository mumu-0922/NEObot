import { ApiClientError } from "../errors";
import type { ServerStreamEvent } from "../types";

export interface ParsedSseFrame {
  event: string;
  data: ServerStreamEvent;
}

export interface SseStreamParser {
  push(chunk: string): ParsedSseFrame[];
  flush(): ParsedSseFrame[];
}

export function createSseStreamParser(): SseStreamParser {
  let buffer = "";
  let pendingCarriageReturn = false;

  return {
    push(chunk) {
      buffer += normalizeStreamingChunk(chunk);
      return drainCompleteFrames();
    },
    flush() {
      if (pendingCarriageReturn) {
        buffer += "\n";
        pendingCarriageReturn = false;
      }
      const frame = parseSseBlock(buffer);
      buffer = "";
      return frame ? [frame] : [];
    },
  };

  function drainCompleteFrames(): ParsedSseFrame[] {
    const frames: ParsedSseFrame[] = [];

    while (true) {
      const delimiter = /\n\n+/.exec(buffer);
      if (!delimiter) break;

      const block = buffer.slice(0, delimiter.index);
      buffer = buffer.slice(delimiter.index + delimiter[0].length);
      const frame = parseSseBlock(block);
      if (frame) frames.push(frame);
    }

    return frames;
  }

  function normalizeStreamingChunk(chunk: string): string {
    let input = pendingCarriageReturn ? `\r${chunk}` : chunk;
    pendingCarriageReturn = false;

    if (input.endsWith("\r")) {
      pendingCarriageReturn = true;
      input = input.slice(0, -1);
    }

    return normalizeNewlines(input);
  }
}

export function parseSseFrames(input: string): ParsedSseFrame[] {
  const frames: ParsedSseFrame[] = [];
  const normalized = normalizeNewlines(input);

  for (const block of normalized.split(/\n\n+/)) {
    const frame = parseSseBlock(block);
    if (frame) frames.push(frame);
  }

  return frames;
}

export function parseSseBlock(block: string): ParsedSseFrame | null {
  const lines = block.split("\n");
  let event = "message";
  const dataLines: string[] = [];

  for (const line of lines) {
    if (!line || line.startsWith(":")) continue;
    const separatorIndex = line.indexOf(":");
    const field = separatorIndex === -1 ? line : line.slice(0, separatorIndex);
    const rawValue =
      separatorIndex === -1 ? "" : line.slice(separatorIndex + 1);
    const value = rawValue.startsWith(" ") ? rawValue.slice(1) : rawValue;

    if (field === "event") {
      event = value;
    } else if (field === "data") {
      dataLines.push(value);
    }
  }

  if (dataLines.length === 0) return null;

  let data: ServerStreamEvent;
  try {
    data = JSON.parse(dataLines.join("\n")) as ServerStreamEvent;
  } catch {
    throw new ApiClientError(
      "STREAM_PROTOCOL_ERROR",
      "SSE data frame is not valid JSON.",
      { recoverable: true },
    );
  }

  if (typeof data.type !== "string") {
    throw new ApiClientError(
      "STREAM_PROTOCOL_ERROR",
      "SSE data frame is missing a string type.",
      { recoverable: true },
    );
  }

  if (event !== data.type) {
    throw new ApiClientError(
      "STREAM_PROTOCOL_ERROR",
      `SSE event "${event}" does not match data.type "${data.type}".`,
      { recoverable: true },
    );
  }

  return { event, data };
}

function normalizeNewlines(input: string): string {
  return input.replace(/\r\n/g, "\n").replace(/\r/g, "\n");
}
