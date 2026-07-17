import { existsSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

describe("Next middleware convention", () => {
  it("keeps the Edge Middleware convention required by OpenNext Cloudflare", () => {
    expect(existsSync(resolve(process.cwd(), "src/middleware.ts"))).toBe(true);
    expect(existsSync(resolve(process.cwd(), "src/proxy.ts"))).toBe(false);
  });

  it("allows long-running backend generation streams through the rewrite proxy", () => {
    const nextConfig = readFileSync(
      resolve(process.cwd(), "next.config.ts"),
      "utf8",
    );

    expect(nextConfig).toContain("proxyTimeout: 300_000");
  });
});
