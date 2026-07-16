import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import en from "../i18n/locales/en";
import zh from "../i18n/locales/zh";
import ja from "../i18n/locales/ja";
import { describe, expect, it } from "vitest";

describe("G8.2 team settings UI composition", () => {
  it("exposes Teams as a deep-linkable settings tab with localized copy", () => {
    const settingsPage = readFileSync(
      resolve(process.cwd(), "src/components/settings/SettingsPage.tsx"),
      "utf8",
    );
    const panelUrlState = readFileSync(
      resolve(process.cwd(), "src/lib/chat/panelUrlState.ts"),
      "utf8",
    );

    expect(settingsPage).toContain("TeamSettings");
    expect(settingsPage).toContain('id: "teams"');
    expect(settingsPage).toContain("tabTeams");
    expect(panelUrlState).toContain('"teams"');
    expect(en.SettingsPage.tabTeams).toBeTruthy();
    expect(zh.SettingsPage.tabTeams).toBeTruthy();
    expect(ja.SettingsPage.tabTeams).toBeTruthy();
    expect(en.Team.title).toBeTruthy();
    expect(zh.Team.title).toBeTruthy();
    expect(ja.Team.title).toBeTruthy();
  });

  it("wires visible team actions through the frontend API client only", () => {
    const teamSettings = readFileSync(
      resolve(process.cwd(), "src/components/settings/TeamSettings.tsx"),
      "utf8",
    );

    expect(teamSettings).toContain("createNeoChatApiClient");
    expect(teamSettings).toContain("apiClient.capabilities.teams");
    expect(teamSettings).toContain("apiClient.teams.listTeams");
    expect(teamSettings).toContain("apiClient.teams.createTeam");
    expect(teamSettings).toContain("apiClient.teams.updateTeam");
    expect(teamSettings).toContain("apiClient.teams.listMembers");
    expect(teamSettings).toContain("apiClient.teams.updateMember");
    expect(teamSettings).toContain("apiClient.teams.createInvite");
    expect(teamSettings).toContain("apiClient.teams.listInvites");
    expect(teamSettings).toContain("apiClient.teams.revokeInvite");
    expect(teamSettings).toContain("apiClient.teams.leaveTeam");
    expect(teamSettings).toContain("unsupportedDescription");
    expect(teamSettings).not.toContain("/v1/teams");
    expect(teamSettings).not.toContain("/api/teams");
  });

  it("keeps caller identity and authorization fields out of the Teams UI payload", () => {
    const teamSettings = readFileSync(
      resolve(process.cwd(), "src/components/settings/TeamSettings.tsx"),
      "utf8",
    );

    expect(teamSettings).toContain("idempotencyKey");
    expect(teamSettings).toContain("teamRole");
    expect(teamSettings).not.toContain("actorUserId");
    expect(teamSettings).not.toContain("ownerUserId");
    expect(teamSettings).not.toContain("impersonateUserId");
    expect(teamSettings).not.toContain("allowedCollectionIds");
  });
});
