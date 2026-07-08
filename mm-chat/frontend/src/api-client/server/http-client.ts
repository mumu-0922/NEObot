import { ApiClientError } from "../errors";
import type { ApiErrorEnvelope } from "../types";

export interface HttpClientOptions {
  baseUrl: string;
  fetchImpl?: typeof fetch;
  defaultHeaders?: Record<string, string>;
}

export interface JsonRequestOptions {
  method?: string;
  body?: unknown;
  headers?: Record<string, string>;
  signal?: AbortSignal;
}

export interface HttpClient {
  buildUrl(path: string): string;
  requestJson<T>(path: string, options?: JsonRequestOptions): Promise<T>;
}

export function createHttpClient(options: HttpClientOptions): HttpClient {
  const baseUrl = options.baseUrl.replace(/\/+$/, "");
  const fetchImpl = options.fetchImpl ?? fetch;

  return {
    buildUrl(path: string): string {
      return joinUrl(baseUrl, path);
    },

    async requestJson<T>(
      path: string,
      request: JsonRequestOptions = {},
    ): Promise<T> {
      let response: Response;
      try {
        response = await fetchImpl(joinUrl(baseUrl, path), {
          method:
            request.method ?? (request.body === undefined ? "GET" : "POST"),
          headers: {
            Accept: "application/json",
            ...(request.body === undefined
              ? {}
              : { "Content-Type": "application/json" }),
            ...options.defaultHeaders,
            ...request.headers,
          },
          body:
            request.body === undefined
              ? undefined
              : JSON.stringify(request.body),
          signal: request.signal,
        });
      } catch (error) {
        throw networkErrorFrom(error);
      }

      if (response.status === 204) return undefined as T;

      const text = await response.text();
      const data = parseJson(text);

      if (!response.ok) {
        throw errorFromResponse(response, data);
      }
      if (data === null) {
        throw new ApiClientError(
          "INVALID_SERVER_RESPONSE",
          "Server returned invalid JSON.",
          { status: response.status },
        );
      }
      return data as T;
    },
  };
}

export function joinUrl(baseUrl: string, path: string): string {
  const cleanPath = path.startsWith("/") ? path : `/${path}`;
  if (!baseUrl) return cleanPath;
  return `${baseUrl}${cleanPath}`;
}

function networkErrorFrom(error: unknown): ApiClientError {
  if (error instanceof DOMException && error.name === "AbortError") {
    return new ApiClientError("STREAM_INTERRUPTED", "Request was aborted.", {
      recoverable: true,
    });
  }

  if (error instanceof Error) {
    return new ApiClientError("NETWORK_ERROR", error.message, {
      recoverable: true,
    });
  }

  return new ApiClientError("NETWORK_ERROR", "Network request failed.", {
    recoverable: true,
  });
}

function parseJson(text: string): unknown | null {
  if (!text.trim()) return null;
  try {
    return JSON.parse(text) as unknown;
  } catch {
    return null;
  }
}

function errorFromResponse(response: Response, data: unknown): ApiClientError {
  const envelope = isErrorEnvelope(data) ? data : undefined;
  return new ApiClientError(
    envelope?.error.code ?? `HTTP_${response.status}`,
    envelope?.error.message ?? (response.statusText || "Request failed."),
    {
      status: response.status,
      recoverable: envelope?.error.recoverable,
      requestId:
        envelope?.error.requestId ??
        response.headers.get("x-request-id") ??
        response.headers.get("x-correlation-id") ??
        undefined,
    },
  );
}

function isErrorEnvelope(value: unknown): value is ApiErrorEnvelope {
  if (!value || typeof value !== "object") return false;
  const error = (value as { error?: unknown }).error;
  if (!error || typeof error !== "object") return false;
  const typed = error as { code?: unknown; message?: unknown };
  return typeof typed.code === "string" && typeof typed.message === "string";
}
