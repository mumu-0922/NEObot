import { z } from "zod";

import { ApiClientError } from "../errors";
import type { ResourceApi } from "../types";
import type { HttpClient } from "./httpClient";

const string = z.string().min(1);
const serverRef = z
  .object({
    source: z.enum(["catalog", "manifest", "private"]),
    id: string,
  })
  .strict();

const catalogSchema = z
  .object({
    revision: z.string().regex(/^sha256:[a-f0-9]{64}$/),
    selectionMode: z.union([z.literal(""), z.enum(["inherit", "custom"])]),
    skills: z.array(
      z
        .object({
          installationId: string,
          name: string,
          version: string,
          packageFingerprint: z.string().regex(/^sha256:[a-f0-9]{64}$/),
          description: z.string(),
          allowedTools: z.array(z.string()),
          revision: z.number().int().positive(),
        })
        .strict(),
    ),
    mcpServers: z.array(
      z
        .object({
          ref: serverRef,
          name: string,
          description: z.string().optional(),
          authType: z.enum(["none", "header", "oauth", "env"]),
          status: z.enum([
            "draft",
            "ready",
            "needs_auth",
            "unavailable",
            "disabled",
          ]),
          credentialStatus: z.enum(["not_required", "required", "configured"]),
          toolCount: z.number().int().nonnegative(),
          unsupportedToolCount: z.number().int().nonnegative(),
          selected: z.boolean(),
          disabledTools: z.array(z.string()),
          canManage: z.boolean(),
        })
        .strict(),
    ),
  })
  .strict();

const searchSchema = z
  .object({
    kind: z.enum(["skill", "mcp"]),
    query: z.string(),
    items: z.array(
      z
        .object({
          kind: z.enum(["skill", "mcp"]),
          id: string,
          name: string,
          description: z.string(),
          version: z.string().optional(),
          exactRevision: string,
          status: string,
          source: string,
          authType: z.string().optional(),
          permissionScopes: z.array(z.string()),
        })
        .strict(),
    ),
  })
  .strict();

const installSchema = z
  .object({
    kind: z.enum(["skill", "mcp"]),
    id: string,
    name: string,
    revision: string,
    status: z.literal("installed"),
    refreshRequired: z.boolean(),
    mutationAuditId: z.string().uuid().optional(),
  })
  .strict();

const mutationSchema = z
  .object({
    kind: z.enum(["skill", "mcp"]),
    action: z.enum(["remove", "enable", "disable"]),
    id: string,
    name: string,
    revision: z.number().int().nonnegative(),
    status: z.enum(["removed", "enabled", "disabled"]),
    refreshRequired: z.boolean(),
    mutationAuditId: z.string().uuid().optional(),
  })
  .strict();

function parse<T>(schema: z.ZodType<T>, value: unknown): T {
  const result = schema.safeParse(value);
  if (!result.success) {
    throw new ApiClientError(
      "INVALID_SERVER_RESPONSE",
      "Server returned an invalid Resource response.",
    );
  }
  return result.data;
}

export function createServerResourceApiShell(
  httpClient: HttpClient,
): ResourceApi {
  return {
    async getCatalog(input) {
      const response = await httpClient.requestJson<unknown>(
        `/v1/resources?conversationId=${encodeURIComponent(input.conversationId)}`,
        { signal: input.signal },
      );
      return parse(catalogSchema, response);
    },

    async search(input) {
      const query = new URLSearchParams({ kind: input.kind, q: input.query });
      const response = await httpClient.requestJson<unknown>(
        `/v1/resources/search?${query}`,
        { signal: input.signal },
      );
      return parse(searchSchema, response);
    },

    async install(input) {
      const response = await httpClient.requestJson<unknown>(
        "/v1/resources/install",
        {
          method: "POST",
          body: {
            kind: input.kind,
            id: input.id,
            version: input.version ?? "",
            exactRevision: input.exactRevision,
            conversationId: input.conversationId,
          },
          signal: input.signal,
        },
      );
      return parse(installSchema, response);
    },

    async mutate(input) {
      const response = await httpClient.requestJson<unknown>(
        "/v1/resources/mutate",
        {
          method: "POST",
          body: {
            kind: input.kind,
            action: input.action,
            id: input.id,
            expectedRevision: input.expectedRevision,
            conversationId: input.conversationId,
          },
          signal: input.signal,
        },
      );
      return parse(mutationSchema, response);
    },
  };
}
