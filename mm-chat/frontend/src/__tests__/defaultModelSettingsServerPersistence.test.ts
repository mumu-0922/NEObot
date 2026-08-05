import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

describe("DefaultModelSettings server persistence composition", () => {
  it("bootstraps once, autosaves selections, and rolls back failures", () => {
    const source = readFileSync(
      resolve(
        process.cwd(),
        "src/components/settings/DefaultModelSettings.tsx",
      ),
      "utf8",
    );

    expect(source).toContain("getTaskModels({ signal: controller.signal })");
    expect(source).toContain("getHealth({ signal: controller.signal })");
    expect(source).not.toContain("bootstrapStartedRef");
    expect(source).toContain("[apiClient, serverMode, t, updateDefaultModels]");
    expect(source).toContain("response.configured");
    expect(source).toContain("updateTaskModels({");
    expect(source).toContain("updateDefaultModels({ [valueKey]: previous })");
    expect(source).toContain('setSaveStatus("saving")');
    expect(source).toContain('setSaveStatus("saved")');
    expect(source).toContain('setSaveStatus("error")');
    expect(source).toContain("disabled={savingKey !== undefined");
    expect(source).toContain('t("recallFiltering")');
    expect(source).toContain('t("systemFixed")');
    expect(source).toContain("memoryHealth.judgeModelId");
  });
});
