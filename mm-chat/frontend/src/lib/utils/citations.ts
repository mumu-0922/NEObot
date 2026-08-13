import type { Source } from "../../types";

export function createCitationHref(index: number): string {
  return `#citation-${index}`;
}

export function linkifyCitationReferences(
  content: string,
  sources: Source[] | undefined,
): string {
  if (!sources?.length) return content;

  const webMarkerIndexes = new Map<string, number>();
  sources.forEach((source, index) => {
    const marker = source.metadata?.marker;
    if (typeof marker === "string" && /^\[W\d+\]$/.test(marker)) {
      webMarkerIndexes.set(marker, index);
    }
  });
  const hasAuthoritativeWebMarkers = webMarkerIndexes.size > 0;

  const segments = content.split(/(`+[^`]+`+)/g);
  return segments
    .map((segment, segmentIndex) => {
      if (segmentIndex % 2 === 1) return segment;

      return segment.replace(/\[(W?)(\d+)\]/g, (match, prefix, value) => {
        const positionalIndex = Number.parseInt(value, 10) - 1;
        const sourceIndex =
          prefix === "W" && hasAuthoritativeWebMarkers
            ? webMarkerIndexes.get(match)
            : positionalIndex;
        if (sourceIndex === undefined) return match;
        return sources[sourceIndex]
          ? `[${prefix}${value}](${createCitationHref(sourceIndex)})`
          : match;
      });
    })
    .join("");
}
