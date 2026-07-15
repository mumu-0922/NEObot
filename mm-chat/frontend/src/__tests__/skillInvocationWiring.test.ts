import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

function countOccurrences(source: string, needle: string) {
  return source.split(needle).length - 1;
}

describe("skill invocation wiring", () => {
  it("passes skills context through every ChatApp response generation path", () => {
    const chatApp = readFileSync(
      resolve(process.cwd(), "src/components/app/ChatApp.tsx"),
      "utf8",
    );

    const streamCallCount = countOccurrences(chatApp, "streamChatResponse(");
    const serverStreamCallCount = countOccurrences(
      chatApp,
      "sendServerMessageAndStream({",
    );
    const generationCallCount = streamCallCount + serverStreamCallCount;

    expect(streamCallCount).toBeGreaterThan(0);
    expect(serverStreamCallCount).toBeGreaterThan(0);
    expect(countOccurrences(chatApp, "resolveSkillsForMessage({")).toBe(
      generationCallCount,
    );
    expect(countOccurrences(chatApp, "skillResolution.context")).toBe(
      generationCallCount,
    );
  });
});
