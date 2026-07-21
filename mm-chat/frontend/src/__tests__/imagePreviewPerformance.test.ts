import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

describe("image preview performance composition", () => {
  const source = readFileSync(
    resolve(process.cwd(), "src/components/media/ImagePreview.tsx"),
    "utf8",
  );

  it("keeps continuous high-range zoom without expensive full-screen filters", () => {
    expect(source).toContain("maxScale={32}");
    expect(source).toContain("wheel={{ step: 0.008 }}");
    expect(source).toContain("velocityAnimation={{ disabled: true }}");
    expect(source).toContain("zoomAnimation={{ disabled: true }}");
    expect(source).toContain('willChange: "transform"');
    expect(source).not.toContain("backdrop-blur-2xl");
    expect(source).not.toContain("drop-shadow-2xl");
    expect(source).not.toContain("transition-[opacity,transform]");
  });
});
