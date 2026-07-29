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
    async getGovernance() {
      throw unsupportedFeature("server memory governance");
    },
    async listProjects() {
      throw unsupportedFeature("server memory governance");
    },
    async createProject() {
      throw unsupportedFeature("server memory governance");
    },
    async updateProject() {
      throw unsupportedFeature("server memory governance");
    },
    async getConversationPolicy() {
      throw unsupportedFeature("server memory governance");
    },
    async updateConversationPolicy() {
      throw unsupportedFeature("server memory governance");
    },
    async createGovernanceMemory() {
      throw unsupportedFeature("server memory governance");
    },
    async updateGovernanceMemory() {
      throw unsupportedFeature("server memory governance");
    },
    async deleteGovernanceMemory() {
      throw unsupportedFeature("server memory governance");
    },
    async getGovernanceMemoryDetail() {
      throw unsupportedFeature("server memory governance");
    },
    async getL2SceneDetail() {
      throw unsupportedFeature("server memory governance");
    },
    async setL2SceneEnabled() {
      throw unsupportedFeature("server memory governance");
    },
    async rebuildL2Scene() {
      throw unsupportedFeature("server memory governance");
    },
    async rebuildL2Scenes() {
      throw unsupportedFeature("server memory governance");
    },
    async getL3PersonaDetail() {
      throw unsupportedFeature("server L3 Persona governance");
    },
    async setL3PersonaEnabled() {
      throw unsupportedFeature("server L3 Persona governance");
    },
    async rebuildL3Persona() {
      throw unsupportedFeature("server L3 Persona governance");
    },
    async rebuildL3Personas() {
      throw unsupportedFeature("server L3 Persona governance");
    },
    async listMemoryReviews() {
      throw unsupportedFeature("server memory governance");
    },
    async decideMemoryReview() {
      throw unsupportedFeature("server memory governance");
    },
    async listMessageMemoryActivities() {
      throw unsupportedFeature("server memory governance");
    },
    async undoMemoryActivity() {
      throw unsupportedFeature("server memory governance");
    },
    async exportMemoryPackage() {
      throw unsupportedFeature("server memory portability");
    },
    async dryRunMemoryImport() {
      throw unsupportedFeature("server memory portability");
    },
    async confirmMemoryImport() {
      throw unsupportedFeature("server memory portability");
    },
  };
}
