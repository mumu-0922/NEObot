import type { SearchMode } from "./types";

export const isSearchMode = (value: unknown): value is SearchMode =>
  value === "off" || value === "model_builtin" || value === "external";

export const normalizeSearchMode = (
  value: unknown,
  legacyUseSearch?: unknown,
): SearchMode => {
  if (isSearchMode(value)) return value;
  if (value !== undefined && value !== null) return "off";
  return legacyUseSearch === true ? "external" : "off";
};

export const searchModeEnabled = (mode: SearchMode): boolean => mode !== "off";
