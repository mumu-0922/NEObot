import { describe, expect, it } from "vitest";

import {
  agentMarketCountKey,
  formatAgentMarketCategory,
  missingAgentMarketCountCategories,
} from "@/lib/market/agentCategory";

describe("assistant market category presentation", () => {
  it("localizes the upstream category identifiers", () => {
    expect(formatAgentMarketCategory("academic", "zh")).toBe("学术");
    expect(formatAgentMarketCategory("Copywriting", "zh")).toBe("文案");
    expect(formatAgentMarketCategory("marketing", "zh")).toBe("商业");
    expect(formatAgentMarketCategory("programming", "ja")).toBe(
      "プログラミング",
    );
    expect(formatAgentMarketCategory("career", "en")).toBe("Career");
  });

  it("keeps unknown future categories readable", () => {
    expect(formatAgentMarketCategory("new_category", "zh")).toBe(
      "New Category",
    );
  });

  it("isolates actual result counts by query and category", () => {
    expect(agentMarketCountKey("", "")).toBe("\u0000");
    expect(agentMarketCountKey(" writer ", "Academic")).toBe(
      "writer\u0000academic",
    );
    expect(agentMarketCountKey("writer", "academic")).not.toBe(
      agentMarketCountKey("", "academic"),
    );
  });

  it("prefetches only category counts that are not already known", () => {
    expect(
      missingAgentMarketCountCategories(
        "",
        ["academic", "career", "education"],
        {
          [agentMarketCountKey("", "academic")]: 78,
          [agentMarketCountKey("writer", "career")]: 2,
        },
      ),
    ).toEqual(["career", "education"]);
  });
});
