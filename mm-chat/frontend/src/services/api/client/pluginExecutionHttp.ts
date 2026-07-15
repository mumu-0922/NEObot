import { serializePluginExecutionPayload } from "../../../lib/plugin/execution";
import type { PluginExecuteInput } from "./types";

export const transitionalPluginExecutePath = "/api/plugins/execute";

export function postTransitionalPluginExecution(
  input: PluginExecuteInput,
): Promise<Response> {
  return fetch(transitionalPluginExecutePath, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: serializePluginExecutionPayload(input.payload),
    signal: input.signal,
    cache: "no-store",
  });
}
