import { STORAGE_KEYS } from "@/store/storage/storageConfig";

export const LEGACY_SKILL_STATE_FIELDS = [
  "installedSkills",
  "customSkills",
  "activeSkillIds",
  "skillAutoSelect",
  "skillCatalogs",
  "skillCatalogTimestamps",
  "skillDefinitions",
  "skillDefinitionTimestamps",
] as const;

const normalizedIdPattern = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

export interface LegacySkillInventoryRecord {
  source: "installed" | "custom" | "active" | "catalog" | "definition";
  index: number;
  normalizedId: string | null;
  fingerprint: string;
}

export interface LegacySkillInventory {
  schemaVersion: "neo.legacy-skill-inventory/v1";
  storage: { database: "neo-chat"; store: "app_data"; key: string };
  counts: {
    installed: number;
    custom: number;
    active: number;
    catalogLocales: number;
    catalogEntries: number;
    definitions: number;
    invalid: number;
    orphanActive: number;
  };
  normalizedIds: string[];
  records: LegacySkillInventoryRecord[];
  manifestFingerprint: string;
}

export interface LegacySkillDeletionPlan {
  schemaVersion: "neo.legacy-skill-deletion-plan/v1";
  dryRun: true;
  storageKey: string;
  deleteStateFields: readonly string[];
  deleteStorageKeys: readonly string[];
  preservedDomains: readonly [
    "assistants",
    "mcp",
    "chat",
    "conversations",
    "files",
    "knowledge",
    "memory",
  ];
  sourceManifestFingerprint: string;
}

export interface LegacySkillBackup {
  schemaVersion: "neo.legacy-skill-backup/v1";
  exportedAt: string;
  storage: { database: "neo-chat"; store: "app_data"; key: string };
  payload: unknown;
}

export async function createLegacySkillInventory(
  persistedValue: unknown,
): Promise<LegacySkillInventory> {
  const state = persistedState(persistedValue);
  const installed = array(state.installedSkills);
  const custom = array(state.customSkills);
  const active = array(state.activeSkillIds);
  const catalogs = record(state.skillCatalogs);
  const definitions = record(state.skillDefinitions);
  const records: LegacySkillInventoryRecord[] = [];
  let invalid = 0;

  await appendSkillRecords(records, installed, "installed", () => invalid++);
  await appendSkillRecords(records, custom, "custom", () => invalid++);

  for (const [index, value] of active.entries()) {
    const normalizedId = normalizeLegacySkillId(value);
    if (!normalizedId) invalid++;
    records.push({
      source: "active",
      index,
      normalizedId,
      fingerprint: await fingerprint(value),
    });
  }

  let catalogEntries = 0;
  for (const [localeIndex, [locale, value]] of Object.entries(catalogs)
    .sort(([left], [right]) => left.localeCompare(right))
    .entries()) {
    const skills = array(record(value).skills);
    for (const [entryIndex, skill] of skills.entries()) {
      const normalizedId = normalizeLegacySkillId(record(skill).id);
      if (!normalizedId) invalid++;
      records.push({
        source: "catalog",
        index: localeIndex * 100000 + entryIndex,
        normalizedId,
        fingerprint: await fingerprint({ locale, skill }),
      });
      catalogEntries++;
    }
  }

  for (const [index, [cacheKey, skill]] of Object.entries(definitions)
    .sort(([left], [right]) => left.localeCompare(right))
    .entries()) {
    const normalizedId = normalizeLegacySkillId(record(skill).id);
    if (!normalizedId) invalid++;
    records.push({
      source: "definition",
      index,
      normalizedId,
      fingerprint: await fingerprint({ cacheKey, skill }),
    });
  }

  records.sort((left, right) =>
    `${left.source}:${left.index}:${left.normalizedId ?? ""}`.localeCompare(
      `${right.source}:${right.index}:${right.normalizedId ?? ""}`,
    ),
  );
  const installedIds = new Set(
    [...installed, ...custom]
      .map((skill) => normalizeLegacySkillId(record(skill).id))
      .filter((id): id is string => Boolean(id)),
  );
  const activeIds = active
    .map(normalizeLegacySkillId)
    .filter((id): id is string => Boolean(id));
  const normalizedIds = Array.from(
    new Set(records.flatMap((item) => item.normalizedId ?? [])),
  ).sort();
  const counts = {
    installed: installed.length,
    custom: custom.length,
    active: active.length,
    catalogLocales: Object.keys(catalogs).length,
    catalogEntries,
    definitions: Object.keys(definitions).length,
    invalid,
    orphanActive: activeIds.filter((id) => !installedIds.has(id)).length,
  };
  const manifestFingerprint = await fingerprint({
    schemaVersion: "neo.legacy-skill-inventory/v1",
    counts,
    normalizedIds,
    records,
  });

  return {
    schemaVersion: "neo.legacy-skill-inventory/v1",
    storage: {
      database: "neo-chat",
      store: "app_data",
      key: STORAGE_KEYS.SETTINGS,
    },
    counts,
    normalizedIds,
    records,
    manifestFingerprint,
  };
}

