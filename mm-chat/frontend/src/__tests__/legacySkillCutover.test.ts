import { describe, expect, it } from "vitest";

import {
  LEGACY_SKILL_STATE_FIELDS,
  createLegacySkillBackup,
  createLegacySkillDeletionDryRun,
  createLegacySkillInventory,
  serializeLegacySkillArtifact,
} from "../lib/skills/legacyCutover";

describe("legacy Skill G20.9 preparation", () => {
  const persisted = {
    version: 6,
    state: {
      installedSkills: [
        { id: " Clarity-Rewrite ", name: "Clarity", content: "private body" },
      ],
      customSkills: [{ id: "bad id", content: "another body" }],
      activeSkillIds: ["clarity-rewrite", "orphan-skill"],
      skillAutoSelect: true,
      skillCatalogs: {
        en: { skills: [{ id: "catalog-skill", content: "catalog body" }] },
      },
      skillCatalogTimestamps: { en: 1 },
      skillDefinitions: {
        "en:cached.json": { id: "cached-skill", content: "cached body" },
      },
      skillDefinitionTimestamps: { "en:cached.json": 2 },
      customAgents: [{ id: "assistant-kept" }],
      providers: [{ apiKey: "kept-outside-delete-set" }],
    },
  };

  it("builds a deterministic content-free inventory", async () => {
    const first = await createLegacySkillInventory(persisted);
    const second = await createLegacySkillInventory(
      JSON.stringify({ state: { ...persisted.state }, version: 6 }),
    );

    expect(first).toEqual(second);
    expect(first.counts).toMatchObject({
      installed: 1,
      custom: 1,
      active: 2,
      catalogEntries: 1,
      definitions: 1,
      invalid: 1,
      orphanActive: 1,
    });
    expect(first.normalizedIds).toContain("clarity-rewrite");
    const encoded = JSON.stringify(first);
    expect(encoded).not.toContain("private body");
    expect(encoded).not.toContain("another body");
    expect(first.manifestFingerprint).toMatch(/^sha256:[a-f0-9]{64}$/);
  });

  it("creates only backup and dry-run artifacts without deleting state", async () => {
    const before = serializeLegacySkillArtifact(persisted);
    const inventory = await createLegacySkillInventory(persisted);
    const plan = createLegacySkillDeletionDryRun(inventory);
    const backup = createLegacySkillBackup(
      persisted,
      "2026-08-14T00:00:00.000Z",
    );

    expect(plan.dryRun).toBe(true);
    expect(plan.deleteStateFields).toEqual(LEGACY_SKILL_STATE_FIELDS);
    expect(plan.deleteStorageKeys).toEqual([]);
    expect(plan.preservedDomains).toEqual(
      expect.arrayContaining([
        "assistants",
        "mcp",
        "chat",
        "conversations",
        "files",
      ]),
    );
    expect(backup.payload).toEqual(persisted);
    expect(serializeLegacySkillArtifact(persisted)).toBe(before);
  });
});
