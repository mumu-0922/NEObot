import { z } from "zod";

import { ApiClientError } from "../errors";
import type { SkillStoreApi } from "../types";
import type { HttpClient } from "./httpClient";

const string = z.string().min(1);
const count = z.number().int().nonnegative();
const positive = z.number().int().positive();
const fingerprint = z.string().regex(/^sha256:[a-f0-9]{64}$/);

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
  };
}
