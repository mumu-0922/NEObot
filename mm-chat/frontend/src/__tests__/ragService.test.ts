import { afterEach, describe, expect, it, vi } from "vitest";

describe("legacy RAG service removal", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("fails closed without calling removed Next RAG routes", async () => {
    const { deleteFromRAG, queryRAG, upsertToRAG } =
      await import("../services/api/ragService");
    const fetchMock = vi.spyOn(globalThis, "fetch");

    await expect(queryRAG("hello", "collection")).resolves.toEqual([]);
    await expect(
      upsertToRAG([{ id: "item", data: "chunk" }], "collection"),
    ).resolves.toBe(false);
    await expect(deleteFromRAG(["item"], "collection")).resolves.toBe(false);

    expect(fetchMock).not.toHaveBeenCalled();
  });
});
