import { ApiClientError } from "../errors";
import type {
  ApiPage,
  BindKnowledgeDocumentInput,
  CollectionProcessingConsentInput,
  CreateKnowledgeCollectionInput,
  CreateKnowledgeDocumentVersionInput,
  DeleteKnowledgeDocumentInput,
  DownloadKnowledgeDocumentContentInput,
  KnowledgeApi,
  KnowledgeCollectionDTO,
  KnowledgeDocumentDTO,
  ListKnowledgeCollectionsInput,
  ListKnowledgeDocumentsInput,
  ProcessingConsentDTO,
  ProcessingConsentIdentityInput,
  PutCollectionProcessingConsentInput,
  PutQueryProcessingConsentInput,
  QueryProcessingConsentInput,
  ReprocessKnowledgeDocumentInput,
  UpdateKnowledgeCollectionInput,
} from "../types";
import type { HttpClient } from "./httpClient";

const collectionsPath = "/v1/knowledge/collections";
const documentsPath = "/v1/knowledge/documents";
const queryConsentsPath = "/v1/me/knowledge/query-consents";

export function createServerKnowledgeApiShell(
  httpClient: HttpClient,
): KnowledgeApi {
  return {
    async createCollection(
      input: CreateKnowledgeCollectionInput,
    ): Promise<KnowledgeCollectionDTO> {
      return httpClient.requestJson<KnowledgeCollectionDTO>(collectionsPath, {
        method: "POST",
        body: createCollectionBody(input),
        signal: input.signal,
      });
    },

    async listCollections(
      input: ListKnowledgeCollectionsInput = {},
    ): Promise<ApiPage<KnowledgeCollectionDTO>> {
      return httpClient.requestJson<ApiPage<KnowledgeCollectionDTO>>(
        `${collectionsPath}${collectionListQuery(input)}`,
        { signal: input.signal },
      );
    },

    async getCollection(input: {
      collectionId: string;
      signal?: AbortSignal;
    }): Promise<KnowledgeCollectionDTO> {
      return httpClient.requestJson<KnowledgeCollectionDTO>(
        collectionPath(input.collectionId),
        { signal: input.signal },
      );
    },

    async updateCollection(
      input: UpdateKnowledgeCollectionInput,
    ): Promise<KnowledgeCollectionDTO> {
      return httpClient.requestJson<KnowledgeCollectionDTO>(
        collectionPath(input.collectionId),
        {
          method: "PATCH",
          body: updateCollectionBody(input),
          signal: input.signal,
        },
      );
    },

    async deleteCollection(input: {
      collectionId: string;
      signal?: AbortSignal;
    }): Promise<void> {
      await httpClient.requestJson<void>(collectionPath(input.collectionId), {
        method: "DELETE",
        signal: input.signal,
      });
    },

    async bindDocument(
      input: BindKnowledgeDocumentInput,
    ): Promise<KnowledgeDocumentDTO> {
      return httpClient.requestJson<KnowledgeDocumentDTO>(
        `${collectionPath(input.collectionId)}/documents`,
        {
          method: "POST",
          body: {
            fileId: input.fileId,
            idempotencyKey: input.idempotencyKey,
          },
          signal: input.signal,
        },
      );
    },

    async listDocuments(
      input: ListKnowledgeDocumentsInput,
    ): Promise<ApiPage<KnowledgeDocumentDTO>> {
      return httpClient.requestJson<ApiPage<KnowledgeDocumentDTO>>(
        `${collectionPath(input.collectionId)}/documents${pageQuery(input)}`,
        { signal: input.signal },
      );
    },

    async getDocument(input: {
      documentId: string;
      signal?: AbortSignal;
    }): Promise<KnowledgeDocumentDTO> {
      return httpClient.requestJson<KnowledgeDocumentDTO>(
        documentPath(input.documentId),
        { signal: input.signal },
      );
    },

    async downloadDocumentContent(
      input: DownloadKnowledgeDocumentContentInput,
    ) {
      return httpClient.requestBinary(
        `${documentPath(input.documentId)}/content`,
        {
          signal: input.signal,
        },
      );
    },

    async createDocumentVersion(
      input: CreateKnowledgeDocumentVersionInput,
    ): Promise<KnowledgeDocumentDTO> {
      return httpClient.requestJson<KnowledgeDocumentDTO>(
        `${documentPath(input.documentId)}/versions`,
        {
          method: "POST",
          body: {
            fileId: input.fileId,
            idempotencyKey: input.idempotencyKey,
          },
          signal: input.signal,
        },
      );
    },

    async reprocessDocument(
      input: ReprocessKnowledgeDocumentInput,
    ): Promise<KnowledgeDocumentDTO> {
      return httpClient.requestJson<KnowledgeDocumentDTO>(
        `${documentPath(input.documentId)}/reprocess`,
        {
          method: "POST",
          body: { idempotencyKey: input.idempotencyKey },
          signal: input.signal,
        },
      );
    },

    async deleteDocument(input: DeleteKnowledgeDocumentInput): Promise<void> {
      await httpClient.requestJson<void>(documentPath(input.documentId), {
        method: "DELETE",
        signal: input.signal,
      });
    },

    async listCollectionConsents(input: {
      collectionId: string;
      signal?: AbortSignal;
    }): Promise<ProcessingConsentDTO[]> {
      return httpClient.requestJson<ProcessingConsentDTO[]>(
        `${collectionPath(input.collectionId)}/processing-consents`,
        { signal: input.signal },
      );
    },

    async putCollectionConsent(
      input: PutCollectionProcessingConsentInput,
    ): Promise<ProcessingConsentDTO> {
      return httpClient.requestJson<ProcessingConsentDTO>(
        `${collectionPath(input.collectionId)}/processing-consents/${consentIdentityPath(input)}`,
        {
          method: "PUT",
          body: putConsentBody(input),
          signal: input.signal,
        },
      );
    },

    async revokeCollectionConsent(
      input: CollectionProcessingConsentInput,
    ): Promise<void> {
      await httpClient.requestJson<void>(
        `${collectionPath(input.collectionId)}/processing-consents/${consentIdentityPath(input)}`,
        { method: "DELETE", signal: input.signal },
      );
    },

    async listQueryConsents(
      input: { signal?: AbortSignal } = {},
    ): Promise<ProcessingConsentDTO[]> {
      return httpClient.requestJson<ProcessingConsentDTO[]>(queryConsentsPath, {
        signal: input.signal,
      });
    },

    async putQueryConsent(
      input: PutQueryProcessingConsentInput,
    ): Promise<ProcessingConsentDTO> {
      return httpClient.requestJson<ProcessingConsentDTO>(
        `${queryConsentsPath}/${consentIdentityPath(input)}`,
        {
          method: "PUT",
          body: putConsentBody(input),
          signal: input.signal,
        },
      );
    },

    async revokeQueryConsent(
      input: QueryProcessingConsentInput,
    ): Promise<void> {
      await httpClient.requestJson<void>(
        `${queryConsentsPath}/${consentIdentityPath(input)}`,
        { method: "DELETE", signal: input.signal },
      );
    },
  };
}

