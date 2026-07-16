import localforage from "localforage";
import type { StateStorage } from "zustand/middleware";
import {
  ensureLegacyGeminiCoreSettingsMigration,
  ensureLegacyGeminiNextChatMigration,
} from "./legacyGeminiMigration";
import { logDevError } from "../../lib/utils/devLogger";

/**
 * Storage Configuration
 * Unified IndexedDB storage for all application data
 */

// Unified storage with multiple stores
export const appDb = localforage.createInstance({
  name: "neo-chat",
  storeName: "app_data",
  description: "Unified application storage",
});

export const STORAGE_VERSION = 4;
export type StorageVersion = typeof STORAGE_VERSION;

export const noopStorage: StateStorage = {
  getItem: () => null,
  setItem: () => undefined,
  removeItem: () => undefined,
};

export type BrowserLocalPersistenceAuthority = "runtime" | "import-only";

export function getBrowserLocalPersistenceAuthority(
  apiMode = process.env.NEXT_PUBLIC_API_MODE,
): BrowserLocalPersistenceAuthority {
  return apiMode?.trim() === "server" ? "import-only" : "runtime";
}

export function canUseBrowserLocalRuntimePersistence(): boolean {
  return getBrowserLocalPersistenceAuthority() === "runtime";
}

export class BrowserLocalIndexedDBAuthorityError extends Error {
  readonly code = "BROWSER_LOCAL_INDEXEDDB_IMPORT_ONLY";

  constructor(operation: string) {
    super(
      `${operation} is not available in server mode; browser-local IndexedDB is import-only.`,
    );
    this.name = "BrowserLocalIndexedDBAuthorityError";
  }
}

function assertIndexedDBWriteAuthority(operation: string): void {
  if (canUseBrowserLocalRuntimePersistence()) return;
  throw new BrowserLocalIndexedDBAuthorityError(operation);
}

export async function setRuntimeAppDbItem<T>(
  key: string,
  value: T,
): Promise<T> {
  assertIndexedDBWriteAuthority("Writing to IndexedDB");
  return appDb.setItem<T>(key, value);
}

export async function removeRuntimeAppDbItem(key: string): Promise<void> {
  assertIndexedDBWriteAuthority("Removing from IndexedDB");
  await appDb.removeItem(key);
}

export const getAppDbStorage = (): StateStorage => {
  if (
    typeof window === "undefined" ||
    !canUseBrowserLocalRuntimePersistence()
  ) {
    return noopStorage;
  }

  return {
    getItem: async (name) => {
      try {
        await ensureLegacyGeminiNextChatMigration({
          targetDb: appDb,
          localStorageRef: window.localStorage,
          storageKeys: STORAGE_KEYS,
        });
      } catch (error) {
        logDevError("Legacy Gemini data migration failed:", error);
      }
      return appDb.getItem<string>(name);
    },
    setItem: (name, value) => appDb.setItem(name, value),
    removeItem: (name) => appDb.removeItem(name),
  };
};

function getWindowPreferenceStorage(): StateStorage {
  if (typeof window === "undefined") {
    return noopStorage;
  }

  return {
    getItem: (name) => {
      try {
        ensureLegacyGeminiCoreSettingsMigration({
          localStorageRef: window.localStorage,
          storageKeys: STORAGE_KEYS,
        });
      } catch (error) {
        logDevError("Legacy Gemini core settings migration failed:", error);
      }
      return window.localStorage.getItem(name);
    },
    setItem: (name, value) => window.localStorage.setItem(name, value),
    removeItem: (name) => window.localStorage.removeItem(name),
  };
}

export const getBrowserLocalStorage = (): StateStorage => {
  if (!canUseBrowserLocalRuntimePersistence()) {
    return noopStorage;
  }

  return getWindowPreferenceStorage();
};

// Server mode keeps chat/files/knowledge authority on Go/Postgres/MinIO, but
// browser-owned preferences such as theme, language, and BYOK provider shells
// still need localStorage so a page refresh does not erase the user's UI config.
export const getBrowserPreferenceStorage = (): StateStorage => {
  return getWindowPreferenceStorage();
};

// Storage keys
export const STORAGE_KEYS = {
  // Core settings (localStorage via zustand default)
  CORE_SETTINGS: "neo-chat-core-settings",

  // Store names (IndexedDB)
  SETTINGS: "neo-chat-settings",
  CHAT: "neo-chat-storage",
  KNOWLEDGE: "knowledge-storage",
  MEMORY: "neo-chat-memory",
} as const;
