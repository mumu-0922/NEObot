export function normalizeReadableText(value: string): string {
  return value
    .replaceAll("\u00a0", " ")
    .replaceAll("\u200b", "")
    .replaceAll("\r", "\n")
    .split("\n")
    .map((line) => line.trim().replace(/\s+/gu, " "))
    .filter(Boolean)
    .join("\n");
}

export function getReadableTextFromElement(
  element: HTMLElement | null,
): string {
  return normalizeReadableText(element?.innerText ?? "");
}
