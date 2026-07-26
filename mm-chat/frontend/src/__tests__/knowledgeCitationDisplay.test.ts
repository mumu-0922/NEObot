import { describe, expect, it } from "vitest";
import en from "@/i18n/locales/en";
import ja from "@/i18n/locales/ja";
import zh from "@/i18n/locales/zh";
import {
  formatKnowledgeCitationLocation,
  formatKnowledgeCitationTitle,
  type KnowledgeCitationLocationLabels,
} from "@/lib/knowledge/citationDisplay";
import type { KnowledgeCitation } from "@/types";

const labels: KnowledgeCitationLocationLabels = {
  page: (page) => `page:${page}`,
  slide: (slide) => `slide:${slide}`,
  cell: (cell) => `cell:${cell}`,
  cellRange: (startCell, endCell) => `cells:${startCell}-${endCell}`,
  line: (line) => `line:${line}`,
  lineRange: (startLine, endLine) => `lines:${startLine}-${endLine}`,
};

function citation(input: Partial<KnowledgeCitation> = {}): KnowledgeCitation {
  return {
    id: "cit_1",
    marker: "[K1]",
    snippet: "evidence",
    ...input,
  };
}

describe("Knowledge Citation display", () => {
  it("ships source and locator labels for every supported locale", () => {
    for (const locale of [en, zh, ja]) {
      expect(locale.Knowledge.citationSourceFallback).toBeTruthy();
      expect(locale.Knowledge.citationPage).toContain("{page}");
      expect(locale.Knowledge.citationSlide).toContain("{slide}");
      expect(locale.Knowledge.citationCellRange).toContain("{startCell}");
      expect(locale.Knowledge.citationLineRange).toContain("{startLine}");
    }
  });

  it.each([
    [{ kind: "page", page: 3 } as const, "page:3"],
    [{ kind: "slide", slide: 6 } as const, "slide:6"],
    [
      { kind: "cell_range", startCell: "A3", endCell: "C12" } as const,
      "cells:A3-C12",
    ],
    [
      { kind: "cell_range", startCell: "B2", endCell: "B2" } as const,
      "cell:B2",
    ],
    [
      { kind: "line_range", startLine: 18, endLine: 35 } as const,
      "lines:18-35",
    ],
    [{ kind: "line_range", startLine: 8, endLine: 8 } as const, "line:8"],
  ])("formats a normalized %s locator", (displayLocator, expected) => {
    expect(
      formatKnowledgeCitationLocation(citation({ displayLocator }), labels),
    ).toBe(expected);
  });

  it("keeps bounded human-readable legacy locators", () => {
    expect(
      formatKnowledgeCitationLocation(
        citation({ locator: { sheet: "Summary", cell: "b4" } }),
        labels,
      ),
    ).toBe("Summary · B4");
    expect(
      formatKnowledgeCitationLocation(
        citation({ locator: { section: "Install › Docker" } }),
        labels,
      ),
    ).toBe("Install › Docker");
  });

  it("never serializes current, opaque, malformed, or unknown raw locators", () => {
    const rawLocators = [
      {
        schemaVersion: "g7.4-locator-summary.v1",
        primary: {
          kind: "line_range",
          locator: { kind: "line_range", startLine: 0, endLine: 10 },
        },
        fragments: [{ private: "raw" }],
      },
      {
        sheet:
          "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      },
      { unknown: { nested: true } },
      null,
    ];

    for (const locator of rawLocators) {
      expect(
        formatKnowledgeCitationLocation(citation({ locator }), labels),
      ).toBeNull();
    }
  });

  it("uses a friendly source name and a generic legacy fallback without ids", () => {
    expect(
      formatKnowledgeCitationTitle(
        citation({
          sourceName: "rag-eval-pdf-zh-01.pdf",
          displayLocator: { kind: "page", page: 3 },
        }),
        "Knowledge source",
        labels,
      ),
    ).toBe("[K1] · rag-eval-pdf-zh-01.pdf · page:3");

    const legacyTitle = formatKnowledgeCitationTitle(
      citation({
        documentId: "e7845c02-0000-4000-8000-000000001477",
        locator: { fragments: [{ private: "raw" }] },
      }),
      "Knowledge source",
      labels,
    );
    expect(legacyTitle).toBe("[K1] · Knowledge source");
    expect(legacyTitle).not.toContain("e7845c02");
    expect(legacyTitle).not.toContain("fragments");
  });
});
