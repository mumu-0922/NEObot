import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import en from "@/i18n/locales/en";
import ja from "@/i18n/locales/ja";
import zh from "@/i18n/locales/zh";

const readSource = (path: string) =>
  readFileSync(resolve(process.cwd(), path), "utf8");

describe("account security UI composition", () => {
  it("exposes the account settings tab through durable URL state", () => {
    const settingsPage = readSource("src/components/settings/SettingsPage.tsx");
    const panelUrlState = readSource("src/lib/chat/panelUrlState.ts");

    expect(settingsPage).toContain('id: "account"');
    expect(settingsPage).toContain("AccountSecuritySettings");
    expect(panelUrlState).toContain('"account"');
  });

  it("wires password change and all-session revocation to server auth", () => {
    const accountSecurity = readSource(
      "src/components/settings/AccountSecuritySettings.tsx",
    );

    expect(accountSecurity).toContain("apiClient.auth.changePassword");
    expect(accountSecurity).toContain("apiClient.auth.revokeAllSessions");
    expect(accountSecurity).toContain("clearServerAuthSession");
    expect(accountSecurity).toContain("currentPassword");
    expect(accountSecurity).toContain("confirmPassword");
  });

  it("preserves server-auth password whitespace and exposes Recovery", () => {
    const login = readSource("src/components/app/AccessPasswordPage.tsx");
    const recovery = readSource(
      "src/components/app/ServerAuthRecoveryPanel.tsx",
    );

    expect(login).toContain(
      "const submittedPassword = isServerAuth ? password : password.trim();",
    );
    expect(login).not.toContain("const trimmedPassword");
    expect(login).toContain("ServerAuthRecoveryPanel");
    expect(recovery).toContain("apiClient.auth.requestRecovery");
    expect(recovery).toContain("apiClient.auth.completeRecovery");
    expect(recovery).toContain("newPassword,");
  });

  it("ships account and Recovery labels in every supported locale", () => {
    for (const messages of [en, zh, ja]) {
      expect(messages.SettingsPage.tabAccount).toBeTruthy();
      expect(messages.AccountSecurity.changePassword).toBeTruthy();
      expect(messages.AccessPassword.forgotPassword).toBeTruthy();
      expect(messages.AccessPassword.recoveryTitle).toBeTruthy();
    }
  });
});
