import type {
  AgentApi,
  AgentDetailInput,
  AgentListInput,
  AgentListResponse,
  AssistantDraftInput,
} from "../types";
import type {
  AssistantLibraryEntry,
  AssistantMarketCategory,
  AssistantMarketSearchResult,
  LobeAgent,
} from "@/lib/assistant/types";
import { ApiClientError } from "../errors";
import type { HttpClient } from "./httpClient";

const agentsPath = "/v1/agents";
const assistantsPath = "/v1/assistants";

function localeQuery(locale: AgentListInput["locale"]): string {
  return locale ? `?locale=${encodeURIComponent(locale)}` : "";
}

export function createServerAgentApiShell(httpClient: HttpClient): AgentApi {
  return {
    async listAgents(input: AgentListInput = {}): Promise<AgentListResponse> {
      return httpClient.requestJson<AgentListResponse>(
        `${agentsPath}${localeQuery(input.locale)}`,
      );
    },

    async getAgentDetail(input: AgentDetailInput): Promise<unknown> {
      return httpClient.requestJson<unknown>(
        `${agentsPath}/${encodeURIComponent(input.identifier)}${localeQuery(input.locale)}`,
      );
    },

    async listLibrary(options = {}) {
      const response = await httpClient.requestJson<unknown>(
        `${assistantsPath}/library`,
        { signal: options.signal },
      );
      const record = object(response);
      return array(record?.assistants).map(requireLibraryEntry);
    },

    async getLibraryEntry(assistantId, options = {}) {
      const response = await httpClient.requestJson<unknown>(
        `${assistantsPath}/library/${encodeURIComponent(assistantId)}`,
        { signal: options.signal },
      );
      return requireLibraryEntry(object(response)?.assistant);
    },

    async createCustom(input) {
      const response = await httpClient.requestJson<unknown>(
        `${assistantsPath}/library`,
        {
          method: "POST",
          body: draftBody(input),
          signal: input.signal,
        },
      );
      return requireLibraryEntry(object(response)?.assistant);
    },

    async updateCustom(input) {
      const response = await httpClient.requestJson<unknown>(
        `${assistantsPath}/library/${encodeURIComponent(input.assistantId)}`,
        {
          method: "PUT",
          body: {
            expectedRevision: input.expectedRevision,
            ...draftBody(input),
          },
          signal: input.signal,
        },
      );
      return requireLibraryEntry(object(response)?.assistant);
    },

    async deleteLibraryEntry(input) {
      await httpClient.requestJson<void>(
        `${assistantsPath}/library/${encodeURIComponent(input.assistantId)}?revision=${encodeURIComponent(String(input.revision))}`,
        { method: "DELETE", signal: input.signal },
      );
    },

    async copyToCustom(input) {
      const response = await httpClient.requestJson<unknown>(
        `${assistantsPath}/library/${encodeURIComponent(input.assistantId)}/copy`,
        {
          method: "POST",
          body: { expectedRevision: input.expectedRevision },
          signal: input.signal,
        },
      );
      return requireLibraryEntry(object(response)?.assistant);
    },

    async updateInstalled(input) {
      const response = await httpClient.requestJson<unknown>(
        `${assistantsPath}/library/${encodeURIComponent(input.assistantId)}/update`,
        {
          method: "POST",
          body: { expectedRevision: input.expectedRevision },
          signal: input.signal,
        },
      );
      return requireLibraryEntry(object(response)?.assistant);
    },

    async searchMarket(input) {
      const query = new URLSearchParams({
        page: String(input.page),
        pageSize: String(input.pageSize),
      });
      if (input.query) query.set("q", input.query);
      if (input.category) query.set("category", input.category);
      if (input.locale) query.set("locale", input.locale);
      const response = await httpClient.requestJson<unknown>(
        `${assistantsPath}/market?${query}`,
        { signal: input.signal },
      );
      return requireMarketSearch(response);
    },

    async getMarketDetail(input) {
      const response = await httpClient.requestJson<unknown>(
        `${assistantsPath}/market/items/${encodeURIComponent(input.identifier)}${localeQuery(input.locale)}`,
        { signal: input.signal },
      );
      const record = object(response);
      if (!record) invalid("market detail");
      return {
        assistant: requireMarketAgent(record.assistant),
        canReview: record.canReview === true,
      };
    },

    async installMarket(input) {
      const response = await httpClient.requestJson<unknown>(
        `${assistantsPath}/market/items/${encodeURIComponent(input.identifier)}/install`,
        {
          method: "POST",
          body: { fingerprint: input.fingerprint },
          signal: input.signal,
        },
      );
      return requireLibraryEntry(object(response)?.assistant);
    },

    async reviewMarket(input) {
      const locale = input.locale
        ? `?locale=${encodeURIComponent(input.locale)}`
        : "";
      await httpClient.requestJson<unknown>(
        `${assistantsPath}/market/items/${encodeURIComponent(input.identifier)}/review${locale}`,
        {
          method: "POST",
          body: { status: input.status, fingerprint: input.fingerprint },
          signal: input.signal,
        },
      );
    },
  };
}

