import { afterEach, describe, expect, it, vi } from "vitest";

import { createNeoChatApiClient } from "@/services/api/client";
import { toWorkspace } from "@/services/api/workspaceService";

const workspace = {
  id: "0198ca9a-81c6-7c8d-9444-b16da02de9b4",
  name: "neo-chat",
  systemPrompt: "Work here",
  files: [],
  color: "blue",
  enableSearch: true,
  enableReasoning: false,
  revision: 2,
  bindingStatus: "bound" as const,
  runnerId: "wsl-test-runner",
  canonicalPath: "/mnt/d/projects/neo-chat",
  displayPath: "D:\\projects\\neo-chat",
  pathKind: "windows-mounted" as const,
  directoryFingerprint: `sha256:${"a".repeat(64)}`,
  boundAt: "2026-08-21T08:00:00Z",
  createdAt: "2026-08-21T07:00:00Z",
  updatedAt: "2026-08-21T08:00:00Z",
};

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("Host Workspace API client", () => {
  it("normalizes list, Host status, and directory browse responses", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/v1/workspaces")) {
        return jsonResponse({ workspaces: [workspace] });
      }
      if (url.endsWith("/v1/workspaces/host-status")) {
        return jsonResponse({
          enabled: true,
          status: "ready",
          runnerId: "wsl-test-runner",
          platform: "linux-wsl",
          architecture: "amd64",
          features: {
            workspaceResolve: true,
            directoryBrowse: true,
            nativeDirectoryPicker: true,
            windowsPathInterop: true,
            execution: false,
            permissionModes: [],
          },
        });
      }
      return jsonResponse({
        path: "/home/user",
        displayPath: "/home/user",
        pathKind: "wsl",
        entries: [
          {
            name: "project",
            path: "/home/user/project",
            displayPath: "/home/user/project",
            pathKind: "wsl",
          },
        ],
      });
    });
    vi.stubGlobal("fetch", fetchMock);
    const api = createNeoChatApiClient({
      mode: "server",
      baseUrl: "/mm-api",
    }).workspaces!;

    await expect(api.list()).resolves.toEqual([workspace]);
    await expect(api.getHostStatus()).resolves.toMatchObject({
      status: "ready",
      features: { directoryBrowse: true, nativeDirectoryPicker: true },
    });
    await expect(api.browseDirectories({})).resolves.toMatchObject({
      path: "/home/user",
      entries: [{ name: "project" }],
    });

    const domain = toWorkspace(workspace);
    expect(domain).toMatchObject({
      bindingStatus: "bound",
      revision: 2,
      displayPath: "D:\\projects\\neo-chat",
    });
  });

  it("sends CAS updates and rejects malformed server Workspace data", async () => {
    const requests: Array<{ url: string; init?: RequestInit }> = [];
    const fetchMock = vi.fn(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        requests.push({ url: String(input), init });
        return jsonResponse({
          workspace:
            init?.method === "PATCH"
              ? workspace
              : { ...workspace, revision: 0 },
        });
      },
    );
    vi.stubGlobal("fetch", fetchMock);
    const api = createNeoChatApiClient({
      mode: "server",
      baseUrl: "/mm-api",
    }).workspaces!;

    await expect(
      api.update({
        workspaceId: workspace.id,
        expectedRevision: 2,
        settings: {
          name: workspace.name,
          systemPrompt: workspace.systemPrompt,
          files: [],
          color: workspace.color,
          enableSearch: true,
          enableReasoning: false,
        },
      }),
    ).resolves.toMatchObject({ revision: 2 });
    expect(JSON.parse(String(requests[0].init?.body))).toMatchObject({
      expectedRevision: 2,
      name: "neo-chat",
    });

    await expect(api.get(workspace.id)).rejects.toMatchObject({
      code: "INVALID_SERVER_RESPONSE",
    });
  });

  it("opens and downloads a normalized Workspace file through encoded routes", async () => {
    const requests: string[] = [];
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      requests.push(url);
      if (url.includes("/files/preview?")) {
        return jsonResponse({
          preview: {
            kind: "xlsx",
            fileName: "gold prices.xlsx",
            mimeType:
              "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
            size: 4096,
            version: `sha256:${"b".repeat(64)}`,
            truncated: false,
            sheets: [
              {
                name: "Prices",
                rows: [["Date", "Price"]],
                truncated: false,
              },
            ],
          },
        });
      }
      return new Response(new Uint8Array([0x50, 0x4b]), {
        headers: { "Content-Type": "application/octet-stream" },
      });
    });
    vi.stubGlobal("fetch", fetchMock);
    const api = createNeoChatApiClient({
      mode: "server",
      baseUrl: "/mm-api",
    }).workspaces!;

    await expect(
      api.previewFile({
        workspaceId: workspace.id,
        path: "reports/gold prices.xlsx",
      }),
    ).resolves.toMatchObject({
      kind: "xlsx",
      sheets: [{ name: "Prices", rows: [["Date", "Price"]] }],
    });
    await expect(
      api.readFile({
        workspaceId: workspace.id,
        path: "reports/gold prices.xlsx",
        download: true,
      }),
    ).resolves.toBeInstanceOf(Blob);
    expect(requests[0]).toContain(
      "/files/preview?path=reports%2Fgold+prices.xlsx",
    );
    expect(requests[1]).toContain(
      "/files/content?path=reports%2Fgold+prices.xlsx&download=true",
    );
  });
});

function jsonResponse(value: unknown, status = 200): Response {
  return new Response(JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}
