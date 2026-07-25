import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import en from "../i18n/locales/en";
import ja from "../i18n/locales/ja";
import zh from "../i18n/locales/zh";

describe("G8 server knowledge base UI composition", () => {
  const knowledgeBase = readFileSync(
    resolve(process.cwd(), "src/components/knowledge/KnowledgeBase.tsx"),
    "utf8",
  );
  const serverKnowledgeBase = readFileSync(
    resolve(process.cwd(), "src/components/knowledge/ServerKnowledgeBase.tsx"),
    "utf8",
  );

  it("routes Knowledge Base directly to the Go-backed component", () => {
    expect(knowledgeBase).toContain("ServerKnowledgeBase");
    expect(knowledgeBase).toContain("<ServerKnowledgeBase");
    expect(knowledgeBase).not.toContain("LocalKnowledgeBase");
    expect(knowledgeBase).not.toContain("useKnowledgeStore");
  });

  it("keeps visible server Knowledge actions behind API client adapters", () => {
    expect(serverKnowledgeBase).toContain("createNeoChatApiClient");
    expect(serverKnowledgeBase).toContain("apiClient.capabilities.knowledge");
    expect(serverKnowledgeBase).toContain("apiClient.capabilities.files");
    expect(serverKnowledgeBase).toContain(
      "apiClient.knowledge.listCollections",
    );
    expect(serverKnowledgeBase).toContain(
      "apiClient.knowledge.createCollection",
    );
    expect(serverKnowledgeBase).toContain(
      "apiClient.knowledge.updateCollection",
    );
    expect(serverKnowledgeBase).toContain(
      "apiClient.knowledge.deleteCollection",
    );
    expect(serverKnowledgeBase).toContain("apiClient.files.uploadFile");
    expect(serverKnowledgeBase).toContain("apiClient.knowledge.bindDocument");
    expect(serverKnowledgeBase).toContain(
      "await apiClient.files.deleteFile(fileRecord.id)",
    );
    expect(serverKnowledgeBase).toContain("apiClient.knowledge.listDocuments");
    expect(serverKnowledgeBase).toContain("apiClient.knowledge.deleteDocument");
    expect(serverKnowledgeBase).toContain(
      "apiClient.knowledge.reprocessDocument",
    );
    expect(serverKnowledgeBase).toContain(
      "apiClient.knowledge.downloadDocumentContent",
    );
    expect(serverKnowledgeBase).toContain("triggerBlobDownload");
    expect(serverKnowledgeBase).toContain("sanitizeDownloadFilename");
    expect(serverKnowledgeBase).toContain(
      'document.currentVersion?.status === "active"',
    );
    expect(serverKnowledgeBase).toContain(
      'document.pendingVersion?.status === "failed"',
    );
    expect(serverKnowledgeBase).not.toContain("/v1/knowledge");
    expect(serverKnowledgeBase).not.toContain("/api/rag");
    expect(serverKnowledgeBase).not.toContain("/api/doc-parse");
  });

  it("makes document deletion immediately invisible with rollback on failure", () => {
    const optimisticDeleteIndex = serverKnowledgeBase.indexOf(
      "setDocuments((current) =>",
    );
    const apiDeleteIndex = serverKnowledgeBase.indexOf(
      "apiClient.knowledge.deleteDocument",
    );
    const rollbackIndex = serverKnowledgeBase.indexOf(
      "setDocuments(previousDocuments)",
    );

    expect(optimisticDeleteIndex).toBeGreaterThan(-1);
    expect(apiDeleteIndex).toBeGreaterThan(optimisticDeleteIndex);
    expect(rollbackIndex).toBeGreaterThan(apiDeleteIndex);
  });

  it("supports bounded bulk deletion while retaining failed selections", () => {
    expect(serverKnowledgeBase).toContain(
      "deleteKnowledgeDocumentsWithConcurrency",
    );
    expect(serverKnowledgeBase).toContain("BULK_DELETE_CONCURRENCY = 3");
    expect(serverKnowledgeBase).toContain("serverSelectAllDocuments");
    expect(serverKnowledgeBase).toContain("serverBulkDeleteDocuments");
    expect(serverKnowledgeBase).toContain(
      "setSelectedDocumentIds(new Set(result.failedIds))",
    );
    expect(en.Knowledge.serverBulkDeleteResult).toBeTruthy();
    expect(zh.Knowledge.serverBulkDeleteResult).toBeTruthy();
    expect(ja.Knowledge.serverBulkDeleteResult).toBeTruthy();
  });

  it("keeps caller identity and ACL fields out of server Knowledge UI payloads", () => {
    expect(serverKnowledgeBase).toContain("idempotencyKey");
    expect(serverKnowledgeBase).toContain('scope: "personal"');
    expect(serverKnowledgeBase).not.toContain("teamId");
    expect(serverKnowledgeBase).not.toContain("actorUserId");
    expect(serverKnowledgeBase).not.toContain("ownerUserId");
    expect(serverKnowledgeBase).not.toContain("impersonateUserId");
    expect(serverKnowledgeBase).not.toContain("allowedCollectionIds");
  });

  it("preserves the original collection grid and modal interaction layout", () => {
    expect(serverKnowledgeBase).toContain("CreateCollectionCard");
    expect(serverKnowledgeBase).toContain("ServerCollectionCard");
    expect(serverKnowledgeBase).toContain("ServerCollectionModal");
    expect(serverKnowledgeBase).toContain("searchCollectionsPlaceholder");
    expect(serverKnowledgeBase).toContain("md:grid-cols-2");
    expect(serverKnowledgeBase).toContain("xl:grid-cols-4");
    expect(serverKnowledgeBase).toContain("slide-in-from-right-8");
    expect(serverKnowledgeBase).not.toContain(
      "lg:grid-cols-[minmax(260px,340px)_1fr]",
    );
    expect(serverKnowledgeBase).not.toContain(
      "serverCollectionNamePlaceholder",
    );
  });

  it("does not expose multi-user consent management in single-user mode", () => {
    expect(serverKnowledgeBase).not.toContain("ConsentSection");
    expect(serverKnowledgeBase).not.toContain("listCollectionConsents");
    expect(serverKnowledgeBase).not.toContain("putCollectionConsent");
    expect(serverKnowledgeBase).not.toContain("revokeCollectionConsent");
    expect(serverKnowledgeBase).not.toContain("listQueryConsents");
    expect(serverKnowledgeBase).not.toContain("putQueryConsent");
    expect(serverKnowledgeBase).not.toContain("revokeQueryConsent");
    expect(serverKnowledgeBase).not.toContain("apiKey");
    expect(serverKnowledgeBase).not.toContain("apiToken");
    expect(serverKnowledgeBase).not.toContain("RAG_MINERU_API_TOKEN");
    expect(serverKnowledgeBase).not.toContain("RAG_JINA_API_KEY");
    expect(serverKnowledgeBase).not.toContain("useKnowledgeStore");
  });

  it("ships localized server Knowledge copy for all supported locales", () => {
    expect(en.Knowledge.serverUnsupportedDescription).toBeTruthy();
    expect(zh.Knowledge.serverUnsupportedDescription).toBeTruthy();
    expect(ja.Knowledge.serverUnsupportedDescription).toBeTruthy();
    expect(en.Knowledge.serverDocumentStatus.active).toBeTruthy();
    expect(zh.Knowledge.serverDocumentStatus.processing).toBeTruthy();
    expect(ja.Knowledge.serverDocumentStatus.tombstoned).toBeTruthy();
    expect(en.Knowledge.serverConsentEnvBackedNote).toBeTruthy();
    expect(zh.Knowledge.serverConsentFailClosedNote).toBeTruthy();
    expect(ja.Knowledge.serverConsentStatus.granted).toBeTruthy();
    expect(en.Knowledge.serverSingleUserScope).toBeTruthy();
    expect(zh.Knowledge.serverSingleUserScope).toBeTruthy();
    expect(ja.Knowledge.serverSingleUserScope).toBeTruthy();
    expect(en.Knowledge.serverDownloadDocument).toBeTruthy();
    expect(zh.Knowledge.serverDownloadDocumentFailed).toBeTruthy();
    expect(ja.Knowledge.serverDownloadDocument).toBeTruthy();
  });
});
