import type { Source } from "../../types";
import { logDevWarn } from "../../lib/utils/devLogger";

export const LEGACY_RAG_ROUTES_REMOVED_MESSAGE =
  "Legacy Next RAG routes were removed in G9.2; use server Knowledge/RAG through the Go backend.";

function warnLegacyRagRouteRemoved(operation: string): void {
  logDevWarn(`${operation}: ${LEGACY_RAG_ROUTES_REMOVED_MESSAGE}`);
}

export async function queryRAG(
  text: string,
  namespace = "",
): Promise<Source[]> {
  void text;
  void namespace;
  warnLegacyRagRouteRemoved("RAG query skipped");
  return [];
}

export async function upsertToRAG(
  items: { id: string; data: string; metadata?: unknown }[],
  namespace = "",
): Promise<boolean> {
  void items;
  void namespace;
  warnLegacyRagRouteRemoved("RAG upsert skipped");
  return false;
}

export async function deleteFromRAG(
  ids: string[],
  namespace = "",
): Promise<boolean> {
  void ids;
  void namespace;
  warnLegacyRagRouteRemoved("RAG delete skipped");
  return false;
}
