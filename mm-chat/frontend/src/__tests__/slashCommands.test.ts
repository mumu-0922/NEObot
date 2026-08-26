import { describe, expect, it } from "vitest";

import {
  RESOURCE_MANAGER_OPEN_EVENT,
  type ResourceManagerOpenDetail,
} from "../lib/chat/slashCommands";

describe("resource manager event contract", () => {
  it("keeps process-trace deep links without exposing lifecycle Slash commands", () => {
    const detail: ResourceManagerOpenDetail = {
      kind: "skill",
      query: "xlsx",
      resourceId: "candidate-id",
    };

    expect(RESOURCE_MANAGER_OPEN_EVENT).toBe("neo-chat:open-resource-manager");
    expect(detail).toEqual({
      kind: "skill",
      query: "xlsx",
      resourceId: "candidate-id",
    });
  });
});
