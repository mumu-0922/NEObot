import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const voiceSettings = readFileSync(
  resolve(process.cwd(), "src/components/settings/VoiceSettings.tsx"),
  "utf8",
);

describe("server-mode voice settings", () => {
  it("hides legacy local BYOK controls without deleting their rollback path", () => {
    expect(voiceSettings).toContain(
      'const serverModeEnabled = createNeoChatApiClient().mode === "server";',
    );
    const localControlsGuard = voiceSettings.indexOf(
      "{!serverModeEnabled && (",
    );
    const elevenLabsKey = voiceSettings.indexOf(
      'id="voice-elevenlabs-api-key"',
    );
    const mimoKey = voiceSettings.indexOf('id="voice-mimo-api-key"');
    const sttSection = voiceSettings.indexOf("{/* STT Section */}");

    expect(localControlsGuard).toBeGreaterThan(-1);
    expect(elevenLabsKey).toBeGreaterThan(localControlsGuard);
    expect(mimoKey).toBeGreaterThan(elevenLabsKey);
    expect(sttSection).toBeGreaterThan(mimoKey);
    expect(voiceSettings).toContain(
      "...(!serverModeEnabled && audioInputModels.length > 0",
    );
    expect(voiceSettings).toContain(
      "...(!serverModeEnabled && audioOutputModels.length > 0",
    );
    expect(voiceSettings.match(/\.\.\.\(!serverModeEnabled/g)).toHaveLength(4);
    expect(voiceSettings).not.toContain("...(!serverConfig");
  });
});
