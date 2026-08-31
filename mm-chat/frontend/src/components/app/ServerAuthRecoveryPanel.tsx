"use client";

import React, { useMemo, useState } from "react";
import { ArrowLeft, ArrowRight, KeyRound, LoaderCircle } from "lucide-react";
import { useTranslations } from "next-intl";
import { ApiClientError, createNeoChatApiClient } from "@/services/api/client";
import { validatePasswordPolicy } from "@/lib/auth/passwordPolicy";

interface ServerAuthRecoveryPanelProps {
  onBackToLogin: () => void;
}

type RecoveryStep = "request" | "complete" | "completed";
type RecoveryError =
  | "recoveryEmailRequired"
  | "recoveryTokenRequired"
  | "newPasswordRequired"
  | "passwordTooShort"
  | "passwordTooLong"
  | "passwordInvalidCharacters"
  | "passwordMismatch"
  | "recoveryTokenInvalid"
  | "genericError";

const ServerAuthRecoveryPanel: React.FC<ServerAuthRecoveryPanelProps> = ({
  onBackToLogin,
}) => {
  const t = useTranslations("AccessPassword");
  const apiClient = useMemo(() => createNeoChatApiClient(), []);
  const [step, setStep] = useState<RecoveryStep>("request");
  const [email, setEmail] = useState("");
  const [token, setToken] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [errorKey, setErrorKey] = useState<RecoveryError | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const handleRequest = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const canonicalInput = email.trim();
    if (!canonicalInput || isSubmitting) {
      setErrorKey("recoveryEmailRequired");
      return;
    }

    setIsSubmitting(true);
    setErrorKey(null);
    try {
      await apiClient.auth.requestRecovery({ email: canonicalInput });
      setStep("complete");
    } catch {
      setErrorKey("genericError");
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleComplete = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (isSubmitting) return;
    if (!token.trim()) {
      setErrorKey("recoveryTokenRequired");
      return;
    }
    if (!newPassword) {
      setErrorKey("newPasswordRequired");
      return;
    }
    const policyFailure = validatePasswordPolicy(newPassword);
    if (policyFailure === "too-short") {
      setErrorKey("passwordTooShort");
      return;
    }
    if (policyFailure === "too-long") {
      setErrorKey("passwordTooLong");
      return;
    }
    if (policyFailure === "invalid-characters") {
      setErrorKey("passwordInvalidCharacters");
      return;
    }
    if (newPassword !== confirmPassword) {
      setErrorKey("passwordMismatch");
      return;
    }

    setIsSubmitting(true);
    setErrorKey(null);
    try {
      await apiClient.auth.completeRecovery({
        token: token.trim(),
        newPassword,
      });
      setToken("");
      setNewPassword("");
      setConfirmPassword("");
      setStep("completed");
    } catch (error) {
      if (error instanceof ApiClientError && error.status === 401) {
        setErrorKey("recoveryTokenInvalid");
      } else {
        setErrorKey("genericError");
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <main className="min-h-dvh bg-background text-foreground">
      <div className="mx-auto flex min-h-dvh w-full max-w-md flex-col justify-center px-5 py-10">
        <div className="mb-7 flex items-center gap-3">
          <div className="flex h-11 w-11 items-center justify-center rounded-lg bg-muted text-muted-foreground ring-1 ring-border">
            <KeyRound size={20} aria-hidden="true" />
          </div>
          <div className="min-w-0">
            <h1 className="text-xl font-semibold text-foreground">
              {t("recoveryTitle")}
            </h1>
            <p className="mt-1 text-sm text-muted-foreground">
              {step === "request"
                ? t("recoveryRequestSubtitle")
                : step === "complete"
                  ? t("recoveryCompleteSubtitle")
                  : t("recoveryCompletedSubtitle")}
            </p>
          </div>
        </div>

        {step === "request" ? (
          <form
            onSubmit={handleRequest}
            className="glass-surface rounded-lg border p-4 shadow-sm"
          >
            <label
              htmlFor="recovery-email"
              className="mb-2 block text-sm font-medium"
            >
              {t("emailLabel")}
            </label>
            <input
              id="recovery-email"
              type="email"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              autoComplete="email"
              disabled={isSubmitting}
              placeholder={t("emailPlaceholder")}
              className="w-full rounded-lg border border-input bg-muted px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-500/20 disabled:opacity-60"
            />
            <p className="mt-3 text-xs text-muted-foreground">
              {t("recoveryPrivacyNotice")}
            </p>
            {errorKey ? (
              <p className="mt-3 text-sm text-red-600" role="alert">
                {t(errorKey)}
              </p>
            ) : null}
            <div className="mt-4 flex items-center justify-between gap-3">
              <BackButton onClick={onBackToLogin} label={t("backToLogin")} />
              <SubmitButton
                busy={isSubmitting}
                label={t("sendRecovery")}
                busyLabel={t("sendingRecovery")}
              />
            </div>
          </form>
        ) : step === "complete" ? (
          <form
            onSubmit={handleComplete}
            className="glass-surface rounded-lg border p-4 shadow-sm"
          >
            <label
              htmlFor="recovery-token"
              className="mb-2 block text-sm font-medium"
            >
              {t("recoveryTokenLabel")}
            </label>
            <input
              id="recovery-token"
              type="password"
              value={token}
              onChange={(event) => setToken(event.target.value)}
              autoComplete="one-time-code"
              disabled={isSubmitting}
              className="w-full rounded-lg border border-input bg-muted px-3 py-2 font-mono text-sm focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-500/20 disabled:opacity-60"
            />
            <div className="mt-4 grid gap-4">
              <PasswordInput
                id="recovery-new-password"
                label={t("newPasswordLabel")}
                value={newPassword}
                onChange={setNewPassword}
                disabled={isSubmitting}
              />
              <PasswordInput
                id="recovery-confirm-password"
                label={t("confirmPasswordLabel")}
                value={confirmPassword}
                onChange={setConfirmPassword}
                disabled={isSubmitting}
              />
            </div>
            <p className="mt-3 text-xs text-muted-foreground">
              {t("passwordPolicy")}
            </p>
            {errorKey ? (
              <p className="mt-3 text-sm text-red-600" role="alert">
                {t(errorKey)}
              </p>
            ) : null}
            <div className="mt-4 flex items-center justify-between gap-3">
              <BackButton
                onClick={() => {
                  setErrorKey(null);
                  setStep("request");
                }}
                label={t("requestAgain")}
              />
              <SubmitButton
                busy={isSubmitting}
                label={t("resetPassword")}
                busyLabel={t("resettingPassword")}
              />
            </div>
          </form>
        ) : (
          <div className="glass-surface rounded-lg border p-4 shadow-sm">
            <p className="text-sm text-foreground">
              {t("recoveryCompletedMessage")}
            </p>
            <button
              type="button"
              onClick={onBackToLogin}
              className="mt-4 inline-flex w-full items-center justify-center gap-2 rounded-lg bg-foreground px-4 py-2 text-sm font-medium text-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              {t("backToLogin")}
              <ArrowRight size={16} aria-hidden="true" />
            </button>
          </div>
        )}
      </div>
    </main>
  );
};

const BackButton = ({
  onClick,
  label,
}: {
  onClick: () => void;
  label: string;
}) => (
  <button
    type="button"
    onClick={onClick}
    className="inline-flex items-center gap-1.5 rounded-lg px-2 py-2 text-sm text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
  >
    <ArrowLeft size={15} aria-hidden="true" />
    {label}
  </button>
);

const SubmitButton = ({
  busy,
  label,
  busyLabel,
}: {
  busy: boolean;
  label: string;
  busyLabel: string;
}) => (
  <button
    type="submit"
    disabled={busy}
    className="inline-flex items-center gap-2 rounded-lg bg-foreground px-4 py-2 text-sm font-medium text-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
  >
    {busy ? (
      <LoaderCircle className="animate-spin" size={16} aria-hidden="true" />
    ) : (
      <ArrowRight size={16} aria-hidden="true" />
    )}
    {busy ? busyLabel : label}
  </button>
);

const PasswordInput = ({
  id,
  label,
  value,
  onChange,
  disabled,
}: {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  disabled: boolean;
}) => (
  <div>
    <label htmlFor={id} className="mb-2 block text-sm font-medium">
      {label}
    </label>
    <input
      id={id}
      type="password"
      value={value}
      onChange={(event) => onChange(event.target.value)}
      autoComplete="new-password"
      disabled={disabled}
      className="w-full rounded-lg border border-input bg-muted px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-500/20 disabled:opacity-60"
    />
  </div>
);

export default ServerAuthRecoveryPanel;
