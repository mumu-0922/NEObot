import localforage from "localforage";
import { appDb, STORAGE_KEYS } from "../../store/storage/storageConfig";
import { deleteOPFSDirectory } from "../../utils/opfs";
import { logDevWarn } from "../utils/devLogger";
import { deleteLocalSecretMasterKey } from "../security/localSecrets";

const APP_OPFS_DIRECTORIES = ["knowledge-base", "workspaces", "images", "chat"];

async function cleanupOPFSDirectories(): Promise<void> {
  for (const directory of APP_OPFS_DIRECTORIES) {
    try {
      await deleteOPFSDirectory(directory);
    } catch (error) {
      logDevWarn(`Failed to delete OPFS directory "${directory}":`, error);
    }
  }
}

async function clearLocalStorageKeys(): Promise<void> {
  if (typeof window === "undefined") return;

  window.localStorage.removeItem(STORAGE_KEYS.CORE_SETTINGS);
  window.localStorage.removeItem(STORAGE_KEYS.SETTINGS);
  window.localStorage.removeItem(STORAGE_KEYS.CHAT);
  window.localStorage.removeItem(STORAGE_KEYS.KNOWLEDGE);
  window.localStorage.removeItem(STORAGE_KEYS.MEMORY);
  await deleteLocalSecretMasterKey();
}

export async function clearBrowserAppData(): Promise<void> {
  await cleanupOPFSDirectories();
  await clearLocalStorageKeys();
  await localforage.clear();
  await appDb.clear();
}
