import { describe, expect, it } from "vitest";
import {
  MAX_PASSWORD_BYTES,
  MIN_PASSWORD_RUNES,
  validatePasswordPolicy,
} from "@/lib/auth/passwordPolicy";

describe("server auth password policy", () => {
  it("accepts the exact rune minimum and preserves spaces", () => {
    expect(validatePasswordPolicy("123456789")).toBeNull();
    expect(validatePasswordPolicy(" 1234567 ")).toBeNull();
    expect(MIN_PASSWORD_RUNES).toBe(9);
  });

  it("counts Unicode code points rather than UTF-16 code units", () => {
    expect(validatePasswordPolicy("😀😀😀😀😀😀😀😀")).toBe("too-short");
    expect(validatePasswordPolicy("😀😀😀😀😀😀😀😀😀")).toBeNull();
  });

  it("enforces the encoded-byte maximum", () => {
    expect(validatePasswordPolicy("a".repeat(MAX_PASSWORD_BYTES))).toBeNull();
    expect(validatePasswordPolicy("a".repeat(MAX_PASSWORD_BYTES + 1))).toBe(
      "too-long",
    );
  });
});
