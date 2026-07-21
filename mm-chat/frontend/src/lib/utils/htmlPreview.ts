import { HTML_PREVIEW_LIMITS } from "../../config/limits";

export const HTML_PREVIEW_TRUNCATION_NOTICE =
  '\n<div style="position:fixed;left:16px;right:16px;bottom:16px;padding:10px 12px;border:1px solid #f59e0b;background:#fffbeb;color:#92400e;font:13px system-ui,sans-serif;z-index:2147483647">Preview truncated to fit local rendering limits.</div>';

const HTML_PREVIEW_BOOTSTRAP = `<script>
(() => {
  function createPreviewStorage() {
    const values = new Map();
    return {
      get length() {
        return values.size;
      },
      key(index) {
        return Array.from(values.keys())[index] ?? null;
      },
      getItem(key) {
        key = String(key);
        return values.has(key) ? values.get(key) : null;
      },
      setItem(key, value) {
        values.set(String(key), String(value));
      },
      removeItem(key) {
        values.delete(String(key));
      },
      clear() {
        values.clear();
      }
    };
  }

  function installPreviewStorage(name) {
    try {
      const storage = window[name];
      const probeKey = "__neo_preview_storage_probe__";
      storage.setItem(probeKey, probeKey);
      storage.removeItem(probeKey);
    } catch {
      Object.defineProperty(window, name, {
        configurable: true,
        value: createPreviewStorage()
      });
    }
  }

  installPreviewStorage("localStorage");
  installPreviewStorage("sessionStorage");
})();
</script>`;

const DEFAULT_HTML_VISUAL_CANVAS_WIDTH = 960;
const DEFAULT_HTML_VISUAL_CANVAS_HEIGHT = 540;

function clampVisualDimension(value: number, minimum: number): number {
  return Math.min(1920, Math.max(minimum, Math.round(value)));
}

function resolveHtmlVisualCanvas(rawHtml: string): {
  width: number;
  height: number;
} {
  const rootTag = rawHtml.match(
    /^\s*<(?:div|section|article|aside|main|details|table)\b[^>]*>/i,
  )?.[0];
  const rootStyleSource = rootTag || rawHtml;
  const widthMatch = rootStyleSource.match(
    /(?:max-width|width)\s*:\s*(\d+(?:\.\d+)?)px/i,
  );
  const width = clampVisualDimension(
    Number.parseFloat(widthMatch?.[1] || "") ||
      DEFAULT_HTML_VISUAL_CANVAS_WIDTH,
    320,
  );
  const ratioMatch = rootStyleSource.match(
    /aspect-ratio\s*:\s*(\d+(?:\.\d+)?)\s*\/\s*(\d+(?:\.\d+)?)/i,
  );
  const explicitHeightMatch = rootStyleSource.match(
    /height\s*:\s*(\d+(?:\.\d+)?)px/i,
  );
  const ratioWidth = Number.parseFloat(ratioMatch?.[1] || "");
  const ratioHeight = Number.parseFloat(ratioMatch?.[2] || "");
  const inferredHeight =
    ratioWidth > 0 && ratioHeight > 0
      ? (width * ratioHeight) / ratioWidth
      : Number.parseFloat(explicitHeightMatch?.[1] || "") ||
        DEFAULT_HTML_VISUAL_CANVAS_HEIGHT;
  return {
    width,
    height: clampVisualDimension(inferredHeight, 180),
  };
}

function createHtmlVisualNonce(): string {
  const bytes = new Uint8Array(16);
  globalThis.crypto.getRandomValues(bytes);
  return Array.from(bytes, (value) => value.toString(16).padStart(2, "0")).join(
    "",
  );
}

function createHtmlVisualSandboxHead(
  nonce: string,
  width: number,
  height: number,
): string {
  return `<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data: blob: https:; style-src 'unsafe-inline'; script-src 'nonce-${nonce}'"><style>html,body{margin:0;width:100%;height:100%;overflow:hidden;background:transparent}#neo-html-visual-stage{position:absolute;left:50%;top:0;width:${width}px;height:${height}px;transform-origin:top center}</style>`;
}

function createHtmlVisualFitScript(
  nonce: string,
  width: number,
  height: number,
): string {
  return `<script nonce="${nonce}">(()=>{const stage=document.getElementById("neo-html-visual-stage");if(!stage)return;const fit=()=>{const scale=Math.max(0.01,Math.min(1,innerWidth/${width},innerHeight/${height}));stage.style.transform="translateX(-50%) scale("+scale+")"};fit();addEventListener("resize",fit,{passive:true})})()</script>`;
}

function clampPreviewHtml(rawHtml: string, maxChars: number): string {
  if (rawHtml.length <= maxChars) return rawHtml;

  const notice =
    maxChars > HTML_PREVIEW_TRUNCATION_NOTICE.length
      ? HTML_PREVIEW_TRUNCATION_NOTICE
      : "";
  return rawHtml.slice(0, Math.max(0, maxChars - notice.length)) + notice;
}

function ensurePreviewDocument(html: string): string {
  if (/<head(?:\s[^>]*)?>/i.test(html)) {
    return html.replace(
      /<head(?:\s[^>]*)?>/i,
      (match) => `${match}\n${HTML_PREVIEW_BOOTSTRAP}`,
    );
  }

  if (/<html(?:\s[^>]*)?>/i.test(html)) {
    return html.replace(
      /<html(?:\s[^>]*)?>/i,
      (match) => `${match}\n<head>${HTML_PREVIEW_BOOTSTRAP}</head>`,
    );
  }

  return `<!doctype html><html><head>${HTML_PREVIEW_BOOTSTRAP}</head><body>${html}</body></html>`;
}

export function createSandboxedHtmlPreviewSrcDoc(
  rawHtml: string,
  maxChars: number = HTML_PREVIEW_LIMITS.maxSrcDocChars,
): string {
  const finalMaxChars = Math.max(0, Math.floor(maxChars));

  let html = clampPreviewHtml(rawHtml, finalMaxChars);
  let srcDoc = ensurePreviewDocument(html);
  if (srcDoc.length <= finalMaxChars) return srcDoc;

  const wrapperOverhead = Math.max(0, srcDoc.length - html.length);
  html = clampPreviewHtml(
    rawHtml,
    Math.max(0, finalMaxChars - wrapperOverhead),
  );
  srcDoc = ensurePreviewDocument(html);

  return srcDoc.length <= finalMaxChars
    ? srcDoc
    : srcDoc.slice(0, finalMaxChars);
}

export function createSandboxedHtmlVisualSrcDoc(
  rawHtml: string,
  maxChars: number = HTML_PREVIEW_LIMITS.maxSrcDocChars,
): string {
  const finalMaxChars = Math.max(0, Math.floor(maxChars));
  const { width, height } = resolveHtmlVisualCanvas(rawHtml);
  const nonce = createHtmlVisualNonce();
  const head = createHtmlVisualSandboxHead(nonce, width, height);
  const script = createHtmlVisualFitScript(nonce, width, height);
  const prefix = `<!doctype html><html><head>${head}</head><body><div id="neo-html-visual-stage">`;
  const bodySuffix = `</div>${script}`;
  const suffix = "</body></html>";
  const html = clampPreviewHtml(
    rawHtml,
    Math.max(
      0,
      finalMaxChars - prefix.length - bodySuffix.length - suffix.length,
    ),
  );
  return `${prefix}${html}${bodySuffix}${suffix}`.slice(0, finalMaxChars);
}
