import { existsSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const removedRoutes = [
  "/api/chat/rag-queries",
  "/api/doc-parse",
  "/api/rag/delete",
  "/api/rag/query",
  "/api/rag/upsert",
] as const;

function readSource(path: string): string {
  return readFileSync(resolve(process.cwd(), path), "utf8");
}

describe("G9.2 legacy RAG/doc-parse route removal", () => {
  it("keeps retained frontend code from calling deleted Next routes", () => {
    const checkedSources = [
      "src/config/api.ts",
      "src/services/api/chatService.ts",
      "src/lib/data/clearAppData.ts",
    ].map(readSource);

    for (const source of checkedSources) {
      for (const route of removedRoutes) {
        expect(source).not.toContain(route);
      }
    }
  });

  it("removes the local RAG and document parsing service entrypoints", () => {
    for (const path of [
      "src/services/api/docParseService.ts",
      "src/services/api/ragService.ts",
      "src/lib/api/docParseJobs.ts",
      "src/lib/utils/rag.ts",
      "src/lib/utils/knowledgeFiles.ts",
      "src/lib/utils/knowledgeVectors.ts",
      "src/utils/textSplitter.ts",
    ]) {
      expect(existsSync(resolve(process.cwd(), path))).toBe(false);
    }
  });

  it("removes retired browser request schemas and provider constants", () => {
    const retainedSources = [
      "src/config/api.ts",
      "src/lib/api/schemas.ts",
      "src/lib/byok/shared.ts",
      "src/lib/security/urlPolicy.ts",
    ].map(readSource);

    for (const source of retainedSources) {
      for (const retired of [
        "DocumentParseSchema",
        "RAGQuerySchema",
        "RAGUpsertSchema",
        "llamaParse",
        '"docs"',
      ]) {
        expect(source).not.toContain(retired);
      }
    }
  });
});
