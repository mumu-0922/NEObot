import { describe, expect, it } from "vitest";
import {
  formatRecordingTime,
  isNativeMediaFile,
  resolveOpenComposerSection,
  shouldSubmitOnEnter,
  truncateMiddle,
} from "../lib/utils/messageInputHelpers";

describe("message input helpers", () => {
  it("keeps exactly one composer popover open", () => {
    expect(resolveOpenComposerSection(null, "skill", true)).toBe("skill");
    expect(resolveOpenComposerSection("skill", "mcp", true)).toBe("mcp");
    expect(resolveOpenComposerSection("mcp", "skill", false)).toBe("mcp");
    expect(resolveOpenComposerSection("mcp", "mcp", false)).toBeNull();
    expect(resolveOpenComposerSection("attachment", "model", true)).toBe(
      "model",
    );
  });

  it("truncates long labels from the middle", () => {
    expect(truncateMiddle("abcdefghijklmnopqrstuvwxyz", 10)).toBe("abcde…wxyz");
    expect(truncateMiddle("short", 10)).toBe("short");
  });

  it("formats recording seconds as minute and padded seconds", () => {
    expect(formatRecordingTime(0)).toBe("0:00");
    expect(formatRecordingTime(65)).toBe("1:05");
  });

  it("detects files that can be attached through native media handling", () => {
    expect(
      isNativeMediaFile(new File([""], "image.png", { type: "image/png" })),
    ).toBe(true);
    expect(
      isNativeMediaFile(new File([""], "audio.mp3", { type: "audio/mpeg" })),
    ).toBe(true);
    expect(
      isNativeMediaFile(new File([""], "doc.txt", { type: "text/plain" })),
    ).toBe(false);
  });

  it("does not submit Enter while an IME composition is active", () => {
    expect(
      shouldSubmitOnEnter({
        key: "Enter",
        shiftKey: false,
        isComposing: true,
      }),
    ).toBe(false);
    expect(
      shouldSubmitOnEnter({
        key: "Enter",
        shiftKey: false,
        isComposing: false,
      }),
    ).toBe(true);
    expect(
      shouldSubmitOnEnter({
        key: "Enter",
        shiftKey: true,
        isComposing: false,
      }),
    ).toBe(false);
  });
});
