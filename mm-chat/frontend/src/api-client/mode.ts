import type {
  ApiCapabilities,
  ApiClientConfig,
  ApiClientEnv,
  ApiMode,
  NetworkEdge,
  ResolvedApiClientConfig,
} from "./types";

const DEFAULT_DIRECT_SERVER_BASE_URL = "http://127.0.0.1:8080";

export const phase11Capabilities: ApiCapabilities = {
  chatCrud: false,
  chatStream: false,
  files: false,
  auth: false,
  imports: false,
  rag: false,
  plugins: false,
  providerSettings: false,
};

export function normalizeApiMode(value: string | undefined): ApiMode {
  return value === "server" ? "server" : "local";
}

export function readApiClientEnv(
  env: ApiClientEnv | undefined = getProcessEnv(),
): Required<ApiClientEnv> {
  return {
    NEXT_PUBLIC_API_MODE: env?.NEXT_PUBLIC_API_MODE ?? "",
    NEXT_PUBLIC_API_BASE_URL: env?.NEXT_PUBLIC_API_BASE_URL ?? "",
  };
}

export function resolveApiClientConfig(
  input: ApiClientConfig = {},
): ResolvedApiClientConfig {
  const env = readApiClientEnv(input.env);
  const requestedMode = (input.mode ?? env.NEXT_PUBLIC_API_MODE).trim();
  const requestedApiMode = normalizeApiMode(requestedMode);
  const baseUrl = normalizeBaseUrl(
    input.baseUrl ?? env.NEXT_PUBLIC_API_BASE_URL,
  );
  const warnings: string[] = [];

  if (
    requestedMode &&
    requestedMode !== "local" &&
    requestedMode !== "server"
  ) {
    warnings.push(`Unsupported API mode "${requestedMode}"; using local mode.`);
  }

  if (requestedApiMode === "server" && !baseUrl) {
    warnings.push("Server mode requested without NEXT_PUBLIC_API_BASE_URL.");
  }

  const mode: ApiMode =
    requestedApiMode === "server" && !baseUrl ? "local" : requestedApiMode;

  return {
    mode,
    requestedMode: requestedMode || "local",
    baseUrl,
    networkEdge:
      input.networkEdge ?? inferNetworkEdge(baseUrl, input.frontendOrigin),
    serverConfigured: mode === "server",
    warnings,
  };
}

export function normalizeBaseUrl(value: string | undefined): string {
  const trimmed = value?.trim() ?? "";
  if (!trimmed) return "";
  if (trimmed === "/") return "";
  return trimmed.replace(/\/+$/, "");
}

export function inferNetworkEdge(
  baseUrl: string,
  frontendOrigin?: string,
): NetworkEdge {
  if (!baseUrl) return "same-origin-proxy";
  if (baseUrl.startsWith("/")) return "same-origin-proxy";

  try {
    const backend = new URL(baseUrl);
    if (!frontendOrigin) return "direct-cors";
    const frontend = new URL(frontendOrigin);
    return backend.origin === frontend.origin
      ? "same-origin-proxy"
      : "direct-cors";
  } catch {
    return "same-origin-proxy";
  }
}

export function defaultServerBaseUrl(): string {
  return DEFAULT_DIRECT_SERVER_BASE_URL;
}

function getProcessEnv(): ApiClientEnv | undefined {
  const runtime = globalThis as typeof globalThis & {
    process?: { env?: ApiClientEnv };
  };
  return runtime.process?.env;
}
