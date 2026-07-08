import { ApiClientError } from "../errors";
import type { ServerStreamEvent } from "../types";

export interface ParsedSseFrame {
  event: string;
  data: ServerStreamEvent;
}

export function parseSseFrames(input: string): ParsedSseFrame[] {
  const frames: ParsedSseFrame[] = [];
  const normalized = input.replace(/\r\n/g, "\n").replace(/\r/g, "\n");

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
