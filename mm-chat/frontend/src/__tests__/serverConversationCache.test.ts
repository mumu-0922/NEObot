import { beforeEach, describe, expect, it } from "vitest";

import {
  clearServerConversationMemoryCache,
  getServerConversationMemorySnapshot,
  isServerConversationMemorySnapshotCurrent,
  SERVER_CONVERSATION_MEMORY_CACHE_LIMIT,
  setServerConversationMemorySnapshot,
} from "@/lib/chat/serverConversationCache";
import { normalizeSessionMessageTree } from "@/lib/chat/messageTree";
import type { Message } from "@/types";

const makeTree = (id: string, content = id) =>
  normalizeSessionMessageTree([
    {
      id,
      role: "user",
      content,
      timestamp: 1,
    } satisfies Message,
  ]);

describe("server conversation memory cache", () => {
  beforeEach(() => clearServerConversationMemoryCache());

  it("keeps a bounded least-recently-used snapshot set", () => {
    for (
      let index = 0;
      index < SERVER_CONVERSATION_MEMORY_CACHE_LIMIT;
      index++
    ) {
      setServerConversationMemorySnapshot(`c${index}`, makeTree(`m${index}`));
    }
    expect(getServerConversationMemorySnapshot("c0")).not.toBeNull();

    setServerConversationMemorySnapshot("overflow", makeTree("overflow"));

    expect(getServerConversationMemorySnapshot("c1")).toBeNull();
    expect(getServerConversationMemorySnapshot("c0")).not.toBeNull();
    expect(getServerConversationMemorySnapshot("overflow")).not.toBeNull();
  });

  it("detects whether background revalidation changed the tree", () => {
    const snapshot = setServerConversationMemorySnapshot("c1", makeTree("m1"));

    expect(
      isServerConversationMemorySnapshotCurrent(snapshot, makeTree("m1")),
    ).toBe(true);
    expect(
      isServerConversationMemorySnapshotCurrent(
        snapshot,
        makeTree("m1", "changed"),
      ),
    ).toBe(false);
  });
});
