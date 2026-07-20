import { describe, expect, it, vi } from "vitest";
import { ProviderRuntimeConfigSchema } from "../lib/api/schemas";
import { ProviderFactory } from "../lib/providers/base";

vi.mock("server-only", () => ({}));

describe("provider runtime schema", () => {
  it("accepts Gemini, OpenAI, OpenAI Compatible, and Anthropic provider types", () => {
    expect(ProviderRuntimeConfigSchema.parse({ type: "Gemini" }).type).toBe(
      "Gemini",
    );
    expect(ProviderRuntimeConfigSchema.parse({ type: "OpenAI" }).type).toBe(
      "OpenAI",
    );
    expect(
      ProviderRuntimeConfigSchema.parse({ type: "OpenAI Compatible" }).type,
    ).toBe("OpenAI Compatible");
    expect(ProviderRuntimeConfigSchema.parse({ type: "Anthropic" }).type).toBe(
      "Anthropic",
    );
  });

  it("fails closed instead of treating Anthropic as Gemini in local mode", () => {
    expect(() =>
      ProviderFactory.createClient({
        type: "Anthropic",
        apiKey: "fixture",
        baseUrl: "https://api.anthropic.com",
      }),
    ).toThrow(/Go server runtime/);
  });
});
