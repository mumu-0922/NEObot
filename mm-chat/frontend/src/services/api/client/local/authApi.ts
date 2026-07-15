import { unsupportedFeature } from "../errors";
import type { AuthApi, AuthUserDTO, LoginInput, LoginResult } from "../types";
import { requestLocalJson } from "./http";

const localUser: AuthUserDTO = {
  id: "local-user",
  displayName: "Local User",
  role: "owner",
};

export function createLocalAuthApiShell(): AuthApi {
  return {
    async getCurrentUser(): Promise<AuthUserDTO> {
      return localUser;
    },

    async login(input: LoginInput): Promise<LoginResult> {
      await requestLocalJson<{ ok?: boolean }>("/api/access/verify", {
        method: "POST",
        body: { password: input.password },
      });
      return { user: localUser };
    },

    async acceptInvite(): Promise<LoginResult> {
      throw unsupportedFeature("local invite acceptance");
    },

    async requestRecovery(): Promise<void> {
      throw unsupportedFeature("local password recovery request");
    },

    async completeRecovery(): Promise<void> {
      throw unsupportedFeature("local password recovery completion");
    },

    async logout(): Promise<void> {
      return undefined;
    },

    async revokeAllSessions(): Promise<void> {
      throw unsupportedFeature("local session revocation");
    },
  };
}
