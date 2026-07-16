import { readdirSync, statSync } from "node:fs";
import { join, relative, sep } from "node:path";
import { describe, expect, it } from "vitest";

const expectedTransitionalRoutes = [
  "/api/access/verify",
  "/api/agents",
  "/api/agents/[identifier]",
  "/api/byok/public-key",
  "/api/chat",
  "/api/chat/execute-code",
  "/api/chat/generate",
  "/api/chat/generate-image",
  "/api/chat/generate-title",
  "/api/chat/rag-queries",
  "/api/chat/related-questions",
  "/api/config",
  "/api/doc-parse",
  "/api/doc-parse/jobs/[id]",
  "/api/health",
  "/api/plugins/execute",
  "/api/plugins/install",
  "/api/plugins/list",
  "/api/providers/models",
  "/api/rag/delete",
  "/api/rag/query",
  "/api/rag/upsert",
  "/api/search",
  "/api/voice/synthesize",
  "/api/voice/transcribe",
] as const;

function collectRouteFiles(directory: string): string[] {
  const entries = readdirSync(directory).sort();
  const routes: string[] = [];

  for (const entry of entries) {
    const path = join(directory, entry);
    const stats = statSync(path);

    if (stats.isDirectory()) {
      routes.push(...collectRouteFiles(path));
      continue;
    }

    if (entry === "route.ts") {
      routes.push(path);
    }
  }

  return routes;
}

function toApiRoute(filePath: string): string {
  const relativePath = relative(join(process.cwd(), "src/app/api"), filePath);
  const routeSegments = relativePath.split(sep).slice(0, -1);
  return `/api/${routeSegments.join("/")}`;
}

describe("G9.1 transitional Next API route inventory", () => {
  it("freezes the currently registered src/app/api route handlers", () => {
    const actualRoutes = collectRouteFiles(join(process.cwd(), "src/app/api"))
      .map(toApiRoute)
      .sort();

    expect(actualRoutes).toEqual([...expectedTransitionalRoutes].sort());
  });
});
