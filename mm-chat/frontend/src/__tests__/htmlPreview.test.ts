import { describe, expect, it } from "vitest";
import { HTML_PREVIEW_LIMITS } from "../config/limits";
import {
  createSandboxedHtmlPreviewSrcDoc,
  createSandboxedHtmlVisualSrcDoc,
} from "../lib/utils/htmlPreview";

describe("HTML preview srcdoc", () => {
  it("preserves scripts without injecting a restrictive CSP", () => {
    const srcDoc = createSandboxedHtmlPreviewSrcDoc(
      "<!doctype html><html><head><title>x</title></head><body><script>window.previewRan=true</script></body></html>",
    );

    expect(srcDoc).toContain("<script>window.previewRan=true</script>");
    expect(srcDoc).toContain("installPreviewStorage");
    expect(srcDoc).not.toContain("Content-Security-Policy");
    expect(srcDoc).not.toContain("script-src 'none'");
    expect(srcDoc.indexOf("installPreviewStorage")).toBeLessThan(
      srcDoc.indexOf("<title>x</title>"),
    );
  });

  it("wraps HTML fragments without adding a CSP-bearing document shell", () => {
    const srcDoc = createSandboxedHtmlPreviewSrcDoc("<div>Hello</div>");

    expect(srcDoc.startsWith("<!doctype html>")).toBe(true);
    expect(srcDoc).toContain("installPreviewStorage");
    expect(srcDoc).toContain(`<body><div>Hello</div></body>`);
    expect(srcDoc).not.toContain("Content-Security-Policy");
  });

  it("caps large previews and adds a visible truncation notice", () => {
    const srcDoc = createSandboxedHtmlPreviewSrcDoc(
      `<div>${"x".repeat(HTML_PREVIEW_LIMITS.maxSrcDocChars + 100)}</div>`,
    );

    expect(srcDoc.length).toBeLessThanOrEqual(
      HTML_PREVIEW_LIMITS.maxSrcDocChars,
    );
    expect(srcDoc).toContain("Preview truncated");
  });

  it("builds a nonce-constrained, responsive visual sandbox document", () => {
    const srcDoc = createSandboxedHtmlVisualSrcDoc(
      '<div style="position:relative;max-width:960px;aspect-ratio:16/9">Poster</div><script>bad()</script>',
    );

    expect(srcDoc).toContain("Content-Security-Policy");
    expect(srcDoc).toContain("default-src 'none'");
    expect(srcDoc).toContain("style-src 'unsafe-inline'");
    expect(srcDoc).toContain("script-src 'nonce-");
    expect(srcDoc).toContain("width:960px;height:540px");
    expect(srcDoc).toContain("Math.min(1,innerWidth/960,innerHeight/540)");
    expect(srcDoc).toContain('addEventListener("resize",fit');
    expect(srcDoc).not.toContain("installPreviewStorage");
  });

  it("derives non-default canvas dimensions before fitting", () => {
    const srcDoc = createSandboxedHtmlVisualSrcDoc(
      '<section style="max-width:1200px;aspect-ratio:4/3">Visual</section>',
    );

    expect(srcDoc).toContain("width:1200px;height:900px");
    expect(srcDoc).toContain("innerWidth/1200,innerHeight/900");
  });
});
