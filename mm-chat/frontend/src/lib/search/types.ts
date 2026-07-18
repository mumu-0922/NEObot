export interface Source {
  title: string;
  url: string;
  content: string;
  metadata?: Record<string, unknown>;
}

export interface ImageSource {
  url: string;
  description?: string;
}

export type SearchProviderID = "default";

export interface SearchServiceConfig {
  serverAvailable?: boolean;
}
