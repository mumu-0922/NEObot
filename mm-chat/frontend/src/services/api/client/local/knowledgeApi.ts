import { unsupportedFeature } from "../errors";
import type {
  ApiPage,
  DownloadedFileContent,
  KnowledgeApi,
  KnowledgeCollectionDTO,
  KnowledgeDocumentDTO,
  ProcessingConsentDTO,
} from "../types";

export function createLocalKnowledgeApiShell(): KnowledgeApi {
  return {
    async createCollection(): Promise<KnowledgeCollectionDTO> {
      throw unsupportedFeature("local knowledge adapter wiring");
    },
    async listCollections(): Promise<ApiPage<KnowledgeCollectionDTO>> {
      throw unsupportedFeature("local knowledge adapter wiring");
    },
    async getCollection(): Promise<KnowledgeCollectionDTO> {
      throw unsupportedFeature("local knowledge adapter wiring");
    },
    async updateCollection(): Promise<KnowledgeCollectionDTO> {
      throw unsupportedFeature("local knowledge adapter wiring");
    },
    async deleteCollection(): Promise<void> {
      throw unsupportedFeature("local knowledge adapter wiring");
    },
    async bindDocument(): Promise<KnowledgeDocumentDTO> {
      throw unsupportedFeature("local knowledge adapter wiring");
    },
    async listDocuments(): Promise<ApiPage<KnowledgeDocumentDTO>> {
      throw unsupportedFeature("local knowledge adapter wiring");
    },
    async getDocument(): Promise<KnowledgeDocumentDTO> {
      throw unsupportedFeature("local knowledge adapter wiring");
    },
    async downloadDocumentContent(): Promise<DownloadedFileContent> {
      throw unsupportedFeature("local knowledge adapter wiring");
    },
    async createDocumentVersion(): Promise<KnowledgeDocumentDTO> {
      throw unsupportedFeature("local knowledge adapter wiring");
    },
    async reprocessDocument(): Promise<KnowledgeDocumentDTO> {
      throw unsupportedFeature("local knowledge adapter wiring");
    },
    async deleteDocument(): Promise<void> {
      throw unsupportedFeature("local knowledge adapter wiring");
    },
    async listCollectionConsents(): Promise<ProcessingConsentDTO[]> {
      throw unsupportedFeature("local knowledge adapter wiring");
    },
    async putCollectionConsent(): Promise<ProcessingConsentDTO> {
      throw unsupportedFeature("local knowledge adapter wiring");
    },
    async revokeCollectionConsent(): Promise<void> {
      throw unsupportedFeature("local knowledge adapter wiring");
    },
    async listQueryConsents(): Promise<ProcessingConsentDTO[]> {
      throw unsupportedFeature("local knowledge adapter wiring");
    },
    async putQueryConsent(): Promise<ProcessingConsentDTO> {
      throw unsupportedFeature("local knowledge adapter wiring");
    },
    async revokeQueryConsent(): Promise<void> {
      throw unsupportedFeature("local knowledge adapter wiring");
    },
  };
}
