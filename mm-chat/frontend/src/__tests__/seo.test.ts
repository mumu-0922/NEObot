import { existsSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";

import en from "../i18n/locales/en";
import ja from "../i18n/locales/ja";
import zh from "../i18n/locales/zh";
import * as seo from "../lib/seo";

describe("NeoBot brand", () => {
  it("uses the external brand across metadata and localized product copy", () => {
    expect(seo.SITE_NAME).toBe("NeoBot");
    expect(seo.buildWebApplicationJsonLd("en")).toMatchObject({
      name: "NeoBot",
      alternateName: ["Neo", "NeoBot AI"],
    });

    for (const messages of [en, zh, ja]) {
      const brandedMessages = {
        AccessPassword: messages.AccessPassword,
        AccountSecurity: messages.AccountSecurity,
        ChatApp: messages.ChatApp,
        Mcp: messages.Mcp,
        Sidebar: messages.Sidebar,
      };

      expect(messages.ChatApp.productName).toBe("NeoBot");
      expect(JSON.stringify(brandedMessages)).toContain("NeoBot");
      expect(JSON.stringify(brandedMessages)).not.toContain("Neo Chat");
    }
  });
});

describe("SEO screenshot assets", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it("defines desktop and mobile screenshots for SEO surfaces", () => {
    expect((seo as { SEO_SCREENSHOTS?: unknown }).SEO_SCREENSHOTS).toEqual([
      {
        src: "/neobot-agent-workspace.png",
        sizes: "2880x1800",
        type: "image/png",
        form_factor: "wide",
        label: "NeoBot desktop workspace screenshot",
      },
      {
        src: "/neobot-mobile-rag.png",
        sizes: "860x1440",
        type: "image/png",
        form_factor: "narrow",
        label: "NeoBot mobile workspace screenshot",
      },
    ]);
  });

  it("keeps declared screenshot sizes aligned with the PNG assets", () => {
    for (const screenshot of seo.SEO_SCREENSHOTS) {
      const png = readFileSync(
        resolve(process.cwd(), "public", screenshot.src.slice(1)),
      );
      const actualSize = `${png.readUInt32BE(16)}x${png.readUInt32BE(20)}`;

      expect(actualSize).toBe(screenshot.sizes);
    }
  });

  it("uses screenshots as structured data images", () => {
    expect(seo.buildWebApplicationJsonLd("en").image).toEqual([
      "http://localhost:3000/neobot-agent-workspace.png",
      "http://localhost:3000/neobot-mobile-rag.png",
    ]);
  });

  it("keeps Japanese metadata and structured data in Japanese", () => {
    expect(seo.normalizeSeoLocale("ja")).toBe("ja");
    expect(seo.getSeoContent("ja")).toMatchObject({
      openGraphLocale: "ja_JP",
      structuredDataLanguage: "ja-JP",
    });
    expect(seo.getSeoContent("ja").title).toContain("ローカル優先");
    expect(seo.buildWebApplicationJsonLd("ja")).toMatchObject({
      inLanguage: "ja-JP",
    });
  });

  it("builds screenshot URLs from NEXT_PUBLIC_SITE_URL", () => {
    vi.stubEnv("NEXT_PUBLIC_SITE_URL", "https://chat.example.com/");

    expect(seo.getSeoScreenshotUrls()).toEqual([
      "https://chat.example.com/neobot-agent-workspace.png",
      "https://chat.example.com/neobot-mobile-rag.png",
    ]);
    expect(seo.getSeoOpenGraphImages("NeoBot")[0]).toMatchObject({
      url: "https://chat.example.com/neobot-agent-workspace.png",
      width: 2880,
      height: 1800,
      alt: "NeoBot",
    });
  });

  it("uses static screenshot assets instead of a dynamic Open Graph image route", () => {
    expect(
      existsSync(resolve(process.cwd(), "src/app/opengraph-image.tsx")),
    ).toBe(false);
    expect(
      seo.getSeoOpenGraphImages("NeoBot").map((image) => image.url),
    ).toEqual([
      "http://localhost:3000/neobot-agent-workspace.png",
      "http://localhost:3000/neobot-mobile-rag.png",
    ]);
  });
});