function draftBody(input: AssistantDraftInput) {
  return {
    avatar: input.avatar,
    title: input.title,
    description: input.description,
    category: input.category,
    tags: input.tags,
    systemPrompt: input.systemPrompt,
  };
}

function requireLibraryEntry(value: unknown): AssistantLibraryEntry {
  const raw = object(value);
  if (
    !raw ||
    typeof raw.id !== "string" ||
    (raw.source !== "custom" && raw.source !== "lobehub") ||
    typeof raw.title !== "string" ||
    typeof raw.systemPrompt !== "string" ||
    typeof raw.revision !== "number" ||
    typeof raw.contentFingerprint !== "string" ||
    (raw.requiredTools !== undefined && !stringArray(raw.requiredTools))
  ) {
    invalid("library entry");
  }
  return {
    id: raw.id as string,
    source: raw.source as "custom" | "lobehub",
    ...(typeof raw.sourceIdentifier === "string"
      ? { sourceIdentifier: raw.sourceIdentifier }
      : {}),
    avatar: string(raw.avatar, "🤖"),
    title: raw.title as string,
    description: string(raw.description),
    category: string(raw.category, "general"),
    tags: array(raw.tags).filter(
      (tag): tag is string => typeof tag === "string",
    ),
    systemPrompt: raw.systemPrompt as string,
    author: string(raw.author),
    homepage: string(raw.homepage),
    ...(typeof raw.sourceVersion === "string"
      ? { sourceVersion: raw.sourceVersion }
      : {}),
    ...(typeof raw.sourceUpdatedAt === "string"
      ? { sourceUpdatedAt: raw.sourceUpdatedAt }
      : {}),
    ...(Array.isArray(raw.requiredTools)
      ? { requiredTools: raw.requiredTools as string[] }
      : {}),
    contentFingerprint: raw.contentFingerprint as string,
    revision: raw.revision as number,
    createdAt: string(raw.createdAt),
    updatedAt: string(raw.updatedAt),
    updateAvailable: raw.updateAvailable === true,
  };
}

function requireMarketSearch(value: unknown): AssistantMarketSearchResult {
  const raw = object(value);
  if (
    !raw ||
    !Array.isArray(raw.agents) ||
    !Array.isArray(raw.categories) ||
    typeof raw.page !== "number" ||
    typeof raw.pageSize !== "number" ||
    typeof raw.totalCount !== "number" ||
    typeof raw.totalPages !== "number" ||
    typeof raw.source !== "string"
  ) {
    invalid("market search");
  }
  if (
    !integer(raw.page, 1) ||
    !integer(raw.pageSize, 1) ||
    !integer(raw.totalCount, 0) ||
    !integer(raw.totalPages, 0)
  ) {
    invalid("market search pagination");
  }
  const totalMarketCount = raw.totalMarketCount;
  if (totalMarketCount !== undefined && !integer(totalMarketCount, 0)) {
    invalid("market search total");
  }
  return {
    agents: raw.agents.map(requireMarketAgent),
    categories: raw.categories.map(requireMarketCategory),
    page: raw.page as number,
    pageSize: raw.pageSize as number,
    totalCount: raw.totalCount as number,
    totalPages: raw.totalPages as number,
    ...(typeof totalMarketCount === "number" ? { totalMarketCount } : {}),
    source: raw.source as string,
    unavailable: raw.unavailable === true,
    canReview: raw.canReview === true,
  };
}

