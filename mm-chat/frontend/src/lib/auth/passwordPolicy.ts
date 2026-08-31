export const MIN_PASSWORD_CHARACTERS = 8;
export const MAX_PASSWORD_CHARACTERS = 256;

export type PasswordPolicyFailure =
  "too-short" | "too-long" | "invalid-characters" | null;

export function validatePasswordPolicy(
  password: string,
): PasswordPolicyFailure {
  for (let index = 0; index < password.length; index += 1) {
    const code = password.charCodeAt(index);
    if (code < 0x21 || code > 0x7e) {
      return "invalid-characters";
    }
  }
  if (password.length < MIN_PASSWORD_CHARACTERS) {
    return "too-short";
  }
  if (password.length > MAX_PASSWORD_CHARACTERS) {
    return "too-long";
  }
  return null;
}
