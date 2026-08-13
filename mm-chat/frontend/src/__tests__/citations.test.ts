import { describe, expect, it } from "vitest";
import {
  createCitationHref,
  linkifyCitationReferences,
} from "../lib/utils/citations";
import type { Source } from "../types";

describe("citation utilities", () => {
  it("creates internal citation hrefs", () => {
    expect(createCitationHref(2)).toBe("#citation-2");
  });

  it("linkifies citation references without interpolating raw source URLs", () => {
    const sources: Source[] = [
      {
        title: "Unsafe",
        url: "https://example.com/a) injected [x](javascript:alert(1)",
        content: "content",
      },
    ];

    const output = linkifyCitationReferences(
      "Use [1] and [W1], keep [K1] and `[W1]` as-is.",
      sources,
    );

    expect(output).toBe(
      "Use [1](#citation-0) and [W1](#citation-0), keep [K1] and `[W1]` as-is.",
    );
    expect(output).not.toContain("example.com");
    expect(output).not.toContain("javascript:");
  });

  it("leaves missing citation references untouched", () => {
    expect(linkifyCitationReferences("Use [2].", [])).toBe("Use [2].");
  });

  it("links sparse Web markers to their authoritative source metadata", () => {
    const sources: Source[] = [
      {
        title: "Models overview",
        url: "https://docs.example.com/models",
        content: "models",
        metadata: { marker: "[W1]" },
      },
      {
        title: "Release notes",
        url: "https://docs.example.com/releases",
        content: "releases",
        metadata: { marker: "[W5]" },
      },
      {
        title: "Dates",
        url: "https://docs.example.com/dates",
        content: "dates",
        metadata: { marker: "[W7]" },
      },
      {
        title: "Pricing",
        url: "https://docs.example.com/pricing",
        content: "pricing",
        metadata: { marker: "[W10]" },
      },
    ];

    expect(
      linkifyCitationReferences("Sources [W1] [W5] [W7] [W10].", sources),
    ).toBe(
      "Sources [W1](#citation-0) [W5](#citation-1) [W7](#citation-2) [W10](#citation-3).",
    );
  });

  it("does not positionally mislink an unknown sparse Web marker", () => {
    const sources: Source[] = [
      {
        title: "Release notes",
        url: "https://docs.example.com/releases",
        content: "releases",
        metadata: { marker: "[W5]" },
      },
    ];

    expect(linkifyCitationReferences("Use [W1] and [W5].", sources)).toBe(
      "Use [W1] and [W5](#citation-0).",
    );
  });
});