function requireMarketAgent(value: unknown): LobeAgent {
  const raw = object(value);
  const meta = object(raw?.meta);
  if (
    !raw ||
    !meta ||
    typeof raw.identifier !== "string" ||
    typeof meta.avatar !== "string" ||
    typeof meta.title !== "string" ||
    typeof meta.description !== "string" ||
    typeof meta.category !== "string" ||
    !stringArray(meta.tags) ||
    typeof raw.createdAt !== "string" ||
    typeof raw.homepage !== "string" ||
    typeof raw.author !== "string"
  ) {
    invalid("market agent");
  }
  const optionalStrings = [
    meta.systemRole,
    raw.updatedAt,
    raw.version,
    raw.fingerprint,
    raw.safetyCheck,
    raw.libraryId,
  ];
  if (
    optionalStrings.some(
      (item) => item !== undefined && typeof item !== "string",
    ) ||
    (raw.fingerprint !== undefined &&
      !/^[0-9a-f]{64}$/.test(raw.fingerprint as string)) ||
    (raw.requiredTools !== undefined && !stringArray(raw.requiredTools)) ||
    (raw.installCount !== undefined && !integer(raw.installCount, 0))
  ) {
    invalid("market agent fields");
  }
  return {
    identifier: raw.identifier as string,
    meta: {
      avatar: meta.avatar as string,
      title: meta.title as string,
      description: meta.description as string,
      category: meta.category as string,
      tags: meta.tags as string[],
      ...(typeof meta.systemRole === "string"
        ? { systemRole: meta.systemRole }
        : {}),
    },
    createdAt: raw.createdAt as string,
    homepage: raw.homepage as string,
    author: raw.author as string,
    ...(typeof raw.updatedAt === "string" ? { updatedAt: raw.updatedAt } : {}),
    ...(typeof raw.version === "string" ? { version: raw.version } : {}),
    ...(typeof raw.fingerprint === "string"
      ? { fingerprint: raw.fingerprint }
      : {}),
    ...(typeof raw.installCount === "number"
      ? { installCount: raw.installCount }
      : {}),
    ...(typeof raw.safetyCheck === "string"
      ? { safetyCheck: raw.safetyCheck }
      : {}),
    ...(Array.isArray(raw.requiredTools)
      ? { requiredTools: raw.requiredTools as string[] }
      : {}),
    ...(typeof raw.libraryId === "string" ? { libraryId: raw.libraryId } : {}),
    isCustom: raw.isCustom === true,
    isValidated: raw.isValidated === true,
    admitted: raw.admitted === true,
    installed: raw.installed === true,
  };
}

function requireMarketCategory(value: unknown): AssistantMarketCategory {
  const raw = object(value);
  if (!raw || typeof raw.id !== "string" || !integer(raw.count, 0)) {
    invalid("market category");
  }
  return { id: raw.id as string, count: raw.count as number };
}

function object(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

function array(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function stringArray(value: unknown): value is string[] {
  return (
    Array.isArray(value) && value.every((item) => typeof item === "string")
  );
}

function integer(value: unknown, minimum: number): value is number {
  return (
    typeof value === "number" && Number.isInteger(value) && value >= minimum
  );
}

function string(value: unknown, fallback = ""): string {
  return typeof value === "string" ? value : fallback;
}

function invalid(subject: string): never {
  throw new ApiClientError(
    "INVALID_SERVER_RESPONSE",
    `Server returned an invalid assistant ${subject}.`,
  );
}
