"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import {
  Activity,
  ArrowLeft,
  Bot,
  BrainCircuit,
  CalendarClock,
  Check,
  Download,
  Loader2,
  PackageCheck,
  Pause,
  Play,
  RefreshCw,
  ShieldAlert,
  Store,
  Trash2,
  X,
  XCircle,
} from "lucide-react";
import { useTranslations } from "next-intl";

import LegacySkillCutoverCard from "./LegacySkillCutoverCard";
import type { AgentCenterTabId } from "@/lib/chat/panelUrlState";
import {
  ApiClientError,
  createNeoChatApiClient,
  type AgentCenterStatusDTO,
  type AgentDraftDTO,
  type AgentDraftDiffDTO,
  type AgentPackageCandidateDTO,
  type AgentPackageInstallationDTO,
  type AgentRunDetailDTO,
  type AgentRunSummaryDTO,
  type AgentScheduleDetailDTO,
  type AgentScheduleSummaryDTO,
} from "@/services/api/client";

interface AgentCenterProps {
  activeTab: AgentCenterTabId;
  selectedId: string | null;
  onNavigate: (
    tab: AgentCenterTabId,
    id: string | null,
    historyMode?: "push" | "replace",
  ) => void;
  onClose: () => void;
}

const TABS: Array<{
  id: AgentCenterTabId;
  icon: typeof Store;
  admin?: boolean;
}> = [
  { id: "skills", icon: PackageCheck },
  { id: "runs", icon: Activity },
  { id: "schedules", icon: CalendarClock },
  { id: "learning", icon: BrainCircuit, admin: true },
];

export default function AgentCenter({
  activeTab,
  selectedId,
  onNavigate,
  onClose,
}: AgentCenterProps) {
  const t = useTranslations("AgentCenter");
  const client = useMemo(() => createNeoChatApiClient(), []);
  const [status, setStatus] = useState<AgentCenterStatusDTO | null>(null);
  const [statusError, setStatusError] = useState("");
  const [announcement, setAnnouncement] = useState("");

  const loadStatus = useCallback(async () => {
    setStatusError("");
    try {
      setStatus(await client.agentCenter.getStatus());
    } catch (error) {
      setStatusError(errorMessage(error, t("controlUnavailable")));
    }
  }, [client.agentCenter, t]);

  useEffect(() => {
    queueMicrotask(() => void loadStatus());
  }, [loadStatus]);

  useEffect(() => {
    if (status && activeTab === "learning" && !status.isAdministrator) {
      onNavigate("skills", null, "replace");
    }
  }, [activeTab, onNavigate, status]);

  const setShadowOptIn = async () => {
    if (!status || status.shadow.policy.revision < 1) return;
    try {
      const shadow = await client.agentCenter.setShadowOptIn({
        expectedGeneration: status.shadow.optIn.generation,
        policyRevision: status.shadow.policy.revision,
        optedIn: !status.shadow.optIn.optedIn,
        reasonCode: status.shadow.optIn.optedIn
          ? "USER_OPT_OUT"
          : "USER_OPT_IN",
      });
      setStatus({ ...status, shadow });
      setAnnouncement(
        shadow.optIn.optedIn ? t("shadowOptedIn") : t("shadowOptedOut"),
      );
    } catch (error) {
      setAnnouncement(errorMessage(error, t("actionFailed")));
      await loadStatus();
    }
  };

  const shared = {
    selectedId,
    onSelect: (id: string | null) => onNavigate(activeTab, id),
    announce: setAnnouncement,
  };

  return (
    <div className="flex h-full w-full flex-col overflow-hidden bg-slate-50/80 dark:bg-background">
      <a className="skip-link" href="#agent-center-content">
        {t("skipToContent")}
      </a>
      <header className="shrink-0 border-b border-slate-200/80 bg-white/95 px-4 pt-4 dark:border-border dark:bg-card/95 md:px-6">
        <div className="flex items-center justify-between gap-3 pb-4">
          <div className="flex min-w-0 items-center gap-3">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-slate-950 text-white dark:bg-cyan-500 dark:text-slate-950">
              <Bot size={20} aria-hidden="true" />
            </div>
            <div className="min-w-0">
              <h1 className="truncate text-lg font-bold">{t("title")}</h1>
              <p className="mt-0.5 truncate text-xs text-muted-foreground">
                {t("subtitle")}
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label={t("close")}
            className="rounded-full p-2 text-muted-foreground hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/70"
          >
            <X size={20} aria-hidden="true" />
          </button>
        </div>

        <div className="mb-3 flex flex-wrap items-center justify-between gap-2 rounded-xl border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-950 dark:border-amber-900/60 dark:bg-amber-950/25 dark:text-amber-100">
          <div className="flex min-w-0 items-center gap-2">
            <ShieldAlert size={16} className="shrink-0" aria-hidden="true" />
            <span>
              {statusError ||
                (status
                  ? t("heldStatus", { reason: status.runtime.reasonCode })
                  : t("loading"))}
            </span>
          </div>
          {status?.shadow.policy.revision ? (
            <button
              type="button"
              onClick={() => void setShadowOptIn()}
              className="rounded-lg border border-amber-300 bg-white px-2.5 py-1.5 font-medium hover:bg-amber-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-amber-500 dark:border-amber-800 dark:bg-amber-950/40"
            >
              {status.shadow.optIn.optedIn
                ? t("shadowOptOut")
                : t("shadowOptIn")}
            </button>
          ) : null}
        </div>

        <div
          role="tablist"
          aria-label={t("tabs")}
          className="flex gap-1 overflow-x-auto"
        >
          {TABS.filter((tab) => !tab.admin || status?.isAdministrator).map(
            (tab) => {
              const Icon = tab.icon;
              return (
                <button
                  key={tab.id}
                  type="button"
                  role="tab"
                  aria-selected={activeTab === tab.id}
                  onClick={() => onNavigate(tab.id, null)}
                  className={`inline-flex shrink-0 items-center gap-1.5 border-b-2 px-3 py-2 text-sm font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/60 ${
                    activeTab === tab.id
                      ? "border-cyan-600 text-cyan-800 dark:text-cyan-300"
                      : "border-transparent text-muted-foreground hover:text-foreground"
                  }`}
                >
                  <Icon size={15} aria-hidden="true" />
                  {t(`tab.${tab.id}`)}
                </button>
              );
            },
          )}
        </div>
      </header>

      <div
        id="agent-center-content"
        tabIndex={-1}
        className="min-h-0 flex-1 overflow-hidden"
      >
        {activeTab === "skills" ? (
          <PackageSkills {...shared} />
        ) : activeTab === "runs" ? (
          <Runs {...shared} />
        ) : activeTab === "schedules" ? (
          <Schedules {...shared} />
        ) : status?.isAdministrator ? (
          <Learning {...shared} />
        ) : (
          <EmptyState title={t("adminOnly")} />
        )}
      </div>
      <div
        className="sr-only"
        role="status"
        aria-live="polite"
        aria-atomic="true"
      >
        {announcement}
      </div>
    </div>
  );
}

