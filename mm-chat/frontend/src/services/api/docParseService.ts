import type { DocumentParseProvider } from "../../types";

export const LEGACY_DOC_PARSE_ROUTE_REMOVED_MESSAGE =
  "Legacy Next document parsing routes were removed in G9.2; upload and index documents through server Knowledge.";

export async function parseDocumentFile(
  file: File,
  options: {
    provider: DocumentParseProvider;
    apiKey?: string;
    useDefault?: boolean;
  },
): Promise<string> {
  void file;
  void options;
  throw new Error(LEGACY_DOC_PARSE_ROUTE_REMOVED_MESSAGE);
}

export async function parseDocumentWithLlama(
  file: File,
  apiKey?: string,
  useDefault = false,
): Promise<string> {
  return parseDocumentFile(file, {
    provider: "llamaParse",
    apiKey,
    useDefault,
  });
}
