export const LEGACY_SKILL_RETIREMENT_VERSION = 1;
export const LEGACY_SKILL_RETIREMENT_MARKER =
  "neo-chat:migration:legacy-skill-retirement:v1";

export const RETIRED_LEGACY_SKILL_SETTINGS_FIELDS = [
  "installedSkills",
  "customSkills",
  "activeSkillIds",
  "skillAutoSelect",
  "skillCatalogs",
  "skillCatalogTimestamps",
  "skillDefinitions",
  "skillDefinitionTimestamps",
] as const;

type LocalStorageLike = Pick<Storage, "getItem" | "setItem">;

interface AsyncPersistenceStore {
  getItem<T>(key: string): Promise<T | null>;
  setItem<T>(key: string, value: T): Promise<T>;
}

interface LegacySkillRetirementOptions {
  localStorageRef: LocalStorageLike;
  indexedDbStore: AsyncPersistenceStore;
  settingsKey: string;
  chatKey: string;
}

export interface RetirementResult {
  changed: boolean;
  value: unknown;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function transformRecord(
  value: unknown,
  transform: (record: Record<string, unknown>) => RetirementResult,
): RetirementResult {
  const serialized = typeof value === "string";
  const parsed = serialized ? JSON.parse(value) : value;
  if (!isRecord(parsed)) return { changed: false, value };

  const result = transform(parsed);
  if (!result.changed) return { changed: false, value };
  return {
    changed: true,
    value: serialized ? JSON.stringify(result.value) : result.value,
  };
}

function stripSettingsState(state: Record<string, unknown>): RetirementResult {
  let changed = false;
  const next = { ...state };

  for (const field of RETIRED_LEGACY_SKILL_SETTINGS_FIELDS) {
    if (!Object.hasOwn(next, field)) continue;
    delete next[field];
    changed = true;
  }

  return { changed, value: changed ? next : state };
}

export function stripRetiredLegacySkillSettings(
  value: unknown,
): RetirementResult {
  return transformRecord(value, (record) => {
    const topLevel = stripSettingsState(record);
    const next = topLevel.changed
      ? (topLevel.value as Record<string, unknown>)
      : { ...record };
    let changed = topLevel.changed;

    if (isRecord(next.state)) {
      const nested = stripSettingsState(next.state);
      if (nested.changed) {
        next.state = nested.value;
        changed = true;
      }
    }

    return { changed, value: changed ? next : record };
  });
}

function stripSessionSelection(value: unknown): RetirementResult {
  if (!isRecord(value) || !isRecord(value.config)) {
    return { changed: false, value };
  }
  if (!Object.hasOwn(value.config, "activeSkills")) {
    return { changed: false, value };
  }

  const config = { ...value.config };
  delete config.activeSkills;
  return { changed: true, value: { ...value, config } };
}

function stripWorkspaceSelection(value: unknown): RetirementResult {
  if (!isRecord(value) || !Object.hasOwn(value, "activeSkills")) {
    return { changed: false, value };
  }

  const workspace = { ...value };
  delete workspace.activeSkills;
  return { changed: true, value: workspace };
}

function stripChatState(state: Record<string, unknown>): RetirementResult {
  let changed = false;
  const next = { ...state };

  if (Array.isArray(state.sessions)) {
    const sessions = state.sessions.map((session) => {
      const result = stripSessionSelection(session);
      changed ||= result.changed;
      return result.value;
    });
    if (changed) next.sessions = sessions;
  }

  if (Array.isArray(state.workspaces)) {
    let workspacesChanged = false;
    const workspaces = state.workspaces.map((workspace) => {
      const result = stripWorkspaceSelection(workspace);
      workspacesChanged ||= result.changed;
      return result.value;
    });
    if (workspacesChanged) {
      next.workspaces = workspaces;
      changed = true;
    }
  }

  return { changed, value: changed ? next : state };
}

export function stripRetiredLegacySkillChat(value: unknown): RetirementResult {
  return transformRecord(value, (record) => {
    const topLevel = stripChatState(record);
    const next = topLevel.changed
      ? (topLevel.value as Record<string, unknown>)
      : { ...record };
    let changed = topLevel.changed;

    if (isRecord(next.state)) {
      const nested = stripChatState(next.state);
      if (nested.changed) {
        next.state = nested.value;
        changed = true;
      }
    }

    return { changed, value: changed ? next : record };
  });
}

export async function retireBrowserLegacySkillState({
  localStorageRef,
  indexedDbStore,
  settingsKey,
  chatKey,
}: LegacySkillRetirementOptions): Promise<boolean> {
  if (
    localStorageRef.getItem(LEGACY_SKILL_RETIREMENT_MARKER) ===
    String(LEGACY_SKILL_RETIREMENT_VERSION)
  ) {
    return false;
  }

  const localSettings = localStorageRef.getItem(settingsKey);
  const localChat = localStorageRef.getItem(chatKey);
  const indexedSettings = await indexedDbStore.getItem<unknown>(settingsKey);
  const indexedChat = await indexedDbStore.getItem<unknown>(chatKey);
  const records = [
    {
      source: "local" as const,
      key: settingsKey,
      original: localSettings,
      result:
        localSettings === null
          ? { changed: false, value: null }
          : stripRetiredLegacySkillSettings(localSettings),
    },
    {
      source: "local" as const,
      key: chatKey,
      original: localChat,
      result:
        localChat === null
          ? { changed: false, value: null }
          : stripRetiredLegacySkillChat(localChat),
    },
    {
      source: "indexed" as const,
      key: settingsKey,
      original: indexedSettings,
      result:
        indexedSettings === null
          ? { changed: false, value: null }
          : stripRetiredLegacySkillSettings(indexedSettings),
    },
    {
      source: "indexed" as const,
      key: chatKey,
      original: indexedChat,
      result:
        indexedChat === null
          ? { changed: false, value: null }
          : stripRetiredLegacySkillChat(indexedChat),
    },
  ];
  const written: typeof records = [];

  try {
    for (const record of records) {
      if (!record.result.changed) continue;
      if (record.source === "local") {
        localStorageRef.setItem(record.key, String(record.result.value));
      } else {
        await indexedDbStore.setItem(record.key, record.result.value);
      }
      written.push(record);
    }

    localStorageRef.setItem(
      LEGACY_SKILL_RETIREMENT_MARKER,
      String(LEGACY_SKILL_RETIREMENT_VERSION),
    );
  } catch (error) {
    for (const record of written.reverse()) {
      try {
        if (record.source === "local") {
          localStorageRef.setItem(record.key, String(record.original));
        } else {
          await indexedDbStore.setItem(record.key, record.original);
        }
      } catch {
        // Preserve the original migration failure. A retry remains idempotent.
      }
    }
    throw error;
  }

  return records.some((record) => record.result.changed);
}
