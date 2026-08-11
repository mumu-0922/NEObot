import { describe, expect, it } from "vitest";
import {
  shouldResolveSelectedModelAfterBootstrap,
  shouldRunSettingsStartupEffects,
} from "../lib/app/startupEffects";

describe("app startup effects", () => {
  it("waits for settings hydration before running settings writes", () => {
    expect(shouldRunSettingsStartupEffects(false)).toBe(false);
    expect(shouldRunSettingsStartupEffects(true)).toBe(true);
  });

  it("waits for server model bootstrap before auto-selecting a model", () => {
    expect(
      shouldResolveSelectedModelAfterBootstrap({
        chatHydrated: true,
        settingsHydrated: true,
        coreHydrated: true,
        serverModelBootstrapReady: false,
      }),
    ).toBe(false);
  });

  it("allows auto-selection after server model bootstrap succeeds or fails", () => {
    expect(
      shouldResolveSelectedModelAfterBootstrap({
        chatHydrated: true,
        settingsHydrated: true,
        coreHydrated: true,
        serverModelBootstrapReady: true,
      }),
    ).toBe(true);
  });
});
