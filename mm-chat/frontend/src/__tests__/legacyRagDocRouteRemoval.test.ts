import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it, vi } from "vitest";

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
  it("keeps frontend services and API config from calling deleted Next routes", () => {
    const checkedSources = [
      "src/config/api.ts",
      "src/services/api/chatService.ts",
      "src/services/api/docParseService.ts",
      "src/services/api/ragService.ts",
      "src/lib/data/clearAppData.ts",
    ].map(readSource);

    for (const source of checkedSources) {
      for (const route of removedRoutes) {
        expect(source).not.toContain(route);
      }
    }
  });

  it("makes local RAG and document parsing services fail closed", async () => {
    const { parseDocumentFile } =
      await import("../services/api/docParseService");
    const { deleteFromRAG, queryRAG, upsertToRAG } =
      await import("../services/api/ragService");
    const fetchSpy = vi.spyOn(globalThis, "fetch");

    await expect(queryRAG("hello", "collection")).resolves.toEqual([]);
    await expect(
      upsertToRAG([{ id: "chunk", data: "body" }], "collection"),
    ).resolves.toBe(false);
    await expect(deleteFromRAG(["chunk"], "collection")).resolves.toBe(false);
    await expect(
      parseDocumentFile(new File(["pdf"], "scan.pdf"), {
        provider: "mineru",
        useDefault: true,
      }),
    ).rejects.toThrow(/server Knowledge/i);

    expect(fetchSpy).not.toHaveBeenCalled();
  });
});
