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
const marketplaceIdentifier = z
  .string()
  .regex(/^[A-Za-z0-9][A-Za-z0-9._-]{0,255}$/);
const semver = z
  .string()
  .regex(
    /^(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/,
  );
const optionalHttpsUrl = z.string().url().startsWith("https://").optional();

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

const marketplaceSummarySchema = z
  .object({
    identifier: marketplaceIdentifier,
    name: z.string().min(1).max(256),
    description: z.string().max(2048),
    version: semver,
    category: z.string().max(128).optional(),
    author: z.string().max(256).optional(),
    icon: z.string().max(2048).optional(),
    license: z.string().max(128).optional(),
    homepage: optionalHttpsUrl,
    repositoryUrl: optionalHttpsUrl,
    installCount: count,
    rating: z.number().min(0).max(5),
    official: z.boolean(),
    validated: z.boolean(),
    featured: z.boolean(),
    resourceCount: count,
  })
  .strict();

const marketplaceDetailSchema = marketplaceSummarySchema
  .safeExtend({
    manifestName: skillName,
    summary: z.string().max(4096).optional(),
    permissions: z.array(z.string().min(1).max(256)).max(64),
    resources: z
      .array(
        z
          .object({
            path: z.string().min(1).max(512),
            sha256: z.string().min(1).max(128),
            size: count,
          })
          .strict(),
      )
      .max(512),
    versions: z
      .array(
        z
          .object({
            version: semver,
            latest: z.boolean(),
            validated: z.boolean(),
            createdAt: z.string(),
            versionRank: count,
          })
          .strict(),
      )
      .max(100),
    source: z.literal("lobehub"),
    sourceUrl: z.string().url().startsWith("https://lobehub.com/skills/"),
    installed: z.boolean(),
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
    async searchMarketplace(input = {}) {
      const query = new URLSearchParams({
        page: String(input.page ?? 1),
        pageSize: String(input.pageSize ?? 20),
      });
      if (input.query?.trim()) query.set("q", input.query.trim());
      if (input.category?.trim()) query.set("category", input.category.trim());
      if (input.locale) query.set("locale", input.locale);
      if (input.sort) query.set("sort", input.sort);
      const response = await httpClient.requestJson<unknown>(
        `/v1/skills/marketplace?${query}`,
        { signal: input.signal },
      );
      return parse(
        z
          .object({
            items: z.array(marketplaceSummarySchema),
            categories: z.array(
              z
                .object({ category: z.string().min(1).max(128), count })
                .strict(),
            ),
            page: positive,
            pageSize: positive,
            totalCount: count,
            totalPages: count,
            source: z.literal("lobehub"),
            sourceUrl: z.literal("https://lobehub.com/skills"),
          })
          .strict(),
        response,
        "marketplace list",
      );
    },

    async getMarketplaceSkill(identifier, options = {}) {
      const query = new URLSearchParams();
      if (options.version) query.set("version", options.version);
      if (options.locale) query.set("locale", options.locale);
      const suffix = query.size ? `?${query}` : "";
      const response = await httpClient.requestJson<unknown>(
        `/v1/skills/marketplace/items/${encodeURIComponent(identifier)}${suffix}`,
        { signal: options.signal },
      );
      return parse(
        z.object({ skill: marketplaceDetailSchema }).strict(),
        response,
        "marketplace detail",
      ).skill;
    },

    async installMarketplaceSkill(input) {
      const response = await httpClient.requestJson<unknown>(
        `/v1/skills/marketplace/items/${encodeURIComponent(input.identifier)}/install`,
        {
          method: "POST",
          body: { version: input.version },
          signal: input.signal,
        },
      );
      return parse(
        z.object({ skill: installationSchema }).strict(),
        response,
        "marketplace install",
      ).skill;
    },

    async installSkillLink(input) {
      const response = await httpClient.requestJson<unknown>(
        "/v1/skills/direct/install",
        { method: "POST", body: { url: input.url }, signal: input.signal },
      );
      return parse(
        z.object({ skill: installationSchema }).strict(),
        response,
        "direct install",
      ).skill;
    },

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
