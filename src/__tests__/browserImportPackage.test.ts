import { unzipSync } from "fflate";
import { describe, expect, it } from "vitest";
import { createAppExportPayload } from "../lib/data/appExport";
import { createBrowserImportPackageFromSnapshot } from "../lib/data/browserImportPackage";
import type { Message, Session } from "../types";

const fixedNow = new Date("2026-07-09T00:00:00.000Z");

function decodeManifest(zipBytes: Uint8Array) {
  const entries = unzipSync(zipBytes);
  const manifestBytes = entries["manifest.json"];
  if (!manifestBytes) throw new Error("manifest missing");
  return {
    entries,
    manifest: JSON.parse(new TextDecoder().decode(manifestBytes)) as Record<
      string,
      any
    >,
  };
}

function chatExportState(sessions: Session[]) {
  return JSON.stringify({
    state: {
      sessions,
      workspaces: [
        {
          id: "workspace-1",
          name: "Research",
          systemPrompt: "Use citations.",
          knowledgeCollectionIds: [],
          files: [],
          color: "blue",
          createdAt: Date.parse("2026-07-08T00:00:00.000Z"),
        },
      ],
    },
    version: 4,
  });
}

describe("browser import package builder", () => {
  it("creates a Phase 8 ZIP from local sessions, session_messages, and inline attachments", async () => {
    const session: Session = {
      id: "session-1",
      title: "Local chat",
      messageCount: 2,
      updatedAt: Date.parse("2026-07-08T00:00:00.000Z"),
      model: "openai:gpt-5.5",
      workspaceId: "workspace-1",
      config: {
        useSearch: true,
        activePlugins: ["weather"],
        activeSkills: ["summary"],
      },
    };
    const messages: Message[] = [
      {
        id: "user-1",
        role: "user",
        content: "read this",
        timestamp: Date.parse("2026-07-08T00:00:01.000Z"),
        attachments: [
          {
            id: "att-1",
            fileName: "note.txt",
            mimeType: "text/plain",
            data: Buffer.from("hello import", "utf8").toString("base64"),
          },
        ],
      },
      {
        id: "assistant-1",
        role: "model",
        content: "done",
        model: "openai:gpt-5.5",
        timestamp: Date.parse("2026-07-08T00:00:02.000Z"),
        outputBlocks: [{ id: "block-1", type: "text", content: "done" }],
      },
    ];

    const result = await createBrowserImportPackageFromSnapshot(
      {
        appExport: createAppExportPayload({
          exportedAt: "2026-07-08T00:00:00.000Z",
          settings: { providerApiKey: "sk-should-not-enter-manifest" },
          chat: chatExportState([session]),
        }),
        appDbKeys: ["neo-chat-storage", "session_messages_session-1"],
        sessionMessagesById: { "session-1": messages },
        existingOpfsUrls: ["opfs://chat/orphan.txt"],
        origin: "http://localhost:3000",
      },
      { now: fixedNow, idempotencyKey: "import-test-1" },
    );

    const { entries, manifest } = decodeManifest(result.zipBytes);
    const entryNames = Object.keys(entries).sort();
    const blobEntry = entryNames.find((name) =>
      name.startsWith("files/sha256/"),
    );

    expect(result.fileName).toBe("neo-chat-browser-import-v2.zip");
    expect(entryNames).toContain("manifest.json");
    expect(blobEntry).toMatch(/^files\/sha256\/[0-9a-f]{64}$/);
    expect(
      entryNames.every(
        (name) => name === "manifest.json" || name.startsWith("files/sha256/"),
      ),
    ).toBe(true);
    expect(JSON.stringify(manifest)).not.toContain(
      "sk-should-not-enter-manifest",
    );
    expect(manifest).toMatchObject({
      format: "neo-chat-browser-import",
      schemaVersion: "mm-chat.browser-import.v2",
      idempotencyKey: "import-test-1",
      counts: { conversations: 1, messages: 2, files: 1, bytes: 12 },
      source: { app: "neo-chat", origin: "http://localhost:3000" },
    });
    expect(manifest.conversations[0]).toMatchObject({
      clientId: "session-1",
      title: "Local chat",
      workspaceClientId: "workspace-1",
      modelRef: { providerId: "openai", modelId: "gpt-5.5" },
    });
    expect(manifest.messages.map((message: any) => message.role)).toEqual([
      "user",
      "assistant",
    ]);
    expect(manifest.messages.map((message: any) => message.sequenceNo)).toEqual(
      [0, 1],
    );
    expect(manifest.messages[0].attachments[0]).toMatchObject({
      source: "file",
      fileName: "note.txt",
      mimeType: "text/plain",
    });
    expect(manifest.files[0].blobPath).toBe(blobEntry);
    expect(manifest.opfs.orphanUrls).toEqual(["opfs://chat/orphan.txt"]);
  });

  it("omits secret-bearing remote attachment URLs from the import manifest", async () => {
    const session: Session = {
      id: "session-remote",
      title: "Remote files",
      messageCount: 1,
      updatedAt: Date.parse("2026-07-08T00:00:00.000Z"),
      model: "gpt-5.5",
    };
    const messages: Message[] = [
      {
        id: "user-remote",
        role: "user",
        content: "see remote files",
        timestamp: Date.parse("2026-07-08T00:00:01.000Z"),
        attachments: [
          {
            id: "safe-remote",
            fileName: "manual.pdf",
            mimeType: "application/pdf",
            url: "https://cdn.example/files/manual.pdf",
          },
          {
            id: "secret-remote",
            fileName: "signed.pdf",
            mimeType: "application/pdf",
            url: "https://files.example/download?access_token=sk-secret-cookie",
          },
        ],
      },
    ];

    const result = await createBrowserImportPackageFromSnapshot(
      {
        appExport: createAppExportPayload({
          chat: chatExportState([session]),
        }),
        sessionMessagesById: { "session-remote": messages },
      },
      { now: fixedNow, idempotencyKey: "import-test-remote" },
    );

    const { manifest } = decodeManifest(result.zipBytes);
    const manifestJson = JSON.stringify(manifest);
    const attachments = manifest.messages[0].attachments;

    expect(manifestJson).not.toContain("sk-secret-cookie");
    expect(manifestJson).not.toContain("access_token");
    expect(
      attachments.find(
        (attachment: any) => attachment.fileName === "manual.pdf",
      )?.url,
    ).toBe("https://cdn.example/files/manual.pdf");
    expect(
      attachments.find(
        (attachment: any) => attachment.fileName === "signed.pdf",
      ),
    ).not.toHaveProperty("url");
  });

  it("refuses to build a server import package when referenced OPFS bytes are missing", async () => {
    const session: Session = {
      id: "session-1",
      title: "Missing OPFS",
      messageCount: 1,
      updatedAt: Date.parse("2026-07-08T00:00:00.000Z"),
      model: "gpt-5.5",
    };
    const messages: Message[] = [
      {
        id: "user-1",
        role: "user",
        content: "see file",
        timestamp: Date.parse("2026-07-08T00:00:01.000Z"),
        attachments: [
          {
            id: "att-1",
            fileName: "missing.txt",
            mimeType: "text/plain",
            url: "opfs://chat/session-1/missing.txt",
          },
        ],
      },
    ];

    await expect(
      createBrowserImportPackageFromSnapshot(
        {
          appExport: createAppExportPayload({
            chat: chatExportState([session]),
          }),
          sessionMessagesById: { "session-1": messages },
          existingOpfsUrls: [],
        },
        { now: fixedNow, readOpfsBlob: async () => null },
      ),
    ).rejects.toMatchObject({
      code: "MISSING_OPFS_FILE",
      missingOpfsUrls: ["opfs://chat/session-1/missing.txt"],
    });
  });
});
