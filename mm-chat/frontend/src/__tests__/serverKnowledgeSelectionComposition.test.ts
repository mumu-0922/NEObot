import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

describe("G8.5 server knowledge selection isolation", () => {
  const selectionModal = readFileSync(
    resolve(
      process.cwd(),
      "src/components/knowledge/KnowledgeSelectionModal.tsx",
    ),
    "utf8",
  );
  const chatApp = readFileSync(
    resolve(process.cwd(), "src/components/app/ChatApp.tsx"),
    "utf8",
  );
  const attachmentUtils = readFileSync(
    resolve(process.cwd(), "src/lib/utils/knowledgeAttachments.ts"),
    "utf8",
  );

  it("loads visible personal/team server collections through the API client", () => {
    expect(selectionModal).toContain("createNeoChatApiClient");
    expect(selectionModal).toContain('apiClient.mode === "server"');
    expect(selectionModal).toContain("apiClient.capabilities.knowledge");
    expect(selectionModal).toContain("apiClient.knowledge");
    expect(selectionModal).toContain("listCollections({ limit: 100 })");
    expect(selectionModal).toContain("serverCollections");
    expect(selectionModal).toContain("renderServerCollectionRow");
    expect(selectionModal).toContain("collection.scope");
    expect(selectionModal).not.toContain("/v1/knowledge");
    expect(selectionModal).not.toContain("/api/rag");
    expect(selectionModal).not.toContain("actorUserId");
    expect(selectionModal).not.toContain("ownerUserId");
    expect(selectionModal).not.toContain("allowedCollectionIds");
    expect(selectionModal).not.toContain("impersonateUserId");
  });

  it("keeps server selection at collection scope instead of local OPFS file scope", () => {
    const serverBranchIndex = selectionModal.indexOf(
      "if (serverKnowledgeEnabled)",
    );
    const localFileBranchIndex = selectionModal.indexOf(
      'if (key.startsWith("file:"))',
    );

    expect(serverBranchIndex).toBeGreaterThan(-1);
    expect(localFileBranchIndex).toBeGreaterThan(serverBranchIndex);
    expect(selectionModal).toContain("createKnowledgeCollectionAttachment");
    expect(selectionModal).toContain("collectionId: collection.id");
    expect(selectionModal).toContain("collectionName: collection.name");
  });

  it("passes selected server collection IDs into strict Go RAG config and metadata", () => {
    expect(attachmentUtils).toContain("KNOWLEDGE_COLLECTION_MIME");
    expect(attachmentUtils).toContain("getKnowledgeAttachmentCollectionIds");
    expect(chatApp).toContain("getServerKnowledgeSelectionIds");
    expect(chatApp).toContain("buildServerKnowledgeStreamConfig");
    expect(chatApp).toContain("buildServerKnowledgeMessageMetadata");
    expect(chatApp).toContain("selectedKnowledgeCollectionIds");
    expect(chatApp).toContain("ragStrict: true");
    expect(chatApp).toContain("knowledgeStrict: true");
    expect(chatApp).toContain("getServerKnowledgeSelectionIdsFromMessage");
  });
});