interface PanelProps {
  selectedId: string | null;
  onSelect: (id: string | null) => void;
  announce: (message: string) => void;
}

function PackageSkills({ selectedId, onSelect, announce }: PanelProps) {
  const t = useTranslations("AgentCenter");
  const client = useMemo(() => createNeoChatApiClient(), []);
  const [items, setItems] = useState<AgentPackageCandidateDTO[]>([]);
  const [installed, setInstalled] = useState<AgentPackageInstallationDTO[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [actionId, setActionId] = useState("");
  const restoreFocus = useRef<HTMLButtonElement | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [store, library] = await Promise.all([
        client.agentCenter.listPackageStore(),
        client.agentCenter.listPackageLibrary(),
      ]);
      setItems(store.items);
      setInstalled(library);
    } catch (error) {
      setError(errorMessage(error, t("loadFailed")));
    } finally {
      setLoading(false);
    }
  }, [client.agentCenter, t]);

  useEffect(() => {
    queueMicrotask(() => void load());
  }, [load]);
  const selected = items.find((item) => item.id === selectedId) ?? null;
  const installedByAdmission = new Map(
    installed.map((item) => [item.admissionId, item]),
  );

  const install = async (item: AgentPackageCandidateDTO) => {
    setActionId(item.id);
    try {
      await client.agentCenter.installPackageSkill({
        candidateId: item.id,
        packageFingerprint: item.package.packageFingerprint,
      });
      announce(t("installedAnnouncement", { name: item.package.name }));
      await load();
    } catch (error) {
      announce(errorMessage(error, t("actionFailed")));
      if (isStale(error)) await load();
    } finally {
      setActionId("");
    }
  };

  const uninstall = async (entry: AgentPackageInstallationDTO) => {
    setActionId(entry.id);
    try {
      await client.agentCenter.uninstallPackageSkill({
        installationId: entry.id,
        revision: entry.revision,
      });
      announce(t("uninstalledAnnouncement", { name: entry.name }));
      await load();
    } catch (error) {
      announce(errorMessage(error, t("actionFailed")));
      if (isStale(error)) await load();
    } finally {
      setActionId("");
    }
  };

  return (
    <SplitShell
      selected={Boolean(selected)}
      list={
        <>
          <PanelHeader
            title={t("packageSkills")}
            onReload={() => void load()}
          />
          <div className="space-y-5 p-4">
            <section>
              <h2 className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                {t("installedPackages", { count: installed.length })}
              </h2>
              {installed.length ? (
                <div className="space-y-2">
                  {installed.map((entry) => (
                    <div
                      key={entry.id}
                      className="rounded-xl border bg-card p-3"
                    >
                      <div className="flex items-start justify-between gap-2">
                        <div>
                          <p className="font-medium">{entry.name}</p>
                          <p className="text-xs text-muted-foreground">
                            v{entry.version}
                          </p>
                        </div>
                        <button
                          type="button"
                          disabled={actionId === entry.id}
                          onClick={() => void uninstall(entry)}
                          aria-label={t("uninstallAria", { name: entry.name })}
                          className="rounded-lg p-2 text-red-600 hover:bg-red-50 disabled:opacity-50 dark:hover:bg-red-950/30"
                        >
                          <Trash2 size={15} aria-hidden="true" />
                        </button>
                      </div>
                      <Fingerprint value={entry.packageFingerprint} />
                    </div>
                  ))}
                </div>
              ) : (
                <p className="text-sm text-muted-foreground">
                  {t("noInstalledPackages")}
                </p>
              )}
            </section>

            <section>
              <h2 className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                {t("packageStore")}
              </h2>
              {loading ? (
                <Loading />
              ) : error ? (
                <InlineError message={error} retry={() => void load()} />
              ) : items.length ? (
                <div className="space-y-2">
                  {items.map((item) => (
                    <button
                      key={item.id}
                      type="button"
                      onClick={(event) => {
                        restoreFocus.current = event.currentTarget;
                        onSelect(item.id);
                      }}
                      aria-current={selectedId === item.id ? "true" : undefined}
                      className={`w-full rounded-xl border p-3 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500 ${
                        selectedId === item.id
                          ? "border-cyan-500 bg-cyan-50 dark:bg-cyan-950/20"
                          : "bg-card hover:border-slate-300 dark:hover:border-slate-700"
                      }`}
                    >
                      <div className="flex items-center justify-between gap-2">
                        <span className="font-medium">{item.package.name}</span>
                        <StatusPill value={item.status} />
                      </div>
                      <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">
                        {item.package.description}
                      </p>
                    </button>
                  ))}
                </div>
              ) : (
                <EmptyState title={t("emptyStore")} />
              )}
            </section>
            <LegacySkillCutoverCard />
          </div>
        </>
      }
      detail={
        selected ? (
          <DetailFrame
            title={selected.package.name}
            onBack={() => closeDetail(onSelect, restoreFocus)}
          >
            <p className="text-sm text-muted-foreground">
              {selected.package.description}
            </p>
            <DetailGrid
              rows={[
                [t("version"), selected.package.version],
                [t("source"), selected.sourceType],
                [
                  t("runtime"),
                  selected.package.hasRuntime
                    ? t("runtimePackage")
                    : t("textOnlyPackage"),
                ],
                [t("validation"), selected.validationSummary],
              ]}
            />
            <Fingerprint
              label={t("packageFingerprint")}
              value={selected.package.packageFingerprint}
              full
            />
            <Fingerprint
              label={t("runtimeFingerprint")}
              value={selected.package.runtimeBundleFingerprint ?? t("none")}
              full
            />
            <Fingerprint
              label={t("sbomFingerprint")}
              value={selected.package.sbomFingerprint}
              full
            />
            <div>
              <h3 className="text-sm font-semibold">{t("declaredTools")}</h3>
              <p className="mt-1 text-sm text-muted-foreground">
                {selected.package.allowedTools.join(", ") || t("none")}
              </p>
            </div>
            <button
              type="button"
              disabled={
                Boolean(installedByAdmission.get(selected.id)) ||
                actionId === selected.id
              }
              onClick={() => void install(selected)}
              className="inline-flex items-center gap-2 rounded-xl bg-slate-950 px-4 py-2.5 text-sm font-medium text-white disabled:cursor-not-allowed disabled:opacity-50 dark:bg-cyan-500 dark:text-slate-950"
            >
              {actionId === selected.id ? (
                <Loader2 size={16} className="animate-spin" />
              ) : (
                <Download size={16} />
              )}
              {installedByAdmission.has(selected.id)
                ? t("installed")
                : t("installPackage")}
            </button>
          </DetailFrame>
        ) : (
          <EmptyDetail title={t("selectPackage")} />
        )
      }
    />
  );
}

