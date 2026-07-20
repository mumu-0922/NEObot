import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

describe("generation error UI wiring", () => {
  it("renders generation errors as metadata instead of assistant text", () => {
    const chatApp = readFileSync(
      resolve(process.cwd(), "src/components/app/ChatApp.tsx"),
      "utf8",
    );
    const messageItem = readFileSync(
      resolve(process.cwd(), "src/components/chat/MessageItem.tsx"),
      "utf8",
    );

    expect(messageItem).toContain("message.generationError");
    expect(messageItem).toContain("generationFailed");
    expect(chatApp).not.toContain("`Error: ${errorMessage}`");
  });

  it("localizes provider content-policy failures in every supported locale", () => {
    const messageItem = readFileSync(
      resolve(process.cwd(), "src/components/chat/MessageItem.tsx"),
      "utf8",
    );
    expect(messageItem).toContain("IMAGE_CONTENT_POLICY_VIOLATION_CODE");
    expect(messageItem).toContain('t("imageContentPolicyViolation")');
    expect(messageItem).toContain("IMAGE_PROVIDER_TIMEOUT_CODE");
    expect(messageItem).toContain('t("imageProviderTimeout")');

    for (const locale of ["zh", "en", "ja"]) {
      const messages = JSON.parse(
        readFileSync(
          resolve(process.cwd(), `src/i18n/locales/${locale}/Message.json`),
          "utf8",
        ),
      ) as Record<string, string>;
      expect(messages.imageContentPolicyViolation).toBeTruthy();
      expect(messages.imageProviderTimeout).toBeTruthy();
    }
  });
});
