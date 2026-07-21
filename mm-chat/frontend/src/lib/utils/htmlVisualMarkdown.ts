const MARKDOWN_LITERAL_FRAGMENT_RE =
  /(```[\s\S]*?```|~~~[\s\S]*?~~~|`[^`\n]*`)/g;
const HTML_TAG_RE = /<\/?[A-Za-z][^>\n]*>/g;
const HTML_VISUAL_MARKDOWN_FENCE_RE =
  /(^|\n)([ \t]{0,3})(```|~~~)[ \t]*(?:(?:markdown|md)\b[^\n]*)?\n([\s\S]*?)\n[ \t]*\3[ \t]*(?=\n|$)/gi;
const HTML_VISUAL_FRAGMENT_RE =
  /^\s*<(div|section|article|aside|main|details|table)\b[\s\S]*<\/\1>\s*$/i;
const HTML_VISUAL_STYLE_RE = /\sstyle\s*=\s*["'][^"']{8,}["']/i;
const HTML_VISUAL_ROOT_START_RE =
  /<(?:div|section|article|aside|main|details|table)\b(?=[^>]*\sstyle\s*=)[^>]*>/gi;
const HTML_TAG_TOKEN_RE = /<\s*(\/?)\s*([A-Za-z][\w:-]*)\b[^>]*>/g;
const SANDBOX_ONLY_HTML_VISUAL_STYLE_RE =
  /(?:^|[;"'])\s*(?:position|inset|inset-block|inset-inline|top|right|bottom|left|transform|aspect-ratio)\s*:/im;
const HTML_VOID_TAGS = new Set([
  "area",
  "base",
  "br",
  "col",
  "embed",
  "hr",
  "img",
  "input",
  "link",
  "meta",
  "param",
  "source",
  "track",
  "wbr",
]);
const UNSAFE_HTML_VISUAL_RE =
  /<\s*(?:script|style|iframe|object|embed|form|input|textarea)\b|\s(?:class|on[a-z]+)\s*=|javascript:|url\s*\(|@import|expression\s*\(/i;

function isMarkdownLiteralFragment(fragment: string): boolean {
  return (
    fragment.startsWith("```") ||
    fragment.startsWith("~~~") ||
    fragment.startsWith("`")
  );
}

function mapMarkdownTextFragments(
  source: string,
  transform: (fragment: string) => string,
): string {
  return source
    .split(MARKDOWN_LITERAL_FRAGMENT_RE)
    .map((fragment) => {
      if (!fragment || isMarkdownLiteralFragment(fragment)) {
        return fragment;
      }
      return transform(fragment);
    })
    .join("");
}

function normalizeEscapedHtmlAttributeQuotesInText(source: string): string {
  if (!source.includes('\\"') && !source.includes("\\'")) {
    return source;
  }

  return source.replace(HTML_TAG_RE, (tag) =>
    tag.replace(/\\"/g, '"').replace(/\\'/g, "'"),
  );
}

function isSafeHtmlVisualFragment(source: string): boolean {
  return (
    HTML_VISUAL_FRAGMENT_RE.test(source) &&
    HTML_VISUAL_STYLE_RE.test(source) &&
    !UNSAFE_HTML_VISUAL_RE.test(source)
  );
}

function findBalancedHtmlFragmentEnd(
  source: string,
  startIndex: number,
): number | undefined {
  const stack: string[] = [];
  HTML_TAG_TOKEN_RE.lastIndex = startIndex;

  for (
    let match = HTML_TAG_TOKEN_RE.exec(source);
    match;
    match = HTML_TAG_TOKEN_RE.exec(source)
  ) {
    const [token, closingMarker, rawName] = match;
    const tagName = rawName.toLowerCase();
    if (closingMarker) {
      if (stack.pop() !== tagName) return undefined;
      if (stack.length === 0) return match.index + token.length;
      continue;
    }
    if (!HTML_VOID_TAGS.has(tagName) && !/\/\s*>$/.test(token)) {
      stack.push(tagName);
    }
  }

  return undefined;
}

function normalizeCompleteHtmlVisualFragments(source: string): string {
  if (!HTML_VISUAL_STYLE_RE.test(source)) return source;

  const rootPattern = new RegExp(HTML_VISUAL_ROOT_START_RE.source, "gi");
  let cursor = 0;
  let normalized = "";

  for (
    let match = rootPattern.exec(source);
    match;
    match = rootPattern.exec(source)
  ) {
    if (match.index < cursor) continue;
    const endIndex = findBalancedHtmlFragmentEnd(source, match.index);
    if (endIndex === undefined) {
      normalized += source.slice(cursor, match.index);
      normalized += wrapHtmlVisualAsPlainCode(source.slice(match.index));
      cursor = source.length;
      break;
    }

    const candidate = source.slice(match.index, endIndex);
    if (!isSafeHtmlVisualFragment(candidate)) continue;

    normalized += source.slice(cursor, match.index);
    normalized += SANDBOX_ONLY_HTML_VISUAL_STYLE_RE.test(candidate)
      ? wrapHtmlVisualAsCode(candidate)
      : candidate.replace(/\n[ \t]*\n+/g, "\n");
    cursor = endIndex;
    rootPattern.lastIndex = endIndex;
  }

  return cursor === 0 ? source : normalized + source.slice(cursor);
}

function wrapHtmlVisualAsCode(source: string): string {
  let fence = "```";
  while (source.includes(fence)) fence += "`";
  return `${fence}htmlvisual\n${source}\n${fence}`;
}

function wrapHtmlVisualAsPlainCode(source: string): string {
  let fence = "```";
  while (source.includes(fence)) fence += "`";
  return `${fence}\n${source}\n${fence}`;
}

function normalizeHtmlVisualMarkdownFences(source: string): string {
  if (!source.includes("```") && !source.includes("~~~")) {
    return source;
  }

  return source.replace(
    HTML_VISUAL_MARKDOWN_FENCE_RE,
    (
      match: string,
      prefix: string,
      _indent: string,
      _fence: string,
      code: string,
    ) => {
      const normalizedCode = normalizeEscapedHtmlAttributeQuotesInText(
        code.trim(),
      );
      if (!isSafeHtmlVisualFragment(normalizedCode)) {
        return match;
      }
      return `${prefix}${normalizedCode}`;
    },
  );
}

export function normalizeHtmlVisualMarkdown(source: string): string {
  const withVisualFences = normalizeHtmlVisualMarkdownFences(source);
  const withNormalizedQuotes = mapMarkdownTextFragments(
    withVisualFences,
    normalizeEscapedHtmlAttributeQuotesInText,
  );
  return mapMarkdownTextFragments(
    withNormalizedQuotes,
    normalizeCompleteHtmlVisualFragments,
  );
}
