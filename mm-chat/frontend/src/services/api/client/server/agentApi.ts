import type {
  AgentApi,
  AgentDetailInput,
  AgentListInput,
  AgentListResponse,
} from "../types";
import type { HttpClient } from "./httpClient";

const agentsPath = "/v1/agents";

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
        `${agentsPath}/${encodeURIComponent(input.identifier)}${localeQuery(
          input.locale,
        )}`,
      );
    },
  };
}
