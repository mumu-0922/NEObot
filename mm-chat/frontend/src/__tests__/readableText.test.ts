import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import {
  getReadableTextFromElement,
  normalizeReadableText,
} from "@/lib/voice/readableText";

describe("read-aloud visible text", () => {
  it("normalizes rendered innerText without reintroducing source markup", () => {
    const element = {
      innerText:
        "  西安天气\r\n\r\n\t当前晴，约 33°C  \n\u00a0出行提示： 防暑  \u200b\n",
    } as HTMLElement;

    expect(getReadableTextFromElement(element)).toBe(
      "西安天气\n当前晴，约 33°C\n出行提示： 防暑",
    );
    expect(normalizeReadableText("  \n\t ")).toBe("");
    expect(getReadableTextFromElement(null)).toBe("");
  });

  it("wires MessageItem to the forwarded rendered-output ref", () => {
    const messageItem = readFileSync(
      resolve(process.cwd(), "src/components/chat/MessageItem.tsx"),
      "utf8",
    );
    const outputRenderer = readFileSync(
      resolve(
        process.cwd(),
        "src/components/content/MessageOutputRenderer.tsx",
      ),
      "utf8",
    );

    expect(messageItem).toContain("readAloudContentRef");
    expect(messageItem).toContain(
      "getReadableTextFromElement(\n          readAloudContentRef.current",
    );
    expect(messageItem).toContain("readableText,");
    expect(messageItem).toContain("ref={readAloudContentRef}");
    expect(outputRenderer).toContain("React.forwardRef<");
    expect(outputRenderer).toContain("ref={ref}");
  });
});
