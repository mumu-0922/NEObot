import { readdirSync, statSync } from "node:fs";
import { join, relative, sep } from "node:path";
import { describe, expect, it } from "vitest";

const expectedTransitionalRoutes = [
  "/api/access/verify",
  "/api/chat",
  "/api/chat/execute-code",
  "/api/chat/generate",
  "/api/chat/generate-image",
  "/api/chat/generate-title",
  "/api/chat/related-questions",
  "/api/health",
  "/api/voice/synthesize",
  "/api/voice/transcribe",
] as const;

const removedG92Routes = [
  "/api/chat/rag-queries",
  "/api/doc-parse",
  "/api/doc-parse/jobs/[id]",
  "/api/rag/delete",
  "/api/rag/query",
  "/api/rag/upsert",
] as const;

const removedG93Routes = [
  "/api/byok/public-key",
  "/api/config",
  "/api/providers/models",
] as const;

const removedG94Routes = [
  "/api/agents",
  "/api/agents/[identifier]",
  "/api/plugins/execute",
  "/api/plugins/install",
  "/api/plugins/list",
] as const;

const removedG119ERoutes = ["/api/search"] as const;

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

  it("keeps G9.2 RAG and document parsing handlers deleted", () => {
    const actualRoutes = new Set(
      collectRouteFiles(join(process.cwd(), "src/app/api")).map(toApiRoute),
    );

    for (const removedRoute of removedG92Routes) {
      expect(actualRoutes.has(removedRoute)).toBe(false);
    }
  });

  it("keeps G9.3 config, provider, and BYOK handlers deleted", () => {
    const actualRoutes = new Set(
      collectRouteFiles(join(process.cwd(), "src/app/api")).map(toApiRoute),
    );

    for (const removedRoute of removedG93Routes) {
      expect(actualRoutes.has(removedRoute)).toBe(false);
    }
  });

  it("keeps G9.4 plugin and agent handlers deleted", () => {
    const actualRoutes = new Set(
      collectRouteFiles(join(process.cwd(), "src/app/api")).map(toApiRoute),
    );

    for (const removedRoute of removedG94Routes) {
      expect(actualRoutes.has(removedRoute)).toBe(false);
    }
  });

  it("keeps the G11.9E legacy Search handler deleted", () => {
    const actualRoutes = new Set(
      collectRouteFiles(join(process.cwd(), "src/app/api")).map(toApiRoute),
    );

    for (const removedRoute of removedG119ERoutes) {
      expect(actualRoutes.has(removedRoute)).toBe(false);
    }
  });
});
