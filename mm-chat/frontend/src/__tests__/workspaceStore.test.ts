import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Workspace } from "@/types";
import { useChatStore } from "@/store/core/chatStore";

const serverWorkspace = workspaceDTO({
  id: "0198ca9a-81c6-7c8d-9444-b16da02de9b4",
  name: "Server authority",
  revision: 4,
});
const localWorkspace: Workspace = {
  id: "0198ca9a-81c6-7c8d-9444-b16da02de9b5",
  name: "Legacy local",
  files: [],
  color: "green",
  enableSearch: true,
  enableReasoning: false,
  createdAt: Date.parse("2026-08-21T07:00:00Z"),
};

beforeEach(() => {
  vi.stubEnv("NEXT_PUBLIC_API_MODE", "server");
  vi.stubEnv("NEXT_PUBLIC_API_BASE_URL", "/mm-api");
  useChatStore.setState({ workspaces: [] });
});

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
  useChatStore.setState({ workspaces: [] });
});

describe("Workspace server convergence", () => {
  it("imports only missing legacy records and then adopts server authority", async () => {
    let listCount = 0;
    const calls: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const method = init?.method ?? "GET";
        calls.push(`${method} ${String(input)}`);
        if (method === "PUT") {
          return jsonResponse({
            workspace: workspaceDTO({
              id: localWorkspace.id,
              name: localWorkspace.name,
              revision: 1,
            }),
          });
        }
        listCount += 1;
        return jsonResponse({
          workspaces:
            listCount === 1
              ? [serverWorkspace]
              : [
                  serverWorkspace,
                  workspaceDTO({
                    id: localWorkspace.id,
                    name: localWorkspace.name,
                    revision: 1,
                  }),
                ],
        });
      }),
    );
    useChatStore.setState({
      workspaces: [
        {
          ...localWorkspace,
          id: serverWorkspace.id,
          name: "Stale browser copy",
        },
        localWorkspace,
      ],
    });

    await expect(
      useChatStore.getState().refreshServerWorkspaces(),
    ).resolves.toBe(true);

    expect(calls.filter((call) => call.startsWith("PUT"))).toHaveLength(1);
    expect(calls.find((call) => call.startsWith("PUT"))).toContain(
      encodeURIComponent(localWorkspace.id),
    );
    expect(useChatStore.getState().workspaces).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: serverWorkspace.id,
          name: "Server authority",
          revision: 4,
        }),
        expect.objectContaining({ id: localWorkspace.id, revision: 1 }),
      ]),
    );
  });

  it("refreshes the authoritative record after a revision conflict", async () => {
    const current = {
      ...localWorkspace,
      id: serverWorkspace.id,
      revision: 3,
      bindingStatus: "unbound" as const,
    };
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
        if (init?.method === "PATCH") {
          return jsonResponse(
            {
              error: {
                code: "WORKSPACE_REVISION_CONFLICT",
                message: "Workspace changed; reload and retry",
              },
            },
            409,
          );
        }
        return jsonResponse({ workspaces: [serverWorkspace] });
      }),
    );
    useChatStore.setState({ workspaces: [current] });

    await expect(
      useChatStore
        .getState()
        .updateWorkspace(current.id, { name: "Stale overwrite" }),
    ).rejects.toMatchObject({ code: "WORKSPACE_REVISION_CONFLICT" });
    expect(useChatStore.getState().workspaces[0]).toMatchObject({
      name: "Server authority",
      revision: 4,
    });
  });
});

function workspaceDTO(input: { id: string; name: string; revision: number }) {
  return {
    id: input.id,
    name: input.name,
    systemPrompt: "",
    files: [],
    color: "blue",
    enableSearch: false,
    enableReasoning: false,
    revision: input.revision,
    bindingStatus: "unbound",
    createdAt: "2026-08-21T07:00:00Z",
    updatedAt: "2026-08-21T08:00:00Z",
  };
}

function jsonResponse(value: unknown, status = 200): Response {
  return new Response(JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}
