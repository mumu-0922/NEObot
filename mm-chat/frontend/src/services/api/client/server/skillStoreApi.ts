import { z } from "zod";

import { ApiClientError } from "../errors";
import type { SkillStoreApi } from "../types";
import type { HttpClient } from "./httpClient";

const string = z.string().min(1);
const count = z.number().int().nonnegative();
const positive = z.number().int().positive();
const fingerprint = z.string().regex(/^sha256:[a-f0-9]{64}$/);
const skillName = z
  .string()
  .regex(/^[a-z0-9]+(?:-[a-z0-9]+)*$/)
  .max(64);

const packageVersionSchema = z
  .object({
    packageFingerprint: fingerprint,
    runtimeBundleFingerprint: fingerprint.optional(),
    sbomFingerprint: fingerprint,
    name: string,
    version: string,
    description: z.string(),
    license: z.string().optional(),
    compatibility: z.string().optional(),
    allowedTools: z.array(z.string()),
    capabilityRequests: z.array(
      z
        .object({
          capability: string,
          actions: z.array(z.string()),
          reason: string,
        })
        .strict(),
    ),
    hasRuntime: z.boolean(),
    fileCount: count,
    packageBytes: count,
    expandedBytes: count,
    createdAt: string,
  })
  .strict();

const packageCandidateSchema = z
  .object({
    id: string,
    sourceType: string,
    sourceRef: string,
    sourceArtifactSha256: fingerprint,
    package: packageVersionSchema,
    status: string,
    admissionEligible: z.boolean(),
    validationSummary: z.string(),
    reviewReason: z.string().optional(),
    revision: positive,
    createdAt: string,
    updatedAt: string,
  })
  .strict();

const installationSchema = z
  .object({
    id: string,
    admissionId: string,
    packageFingerprint: fingerprint,
    name: string,
    version: string,
    description: z.string(),
    allowedTools: z.array(z.string()),
    revision: positive,
    createdAt: string,
    updatedAt: string,
  })
  .strict();

const conversationSelectionSchema = z
  .object({
    conversationId: string,
    revision: count,
    skills: z.array(installationSchema),
  })
  .strict();

const catalogSummarySchema = z
  .object({
    id: skillName,
    name: skillName,
    repository: z.literal("openai/skills"),
    ref: z.literal("main"),
    path: z.string().regex(/^skills\/\.curated\/[a-z0-9]+(?:-[a-z0-9]+)*$/),
    sourceUrl: z
      .string()
      .url()
      .regex(
        /^https:\/\/github\.com\/openai\/skills\/tree\/main\/skills\/\.curated\/[a-z0-9]+(?:-[a-z0-9]+)*$/,
      ),
    catalogSource: z.literal("openai/skills curated"),
  })
  .strict()
  .refine(
    (value) =>
      value.id === value.name &&
      value.path.endsWith(`/${value.name}`) &&
      value.sourceUrl.endsWith(`/${value.name}`),
    { message: "catalog identity mismatch" },
  );

const catalogItemSchema = catalogSummarySchema
  .safeExtend({
    resolvedCommit: z.string().regex(/^[a-f0-9]{40}$/),
    packageFingerprint: fingerprint,
    version: string,
    description: z.string(),
    license: z.string().optional(),
    compatibility: z.string().optional(),
    allowedTools: z.array(z.string()),
    hasRuntime: z.boolean(),
  })
  .strict();

function parse<T>(schema: z.ZodType<T>, value: unknown, subject: string): T {
  const result = schema.safeParse(value);
  if (!result.success) {
    throw new ApiClientError(
      "INVALID_SERVER_RESPONSE",
      `Server returned an invalid Skill Store ${subject}.`,
    );
  }
  return result.data;
}