function Runs({ selectedId, onSelect, announce }: PanelProps) {
  const t = useTranslations("AgentCenter");
  const client = useMemo(() => createNeoChatApiClient(), []);
  const [runs, setRuns] = useState<AgentRunSummaryDTO[]>([]);
  const [detail, setDetail] = useState<AgentRunDetailDTO | null>(null);
  const [loading, setLoading] = useState(true);
  const [detailLoading, setDetailLoading] = useState(false);
  const [listError, setListError] = useState("");
  const [detailError, setDetailError] = useState("");
  const [action, setAction] = useState("");
  const restoreFocus = useRef<HTMLButtonElement | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setListError("");
    try {
      setRuns(await client.agentCenter.listRuns());
    } catch (error) {
      setListError(errorMessage(error, t("loadFailed")));
    } finally {
      setLoading(false);
    }
  }, [client.agentCenter, t]);

  const loadDetail = useCallback(async () => {
    if (!selectedId) {
      setDetail(null);
      setDetailError("");
      setDetailLoading(false);
      return;
    }
    setDetailLoading(true);
    setDetailError("");
    setDetail(null);
    try {
      setDetail(await client.agentCenter.getRun(selectedId));
    } catch (error) {
      setDetailError(errorMessage(error, t("loadFailed")));
    } finally {
      setDetailLoading(false);
    }
  }, [client.agentCenter, selectedId, t]);

  useEffect(() => {
    queueMicrotask(() => void load());
  }, [load]);
  useEffect(() => {
    queueMicrotask(() => void loadDetail());
  }, [loadDetail]);

  const cancel = async (mode: "cancel" | "kill") => {
    if (!detail) return;
    setAction(mode);
    try {
      await client.agentCenter.cancelRun({
        runId: detail.run.id,
        expectedState: detail.run.state,
        snapshotFingerprint: detail.run.snapshotFingerprint,
        mode,
        reasonCode: mode === "kill" ? "USER_KILLED" : "USER_CANCELED",
      });
      announce(t("runChanged", { state: mode }));
      await Promise.all([load(), loadDetail()]);
    } catch (error) {
      announce(errorMessage(error, t("actionFailed")));
      if (isStale(error)) await Promise.all([load(), loadDetail()]);
    } finally {
      setAction("");
    }
  };

  const decide = async (
    approval: AgentRunDetailDTO["approvals"][number],
    decision: "approved" | "denied",
  ) => {
    if (!detail) return;
    setAction(approval.intentId);
    try {
      await client.agentCenter.decideApproval({
        runId: detail.run.id,
        intentId: approval.intentId,
        intentFingerprint: approval.intentFingerprint,
        decision,
        expectedRevision: approval.approvalRevision,
        reasonCode: decision === "approved" ? "USER_APPROVED" : "USER_DENIED",
      });
      announce(t("approvalChanged", { decision }));
      await loadDetail();
    } catch (error) {
      announce(errorMessage(error, t("actionFailed")));
      if (isStale(error)) await loadDetail();
    } finally {
      setAction("");
    }
  };

  const downloadArtifact = async (
    artifact: AgentRunDetailDTO["artifacts"][number],
  ) => {
    if (!detail) return;
    setAction(artifact.id);
    try {
      const result = await client.agentCenter.downloadArtifact({
        runId: detail.run.id,
        artifactId: artifact.id,
      });
      const url = URL.createObjectURL(result.blob);
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = artifact.name;
      anchor.click();
      setTimeout(() => URL.revokeObjectURL(url), 0);
      announce(t("artifactDownloaded", { name: artifact.name }));
    } catch (error) {
      announce(errorMessage(error, t("actionFailed")));
    } finally {
      setAction("");
    }
  };

  return (
    <SplitShell
      selected={Boolean(selectedId)}
      list={
        <>
          <PanelHeader title={t("runs")} onReload={() => void load()} />
          <div className="p-4">
            {loading ? (
              <Loading />
            ) : listError ? (
              <InlineError message={listError} retry={() => void load()} />
            ) : runs.length ? (
              <div className="space-y-2">
                {runs.map((run) => (
                  <button
                    key={run.id}
                    type="button"
                    onClick={(event) => {
                      restoreFocus.current = event.currentTarget;
                      onSelect(run.id);
                    }}
                    className="w-full rounded-xl border bg-card p-3 text-left hover:border-slate-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500 dark:hover:border-slate-700"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="font-mono text-xs">{run.id}</span>
                      <StatusPill value={run.state} />
                    </div>
                    <p className="mt-2 text-xs text-muted-foreground">
                      {formatDate(run.updatedAt)}
                    </p>
                  </button>
                ))}
              </div>
            ) : (
              <EmptyState title={t("noRuns")} />
            )}
          </div>
        </>
      }
      detail={
        detailLoading && selectedId ? (
          <DetailFrame
            title={selectedId}
            onBack={() => closeDetail(onSelect, restoreFocus)}
          >
            <Loading />
          </DetailFrame>
        ) : detailError && selectedId ? (
          <DetailFrame
            title={selectedId}
            onBack={() => closeDetail(onSelect, restoreFocus)}
          >
            <InlineError
              message={detailError}
              retry={() => void loadDetail()}
            />
          </DetailFrame>
        ) : detail ? (
          <DetailFrame
            title={detail.run.id}
            onBack={() => closeDetail(onSelect, restoreFocus)}
          >
            <div className="flex flex-wrap items-center gap-2">
              <StatusPill value={detail.run.state} />
              {isCancelable(detail.run.state) && (
                <>
                  <Action
                    onClick={() => void cancel("cancel")}
                    disabled={Boolean(action)}
                    icon={<XCircle size={15} />}
                  >
                    {t("cancelRun")}
                  </Action>
                  <Action
                    danger
                    onClick={() => void cancel("kill")}
                    disabled={Boolean(action)}
                    icon={<ShieldAlert size={15} />}
                  >
                    {t("killRun")}
                  </Action>
                </>
              )}
            </div>
            <Fingerprint
              label={t("snapshotFingerprint")}
              value={detail.run.snapshotFingerprint}
              full
            />
            <section>
              <h3 className="mb-2 text-sm font-semibold">{t("approvals")}</h3>
              {detail.approvals.length ? (
                detail.approvals.map((approval) => (
                  <div
                    key={approval.intentId}
                    className="mb-2 rounded-xl border bg-card p-3"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="text-sm font-medium">
                        {approval.toolIdentity} · {approval.action}
                      </span>
                      <StatusPill value={approval.state} />
                    </div>
                    <Fingerprint value={approval.argumentsFingerprint} />
                    {approval.state === "awaiting_approval" && (
                      <div className="mt-3 flex gap-2">
                        <Action
                          onClick={() => void decide(approval, "approved")}
                          disabled={Boolean(action)}
                          icon={<Check size={15} />}
                        >
                          {t("approve")}
                        </Action>
                        <Action
                          danger
                          onClick={() => void decide(approval, "denied")}
                          disabled={Boolean(action)}
                          icon={<XCircle size={15} />}
                        >
                          {t("deny")}
                        </Action>
                      </div>
                    )}
                  </div>
                ))
              ) : (
                <p className="text-sm text-muted-foreground">{t("none")}</p>
              )}
            </section>
            <Timeline detail={detail} />
            <section>
              <h3 className="mb-2 text-sm font-semibold">{t("childRuns")}</h3>
              {detail.children.length ? (
                detail.children.map((child) => (
                  <div
                    key={child.runId}
                    className="mb-2 rounded-xl border bg-card p-3 text-sm"
                  >
                    <span className="font-mono text-xs">{child.runId}</span> ·{" "}
                    {t("depth", { depth: child.depth })} ·{" "}
                    <StatusPill value={child.state} />
                  </div>
                ))
              ) : (
                <p className="text-sm text-muted-foreground">{t("none")}</p>
              )}
            </section>
            <section>
              <h3 className="mb-2 text-sm font-semibold">{t("artifacts")}</h3>
              {detail.artifacts.length ? (
                detail.artifacts.map((artifact) => (
                  <button
                    key={artifact.id}
                    type="button"
                    onClick={() => void downloadArtifact(artifact)}
                    disabled={action === artifact.id}
                    className="mb-2 flex w-full items-center justify-between gap-3 rounded-xl border bg-card p-3 text-left hover:border-cyan-500 disabled:opacity-50"
                  >
                    <span>
                      <span className="block text-sm font-medium">
                        {artifact.name}
                      </span>
                      <span className="text-xs text-muted-foreground">
                        {artifact.mediaType} · {artifact.size} B
                      </span>
                    </span>
                    <Download size={16} aria-hidden="true" />
                  </button>
                ))
              ) : (
                <p className="text-sm text-muted-foreground">{t("none")}</p>
              )}
            </section>
          </DetailFrame>
        ) : (
          <EmptyDetail title={t("selectRun")} />
        )
      }
    />
  );
}

