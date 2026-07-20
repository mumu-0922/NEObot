import { unsupportedFeature } from "../errors";
import type { DurableMemorySettingsDTO, MemoryApi } from "../types";
import type { MemoryRecord } from "../../../../lib/memory/types";

export function createLocalMemoryApiShell(): MemoryApi {
  return {
    async listMemories(): Promise<MemoryRecord[]> {
      throw unsupportedFeature("local memory adapter wiring");
    },
    async createMemory(): Promise<MemoryRecord> {
      throw unsupportedFeature("local memory adapter wiring");
    },
    async updateMemory(): Promise<MemoryRecord> {
      throw unsupportedFeature("local memory adapter wiring");
    },
    async deleteMemory(): Promise<void> {
      throw unsupportedFeature("local memory adapter wiring");
    },
    async getSettings(): Promise<DurableMemorySettingsDTO> {
      throw unsupportedFeature("local memory adapter wiring");
    },
    async updateSettings(): Promise<DurableMemorySettingsDTO> {
      throw unsupportedFeature("local memory adapter wiring");
    },
  };
}
