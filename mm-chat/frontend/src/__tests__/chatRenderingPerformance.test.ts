import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const readSource = (path: string) =>
  readFileSync(resolve(process.cwd(), path), "utf8");

describe("chat rendering performance composition", () => {
  it("keeps message heights stable while scrolling", () => {
    const chatApp = readSource("src/components/app/ChatApp.tsx");
    const globals = readSource("src/app/globals.css");

    expect(chatApp).not.toContain("[content-visibility:auto]");
    expect(globals).not.toMatch(
      /\.message-item\s*\{[\s\S]*?content-visibility/,
    );
  });

  it("keeps the floating composer and embedded visuals off the hot path", () => {
    const messageInput = readSource("src/components/chat/MessageInput.tsx");
    const markdown = readSource(
      "src/components/content/MarkdownRendererClient.tsx",
    );
    const globals = readSource("src/app/globals.css");

    expect(messageInput).toContain("glass-shell chat-composer-surface");
    expect(globals).toMatch(
      /\.chat-composer-surface,[\s\S]*?backdrop-filter: none;/,
    );
    expect(markdown).toMatch(
      /<iframe\s+srcDoc=\{srcDoc\}[\s\S]*?loading="lazy"[\s\S]*?sandbox="allow-scripts"/,
    );
  });
});
