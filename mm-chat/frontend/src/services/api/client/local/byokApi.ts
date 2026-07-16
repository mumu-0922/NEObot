import { unsupportedFeature } from "../errors";
import type { BYOKPublicKeyResponse, ByokApi } from "../types";

export function createLocalByokApiShell(): ByokApi {
  return {
    async getPublicKey(): Promise<BYOKPublicKeyResponse> {
      throw unsupportedFeature(
        "local BYOK public-key loading after G9.3 route removal",
      );
    },
  };
}