export function createLegacySkillDeletionDryRun(
  inventory: LegacySkillInventory,
): LegacySkillDeletionPlan {
  return {
    schemaVersion: "neo.legacy-skill-deletion-plan/v1",
    dryRun: true,
    storageKey: STORAGE_KEYS.SETTINGS,
    deleteStateFields: LEGACY_SKILL_STATE_FIELDS,
    deleteStorageKeys: [],
    preservedDomains: [
      "assistants",
      "mcp",
      "chat",
      "conversations",
      "files",
      "knowledge",
      "memory",
    ],
    sourceManifestFingerprint: inventory.manifestFingerprint,
  };
}

export function createLegacySkillBackup(
  persistedValue: unknown,
  exportedAt = new Date().toISOString(),
): LegacySkillBackup {
  return {
    schemaVersion: "neo.legacy-skill-backup/v1",
    exportedAt,
    storage: {
      database: "neo-chat",
      store: "app_data",
      key: STORAGE_KEYS.SETTINGS,
    },
    payload: persistedValue,
  };
}

export function serializeLegacySkillArtifact(value: unknown): string {
  return `${stableStringify(value)}\n`;
}

async function appendSkillRecords(
  output: LegacySkillInventoryRecord[],
  values: unknown[],
  source: "installed" | "custom",
  invalid: () => void,
) {
  for (const [index, skill] of values.entries()) {
    const normalizedId = normalizeLegacySkillId(record(skill).id);
    if (!normalizedId) invalid();
    output.push({
      source,
      index,
      normalizedId,
      fingerprint: await fingerprint(skill),
    });
  }
}

function persistedState(value: unknown): Record<string, unknown> {
  let parsed = value;
  if (typeof value === "string") {
    try {
      parsed = JSON.parse(value) as unknown;
    } catch {
      return {};
    }
  }
  const envelope = record(parsed);
  return Object.prototype.hasOwnProperty.call(envelope, "state")
    ? record(envelope.state)
    : envelope;
}

function normalizeLegacySkillId(value: unknown): string | null {
  if (typeof value !== "string") return null;
  const normalized = value.trim().toLowerCase();
  return normalizedIdPattern.test(normalized) ? normalized : null;
}

function array(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function record(value: unknown): Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}

async function fingerprint(value: unknown): Promise<string> {
  const bytes = new TextEncoder().encode(stableStringify(value));
  const digest = await crypto.subtle.digest("SHA-256", bytes);
  return `sha256:${Array.from(new Uint8Array(digest), (byte) =>
    byte.toString(16).padStart(2, "0"),
  ).join("")}`;
}

function stableStringify(value: unknown): string {
  return JSON.stringify(stableNormalize(value));
}

function stableNormalize(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(stableNormalize);
  if (value !== null && typeof value === "object") {
    return Object.fromEntries(
      Object.entries(value as Record<string, unknown>)
        .sort(([left], [right]) => left.localeCompare(right))
        .map(([key, item]) => [key, stableNormalize(item)]),
    );
  }
  if (
    value === null ||
    typeof value === "string" ||
    typeof value === "number" ||
    typeof value === "boolean"
  ) {
    return value;
  }
  return null;
}
