export function shouldRunSettingsStartupEffects(
  settingsHydrated: boolean,
): boolean {
  return settingsHydrated;
}

export function shouldResolveSelectedModelAfterBootstrap({
  chatHydrated,
  settingsHydrated,
  coreHydrated,
  serverModelBootstrapReady,
}: {
  chatHydrated: boolean;
  settingsHydrated: boolean;
  coreHydrated: boolean;
  serverModelBootstrapReady: boolean;
}): boolean {
  return (
    chatHydrated &&
    settingsHydrated &&
    coreHydrated &&
    serverModelBootstrapReady
  );
}
