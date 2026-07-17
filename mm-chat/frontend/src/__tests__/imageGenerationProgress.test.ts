import { describe, expect, it } from "vitest";
import { getImageGenerationElapsedSeconds } from "../components/chat/ImageGenerationProgress";
import en from "../i18n/locales/en";
import ja from "../i18n/locales/ja";
import zh from "../i18n/locales/zh";

describe("image generation progress", () => {
  it("reports elapsed whole seconds without going negative", () => {
    expect(getImageGenerationElapsedSeconds(1_000, 4_999)).toBe(3);
    expect(getImageGenerationElapsedSeconds(5_000, 4_000)).toBe(0);
    expect(getImageGenerationElapsedSeconds(Number.NaN, 4_000)).toBe(0);
  });

  it("keeps progress copy localized", () => {
    expect(en.Message.generatingImage).toBe("Generating image");
    expect(en.Message.imageGenerationElapsed).toContain("{seconds}");
    expect(zh.Message.generatingImage).toBe("正在生成图片");
    expect(zh.Message.imageGenerationHint).toContain("自动显示");
    expect(ja.Message.generatingImage).toBe("画像を生成中");
  });
});
