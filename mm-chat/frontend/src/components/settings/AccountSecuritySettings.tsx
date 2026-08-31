"use client";

import React, { useMemo, useState } from "react";
import {
  AlertTriangle,
  KeyRound,
  LoaderCircle,
  LogOut,
  ShieldCheck,
  UserRound,
} from "lucide-react";
import { useTranslations } from "next-intl";
import { ApiClientError, createNeoChatApiClient } from "@/services/api/client";
import {
  clearServerAuthSession,
  getServerAuthSession,
} from "@/services/api/client/authSession";
import { validatePasswordPolicy } from "@/lib/auth/passwordPolicy";

interface AccountSecuritySettingsProps {
  onAuthInvalidated?: () => void;
}

type FormError =
  | "currentRequired"
  | "newRequired"
  | "passwordTooShort"
  | "passwordTooLong"
  | "passwordMismatch"
  | "currentInvalid"
  | "genericError";

const AccountSecuritySettings: React.FC<AccountSecuritySettingsProps> = ({
  onAuthInvalidated,
}) => {
  const t = useTranslations("AccountSecurity");
  const apiClient = useMemo(() => createNeoChatApiClient(), []);
  const session = getServerAuthSession();
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [formError, setFormError] = useState<FormError | null>(null);
  const [isChanging, setIsChanging] = useState(false);
  const [isRevoking, setIsRevoking] = useState(false);

  const finishSessionInvalidation = () => {
    clearServerAuthSession();
    if (onAuthInvalidated) {
      onAuthInvalidated();
      return;
    }
    window.location.reload();
  };

  const handleChangePassword = async (
    event: React.FormEvent<HTMLFormElement>,
  ) => {
    event.preventDefault();
    if (isChanging) return;

    if (!currentPassword) {
      setFormError("currentRequired");
      return;
    }
    if (!newPassword) {
      setFormError("newRequired");
      return;
    }
    const policyFailure = validatePasswordPolicy(newPassword);
    if (policyFailure === "too-short") {
      setFormError("passwordTooShort");
      return;
    }
    if (policyFailure === "too-long") {
      setFormError("passwordTooLong");
      return;
    }
    if (newPassword !== confirmPassword) {
      setFormError("passwordMismatch");
      return;
    }

    setIsChanging(true);
    setFormError(null);
    try {
      await apiClient.auth.changePassword({
        currentPassword,
        newPassword,
      });
      finishSessionInvalidation();
    } catch (error) {
      if (error instanceof ApiClientError && error.status === 401) {
        setFormError("currentInvalid");
      } else {
        setFormError("genericError");
      }
    } finally {
      setIsChanging(false);
    }
  };

  const handleRevokeAllSessions = async () => {
    if (isRevoking || !window.confirm(t("revokeConfirm"))) return;

    setIsRevoking(true);
    setFormError(null);
    try {
      await apiClient.auth.revokeAllSessions();
      finishSessionInvalidation();
    } catch {
      setFormError("genericError");
    } finally {
      setIsRevoking(false);
    }
  };

  if (!apiClient.capabilities.auth || !session) {
    return (
      <section className="rounded-xl border border-border bg-background p-5">
        <div className="flex items-start gap-3">
          <AlertTriangle
            className="mt-0.5 text-amber-500"
            size={20}
            aria-hidden="true"
          />
          <div>
            <h2 className="font-semibold text-foreground">
              {t("unavailableTitle")}
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">
              {t("unavailableDescription")}
            </p>
          </div>
        </div>
      </section>
    );
  }

  return (
    <div className="space-y-5">
      <section aria-labelledby="account-overview-title">
        <div className="mb-3 flex items-center gap-2">
          <UserRound className="text-cyan-500" size={20} aria-hidden="true" />
          <div>
            <h2
              id="account-overview-title"
              className="text-lg font-semibold text-foreground"
            >
              {t("accountTitle")}
            </h2>
            <p className="text-sm text-muted-foreground">
              {t("accountDescription")}
            </p>
          </div>
        </div>
        <div className="rounded-xl border border-border bg-background p-4">
          <dl className="grid gap-4 sm:grid-cols-2">
            <div>
              <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                {t("displayName")}
              </dt>
              <dd className="mt-1 text-sm font-medium text-foreground">
                {session.user.displayName || t("unnamed")}
              </dd>
            </div>
            <div>
              <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                {t("role")}
              </dt>
              <dd className="mt-1 text-sm font-medium text-foreground">
                {t(`roles.${session.user.role}`)}
              </dd>
            </div>
          </dl>
        </div>
      </section>

      <section aria-labelledby="change-password-title">
        <div className="mb-3 flex items-center gap-2">
          <KeyRound className="text-violet-500" size={20} aria-hidden="true" />
          <div>
            <h2
              id="change-password-title"
              className="text-lg font-semibold text-foreground"
            >
              {t("changePasswordTitle")}
            </h2>
            <p className="text-sm text-muted-foreground">
              {t("changePasswordDescription")}
            </p>
          </div>
        </div>
        <form
          onSubmit={handleChangePassword}
          className="rounded-xl border border-border bg-background p-4 sm:p-5"
        >
          <div className="grid gap-4">
            <PasswordField
              id="account-current-password"
              label={t("currentPassword")}
              autoComplete="current-password"
              value={currentPassword}
              onChange={setCurrentPassword}
              disabled={isChanging}
            />
            <div className="grid gap-4 sm:grid-cols-2">
              <PasswordField
                id="account-new-password"
                label={t("newPassword")}
                autoComplete="new-password"
                value={newPassword}
                onChange={setNewPassword}
                disabled={isChanging}
              />
              <PasswordField
                id="account-confirm-password"
                label={t("confirmPassword")}
                autoComplete="new-password"
                value={confirmPassword}
                onChange={setConfirmPassword}
                disabled={isChanging}
              />
            </div>
          </div>
          <p className="mt-3 text-xs text-muted-foreground">
            {t("passwordPolicy")}
          </p>
          {formError ? (
            <p className="mt-3 text-sm text-red-600" role="alert">
              {t(formError)}
            </p>
          ) : null}
          <div className="mt-4 flex justify-end">
            <button
              type="submit"
              disabled={isChanging}
              className="inline-flex items-center gap-2 rounded-lg bg-foreground px-4 py-2 text-sm font-medium text-background transition-opacity hover:opacity-90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
            >
              {isChanging ? (
                <LoaderCircle
                  className="animate-spin"
                  size={16}
                  aria-hidden="true"
                />
              ) : (
                <ShieldCheck size={16} aria-hidden="true" />
              )}
              {isChanging ? t("changingPassword") : t("changePassword")}
            </button>
          </div>
        </form>
      </section>

      <section aria-labelledby="sessions-title">
        <div className="mb-3 flex items-center gap-2">
          <LogOut className="text-rose-500" size={20} aria-hidden="true" />
          <div>
            <h2
              id="sessions-title"
              className="text-lg font-semibold text-foreground"
            >
              {t("sessionsTitle")}
            </h2>
            <p className="text-sm text-muted-foreground">
              {t("sessionsDescription")}
            </p>
          </div>
        </div>
        <div className="flex flex-col gap-3 rounded-xl border border-border bg-background p-4 sm:flex-row sm:items-center sm:justify-between sm:p-5">
          <p className="text-sm text-muted-foreground">
            {t("revokeAllDescription")}
          </p>
          <button
            type="button"
            disabled={isRevoking}
            onClick={handleRevokeAllSessions}
            className="inline-flex shrink-0 items-center justify-center gap-2 rounded-lg border border-red-300 bg-red-50 px-4 py-2 text-sm font-medium text-red-700 transition-colors hover:bg-red-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-500 disabled:cursor-not-allowed disabled:opacity-50 dark:border-red-900 dark:bg-red-950/30 dark:text-red-300 dark:hover:bg-red-950/50"
          >
            {isRevoking ? (
              <LoaderCircle
                className="animate-spin"
                size={16}
                aria-hidden="true"
              />
            ) : (
              <LogOut size={16} aria-hidden="true" />
            )}
            {isRevoking ? t("revokingSessions") : t("revokeAll")}
          </button>
        </div>
      </section>
    </div>
  );
};

interface PasswordFieldProps {
  id: string;
  label: string;
  autoComplete: "current-password" | "new-password";
  value: string;
  onChange: (value: string) => void;
  disabled: boolean;
}

const PasswordField: React.FC<PasswordFieldProps> = ({
  id,
  label,
  autoComplete,
  value,
  onChange,
  disabled,
}) => (
  <div>
    <label htmlFor={id} className="mb-1.5 block text-sm font-medium">
      {label}
    </label>
    <input
      id={id}
      type="password"
      value={value}
      onChange={(event) => onChange(event.target.value)}
      autoComplete={autoComplete}
      disabled={disabled}
      className="w-full rounded-lg border border-input bg-muted px-3 py-2 text-sm text-foreground focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-500/20 disabled:cursor-not-allowed disabled:opacity-60"
    />
  </div>
);

export default AccountSecuritySettings;
