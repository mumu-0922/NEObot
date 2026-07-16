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
  };
}