function Schedules({ selectedId, onSelect, announce }: PanelProps) {
  const t = useTranslations("AgentCenter");
  const client = useMemo(() => createNeoChatApiClient(), []);
  const [items, setItems] = useState<AgentScheduleSummaryDTO[]>([]);
  const [detail, setDetail] = useState<AgentScheduleDetailDTO | null>(null);
  const [loading, setLoading] = useState(true);
  const [detailLoading, setDetailLoading] = useState(false);
  const [listError, setListError] = useState("");
  const [detailError, setDetailError] = useState("");
  const [action, setAction] = useState(false);
  const [templateId, setTemplateId] = useState("");
  const [specText, setSpecText] = useState("{}");
  const restoreFocus = useRef<HTMLButtonElement | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setListError("");
    try {
      setItems(await client.agentCenter.listSchedules());
    } catch (error) {
      setListError(errorMessage(error, t("loadFailed")));
    } finally {
      setLoading(false);
    }
  }, [client.agentCenter, t]);

  const loadDetail = useCallback(async () => {
    if (!selectedId) {
      setDetail(null);
      setDetailError("");
      setDetailLoading(false);
      return;
    }
    setDetailLoading(true);
    setDetailError("");
    setDetail(null);
    try {
      setDetail(await client.agentCenter.getSchedule(selectedId));
    } catch (error) {
      setDetailError(errorMessage(error, t("loadFailed")));
    } finally {
      setDetailLoading(false);
    }
  }, [client.agentCenter, selectedId, t]);

  useEffect(() => {
    queueMicrotask(() => void load());
  }, [load]);
  useEffect(() => {
    queueMicrotask(() => void loadDetail());
  }, [loadDetail]);

  const lifecycle = async (state: "active" | "paused" | "deleted") => {
    if (!detail) return;
    setAction(true);
    try {
      const updated = await client.agentCenter.changeScheduleLifecycle({
        scheduleId: detail.id,
        expectedRevision: detail.currentRevision,
        state,
        reasonCode: `USER_${state.toUpperCase()}`,
      });
      setDetail(updated);
      announce(t("scheduleChanged", { state }));
      await load();
    } catch (error) {
      announce(errorMessage(error, t("actionFailed")));
      if (isStale(error)) await Promise.all([load(), loadDetail()]);
    } finally {
      setAction(false);
    }
  };

  const createSchedule = async () => {
    setAction(true);
    try {
      const parsed = JSON.parse(specText) as unknown;
      if (!parsed || typeof parsed !== "object" || Array.isArray(parsed))
        throw new Error();
      const created = await client.agentCenter.createSchedule({
        templateId,
        expectedRevision: 0,
        spec: parsed as Record<string, unknown>,
        reasonCode: "USER_CREATED",
      });
      announce(t("scheduleCreated"));
      await load();
      onSelect(created.id);
    } catch (error) {
      announce(errorMessage(error, t("invalidScheduleSpec")));
    } finally {
      setAction(false);
    }
  };

  return (
    <SplitShell
      selected={Boolean(selectedId)}
      list={
        <>
          <PanelHeader title={t("schedules")} onReload={() => void load()} />
          <div className="space-y-4 p-4">
            <details className="rounded-xl border bg-card p-3">
              <summary className="cursor-pointer text-sm font-medium">
                {t("createSchedule")}
              </summary>
              <label className="mt-3 block text-xs font-medium">
                {t("templateId")}
                <input
                  value={templateId}
                  onChange={(event) => setTemplateId(event.target.value)}
                  className="mt-1 w-full rounded-lg border bg-background px-3 py-2 font-mono"
                />
              </label>
              <label className="mt-3 block text-xs font-medium">
                {t("scheduleSpec")}
                <textarea
                  value={specText}
                  onChange={(event) => setSpecText(event.target.value)}
                  rows={8}
                  className="mt-1 w-full rounded-lg border bg-background px-3 py-2 font-mono"
                />
              </label>
              <button
                type="button"
                disabled={action || !templateId}
                onClick={() => void createSchedule()}
                className="mt-3 rounded-lg bg-slate-950 px-3 py-2 text-xs font-medium text-white disabled:opacity-50 dark:bg-cyan-500 dark:text-slate-950"
              >
                {t("create")}
              </button>
            </details>
            {loading ? (
              <Loading />
            ) : listError ? (
              <InlineError message={listError} retry={() => void load()} />
            ) : items.length ? (
              <div className="space-y-2">
                {items.map((item) => (
                  <button
                    key={item.id}
                    type="button"
                    onClick={(event) => {
                      restoreFocus.current = event.currentTarget;
                      onSelect(item.id);
                    }}
                    className="w-full rounded-xl border bg-card p-3 text-left hover:border-slate-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="font-mono text-xs">{item.id}</span>
                      <StatusPill value={item.state} />
                    </div>
                    <p className="mt-2 text-sm">
                      {item.scheduleExpression} · {item.timezone}
                    </p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {item.nextTriggerAt
                        ? t("nextTrigger", {
                            time: formatDate(item.nextTriggerAt),
                          })
                        : t("noNextTrigger")}
                    </p>
                  </button>
                ))}
              </div>
            ) : (
              <EmptyState title={t("noSchedules")} />
            )}
          </div>
        </>
      }
      detail={
        detailLoading && selectedId ? (
          <DetailFrame
            title={selectedId}
            onBack={() => closeDetail(onSelect, restoreFocus)}
          >
            <Loading />
          </DetailFrame>
        ) : detailError && selectedId ? (
          <DetailFrame
            title={selectedId}
            onBack={() => closeDetail(onSelect, restoreFocus)}
          >
            <InlineError
              message={detailError}
              retry={() => void loadDetail()}
            />
          </DetailFrame>
        ) : detail ? (
          <DetailFrame
            title={detail.id}
            onBack={() => closeDetail(onSelect, restoreFocus)}
          >
            <div className="flex flex-wrap items-center gap-2">
              <StatusPill value={detail.state} />
              {detail.state === "active" ? (
                <Action
                  onClick={() => void lifecycle("paused")}
                  disabled={action}
                  icon={<Pause size={15} />}
                >
                  {t("pause")}
                </Action>
              ) : detail.state === "paused" ? (
                <Action
                  onClick={() => void lifecycle("active")}
                  disabled={action}
                  icon={<Play size={15} />}
                >
                  {t("resume")}
                </Action>
              ) : null}
              {detail.state !== "deleted" && (
                <Action
                  danger
                  onClick={() => void lifecycle("deleted")}
                  disabled={action}
                  icon={<Trash2 size={15} />}
                >
                  {t("delete")}
                </Action>
              )}
            </div>
            <DetailGrid
              rows={[
                [t("revision"), String(detail.currentRevision)],
                [
                  t("nextTriggerLabel"),
                  detail.nextTriggerAt
                    ? formatDate(detail.nextTriggerAt)
                    : t("none"),
                ],
              ]}
            />
            <Fingerprint
              label={t("revisionFingerprint")}
              value={detail.revisionFingerprint}
              full
            />
            <section>
              <h3 className="mb-2 text-sm font-semibold">{t("frozenSpec")}</h3>
              <pre className="max-h-[28rem] overflow-auto rounded-xl bg-slate-950 p-4 text-xs text-slate-100">
                {JSON.stringify(detail.spec, null, 2)}
              </pre>
            </section>
          </DetailFrame>
        ) : (
          <EmptyDetail title={t("selectSchedule")} />
        )
      }
    />
  );
}

