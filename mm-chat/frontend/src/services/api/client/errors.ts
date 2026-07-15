import type { ApiErrorEnvelope, UnsupportedFeatureCode } from "./types";

export class ApiClientError extends Error {
  readonly code: string;
  readonly recoverable: boolean;
  readonly status?: number;
  readonly requestId?: string;

  constructor(
    code: string,
    message: string,
    options: {
      recoverable?: boolean;
      status?: number;
      requestId?: string;
    } = {},
  ) {
    super(message);
    this.name = "ApiClientError";
    this.code = code;
    this.recoverable = options.recoverable ?? false;
    this.status = options.status;
    this.requestId = options.requestId;
  }

  toEnvelope(): ApiErrorEnvelope {
    return {
      error: {
        code: this.code,
        message: this.message,
        recoverable: this.recoverable || undefined,
        requestId: this.requestId,
      },
    };
  }
}

export function unsupportedFeature(
  feature: string,
  code: UnsupportedFeatureCode = "FEATURE_NOT_IMPLEMENTED",
): ApiClientError {
  return new ApiClientError(
    code,
    `${feature} is not implemented in the Phase 11.1 adapter scaffold.`,
  );
}

export function normalizeUnknownError(error: unknown): ApiClientError {
  if (error instanceof ApiClientError) return error;
  if (error instanceof Error) {
    return new ApiClientError("UNKNOWN_ERROR", error.message);
  }
  return new ApiClientError("UNKNOWN_ERROR", "Unknown API client error.");
}
