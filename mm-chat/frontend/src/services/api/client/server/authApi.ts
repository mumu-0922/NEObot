import type {
  AcceptInviteInput,
  AuthApi,
  AuthenticatedRequestInput,
  AuthUserDTO,
  CompleteRecoveryInput,
  LoginInput,
  LoginResult,
  RecoveryRequestInput,
} from "../types";
import type { HttpClient } from "./httpClient";

function authHeaders(
  input?: AuthenticatedRequestInput,
): Record<string, string> {
  return input?.token ? { Authorization: `Bearer ${input.token}` } : {};
}

export function createServerAuthApiShell(httpClient: HttpClient): AuthApi {
  return {
    async getCurrentUser(
      input?: AuthenticatedRequestInput,
    ): Promise<AuthUserDTO> {
      return httpClient.requestJson<AuthUserDTO>("/v1/me", {
        headers: authHeaders(input),
      });
    },

    async login(input: LoginInput): Promise<LoginResult> {
      return httpClient.requestJson<LoginResult>("/v1/auth/login", {
        method: "POST",
        body: { email: input.email, password: input.password },
      });
    },

    async acceptInvite(input: AcceptInviteInput): Promise<LoginResult> {
      return httpClient.requestJson<LoginResult>("/v1/auth/invites/accept", {
        method: "POST",
        body: input,
      });
    },

    async requestRecovery(input: RecoveryRequestInput): Promise<void> {
      await httpClient.requestJson<void>("/v1/auth/recovery/request", {
        method: "POST",
        body: input,
      });
    },

    async completeRecovery(input: CompleteRecoveryInput): Promise<void> {
      await httpClient.requestJson<void>("/v1/auth/recovery/complete", {
        method: "POST",
        body: input,
      });
    },

    async logout(input?: AuthenticatedRequestInput): Promise<void> {
      await httpClient.requestJson<void>("/v1/auth/logout", {
        method: "POST",
        headers: authHeaders(input),
      });
    },

    async revokeAllSessions(input?: AuthenticatedRequestInput): Promise<void> {
      await httpClient.requestJson<void>("/v1/me/sessions", {
        method: "DELETE",
        headers: authHeaders(input),
      });
    },
  };
}