function Learning({ selectedId, onSelect, announce }: PanelProps) {
  const t = useTranslations("AgentCenter");
  const client = useMemo(() => createNeoChatApiClient(), []);
  const [items, setItems] = useState<AgentDraftDTO[]>([]);
  const [detail, setDetail] = useState<AgentDraftDTO | null>(null);
  const [diff, setDiff] = useState<AgentDraftDiffDTO[]>([]);
  const [loading, setLoading] = useState(true);
  const [detailLoading, setDetailLoading] = useState(false);
  const [listError, setListError] = useState("");
  const [detailError, setDetailError] = useState("");
  const [action, setAction] = useState(false);
  const restoreFocus = useRef<HTMLButtonElement | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setListError("");
    try {
      setItems(await client.agentCenter.listDrafts());
    } catch (error) {
      setListError(errorMessage(error, t("loadFailed")));
    } finally {
      setLoading(false);
    }
  }, [client.agentCenter, t]);

  const loadDetail = useCallback(async () => {
    if (!selectedId) {
      setDetail(null);
      setDiff([]);
      setDetailError("");
      setDetailLoading(false);
      return;
    }
    setDetailLoading(true);
    setDetailError("");
    setDetail(null);
    setDiff([]);
    try {
      const [draft, files] = await Promise.all([
        client.agentCenter.getDraft(selectedId),
        client.agentCenter.getDraftDiff(selectedId),
      ]);
      setDetail(draft);
      setDiff(files);
    } catch (error) {
      setDetailError(errorMessage(error, t("loadFailed")));
    } finally {
      setDetailLoading(false);
    }
  }, [client.agentCenter, selectedId, t]);

  useEffect(() => {
    queueMicrotask(() => void load());
  }, [load]);
  useEffect(() => {
    queueMicrotask(() => void loadDetail());
  }, [loadDetail]);

  const review = async (decision: "reject" | "promote") => {
    if (!detail) return;
    setAction(true);
    try {
      await client.agentCenter.reviewDraft({
        draftId: detail.id,
        decision,
        expectedRevision: detail.revision,
        draftFingerprint: detail.draftFingerprint,
        proposedPackageFingerprint: detail.proposedPackageFingerprint,
        reasonCode:
          decision === "promote" ? "ADMIN_PROMOTED" : "ADMIN_REJECTED",
      });
      announce(t("draftReviewed", { decision }));
      await Promise.all([load(), loadDetail()]);
    } catch (error) {
      announce(errorMessage(error, t("actionFailed")));
      if (isStale(error)) await Promise.all([load(), loadDetail()]);
    } finally {
      setAction(false);
    }
  };

  return (
    <SplitShell
      selected={Boolean(selectedId)}
      list={
        <>
          <PanelHeader title={t("learning")} onReload={() => void load()} />
          <div className="p-4">
            {loading ? (
              <Loading />
            ) : listError ? (
              <InlineError message={listError} retry={() => void load()} />
            ) : items.length ? (
              <div className="space-y-2">
                {items.map((item) => (
                  <button
                    key={item.id}
                    type="button"
                    onClick={(event) => {
                      restoreFocus.current = event.currentTarget;
                      onSelect(item.id);
                    }}
                    className="w-full rounded-xl border bg-card p-3 text-left hover:border-slate-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="font-medium">
                        {item.name} · {item.version}
                      </span>
                      <StatusPill value={item.state} />
                    </div>
                    <p className="mt-2 font-mono text-xs text-muted-foreground">
                      {item.id}
                    </p>
                  </button>
                ))}
              </div>
            ) : (
              <EmptyState title={t("noDrafts")} />
            )}
          </div>
        </>
      }
      detail={
        detailLoading && selectedId ? (
          <DetailFrame
            title={selectedId}
            onBack={() => closeDetail(onSelect, restoreFocus)}
          >
            <Loading />
          </DetailFrame>
        ) : detailError && selectedId ? (
          <DetailFrame
            title={selectedId}
            onBack={() => closeDetail(onSelect, restoreFocus)}
          >
            <InlineError
              message={detailError}
              retry={() => void loadDetail()}
            />
          </DetailFrame>
        ) : detail ? (
          <DetailFrame
            title={`${detail.name} · ${detail.version}`}
            onBack={() => closeDetail(onSelect, restoreFocus)}
          >
            <div className="flex flex-wrap items-center gap-2">
              <StatusPill value={detail.state} />
              {detail.state === "reviewable" && (
                <>
                  <Action
                    onClick={() => void review("promote")}
                    disabled={action}
                    icon={<Check size={15} />}
                  >
                    {t("promote")}
                  </Action>
                  <Action
                    danger
                    onClick={() => void review("reject")}
                    disabled={action}
                    icon={<XCircle size={15} />}
                  >
                    {t("reject")}
                  </Action>
                </>
              )}
            </div>
            <DetailGrid
              rows={[
                [t("revision"), String(detail.revision)],
                [t("checkGeneration"), String(detail.checkGeneration)],
                [t("checkAttempts"), String(detail.checkAttempts)],
              ]}
            />
            <Fingerprint
              label={t("draftFingerprint")}
              value={detail.draftFingerprint}
              full
            />
            <Fingerprint
              label={t("proposedFingerprint")}
              value={detail.proposedPackageFingerprint}
              full
            />
            <section>
              <h3 className="mb-2 text-sm font-semibold">
                {t("checkReceipts")}
              </h3>
              {detail.checks.map((check) => (
                <div
                  key={check.id}
                  className="mb-2 rounded-xl border bg-card p-3 text-sm"
                >
                  <div className="flex justify-between gap-2">
                    <span>{check.kind}</span>
                    <StatusPill value={check.status} />
                  </div>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {check.reasonCode} · {check.durationMillis} ms
                  </p>
                </div>
              ))}
            </section>
            <section>
              <h3 className="mb-2 text-sm font-semibold">{t("boundedDiff")}</h3>
              {diff.length ? (
                diff.map((file) => (
                  <details
                    key={file.path}
                    className="mb-2 rounded-xl border bg-card p-3"
                  >
                    <summary className="cursor-pointer text-sm font-medium">
                      {file.change} · {file.path}
                    </summary>
                    {file.binary ? (
                      <p className="mt-2 text-xs text-muted-foreground">
                        {t("binaryDiff")}
                      </p>
                    ) : (
                      <div className="mt-3 grid gap-2 lg:grid-cols-2">
                        <pre className="max-h-64 overflow-auto rounded-lg bg-red-950/90 p-3 text-xs text-red-100">
                          {file.beforeText}
                        </pre>
                        <pre className="max-h-64 overflow-auto rounded-lg bg-emerald-950/90 p-3 text-xs text-emerald-100">
                          {file.afterText}
                        </pre>
                      </div>
                    )}
                  </details>
                ))
              ) : (
                <p className="text-sm text-muted-foreground">{t("none")}</p>
              )}
            </section>
          </DetailFrame>
        ) : (
          <EmptyDetail title={t("selectDraft")} />
        )
      }
    />
  );
}

