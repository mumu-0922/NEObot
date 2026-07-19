const WEB_DEGRADATION_REASONS = new Set([
  "not_configured",
  "resolution_failed",
  "invalid_config",
  "invalid_request",
  "model_builtin_unsupported",
  "provider_failed",
  "unavailable",
]);

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}

export function getWebSearchDegradationReason(
  metadata: Record<string, unknown> | undefined,
): string | undefined {
  if (!metadata || !isRecord(metadata.fusion)) return undefined;
  const fusion = metadata.fusion;
  if (
    fusion.version !== "source-fusion/v1" ||
    fusion.searchRequested !== true
  ) {
    return undefined;
  }
  const reason =
    typeof fusion.degradationReason === "string"
      ? fusion.degradationReason.trim()
      : "";
  return WEB_DEGRADATION_REASONS.has(reason) ? reason : undefined;
}
