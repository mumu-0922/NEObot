import { describe, expect, it } from "vitest";
import en from "../i18n/locales/en";
import zh from "../i18n/locales/zh";

describe("default model labels", () => {
  it("renames prompt optimization to text polishing in both locales", () => {
    expect(zh.DefaultModels.promptOptimization).toBe("文本润色");
    expect(zh.DefaultModels.promptOptimizationDesc).toBe(
      "使用 AI 对文本内容进行优化",
    );
    expect(en.DefaultModels.promptOptimization).toBe("Text Polishing");
  });

  it("separates selectable Memory maintenance from fixed recall filtering", () => {
    expect(zh.DefaultModels.memory).toBe("记忆提取与维护");
    expect(zh.DefaultModels.recallFiltering).toBe("召回筛选");
    expect(zh.DefaultModels.systemFixed).toBe("系统固定");
    expect(en.DefaultModels.memory).toBe("Memory extraction and maintenance");
    expect(en.DefaultModels.recallFiltering).toBe("Recall filtering");
  });
});
