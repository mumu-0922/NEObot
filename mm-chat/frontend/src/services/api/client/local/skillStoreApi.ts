import { unsupportedFeature } from "../errors";
import type { SkillStoreApi } from "../types";

const unavailable = () => {
  throw unsupportedFeature("server-owned Skill Store");
};

export function createLocalSkillStoreApiShell(): SkillStoreApi {
  return {
    listCatalog: unavailable,
    getCatalogSkill: unavailable,
    installCatalogSkill: unavailable,
    listPackageStore: unavailable,
    getPackageSkill: unavailable,
    listPackageLibrary: unavailable,
    installPackageSkill: unavailable,
    uninstallPackageSkill: unavailable,
    getConversationSelection: unavailable,
    replaceConversationSelection: unavailable,
  };
}
