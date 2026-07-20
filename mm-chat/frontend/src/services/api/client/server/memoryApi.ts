import type {
  ApiPage,
  DurableMemorySettingsDTO,
  MemoryApi,
  MemoryMutationInput,
  UpdateDurableMemorySettingsInput,
  UpdateMemoryInput,
} from "../types";
import type { MemoryRecord } from "../../../../lib/memory/types";
import type { HttpClient } from "./httpClient";

const memoriesPath = "/v1/memories";
const memorySettingsPath = "/v1/memory-settings";

export function createServerMemoryApiShell(httpClient: HttpClient): MemoryApi {
  return {
    async listMemories(input = {}): Promise<MemoryRecord[]> {
      const page = await httpClient.requestJson<ApiPage<MemoryRecord>>(
        memoriesPath,
        { signal: input.signal },
      );
      return Array.isArray(page.items) ? page.items : [];
    },

    async createMemory(input: MemoryMutationInput): Promise<MemoryRecord> {
      return httpClient.requestJson<MemoryRecord>(memoriesPath, {
        method: "POST",
        body: memoryBody(input),
        signal: input.signal,
      });
    },

    async updateMemory(input: UpdateMemoryInput): Promise<MemoryRecord> {
      return httpClient.requestJson<MemoryRecord>(memoryPath(input.memoryId), {
        method: "PATCH",
        body: memoryBody(input),
        signal: input.signal,
      });
    },

    async deleteMemory(input): Promise<void> {
      await httpClient.requestJson<void>(memoryPath(input.memoryId), {
        method: "DELETE",
        signal: input.signal,
      });
    },

    async getSettings(input = {}): Promise<DurableMemorySettingsDTO> {
      return httpClient.requestJson<DurableMemorySettingsDTO>(
        memorySettingsPath,
        { signal: input.signal },
      );
    },

    async updateSettings(
      input: UpdateDurableMemorySettingsInput,
    ): Promise<DurableMemorySettingsDTO> {
      const { signal, ...body } = input;
      return httpClient.requestJson<DurableMemorySettingsDTO>(
        memorySettingsPath,
        { method: "PATCH", body, signal },
      );
    },
  };
}

function memoryPath(memoryId: string): string {
  return `${memoriesPath}/${encodeURIComponent(memoryId)}`;
}

function memoryBody(input: MemoryMutationInput) {
  return {
    type: input.type,
    content: input.content,
    importance: input.importance ?? 3,
    tags: input.tags ?? [],
  };
}
