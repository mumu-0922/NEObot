import { describe, expect, it } from "vitest";
import enMessageInput from "../i18n/locales/en/MessageInput.json";
import jaMessageInput from "../i18n/locales/ja/MessageInput.json";
import zhMessageInput from "../i18n/locales/zh/MessageInput.json";
import {
  getSearchCompatibility,
  getSearchCompatibilityErrorMessage,
  getSearchProviderLabel,
} from "../lib/settings/search";

describe("search compatibility", () => {
  it("uses only server-published search availability", () => {
    expect(
      getSearchCompatibility({
        searchProvider: "default",
        searchConfig: { serverAvailable: true },
      }),
    ).toEqual({ enabled: true, mode: "server", provider: "default" });

    const unavailable = getSearchCompatibility({
      searchProvider: "default",
      searchConfig: { serverAvailable: false },
    });
    expect(unavailable).toEqual({
      enabled: false,
      mode: "unavailable",
      provider: "default",
      reason: "server_search_unavailable",
    });
    expect(getSearchCompatibilityErrorMessage(unavailable)).toContain("server");
    expect(getSearchProviderLabel("default")).toBe("Tavily");
  });

  it("uses product-facing built-in and Tavily menu labels", () => {
    const provider = getSearchProviderLabel("default");

    expect(zhMessageInput.searchModeOpenAIWeb).toBe("内置搜索");
    expect(
      zhMessageInput.searchModeExternal.replace("{provider}", provider),
    ).toBe("Tavily 搜索");
    expect(enMessageInput.searchModeOpenAIWeb).toBe("Built-in search");
    expect(jaMessageInput.searchModeOpenAIWeb).toBe("内蔵検索");
  });
});
