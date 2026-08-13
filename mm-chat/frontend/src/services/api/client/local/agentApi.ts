import { unsupportedFeature } from "../errors";
import type {
  AgentApi,
  AgentDetailInput,
  AgentListInput,
  AgentListResponse,
} from "../types";

export function createLocalAgentApiShell(): AgentApi {
  return {
    async listAgents(input: AgentListInput = {}): Promise<AgentListResponse> {
      void input;
      throw unsupportedFeature("local agent catalog after G9.4 route removal");
    },

    async getAgentDetail(input: AgentDetailInput): Promise<unknown> {
      void input;
      throw unsupportedFeature("local agent detail after G9.4 route removal");
    },
    async listLibrary() {
      throw unsupportedFeature("server-owned assistant library");
    },
    async getLibraryEntry() {
      throw unsupportedFeature("server-owned assistant library");
    },
    async createCustom() {
      throw unsupportedFeature("server-owned assistant library");
    },
    async updateCustom() {
      throw unsupportedFeature("server-owned assistant library");
    },
    async deleteLibraryEntry() {
      throw unsupportedFeature("server-owned assistant library");
    },
    async copyToCustom() {
      throw unsupportedFeature("server-owned assistant library");
    },
    async updateInstalled() {
      throw unsupportedFeature("server-owned assistant library");
    },
    async searchMarket() {
      throw unsupportedFeature("server-owned assistant market");
    },
    async getMarketDetail() {
      throw unsupportedFeature("server-owned assistant market");
    },
    async installMarket() {
      throw unsupportedFeature("server-owned assistant market");
    },
    async reviewMarket() {
      throw unsupportedFeature("server-owned assistant market");
    },
  };
}
