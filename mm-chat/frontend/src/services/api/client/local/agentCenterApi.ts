import { unsupportedFeature } from "../errors";
import type { AgentCenterApi } from "../types";

const unavailable = () => {
  throw unsupportedFeature("server-owned Agent Center");
};

export function createLocalAgentCenterApiShell(): AgentCenterApi {
  return {
    getStatus: unavailable,
    listPackageStore: unavailable,
    getPackageSkill: unavailable,
    listPackageLibrary: unavailable,
    installPackageSkill: unavailable,
    uninstallPackageSkill: unavailable,
    listRuns: unavailable,
    getRun: unavailable,
    downloadArtifact: unavailable,
    enqueueRun: unavailable,
    cancelRun: unavailable,
    decideApproval: unavailable,
    listSchedules: unavailable,
    getSchedule: unavailable,
    createSchedule: unavailable,
    changeScheduleLifecycle: unavailable,
    listDrafts: unavailable,
    getDraft: unavailable,
    getDraftDiff: unavailable,
    reviewDraft: unavailable,
    setShadowOptIn: unavailable,
  };
}
