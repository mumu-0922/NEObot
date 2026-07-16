import { unsupportedFeature } from "../errors";
import type {
  ApiPage,
  TeamApi,
  TeamDTO,
  TeamInviteDTO,
  TeamMemberDTO,
} from "../types";

export function createLocalTeamApiShell(): TeamApi {
  return {
    async createTeam(): Promise<TeamDTO> {
      throw unsupportedFeature("local team adapter wiring");
    },
    async listTeams(): Promise<ApiPage<TeamDTO>> {
      throw unsupportedFeature("local team adapter wiring");
    },
    async getTeam(): Promise<TeamDTO> {
      throw unsupportedFeature("local team adapter wiring");
    },
    async updateTeam(): Promise<TeamDTO> {
      throw unsupportedFeature("local team adapter wiring");
    },
    async listMembers(): Promise<ApiPage<TeamMemberDTO>> {
      throw unsupportedFeature("local team adapter wiring");
    },
    async leaveTeam(): Promise<void> {
      throw unsupportedFeature("local team adapter wiring");
    },
    async updateMember(): Promise<TeamMemberDTO> {
      throw unsupportedFeature("local team adapter wiring");
    },
    async removeMember(): Promise<void> {
      throw unsupportedFeature("local team adapter wiring");
    },
    async createInvite(): Promise<TeamInviteDTO> {
      throw unsupportedFeature("local team adapter wiring");
    },
    async listInvites(): Promise<ApiPage<TeamInviteDTO>> {
      throw unsupportedFeature("local team adapter wiring");
    },
    async revokeInvite(): Promise<void> {
      throw unsupportedFeature("local team adapter wiring");
    },
  };
}
