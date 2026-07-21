export const SERVER_ONLY_RAG_MIGRATION_VERSION = 1;
export const SERVER_ONLY_RAG_MIGRATION_MARKER =
  "neo-chat:migration:server-only-rag:v1";

type LocalStorageLike = Pick<Storage, "getItem" | "setItem">;

interface AsyncSettingsStore {
  getItem<T>(key: string): Promise<T | null>;
  setItem<T>(key: string, value: T): Promise<T>;
}

interface RetiredRAGMigrationOptions {
  localStorageRef: LocalStorageLike;
  indexedDbStore: AsyncSettingsStore;
  settingsKey: string;
}

type MigrationResult = {
  changed: boolean;
  value: unknown;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

export function stripRetiredLocalRAGState(value: unknown): MigrationResult {
  const serialized = typeof value === "string";
  const parsed = typeof value === "string" ? JSON.parse(value) : value;
  if (!isRecord(parsed)) return { changed: false, value };

  let changed = false;
  const next = { ...parsed };
  if (Object.hasOwn(next, "rag")) {
    delete next.rag;
    changed = true;
  }
  if (isRecord(next.state) && Object.hasOwn(next.state, "rag")) {
    const state = { ...next.state };
    delete state.rag;
    next.state = state;
    changed = true;
  }

  if (!changed) return { changed: false, value };
  return {
    changed: true,
    value: serialized ? JSON.stringify(next) : next,
  };
}

export async function retireBrowserLocalRAGState({
  localStorageRef,
  indexedDbStore,
  settingsKey,
}: RetiredRAGMigrationOptions): Promise<boolean> {
  if (
    localStorageRef.getItem(SERVER_ONLY_RAG_MIGRATION_MARKER) ===
    String(SERVER_ONLY_RAG_MIGRATION_VERSION)
  ) {
    return false;
  }

  const localValue = localStorageRef.getItem(settingsKey);
  const indexedValue = await indexedDbStore.getItem<unknown>(settingsKey);
  const localMigration =
    localValue === null
      ? { changed: false, value: null }
      : stripRetiredLocalRAGState(localValue);
  const indexedMigration =
    indexedValue === null
      ? { changed: false, value: null }
      : stripRetiredLocalRAGState(indexedValue);

  if (localMigration.changed) {
    localStorageRef.setItem(settingsKey, String(localMigration.value));
  }
  if (indexedMigration.changed) {
    await indexedDbStore.setItem(settingsKey, indexedMigration.value);
  }

  localStorageRef.setItem(
    SERVER_ONLY_RAG_MIGRATION_MARKER,
    String(SERVER_ONLY_RAG_MIGRATION_VERSION),
  );
  return localMigration.changed || indexedMigration.changed;
}
