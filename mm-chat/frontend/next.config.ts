import type { NextConfig } from "next";
import createNextIntlPlugin from "next-intl/plugin";
import { initOpenNextCloudflareForDev } from "@opennextjs/cloudflare";
import { getSecurityHeaders } from "./src/lib/security/headers";

initOpenNextCloudflareForDev();

const withNextIntl = createNextIntlPlugin("./src/i18n/request.ts");
const backendInternalUrl = resolveBackendInternalUrl(
  process.env.MM_CHAT_BACKEND_INTERNAL_URL,
);

const nextConfig: NextConfig = {
  /* config options here */
  output: "standalone",
  outputFileTracingRoot: process.cwd(),
  reactCompiler: true,
  turbopack: {
    root: process.cwd(),
  },
  experimental: {
    optimizePackageImports: ["lucide-react"],
    proxyTimeout: 300_000,
  },
  async headers() {
    return [
      {
        source: "/:path*",
        headers: getSecurityHeaders(),
      },
    ];
  },
  async rewrites() {
    return [
      {
        source: "/mm-api/:path*",
        destination: `${backendInternalUrl}/:path*`,
      },
    ];
  },
};

export default withNextIntl(nextConfig);

function resolveBackendInternalUrl(value: string | undefined): string {
  const candidate = value?.trim() || "http://127.0.0.1:8080";
  const parsed = new URL(candidate);

  if (
    (parsed.protocol !== "http:" && parsed.protocol !== "https:") ||
    parsed.username ||
    parsed.password ||
    parsed.search ||
    parsed.hash ||
    (parsed.pathname !== "/" && parsed.pathname !== "")
  ) {
    throw new Error(
      "MM_CHAT_BACKEND_INTERNAL_URL must be an HTTP(S) origin without credentials, path, query, or fragment.",
    );
  }

  return parsed.origin;
}
