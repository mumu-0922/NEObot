import { describe, expect, it } from "vitest";
import {
  MAX_PASSWORD_CHARACTERS,
  MIN_PASSWORD_CHARACTERS,
  validatePasswordPolicy,
} from "@/lib/auth/passwordPolicy";

describe("server auth password policy", () => {
  it("accepts the exact minimum with ASCII letters, numbers, and symbols", () => {
    expect(validatePasswordPolicy("A1!bcdef")).toBeNull();
    expect(validatePasswordPolicy("!@#$%^&*")).toBeNull();
    expect(MIN_PASSWORD_CHARACTERS).toBe(8);
  });

  it("rejects whitespace, control characters, and non-ASCII characters", () => {
    expect(validatePasswordPolicy("Pass word1!")).toBe("invalid-characters");
    expect(validatePasswordPolicy("Pass\tword1!")).toBe("invalid-characters");
    expect(validatePasswordPolicy("Pass\nword1!")).toBe("invalid-characters");
    expect(validatePasswordPolicy("密码Password1!")).toBe("invalid-characters");
    expect(validatePasswordPolicy("Password1!😀")).toBe("invalid-characters");
  });

  it("enforces the exact minimum and maximum", () => {
    expect(validatePasswordPolicy("A1!bcde")).toBe("too-short");
    expect(
      validatePasswordPolicy("a".repeat(MAX_PASSWORD_CHARACTERS)),
    ).toBeNull();
    expect(
      validatePasswordPolicy("a".repeat(MAX_PASSWORD_CHARACTERS + 1)),
    ).toBe("too-long");
  });
});
