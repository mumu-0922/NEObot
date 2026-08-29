import type { Route } from "@playwright/test";

import type {
  FileRecordDTO,
  KnowledgeCollectionDTO,
  KnowledgeDocumentDTO,
  KnowledgeDocumentVersionDTO,
} from "../../src/services/api/client/types";

import { json, NOW } from "./neoChatApiSupport";

type CollectionSeed = Partial<KnowledgeCollectionDTO> &
  Pick<KnowledgeCollectionDTO, "id" | "name">;

type UploadSeed = {
  id?: string;
  fileName: string;
  mimeType?: string;
  size?: number;
};

export class NeoChatKnowledgeApiFixture {
  readonly collections: KnowledgeCollectionDTO[] = [];
  readonly documents = new Map<string, KnowledgeDocumentDTO[]>();
  private readonly files = new Map<string, FileRecordDTO>();
  private readonly uploadQueue: UploadSeed[] = [];
  private sequence = 0;

  seedCollection(seed: CollectionSeed): KnowledgeCollectionDTO {
    const collection: KnowledgeCollectionDTO = {
      id: seed.id,
      name: seed.name,
      description: seed.description ?? "Deterministic RAG E2E collection",
      icon: seed.icon ?? "BookText",
      color: seed.color ?? "purple",
      scope: seed.scope ?? "personal",
      permissions: seed.permissions ?? {
        read: true,
        manage: true,
        manageConsent: true,
      },
      aclRevision: seed.aclRevision ?? 1,
      visibilityEpoch: seed.visibilityEpoch ?? 1,
      collectionProcessingRevision: seed.collectionProcessingRevision ?? 1,
      createdAt: seed.createdAt ?? NOW,
      updatedAt: seed.updatedAt ?? NOW,
      ...(seed.teamId ? { teamId: seed.teamId } : {}),
    };
    this.collections.push(collection);
    this.documents.set(collection.id, []);
    return collection;
  }

  queueUpload(seed: UploadSeed): void {
    this.uploadQueue.push(seed);
  }

  seedActiveDocument(
    collectionId: string,
    fileName: string,
  ): KnowledgeDocumentDTO {
    const file = this.createFile({ fileName });
    const document = this.createDocument(collectionId, file, "active");
    this.documents.get(collectionId)?.push(document);
    return document;
  }

  activateDocument(documentId: string): void {
    const document = this.requireDocument(documentId);
    const pending = document.pendingVersion;
    if (!pending) {
      throw new Error(`Document ${documentId} has no pending version`);
    }
    document.status = "active";
    document.currentVersion = { ...pending, status: "active", updatedAt: NOW };
    delete document.pendingVersion;
    document.updatedAt = NOW;
  }

  async handle(route: Route, path: string, method: string): Promise<boolean> {
    if (path === "/v1/files" && method === "POST") {
      const queued = this.uploadQueue.shift();
      if (!queued) return false;
      const file = this.createFile(queued);
      await json(route, file, 201);
      return true;
    }

    const fileMatch = path.match(/^\/v1\/files\/([^/]+)$/);
    if (fileMatch && method === "DELETE") {
      this.files.delete(decodeURIComponent(fileMatch[1]));
      await route.fulfill({ status: 204 });
      return true;
    }

    if (path === "/v1/knowledge/collections" && method === "GET") {
      await json(route, { items: this.collections });
      return true;
    }

    if (path === "/v1/knowledge/collections" && method === "POST") {
      const body = route.request().postDataJSON() as Record<string, unknown>;
      const collection = this.seedCollection({
        id: this.nextUuid("71000000"),
        name: typeof body.name === "string" ? body.name : "E2E collection",
        description:
          typeof body.description === "string" ? body.description : "",
        icon: typeof body.icon === "string" ? body.icon : "Folder",
        color: typeof body.color === "string" ? body.color : "purple",
      });
      await json(route, collection, 201);
      return true;
    }

    const collectionDocumentsMatch = path.match(
      /^\/v1\/knowledge\/collections\/([^/]+)\/documents$/,
    );
    if (collectionDocumentsMatch && method === "GET") {
      const collectionId = decodeURIComponent(collectionDocumentsMatch[1]);
      await json(route, { items: this.documents.get(collectionId) ?? [] });
      return true;
    }

    if (collectionDocumentsMatch && method === "POST") {
      const collectionId = decodeURIComponent(collectionDocumentsMatch[1]);
      const body = route.request().postDataJSON() as Record<string, unknown>;
      const fileId = typeof body.fileId === "string" ? body.fileId : "";
      const file = this.files.get(fileId);
      if (!file || !this.documents.has(collectionId)) {
        await json(
          route,
          { error: { code: "NOT_FOUND", message: "Fixture resource missing" } },
          404,
        );
        return true;
      }
      const document = this.createDocument(collectionId, file, "processing");
      this.documents.get(collectionId)?.push(document);
      await json(route, document, 201);
      return true;
    }

    const collectionMatch = path.match(
      /^\/v1\/knowledge\/collections\/([^/]+)$/,
    );
    if (collectionMatch && method === "GET") {
      const collectionId = decodeURIComponent(collectionMatch[1]);
      const collection = this.collections.find(
        (item) => item.id === collectionId,
      );
      if (!collection) {
        await json(
          route,
          { error: { code: "NOT_FOUND", message: "Collection not found" } },
          404,
        );
        return true;
      }
      await json(route, collection);
      return true;
    }

    const documentMatch = path.match(/^\/v1\/knowledge\/documents\/([^/]+)$/);
    if (documentMatch && method === "GET") {
      const document = this.requireDocument(
        decodeURIComponent(documentMatch[1]),
      );
      await json(route, document);
      return true;
    }

    return false;
  }

  private createFile(seed: UploadSeed): FileRecordDTO {
    const id = seed.id ?? this.nextUuid("72000000");
    const file: FileRecordDTO = {
      id,
      fileName: seed.fileName,
      mimeType: seed.mimeType ?? "text/plain",
      size: seed.size ?? 64,
      sha256: "b".repeat(64),
      purpose: "knowledge",
      createdAt: NOW,
      downloadUrl: `/v1/files/${id}/content`,
    };
    this.files.set(id, file);
    return file;
  }

  private createDocument(
    collectionId: string,
    file: FileRecordDTO,
    status: "processing" | "active",
  ): KnowledgeDocumentDTO {
    const version: KnowledgeDocumentVersionDTO = {
      id: this.nextUuid("74000000"),
      sourceVersion: 1,
      file: {
        id: file.id,
        name: file.fileName,
        mimeType: file.mimeType,
        byteSize: file.size,
      },
      status,
      createdAt: NOW,
      updatedAt: NOW,
    };
    return {
      id: this.nextUuid("73000000"),
      collectionId,
      status,
      ...(status === "active"
        ? { currentVersion: version }
        : { pendingVersion: version }),
      createdAt: NOW,
      updatedAt: NOW,
    };
  }

  private requireDocument(documentId: string): KnowledgeDocumentDTO {
    for (const documents of this.documents.values()) {
      const document = documents.find((item) => item.id === documentId);
      if (document) return document;
    }
    throw new Error(`Unknown knowledge document ${documentId}`);
  }

  private nextUuid(prefix: string): string {
    this.sequence += 1;
    return `${prefix}-0000-4000-8000-${String(this.sequence).padStart(12, "0")}`;
  }
}
