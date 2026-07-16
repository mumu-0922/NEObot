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

  it("routes server mode Knowledge Base to the Go-backed component", () => {
    expect(knowledgeBase).toContain("ServerKnowledgeBase");
    expect(knowledgeBase).toContain("createNeoChatApiClient");
    expect(knowledgeBase).toContain('apiClientSnapshot.mode === "server"');
    expect(knowledgeBase).toContain("apiClientSnapshot.capabilities.knowledge");
    expect(knowledgeBase).toContain("apiClientSnapshot.capabilities.files");
    expect(knowledgeBase).toContain("<LocalKnowledgeBase");
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
    expect(serverKnowledgeBase).toContain("apiClient.knowledge.listDocuments");
    expect(serverKnowledgeBase).toContain("apiClient.knowledge.deleteDocument");
    expect(serverKnowledgeBase).toContain(
      "apiClient.knowledge.reprocessDocument",
    );
    expect(serverKnowledgeBase).toContain(
      "apiClient.knowledge.listCollectionConsents",
    );
    expect(serverKnowledgeBase).toContain(
      "apiClient.knowledge.putCollectionConsent",
    );
    expect(serverKnowledgeBase).toContain(
      "apiClient.knowledge.revokeCollectionConsent",
    );
    expect(serverKnowledgeBase).toContain(
      "apiClient.knowledge.listQueryConsents",
    );
    expect(serverKnowledgeBase).toContain(
      "apiClient.knowledge.putQueryConsent",
    );
    expect(serverKnowledgeBase).toContain(
      "apiClient.knowledge.revokeQueryConsent",
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

  it("keeps caller identity and ACL fields out of server Knowledge UI payloads", () => {
    expect(serverKnowledgeBase).toContain("idempotencyKey");
    expect(serverKnowledgeBase).toContain('scope: "personal"');
    expect(serverKnowledgeBase).not.toContain("teamId");
    expect(serverKnowledgeBase).not.toContain("actorUserId");
    expect(serverKnowledgeBase).not.toContain("ownerUserId");
    expect(serverKnowledgeBase).not.toContain("impersonateUserId");
    expect(serverKnowledgeBase).not.toContain("allowedCollectionIds");
  });

  it("keeps consent UX server-secret backed and fail-closed", () => {
    expect(serverKnowledgeBase).toContain("serverConsentEnvBackedNote");
    expect(serverKnowledgeBase).toContain("serverConsentFailClosedNote");
    expect(serverKnowledgeBase).toContain("canManageCollectionConsent");
    expect(serverKnowledgeBase).toContain("defaultCollectionConsentForm");
    expect(serverKnowledgeBase).toContain("defaultQueryConsentForm");
    expect(serverKnowledgeBase).toContain("mineru");
    expect(serverKnowledgeBase).toContain("jina");
    expect(serverKnowledgeBase).not.toContain("apiKey");
    expect(serverKnowledgeBase).not.toContain("apiToken");
    expect(serverKnowledgeBase).not.toContain("RAG_MINERU_API_TOKEN");
    expect(serverKnowledgeBase).not.toContain("RAG_JINA_API_KEY");
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
  });
});
