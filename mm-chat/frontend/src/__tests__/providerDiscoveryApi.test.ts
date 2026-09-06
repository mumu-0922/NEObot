import { describe, expect, it, vi } from "vitest";
import { createHttpClient } from "../services/api/client/server/httpClient";
import { createServerProviderApiShell } from "../services/api/client/server/providerApi";
import { createLocalProviderApiShell } from "../services/api/client/local/providerApi";

describe("provider discovery API", () => {
  it("uses the explicit administrator endpoint and forwards cancellation", async () => {
    const payload = {
      provider: { id: "test/id" },
      models: ["gpt-6-astra"],
      discoveredModels: ["gpt-6-astra"],
    };
    const fetchImpl = vi
      .fn<typeof fetch>()
      .mockResolvedValue(
        new Response(JSON.stringify(payload), { status: 200 }),
      );
    const client = createServerProviderApiShell(
      createHttpClient({
        baseUrl: "/mm-api",
        fetchImpl,
        getAuthToken: () => "fixture-token",
      }),
    );
    const controller = new AbortController();
    expect(
      await client.discoverAdminProviderModels("test/id", controller.signal),
    ).toEqual(payload);
    expect(fetchImpl).toHaveBeenCalledWith(
      "/mm-api/v1/admin/providers/test%2Fid/discover",
      expect.objectContaining({ method: "POST", signal: controller.signal }),
    );
  });

  it("does not restore unsupported local discovery", async () => {
    await expect(
      createLocalProviderApiShell().discoverAdminProviderModels("test"),
    ).rejects.toThrow();
  });
});
