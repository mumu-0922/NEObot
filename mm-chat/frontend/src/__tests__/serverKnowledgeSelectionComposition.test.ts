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
  const messageInput = readFileSync(
    resolve(process.cwd(), "src/components/chat/MessageInput.tsx"),
    "utf8",
  );
  const evidenceBlock = readFileSync(
    resolve(
      process.cwd(),
      "src/components/knowledge/KnowledgeEvidenceBlock.tsx",
    ),
    "utf8",
  );

  it("loads visible personal server collections through the API client", () => {
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

  it("persists selected collections as the conversation binding authority", () => {
    expect(attachmentUtils).toContain("KNOWLEDGE_COLLECTION_MIME");
    expect(attachmentUtils).toContain("getKnowledgeAttachmentCollectionIds");
    expect(chatApp).toContain("persistConversationKnowledgeSelection");
    expect(chatApp).toContain("updateServerSessionConfig");
    expect(chatApp).toContain("selectedKnowledgeCollectionIds");
    expect(chatApp).toContain("sessionKnowledgeBinding === undefined");
    expect(chatApp).not.toContain("buildServerKnowledgeStreamConfig");
    expect(chatApp).not.toContain("buildServerKnowledgeMessageMetadata");
    expect(chatApp).not.toContain("ragStrict");
    expect(chatApp).not.toContain("knowledgeStrict");
    expect(chatApp).not.toContain("getServerKnowledgeSelectionIdsFromMessage");
  });

  it("uses a dedicated persistent Knowledge control with an eight collection cap", () => {
    expect(messageInput).toContain("onKnowledgeCollectionIdsChange");
    expect(messageInput).toContain("conversationKnowledgeEnabled");
    expect(messageInput).toContain("selectedKnowledgeBases");
    expect(messageInput).toContain("removeKnowledgeBase");
    expect(messageInput).toContain("manageKnowledgeBases");
    expect(messageInput).toContain("MAX_CONVERSATION_KNOWLEDGE_COLLECTIONS");
    expect(selectionModal).toContain("initialSelectedCollectionIds");
    expect(selectionModal).toContain("maxSelectedCollections = 8");
    expect(selectionModal).toContain("onSelectCollections");
    expect(selectionModal).toContain('t("saveSelection")');
  });

  it("colors the Knowledge control only when a collection is selected", () => {
    expect(messageInput).toContain(
      'normalizedKnowledgeCollectionIds.length > 0\n                      ? "bg-purple-50 text-purple-700',
    );
    expect(messageInput).toContain(
      ': "text-gray-500 dark:text-muted-foreground hover:text-gray-700',
    );
    expect(messageInput).not.toContain(
      "showKBModal || normalizedKnowledgeCollectionIds.length > 0",
    );
  });

  it("keeps citation details without rendering an Auto mode badge", () => {
    expect(evidenceBlock).toContain('t("citationsHeading"');
    expect(evidenceBlock).not.toContain("{knowledge.mode}");
    expect(evidenceBlock).not.toContain("uppercase tracking-wide");
    expect(evidenceBlock).not.toContain("verifiedEvidenceUsed");
  });
});
