import type { BYOKPublicKeyResponse, ByokApi } from "../types";
import { requestLocalJson } from "./http";

export function createLocalByokApiShell(): ByokApi {
  return {
    async getPublicKey(): Promise<BYOKPublicKeyResponse> {
      return requestLocalJson<BYOKPublicKeyResponse>("/api/byok/public-key", {
        method: "GET",
      });
    },
  };
}