function createCollectionBody(input: CreateKnowledgeCollectionInput) {
  return {
    name: input.name,
    description: input.description,
    icon: input.icon,
    color: input.color,
    scope: input.scope,
    teamId: input.teamId,
    idempotencyKey: input.idempotencyKey,
  };
}

function updateCollectionBody(input: UpdateKnowledgeCollectionInput) {
  return {
    name: input.name,
    description: input.description,
    icon: input.icon,
    color: input.color,
  };
}

function putConsentBody(input: PutQueryProcessingConsentInput) {
  return {
    purposes: input.purposes,
    dataTypes: input.dataTypes,
    policyVersion: input.policyVersion,
    expiresAt: input.expiresAt,
  };
}

function collectionPath(collectionId: string): string {
  return `${collectionsPath}/${requiredPathId(collectionId, "collection id")}`;
}

function documentPath(documentId: string): string {
  return `${documentsPath}/${requiredPathId(documentId, "document id")}`;
}

function collectionListQuery(input: ListKnowledgeCollectionsInput): string {
  const params = new URLSearchParams();
  if (input.scope !== undefined) params.set("scope", input.scope);
  if (input.teamId !== undefined) params.set("teamId", input.teamId);
  appendPageParams(params, input);
  return queryString(params);
}

function pageQuery(input: { cursor?: string; limit?: number }): string {
  const params = new URLSearchParams();
  appendPageParams(params, input);
  return queryString(params);
}

function appendPageParams(
  params: URLSearchParams,
  input: { cursor?: string; limit?: number },
): void {
  if (input.cursor !== undefined) params.set("cursor", input.cursor);
  if (input.limit !== undefined) params.set("limit", String(input.limit));
}

function queryString(params: URLSearchParams): string {
  const query = params.toString();
  return query ? `?${query}` : "";
}

function consentIdentityPath(input: ProcessingConsentIdentityInput): string {
  const processor = requiredPathId(input.processor, "processor");
  const hasEndpoint = Boolean(input.endpointId?.trim());
  const hasModel = Boolean(input.modelId?.trim());
  if (hasEndpoint !== hasModel) {
    throw new ApiClientError(
      "INVALID_CONSENT_IDENTITY",
      "endpointId and modelId must be provided together.",
    );
  }
  if (!hasEndpoint || !hasModel) return processor;
  const params = new URLSearchParams({
    endpointId: input.endpointId!.trim(),
    modelId: input.modelId!.trim(),
  });
  return `${processor}?${params.toString()}`;
}

function requiredPathId(value: string, label: string): string {
  const normalized = value.trim();
  if (!normalized) {
    throw new ApiClientError(
      "INVALID_RESOURCE_ID",
      `${label} is required for knowledge API requests.`,
    );
  }
  return encodeURIComponent(normalized);
}