export function createServerSkillStoreApiShell(
  httpClient: HttpClient,
): SkillStoreApi {
  return {
    async listCatalog(options = {}) {
      const response = await httpClient.requestJson<unknown>(
        "/v1/skills/catalog",
        { signal: options.signal },
      );
      return parse(
        z
          .object({
            items: z.array(catalogSummarySchema),
            totalCount: count,
            source: z.literal("openai/skills curated"),
          })
          .strict(),
        response,
        "catalog",
      );
    },

    async getCatalogSkill(id, options = {}) {
      const response = await httpClient.requestJson<unknown>(
        `/v1/skills/catalog/items/${encodeURIComponent(id)}`,
        { signal: options.signal },
      );
      return parse(
        z.object({ skill: catalogItemSchema }).strict(),
        response,
        "catalog detail",
      ).skill;
    },

    async installCatalogSkill(input) {
      const response = await httpClient.requestJson<unknown>(
        `/v1/skills/catalog/items/${encodeURIComponent(input.id)}/install`,
        {
          method: "POST",
          body: {
            resolvedCommit: input.resolvedCommit,
            packageFingerprint: input.packageFingerprint,
          },
          signal: input.signal,
        },
      );
      return parse(
        z.object({ skill: installationSchema }).strict(),
        response,
        "catalog install",
      ).skill;
    },

    async listPackageStore(input = {}) {
      const query = new URLSearchParams({
        page: String(input.page ?? 1),
        pageSize: String(input.pageSize ?? 20),
      });
      const response = await httpClient.requestJson<unknown>(
        `/v1/skills/store?${query}`,
        { signal: input.signal },
      );
      return parse(
        z
          .object({
            items: z.array(packageCandidateSchema),
            page: positive,
            pageSize: positive,
            totalCount: count,
            totalPages: count,
          })
          .strict(),
        response,
        "list",
      );
    },

    async getPackageSkill(candidateId, options = {}) {
      const response = await httpClient.requestJson<unknown>(
        `/v1/skills/store/items/${encodeURIComponent(candidateId)}`,
        { signal: options.signal },
      );
      return parse(
        z.object({ skill: packageCandidateSchema }).strict(),
        response,
        "detail",
      ).skill;
    },

    async listPackageLibrary(options = {}) {
      const response = await httpClient.requestJson<unknown>(
        "/v1/skills/library",
        { signal: options.signal },
      );
      return parse(
        z.object({ skills: z.array(installationSchema) }).strict(),
        response,
        "library",
      ).skills;
    },

    async installPackageSkill(input) {
      const response = await httpClient.requestJson<unknown>(
        `/v1/skills/store/items/${encodeURIComponent(input.candidateId)}/install`,
        {
          method: "POST",
          body: { packageFingerprint: input.packageFingerprint },
          signal: input.signal,
        },
      );
      return parse(
        z.object({ skill: installationSchema }).strict(),
        response,
        "install",
      ).skill;
    },

    async uninstallPackageSkill(input) {
      await httpClient.requestJson<void>(
        `/v1/skills/library/${encodeURIComponent(input.installationId)}?revision=${encodeURIComponent(String(input.revision))}`,
        { method: "DELETE", signal: input.signal },
      );
    },

    async getConversationSelection(conversationId, options = {}) {
      const response = await httpClient.requestJson<unknown>(
        `/v1/skills/conversations/${encodeURIComponent(conversationId)}/selection`,
        { signal: options.signal },
      );
      const selection = parse(
        z.object({ selection: conversationSelectionSchema }).strict(),
        response,
        "conversation selection",
      ).selection;
      if (selection.conversationId !== conversationId) {
        throw new ApiClientError(
          "INVALID_SERVER_RESPONSE",
          "Server returned a Skill selection for a different conversation.",
        );
      }
      return selection;
    },

    async replaceConversationSelection(input) {
      const response = await httpClient.requestJson<unknown>(
        `/v1/skills/conversations/${encodeURIComponent(input.conversationId)}/selection`,
        {
          method: "PUT",
          body: {
            revision: input.revision,
            installationIds: input.installationIds,
          },
          signal: input.signal,
        },
      );
      const selection = parse(
        z.object({ selection: conversationSelectionSchema }).strict(),
        response,
        "conversation selection",
      ).selection;
      if (selection.conversationId !== input.conversationId) {
        throw new ApiClientError(
          "INVALID_SERVER_RESPONSE",
          "Server returned a Skill selection for a different conversation.",
        );
      }
      return selection;
    },
  };
}
