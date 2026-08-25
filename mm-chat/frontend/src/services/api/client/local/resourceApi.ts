import { ApiClientError } from "../errors";
import type { ResourceApi } from "../types";

export function createLocalResourceApiShell(): ResourceApi {
  const unavailable = async (): Promise<never> => {
    throw new ApiClientError(
      "SERVER_MODE_REQUIRED",
      "Resources require Server mode.",
    );
  };
  return {
    getCatalog: unavailable,
    search: unavailable,
    install: unavailable,
    mutate: unavailable,
  };
}
