import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import { getWebSearchDegradationReason } from "@/lib/search/fusion";

describe("source fusion presentation", () => {
  it("admits only stable Web degradation reasons", () => {
    expect(
      getWebSearchDegradationReason({
        fusion: {
          version: "source-fusion/v1",
          searchRequested: true,
          degradationReason: "provider_failed",
        },
      }),
    ).toBe("provider_failed");
    expect(
      getWebSearchDegradationReason({
        fusion: {
          version: "source-fusion/v1",
          searchRequested: true,
          degradationReason: "raw upstream credential detail",
        },
      }),
    ).toBeUndefined();
    expect(
      getWebSearchDegradationReason({
        fusion: {
          version: "source-fusion/v1",
          searchRequested: false,
          degradationReason: "provider_failed",
        },
      }),
    ).toBeUndefined();
  });

  it("keeps Knowledge, Web degradation, and Web citations as separate compact blocks", () => {
    const messageItem = readFileSync(
      resolve(process.cwd(), "src/components/chat/MessageItem.tsx"),
      "utf8",
    );
    const knowledgeIndex = messageItem.indexOf("<KnowledgeEvidenceBlock");
    const degradationIndex = messageItem.indexOf("<SourceFusionNotice");
    const outputIndex = messageItem.indexOf("<MessageOutputRenderer");

    expect(knowledgeIndex).toBeGreaterThan(-1);
    expect(degradationIndex).toBeGreaterThan(knowledgeIndex);
    expect(outputIndex).toBeGreaterThan(degradationIndex);
  });
});