function SplitShell({
  selected,
  list,
  detail,
}: {
  selected: boolean;
  list: ReactNode;
  detail: ReactNode;
}) {
  return (
    <div className="grid h-full min-h-0 md:grid-cols-[minmax(17rem,22rem)_1fr]">
      <aside
        className={`${selected ? "hidden md:block" : "block"} min-h-0 overflow-y-auto border-r border-border/70`}
      >
        {list}
      </aside>
      <section
        className={`${selected ? "block" : "hidden md:block"} min-h-0 overflow-y-auto`}
      >
        {detail}
      </section>
    </div>
  );
}

function PanelHeader({
  title,
  onReload,
}: {
  title: string;
  onReload: () => void;
}) {
  const t = useTranslations("AgentCenter");
  return (
    <div className="sticky top-0 z-10 flex items-center justify-between border-b bg-background/95 px-4 py-3 backdrop-blur">
      <h2 className="font-semibold">{title}</h2>
      <button
        type="button"
        onClick={onReload}
        aria-label={t("reloadAria", { title })}
        className="rounded-lg p-2 text-muted-foreground hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500"
      >
        <RefreshCw size={16} aria-hidden="true" />
      </button>
    </div>
  );
}

function DetailFrame({
  title,
  onBack,
  children,
}: {
  title: string;
  onBack: () => void;
  children: ReactNode;
}) {
  const t = useTranslations("AgentCenter");
  return (
    <div className="mx-auto max-w-5xl space-y-5 p-4 md:p-6">
      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={onBack}
          aria-label={t("backToList")}
          className="rounded-lg p-2 hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500 md:hidden"
        >
          <ArrowLeft size={18} aria-hidden="true" />
        </button>
        <h2 className="min-w-0 truncate text-lg font-bold">{title}</h2>
      </div>
      {children}
    </div>
  );
}

