export const MIN_PASSWORD_RUNES = 9;
export const MAX_PASSWORD_BYTES = 256;

export type PasswordPolicyFailure = "too-short" | "too-long" | null;

export function validatePasswordPolicy(
  password: string,
): PasswordPolicyFailure {
  if (Array.from(password).length < MIN_PASSWORD_RUNES) {
    return "too-short";
  }
  if (new TextEncoder().encode(password).length > MAX_PASSWORD_BYTES) {
    return "too-long";
  }
  return null;
}
