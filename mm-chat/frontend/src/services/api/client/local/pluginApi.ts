import { unsupportedFeature } from "../errors";
import type {
  PluginApi,
  PluginExecuteInput,
  PluginInstallInput,
  PluginInstallResponse,
  PluginListAvailableResponse,
} from "../types";

export function createLocalPluginApiShell(): PluginApi {
  return {
    async listAvailable(input = {}): Promise<PluginListAvailableResponse> {
      void input;
      throw unsupportedFeature(
        "local plugin registry after G9.4 route removal",
      );
    },
    async install(input: PluginInstallInput): Promise<PluginInstallResponse> {
      void input;
      throw unsupportedFeature("local plugin install after G9.4 route removal");
    },
    async execute(input: PluginExecuteInput): Promise<Response> {
      void input;
      throw unsupportedFeature(
        "local plugin execution after G9.4 route removal",
      );
    },
  };
}