function Timeline({ detail }: { detail: AgentRunDetailDTO }) {
  const t = useTranslations("AgentCenter");
  return (
    <section>
      <h3 className="mb-2 text-sm font-semibold">{t("timeline")}</h3>
      <div className="space-y-2">
        {detail.steps.map((step) => {
          const attempts = detail.attempts.filter(
            (attempt) => attempt.stepId === step.id,
          );
          return (
            <div key={step.id} className="rounded-xl border bg-card p-3">
              <div className="flex justify-between gap-2 text-sm">
                <span>
                  {step.ordinal}. {step.kind}
                </span>
                <StatusPill value={step.state} />
              </div>
              {attempts.map((attempt) => (
                <p
                  key={attempt.id}
                  className="mt-2 text-xs text-muted-foreground"
                >
                  {t("attempt", {
                    generation: attempt.generation,
                    state: attempt.state,
                  })}
                </p>
              ))}
            </div>
          );
        })}
        {detail.events.length ? (
          <details className="rounded-xl border bg-card p-3">
            <summary className="cursor-pointer text-sm font-medium">
              {t("events", { count: detail.events.length })}
            </summary>
            <ol className="mt-3 space-y-2">
              {detail.events.map((event) => (
                <li key={event.id} className="text-xs">
                  <span className="font-mono">#{event.sequence}</span>{" "}
                  {event.entity}: {event.from ? `${event.from} → ` : ""}
                  {event.to} · {event.reasonCode}
                </li>
              ))}
            </ol>
          </details>
        ) : null}
      </div>
    </section>
  );
}

