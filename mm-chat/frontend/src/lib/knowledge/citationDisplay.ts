import type { KnowledgeCitation } from "./types";

const maxLegacyLabelCharacters = 128;
const maxLegacyCoordinate = 1_000_000_000;
const opaqueHashPattern = /^[a-f0-9]{64}$/i;
const legacyCellPattern = /^[A-Z]{1,4}[1-9][0-9]{0,9}$/;

export interface KnowledgeCitationLocationLabels {
  page: (page: number) => string;
  slide: (slide: number) => string;
  cell: (cell: string) => string;
  cellRange: (startCell: string, endCell: string) => string;
  line: (line: number) => string;
  lineRange: (startLine: number, endLine: number) => string;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}

function legacyCoordinate(value: unknown): number | undefined {
  return typeof value === "number" &&
    Number.isSafeInteger(value) &&
    value > 0 &&
    value <= maxLegacyCoordinate
    ? value
    : undefined;
}

function legacyLabel(value: unknown): string | undefined {
  if (typeof value !== "string") return undefined;
  const label = value.trim();
  if (
    !label ||
    Array.from(label).length > maxLegacyLabelCharacters ||
    /[\u0000-\u001f\u007f]/.test(label) ||
    opaqueHashPattern.test(label)
  ) {
    return undefined;
  }
  return label;
}

function legacyCell(value: unknown): string | undefined {
  if (typeof value !== "string") return undefined;
  const cell = value.trim().toUpperCase();
  return legacyCellPattern.test(cell) ? cell : undefined;
}

export function formatKnowledgeCitationLocation(
  citation: KnowledgeCitation,
  labels: KnowledgeCitationLocationLabels,
): string | null {
  const display = citation.displayLocator;
  if (display) {
    switch (display.kind) {
      case "page":
        return labels.page(display.page);
      case "slide":
        return labels.slide(display.slide);
      case "cell_range":
        return display.startCell === display.endCell
          ? labels.cell(display.startCell)
          : labels.cellRange(display.startCell, display.endCell);
      case "line_range":
        return display.startLine === display.endLine
          ? labels.line(display.startLine)
          : labels.lineRange(display.startLine, display.endLine);
    }
  }

  if (!isRecord(citation.locator)) return null;
  const legacyPage = legacyCoordinate(citation.locator.page);
  if (legacyPage) return labels.page(legacyPage);

  const legacySheet = legacyLabel(citation.locator.sheet);
  const cell = legacyCell(citation.locator.cell);
  if (legacySheet && cell) return `${legacySheet} · ${cell}`;
  if (legacySheet) return legacySheet;
  if (cell) return labels.cell(cell);

  return legacyLabel(citation.locator.section) ?? null;
}

export function formatKnowledgeCitationTitle(
  citation: KnowledgeCitation,
  sourceFallback: string,
  labels: KnowledgeCitationLocationLabels,
): string {
  const source = citation.sourceName ?? sourceFallback;
  const location = formatKnowledgeCitationLocation(citation, labels);
  return location
    ? `${citation.marker} · ${source} · ${location}`
    : `${citation.marker} · ${source}`;
}
