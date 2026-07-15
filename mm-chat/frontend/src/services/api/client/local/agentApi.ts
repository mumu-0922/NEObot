import type {
  AgentApi,
  AgentDetailInput,
  AgentListInput,
  AgentListResponse,
} from "../types";

function localeQuery(locale: AgentListInput["locale"]): string {
  return locale ? `?locale=${encodeURIComponent(locale)}` : "";
}

export function createLocalAgentApiShell(): AgentApi {
  return {
    async listAgents(input: AgentListInput = {}): Promise<AgentListResponse> {
      const response = await fetch(`/api/agents${localeQuery(input.locale)}`);
      if (!response.ok) throw new Error("Failed to fetch agents");
      return (await response.json()) as AgentListResponse;
    },

    async getAgentDetail(input: AgentDetailInput): Promise<unknown> {
      const response = await fetch(
        `/api/agents/${encodeURIComponent(input.identifier)}${localeQuery(
          input.locale,
        )}`,
      );
      if (!response.ok) throw new Error("Failed to fetch agent details");
      return response.json();
    },
  };
}
