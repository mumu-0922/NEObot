import type { BYOKPublicKeyResponse, ByokApi } from "../types";
import type { HttpClient } from "./httpClient";

export function createServerByokApiShell(httpClient: HttpClient): ByokApi {
  return {
    async getPublicKey(): Promise<BYOKPublicKeyResponse> {
      return httpClient.requestJson<BYOKPublicKeyResponse>(
        "/v1/byok/public-key",
      );
    },
  };
}
