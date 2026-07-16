import { ApiClientError } from "../errors";
import type {
  ApiPage,
  CreateTeamInput,
  CreateTeamInviteInput,
  ListTeamInvitesInput,
  ListTeamMembersInput,
  ListTeamsInput,
  RemoveTeamMemberInput,
  RevokeTeamInviteInput,
  TeamApi,
  TeamDTO,
  TeamInviteDTO,
  TeamLookupInput,
  TeamMemberDTO,
  UpdateTeamInput,
  UpdateTeamMemberInput,
} from "../types";
import type { HttpClient } from "./httpClient";

const teamsPath = "/v1/teams";

export function createServerTeamApiShell(httpClient: HttpClient): TeamApi {
  return {
    async createTeam(input: CreateTeamInput): Promise<TeamDTO> {
      return httpClient.requestJson<TeamDTO>(teamsPath, {
        method: "POST",
        body: {
          name: input.name,
          idempotencyKey: input.idempotencyKey,
        },
        signal: input.signal,
      });
    },

    async listTeams(input: ListTeamsInput = {}): Promise<ApiPage<TeamDTO>> {
      return httpClient.requestJson<ApiPage<TeamDTO>>(
        `${teamsPath}${pageQuery(input)}`,
        { signal: input.signal },
      );
    },

    async getTeam(input: TeamLookupInput): Promise<TeamDTO> {
      return httpClient.requestJson<TeamDTO>(teamPath(input.teamId), {
        signal: input.signal,
      });
    },

    async updateTeam(input: UpdateTeamInput): Promise<TeamDTO> {
      return httpClient.requestJson<TeamDTO>(teamPath(input.teamId), {
        method: "PATCH",
        body: { name: input.name },
        signal: input.signal,
      });
    },

    async listMembers(
      input: ListTeamMembersInput,
    ): Promise<ApiPage<TeamMemberDTO>> {
      return httpClient.requestJson<ApiPage<TeamMemberDTO>>(
        `${teamPath(input.teamId)}/members${pageQuery(input)}`,
        { signal: input.signal },
      );
    },

    async leaveTeam(input: TeamLookupInput): Promise<void> {
      await httpClient.requestJson<void>(
        `${teamPath(input.teamId)}/membership`,
        {
          method: "DELETE",
          signal: input.signal,
        },
      );
    },

    async updateMember(input: UpdateTeamMemberInput): Promise<TeamMemberDTO> {
      return httpClient.requestJson<TeamMemberDTO>(
        `${teamPath(input.teamId)}/members/${requiredPathId(
          input.userId,
          "user id",
        )}`,
        {
          method: "PATCH",
          body: { teamRole: input.teamRole },
          signal: input.signal,
        },
      );
    },

    async removeMember(input: RemoveTeamMemberInput): Promise<void> {
      await httpClient.requestJson<void>(
        `${teamPath(input.teamId)}/members/${requiredPathId(
          input.userId,
          "user id",
        )}`,
        { method: "DELETE", signal: input.signal },
      );
    },

    async createInvite(input: CreateTeamInviteInput): Promise<TeamInviteDTO> {
      return httpClient.requestJson<TeamInviteDTO>(
        `${teamPath(input.teamId)}/invites`,
        {
          method: "POST",
          body: {
            email: input.email,
            teamRole: input.teamRole,
            idempotencyKey: input.idempotencyKey,
          },
          signal: input.signal,
        },
      );
    },

    async listInvites(
      input: ListTeamInvitesInput,
    ): Promise<ApiPage<TeamInviteDTO>> {
      return httpClient.requestJson<ApiPage<TeamInviteDTO>>(
        `${teamPath(input.teamId)}/invites${pageQuery(input)}`,
        { signal: input.signal },
      );
    },

    async revokeInvite(input: RevokeTeamInviteInput): Promise<void> {
      await httpClient.requestJson<void>(
        `${teamPath(input.teamId)}/invites/${requiredPathId(
          input.inviteId,
          "invite id",
        )}`,
        { method: "DELETE", signal: input.signal },
      );
    },
  };
}

function teamPath(teamId: string): string {
  return `${teamsPath}/${requiredPathId(teamId, "team id")}`;
}

function pageQuery(input: { cursor?: string; limit?: number }): string {
  const params = new URLSearchParams();
  if (input.cursor !== undefined) params.set("cursor", input.cursor);
  if (input.limit !== undefined) params.set("limit", String(input.limit));
  const query = params.toString();
  return query ? `?${query}` : "";
}

function requiredPathId(value: string, label: string): string {
  const normalized = value.trim();
  if (!normalized) {
    throw new ApiClientError(
      "INVALID_RESOURCE_ID",
      `${label} is required for team API requests.`,
    );
  }
  return encodeURIComponent(normalized);
}