function DetailGrid({ rows }: { rows: Array<[string, string]> }) {
  return (
    <dl className="grid gap-3 rounded-xl border bg-card p-4 sm:grid-cols-2">
      {rows.map(([label, value]) => (
        <div key={label}>
          <dt className="text-xs text-muted-foreground">{label}</dt>
          <dd className="mt-1 break-words text-sm font-medium">{value}</dd>
        </div>
      ))}
    </dl>
  );
}

function Fingerprint({
  value,
  label,
  full = false,
}: {
  value: string;
  label?: string;
  full?: boolean;
}) {
  const shown =
    full || value.length <= 24
      ? value
      : `${value.slice(0, 18)}…${value.slice(-6)}`;
  return (
    <div className="mt-2 min-w-0">
      <span className="block text-[11px] text-muted-foreground">{label}</span>
      <code
        className="block break-all text-[11px] text-muted-foreground"
        title={value}
      >
        {shown}
      </code>
    </div>
  );
}

function StatusPill({ value }: { value: string }) {
  return (
    <span className="inline-flex rounded-full border border-slate-300 bg-slate-100 px-2 py-0.5 text-[11px] font-medium text-slate-700 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200">
      {value}
    </span>
  );
}

function Action({
  children,
  icon,
  onClick,
  disabled,
  danger = false,
}: {
  children: ReactNode;
  icon: ReactNode;
  onClick: () => void;
  disabled?: boolean;
  danger?: boolean;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className={`inline-flex items-center gap-1.5 rounded-lg border px-3 py-2 text-xs font-medium disabled:opacity-50 ${danger ? "border-red-300 text-red-700 hover:bg-red-50 dark:border-red-900 dark:text-red-300 dark:hover:bg-red-950/30" : "border-slate-300 hover:bg-slate-100 dark:border-slate-700 dark:hover:bg-slate-900"}`}
    >
      {icon}
      {children}
    </button>
  );
}

function Loading() {
  const t = useTranslations("AgentCenter");
  return (
    <div className="flex items-center gap-2 p-4 text-sm text-muted-foreground">
      <Loader2 size={16} className="animate-spin" aria-hidden="true" />
      {t("loading")}
    </div>
  );
}
function InlineError({
  message,
  retry,
}: {
  message: string;
  retry: () => void;
}) {
  const t = useTranslations("AgentCenter");
  return (
    <div
      role="alert"
      className="rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-800 dark:border-red-900 dark:bg-red-950/25 dark:text-red-200"
    >
      <p>{message}</p>
      <button
        type="button"
        onClick={retry}
        className="mt-2 font-medium underline"
      >
        {t("retry")}
      </button>
    </div>
  );
}
function EmptyState({ title }: { title: string }) {
  return (
    <div className="rounded-xl border border-dashed p-6 text-center text-sm text-muted-foreground">
      {title}
    </div>
  );
}
function EmptyDetail({ title }: { title: string }) {
  return (
    <div className="flex h-full items-center justify-center p-6">
      <EmptyState title={title} />
    </div>
  );
}

function closeDetail(
  onSelect: (id: string | null) => void,
  restoreFocus: React.RefObject<HTMLButtonElement | null>,
) {
  onSelect(null);
  requestAnimationFrame(() =>
    restoreFocus.current?.focus({ preventScroll: true }),
  );
}

function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error && error.message ? error.message : fallback;
}
function isStale(error: unknown) {
  return (
    error instanceof ApiClientError &&
    (error.code === "AGENT_STALE_CONFLICT" ||
      error.code === "SKILL_REVISION_CONFLICT")
  );
}
function isCancelable(state: string) {
  return ["pending", "admitted", "queued", "running"].includes(state);
}
function formatDate(value: string) {
  const time = Date.parse(value);
  return Number.isFinite(time)
    ? new Intl.DateTimeFormat(undefined, {
        dateStyle: "medium",
        timeStyle: "short",
      }).format(time)
    : value;
}
