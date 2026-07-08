import { ApiClientError } from "../errors";
import { createSseStreamParser, type ParsedSseFrame } from "./sse";
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

export interface SseRequestOptions extends JsonRequestOptions {
  onFrame: (frame: ParsedSseFrame) => void;
}

export interface HttpClient {
  buildUrl(path: string): string;
  requestJson<T>(path: string, options?: JsonRequestOptions): Promise<T>;
  requestSse(path: string, options: SseRequestOptions): Promise<void>;
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

    async requestSse(path: string, request: SseRequestOptions): Promise<void> {
      let response: Response;
      try {
        response = await fetchImpl(joinUrl(baseUrl, path), {
          method: request.method ?? "POST",
          headers: {
            Accept: "text/event-stream",
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

      if (!response.ok) {
        const text = await response.text();
        throw errorFromResponse(response, parseJson(text));
      }

      const contentType = response.headers.get("content-type") ?? "";
      if (!contentType.toLowerCase().includes("text/event-stream")) {
        throw new ApiClientError(
          "INVALID_SERVER_RESPONSE",
          "Server returned a non-SSE stream response.",
          { status: response.status },
        );
      }

      if (!response.body) {
        throw new ApiClientError(
          "INVALID_SERVER_RESPONSE",
          "Server returned an empty stream response.",
          { status: response.status },
        );
      }

      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      const parser = createSseStreamParser();

      try {
        while (true) {
          const { value, done } = await reader.read();
          if (done) break;
          if (!value) continue;
          const chunk = decoder.decode(value, { stream: true });
          for (const frame of parser.push(chunk)) {
            request.onFrame(frame);
          }
        }

        const tail = decoder.decode();
        if (tail) {
          for (const frame of parser.push(tail)) {
            request.onFrame(frame);
          }
        }
        for (const frame of parser.flush()) {
          request.onFrame(frame);
        }
      } catch (error) {
        throw networkErrorFrom(error);
      } finally {
        reader.releaseLock();
      }
    },
  };
}

export function joinUrl(baseUrl: string, path: string): string {
  const cleanPath = path.startsWith("/") ? path : `/${path}`;
  if (!baseUrl) return cleanPath;
  return `${baseUrl}${cleanPath}`;
}

function networkErrorFrom(error: unknown): ApiClientError {
  if (error instanceof ApiClientError) {
    return error;
  }

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
