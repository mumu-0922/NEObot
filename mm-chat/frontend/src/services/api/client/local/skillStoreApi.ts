import { unsupportedFeature } from "../errors";
import type { SkillStoreApi } from "../types";

const unavailable = () => {
  throw unsupportedFeature("server-owned Skill Store");
};

export function createLocalSkillStoreApiShell(): SkillStoreApi {
  return {
    listPackageStore: unavailable,
    getPackageSkill: unavailable,
    listPackageLibrary: unavailable,
    installPackageSkill: unavailable,
    uninstallPackageSkill: unavailable,
  };
}
