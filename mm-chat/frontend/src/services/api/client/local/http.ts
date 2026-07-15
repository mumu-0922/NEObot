import { ApiClientError } from "../errors";

interface LocalJsonOptions {
  method?: string;
  body?: unknown;
  headers?: Record<string, string>;
  signal?: AbortSignal;
}

export async function requestLocalJson<T>(
  path: string,
  options: LocalJsonOptions = {},
): Promise<T> {
  let response: Response;
  try {
    response = await fetch(path, {
      method: options.method ?? (options.body === undefined ? "GET" : "POST"),
      headers: {
        Accept: "application/json",
        ...(options.body === undefined
          ? {}
          : { "Content-Type": "application/json" }),
        ...options.headers,
      },
      body:
        options.body === undefined ? undefined : JSON.stringify(options.body),
      signal: options.signal,
      cache: "no-store",
    });
  } catch (error) {
    throw new ApiClientError(
      "NETWORK_ERROR",
      error instanceof Error ? error.message : "Network request failed.",
      { recoverable: true },
    );
  }

  const text = await response.text();
  const data = text ? parseJson(text) : undefined;
  if (!response.ok) {
    throw localErrorFromResponse(response, data);
  }
  return data as T;
}

function parseJson(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    throw new ApiClientError(
      "INVALID_SERVER_RESPONSE",
      "Server returned invalid JSON.",
    );
  }
}

function localErrorFromResponse(response: Response, data: unknown) {
  if (data && typeof data === "object") {
    const record = data as Record<string, unknown>;
    const nested = record.error;
    if (nested && typeof nested === "object") {
      const error = nested as Record<string, unknown>;
      return new ApiClientError(
        typeof error.code === "string" ? error.code : "REQUEST_FAILED",
        typeof error.message === "string" ? error.message : response.statusText,
        { status: response.status },
      );
    }
    return new ApiClientError(
      typeof record.code === "string" ? record.code : "REQUEST_FAILED",
      typeof record.message === "string"
        ? record.message
        : typeof record.error === "string"
          ? record.error
          : response.statusText,
      { status: response.status },
    );
  }

  return new ApiClientError("REQUEST_FAILED", response.statusText, {
    status: response.status,
  });
}
