import { unsupportedFeature } from "../errors";
import type { VoiceJobApi } from "../types";

export function createLocalVoiceJobApiShell(): VoiceJobApi {
  return {
    async synthesizeVoice() {
      throw unsupportedFeature("local server voice synthesis");
    },
  };
}
