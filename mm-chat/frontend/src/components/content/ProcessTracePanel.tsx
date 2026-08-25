"use client";

import { useId, useMemo, useState } from "react";
import type { ReactNode } from "react";
import {
  BookOpenText,
  Brain,
  Check,
  ChevronDown,
  CircleAlert,
  Globe2,
  FileText,
  Search,
  BriefcaseBusiness,
  Target,
  Compass,
  PackageSearch,
  Copy,
  LoaderCircle,
  RotateCcw,
  SquareTerminal,
  Wrench,
  Zap,
} from "lucide-react";
import { useTranslations } from "next-intl";

import {
  isProcessStepActive,
  processToolLabelForDisplay,
  processReasonCategoryForDisplay,
  projectProcessStepsForDisplay,
  resolveProcessPanelExpanded,
  summarizeProcessRoute,
} from "@/lib/chat/processTrace";
import type {
  ProcessApprovalPresentation,
  ProcessResourceConfigurationPresentation,
  ProcessStep,
  ProcessStepKind,
  ProcessStepPresentation,
  ProcessTranscriptEntry,
} from "@/types";
import { createNeoChatApiClient } from "@/services/api/client";
import type { ChatApprovalDecision } from "@/services/api/client";
import { useChatStore } from "@/store/core/chatStore";
import { RESOURCE_MANAGER_OPEN_EVENT } from "@/lib/chat/slashCommands";
import MarkdownRenderer from "./MarkdownRenderer";

interface ProcessTracePanelProps {
  steps: ProcessStep[];
  reasoning?: string;
}

const kindIcons = {
  reasoning: Brain,
  knowledge: BookOpenText,
  web: Globe2,
  tool: Wrench,
  generation: Zap,
} satisfies Record<ProcessStepKind, typeof Brain>;

const presentationIcons = {
  terminal: SquareTerminal,
  search: Search,
  file: FileText,
  job: BriefcaseBusiness,
  goal: Target,
  resource: PackageSearch,
  browser: Compass,
} satisfies Partial<Record<ProcessStepPresentation["card"], typeof Brain>>;

export default function ProcessTracePanel({
  steps,
  reasoning = "",
}: ProcessTracePanelProps) {
  const t = useTranslations("Content");
  const panelId = useId();
  const visibleSteps = useMemo(
    () => projectProcessStepsForDisplay(steps),
    [steps],
  );
  const hasActiveStep = visibleSteps.some(isProcessStepActive);
  const hasProcessLocalJob = visibleSteps.some(
    (step) => step.detail?.durability === "process_local",
  );
  const [manualExpanded, setManualExpanded] = useState<boolean | null>(null);
  const isExpanded = resolveProcessPanelExpanded(hasActiveStep, manualExpanded);
  const summary = useMemo(
    () => buildProcessSummary(visibleSteps, hasActiveStep, t),
    [hasActiveStep, t, visibleSteps],
  );
  const roundGroups = useMemo(
    () => groupProcessStepsByRound(visibleSteps),
    [visibleSteps],
  );

  if (visibleSteps.length === 0) return null;

  return (
    <div className="mb-3 overflow-hidden rounded-lg border border-gray-200 bg-gray-50/60 dark:border-border dark:bg-muted/25">
      <button
        type="button"
        aria-expanded={isExpanded}
        aria-controls={panelId}
        aria-busy={hasActiveStep || undefined}
        onClick={() => setManualExpanded(!isExpanded)}
        className="flex w-full cursor-pointer select-none items-center gap-2 px-3 py-2 text-xs font-medium text-gray-600 transition-colors hover:bg-gray-100/70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/50 dark:text-muted-foreground dark:hover:bg-accent/30"
      >
        <span className="rounded bg-violet-100 p-1 text-violet-600 dark:bg-violet-900/30 dark:text-violet-400">
          {hasActiveStep ? (
            <LoaderCircle
              size={12}
              className="motion-safe:animate-spin"
              aria-hidden="true"
            />
          ) : (
            <Check size={12} aria-hidden="true" />
          )}
        </span>
        <span className="min-w-0 flex-1 truncate text-left">{summary}</span>
        <ChevronDown
          size={14}
          aria-hidden="true"
          className={`shrink-0 transition-transform duration-200 ${isExpanded ? "rotate-180" : ""}`}
        />
      </button>

      <div
        id={panelId}
        role="region"
        aria-label={t("processDetails")}
        className={`grid transition-[grid-template-rows,opacity] duration-200 ease-out ${isExpanded ? "grid-rows-[1fr] opacity-100" : "grid-rows-[0fr] opacity-0"}`}
      >
        <div className="overflow-hidden">
          <div className="max-h-80 overflow-y-auto border-t border-gray-200/60 bg-white/40 px-3 py-2 custom-scrollbar dark:border-border dark:bg-card/35">
            <div className="space-y-3" aria-label={t("processSteps")}>
              {roundGroups.map((group) => (
                <section key={group.key} className="relative">
                  {group.round !== undefined ? (
                    <div className="mb-2 flex items-center gap-2 text-[10px] font-semibold uppercase tracking-[0.12em] text-gray-400 dark:text-muted-foreground/70">
                      <span>{t("processRound", { round: group.round })}</span>
                      <span className="h-px flex-1 bg-gray-200 dark:bg-border" />
                    </div>
                  ) : null}
                  <ol className="space-y-2 border-l border-gray-200 pl-2.5 dark:border-border">
                    {group.steps.map((step) => (
                      <ProcessStepRow key={step.id} step={step} />
                    ))}
                  </ol>
                </section>
              ))}
            </div>

            {hasProcessLocalJob ? (
              <div
                role="note"
                className="mt-3 flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 px-2.5 py-2 text-[11px] text-amber-800 dark:border-amber-900/70 dark:bg-amber-950/30 dark:text-amber-200"
              >
                <CircleAlert
                  size={13}
                  className="mt-0.5 shrink-0"
                  aria-hidden="true"
                />
                <span>{t("processLocalJobRestartWarning")}</span>
              </div>
            ) : null}

            {reasoning ? (
              <div className="mt-3 border-t border-gray-200/60 pt-3 dark:border-border">
                <div className="mb-2 flex items-center gap-1.5 text-xs font-semibold text-gray-600 dark:text-foreground/80">
                  <Brain size={13} aria-hidden="true" />
                  <span>{t("providerReasoning")}</span>
                </div>
                <MarkdownRenderer
                  content={reasoning}
                  className="text-xs! text-gray-600 md:text-sm! dark:text-foreground/85"
                />
              </div>
            ) : null}
          </div>
        </div>
      </div>
    </div>
  );
}

export function ProcessStepRow({ step }: { step: ProcessStep }) {
  const t = useTranslations("Content");
  const Icon = kindIcons[step.kind];
  const active = isProcessStepActive(step);
  const outcomeUnknown = step.status === "outcome_unknown";
  const failed =
    step.status === "failed" ||
    step.status === "cancelled" ||
    step.status === "interrupted" ||
    outcomeUnknown;
  const reason = processReasonCategoryForDisplay(step);
  const hitCount = numberDetail(step, "hitCount");
  const sourceCount = numberDetail(step, "sourceCount");
  const toolLabel = processToolLabelForDisplay(step);
  const presentation = step.presentation;
  const StepIcon = presentation
    ? (presentationIcons[presentation.card as keyof typeof presentationIcons] ??
      Icon)
    : Icon;

  return (
    <li className="flex min-w-0 items-start gap-2 text-xs text-gray-600 dark:text-muted-foreground">
      <span
        className={`mt-0.5 rounded p-1 ${failed ? "bg-red-100 text-red-600 dark:bg-red-950/40 dark:text-red-300" : active ? "bg-blue-100 text-blue-600 dark:bg-blue-950/40 dark:text-blue-300" : "bg-gray-100 text-gray-500 dark:bg-muted dark:text-muted-foreground"}`}
      >
        {failed ? (
          <CircleAlert size={12} aria-hidden="true" />
        ) : active ? (
          <LoaderCircle
            size={12}
            className="motion-safe:animate-spin"
            aria-hidden="true"
          />
        ) : (
          <StepIcon size={12} aria-hidden="true" />
        )}
      </span>
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-center gap-2">
          <span className="min-w-0 flex-1 truncate font-medium text-gray-700 dark:text-foreground/85">
            {toolLabel || processKindLabel(step.kind, t)}
          </span>
          <span className="shrink-0 whitespace-nowrap text-[11px] text-gray-400 dark:text-muted-foreground/70">
            {processStatusLabel(step.status, t)}
            {typeof step.durationMs === "number"
              ? ` · ${formatDuration(step.durationMs)}`
              : ""}
          </span>
        </div>
        {hitCount !== undefined || sourceCount !== undefined ? (
          <div className="mt-0.5 text-[11px] text-gray-400 dark:text-muted-foreground/70">
            {hitCount !== undefined
              ? t("processKnowledgeHits", { count: hitCount })
              : t("processWebSources", { count: sourceCount ?? 0 })}
          </div>
        ) : null}
        {reason ? (
          <div className="mt-0.5 text-[11px] text-gray-400 dark:text-muted-foreground/70">
            {processReasonLabel(reason, t)}
          </div>
        ) : null}
        {presentation ? (
          <ProcessPresentationCard presentation={presentation} />
        ) : null}
      </div>
    </li>
  );
}

function ProcessPresentationCard({
  presentation,
}: {
  presentation: ProcessStepPresentation;
}) {
  if (presentation.card === "terminal") {
    return <TerminalProcessCard terminal={presentation} />;
  }
  return <GenericProcessCard presentation={presentation} />;
}

function TerminalProcessCard({
  terminal,
}: {
  terminal: Extract<ProcessStepPresentation, { card: "terminal" }>;
}) {
  const t = useTranslations("Content");
  const preview = terminal.command.replace(/\s+/g, " ").trim();
  const exitFailed =
    typeof terminal.exitCode === "number" && terminal.exitCode !== 0;

  return (
    <details
      open={terminal.approval?.status === "pending" || undefined}
      className="group/terminal mt-1.5 overflow-hidden rounded-md border border-slate-800/70 bg-slate-950 text-slate-200 dark:border-slate-700"
    >
      <summary className="flex cursor-pointer list-none items-center gap-2 px-2.5 py-2 marker:content-none [&::-webkit-details-marker]:hidden">
        <span
          className="select-none font-mono text-[11px] text-emerald-400"
          aria-hidden="true"
        >
          $
        </span>
        <code
          className="min-w-0 flex-1 truncate font-mono text-[11px] text-slate-200"
          title={preview}
        >
          {preview}
        </code>
        <ChevronDown
          size={13}
          aria-hidden="true"
          className="shrink-0 text-slate-500 transition-transform group-open/terminal:rotate-180"
        />
      </summary>
      <div className="border-t border-slate-800 px-2.5 py-2">
        <div className="mb-2 flex justify-end">
          <PresentationCopyButton
            text={terminalPresentationText(terminal)}
            dark
          />
        </div>
        {terminal.cwd ? (
          <div className="mb-2 flex min-w-0 items-center gap-2 text-[10px] text-slate-400">
            <span className="shrink-0 uppercase tracking-wide">
              {t("processTerminalCwd")}
            </span>
            <code className="min-w-0 truncate font-mono text-slate-300">
              {terminal.cwd}
            </code>
          </div>
        ) : null}
        <pre className="max-h-48 overflow-auto whitespace-pre-wrap break-words rounded bg-black/35 p-2 font-mono text-[11px] leading-5 text-slate-100 custom-scrollbar">
          <code>{terminal.command}</code>
        </pre>
        {terminal.transcript?.length ? (
          <Transcript entries={terminal.transcript} />
        ) : null}
        <div className="mt-2 flex flex-wrap gap-1.5">
          {typeof terminal.exitCode === "number" ? (
            <TerminalPill tone={exitFailed ? "danger" : "success"}>
              {t("processTerminalExitCode", { code: terminal.exitCode })}
            </TerminalPill>
          ) : null}
          {terminal.timedOut ? (
            <TerminalPill tone="warning">
              {t("processTerminalTimedOut")}
            </TerminalPill>
          ) : null}
          {terminal.truncated ? (
            <TerminalPill tone="warning">
              {t("processTerminalTruncated")}
            </TerminalPill>
          ) : null}
          {terminal.background ? (
            <TerminalPill tone="info">
              {t("processTerminalBackground")}
            </TerminalPill>
          ) : null}
        </div>
        {terminal.approval ? (
          <ApprovalControls approval={terminal.approval} dark />
        ) : null}
      </div>
    </details>
  );
}

function ApprovalControls({
  approval,
  dark = false,
  configuration = false,
}: {
  approval: ProcessApprovalPresentation;
  dark?: boolean;
  configuration?: boolean;
}) {
  const t = useTranslations("Content");
  const client = useMemo(() => createNeoChatApiClient(), []);
  const [resolved, setResolved] = useState(approval);
  const [busy, setBusy] = useState<ChatApprovalDecision | null>(null);
  const [error, setError] = useState(false);
  const current = resolved.revision >= approval.revision ? resolved : approval;
  const decide = async (decision: ChatApprovalDecision) => {
    if (busy || current.status !== "pending") return;
    setBusy(decision);
    setError(false);
    try {
      const response = await client.chat.decideApproval({
        approvalId: current.id,
        expectedRevision: current.revision,
        decision,
      });
      setResolved({
        id: response.id,
        revision: response.revision,
        status: response.status,
        ...(response.decision ? { decision: response.decision } : {}),
        expiresAt: response.expiresAt,
        allowConversation: response.allowConversation,
      });
    } catch {
      setError(true);
    } finally {
      setBusy(null);
    }
  };
  const muted = dark
    ? "text-slate-400"
    : "text-gray-500 dark:text-muted-foreground";
  if (current.status !== "pending") {
    return (
      <div className={`mt-2 text-[10px] ${muted}`} role="status">
        {current.status === "allowed"
          ? t("processApprovalAllowed")
          : current.status === "expired"
            ? t("processApprovalExpired")
            : t("processApprovalDenied")}
      </div>
    );
  }
  return (
    <div
      className={`mt-2 border-t pt-2 ${dark ? "border-slate-800" : "border-gray-200 dark:border-border"}`}
      aria-label={
        configuration
          ? t("processConfigurationRequired")
          : t("processApprovalRequired")
      }
    >
      <div className={`mb-2 text-[10px] ${muted}`}>
        {configuration
          ? t("processConfigurationRequired")
          : t("processApprovalRequired")}
      </div>
      <div className="flex flex-wrap gap-1.5">
        <ApprovalButton
          label={
            configuration
              ? t("processConfigurationContinue")
              : t("processApprovalAllowOnce")
          }
          busy={busy === "allow_once"}
          onClick={() => void decide("allow_once")}
        />
        {current.allowConversation && !configuration ? (
          <ApprovalButton
            label={t("processApprovalAllowConversation")}
            busy={busy === "allow_conversation"}
            onClick={() => void decide("allow_conversation")}
          />
        ) : null}
        <ApprovalButton
          label={
            configuration
              ? t("processConfigurationCancel")
              : t("processApprovalDeny")
          }
          busy={busy === "deny"}
          danger
          onClick={() => void decide("deny")}
        />
      </div>
      {error ? (
        <div className="mt-1.5 text-[10px] text-rose-400" role="alert">
          {t("processApprovalFailed")}
        </div>
      ) : null}
    </div>
  );
}

function ResourceConfigurationControls({
  configuration,
}: {
  configuration: ProcessResourceConfigurationPresentation;
}) {
  const t = useTranslations("Content");
  const openManager = () => {
    window.dispatchEvent(
      new CustomEvent(RESOURCE_MANAGER_OPEN_EVENT, {
        detail: {
          kind: configuration.kind,
          query: configuration.query,
          resourceId: configuration.resourceId,
        },
      }),
    );
  };
  return (
    <button
      type="button"
      onClick={openManager}
      className="inline-flex items-center gap-1.5 rounded-md border border-blue-200 bg-blue-50 px-2 py-1.5 text-[10px] font-medium text-blue-700 transition-colors hover:bg-blue-100 dark:border-blue-900/70 dark:bg-blue-950/35 dark:text-blue-300 dark:hover:bg-blue-950/55"
    >
      <Wrench size={11} aria-hidden="true" />
      {t("processConfigurationOpen")}
    </button>
  );
}

function ApprovalButton({
  label,
  busy,
  danger = false,
  onClick,
}: {
  label: string;
  busy: boolean;
  danger?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      disabled={busy}
      onClick={onClick}
      className={`inline-flex items-center gap-1 rounded px-2 py-1 text-[10px] font-medium transition-colors disabled:cursor-wait disabled:opacity-60 ${danger ? "bg-rose-500/15 text-rose-300 hover:bg-rose-500/25" : "bg-emerald-500/15 text-emerald-300 hover:bg-emerald-500/25"}`}
    >
      {busy ? (
        <LoaderCircle size={10} className="motion-safe:animate-spin" />
      ) : null}
      {label}
    </button>
  );
}

function GenericProcessCard({
  presentation,
}: {
  presentation: Exclude<ProcessStepPresentation, { card: "terminal" }>;
}) {
  const heading =
    ("title" in presentation ? presentation.title : undefined) ||
    ("path" in presentation ? presentation.path : undefined) ||
    ("jobId" in presentation ? presentation.jobId : undefined) ||
    ("operation" in presentation ? presentation.operation : undefined) ||
    presentation.card;
  const transcript =
    "transcript" in presentation ? presentation.transcript : undefined;
  const body =
    "diff" in presentation && presentation.diff
      ? presentation.diff
      : "content" in presentation
        ? presentation.content
        : undefined;
  const bodyLabel =
    "diff" in presentation && presentation.diff ? "diff" : "output";
  const pendingApproval =
    "approval" in presentation && presentation.approval?.status === "pending";

  return (
    <details
      open={pendingApproval || undefined}
      className="group/card mt-1.5 overflow-hidden rounded-md border border-gray-200 bg-white/75 dark:border-border dark:bg-card/70"
    >
      <summary className="flex cursor-pointer list-none items-center gap-2 px-2.5 py-2 marker:content-none [&::-webkit-details-marker]:hidden">
        <span className="min-w-0 flex-1 truncate text-[11px] font-medium text-gray-700 dark:text-foreground/85">
          {heading}
        </span>
        {"provider" in presentation && presentation.provider ? (
          <span className="rounded bg-blue-50 px-1.5 py-0.5 text-[9px] font-semibold text-blue-700 dark:bg-blue-950/40 dark:text-blue-300">
            {presentation.provider}
          </span>
        ) : null}
        <ChevronDown
          size={13}
          aria-hidden="true"
          className="shrink-0 text-gray-400 transition-transform group-open/card:rotate-180"
        />
      </summary>
      <div className="space-y-2 border-t border-gray-200 px-2.5 py-2 text-[11px] dark:border-border">
        <div className="flex justify-end">
          <PresentationCopyButton
            text={genericPresentationText(presentation)}
          />
        </div>
        {"operation" in presentation && presentation.operation ? (
          <MetaLine label="action" value={presentation.operation} />
        ) : null}
        {"jobStatus" in presentation && presentation.jobStatus ? (
          <MetaLine label="status" value={presentation.jobStatus} />
        ) : null}
        {"command" in presentation && presentation.command ? (
          <MetaLine label="command" value={presentation.command} mono />
        ) : null}
        {"cwd" in presentation && presentation.cwd ? (
          <MetaLine label="cwd" value={presentation.cwd} mono />
        ) : null}
        {"jobStartedAt" in presentation && presentation.jobStartedAt ? (
          <MetaLine label="started" value={presentation.jobStartedAt} />
        ) : null}
        {"jobCompletedAt" in presentation && presentation.jobCompletedAt ? (
          <MetaLine label="completed" value={presentation.jobCompletedAt} />
        ) : null}
        {"exitCode" in presentation &&
        typeof presentation.exitCode === "number" ? (
          <MetaLine label="exit" value={String(presentation.exitCode)} />
        ) : null}
        {"path" in presentation && presentation.path ? (
          <MetaLine label="path" value={presentation.path} mono />
        ) : null}
        {"query" in presentation && presentation.query ? (
          <MetaLine label="query" value={presentation.query} />
        ) : null}
        {"summary" in presentation && presentation.summary ? (
          <p className="text-gray-500 dark:text-muted-foreground">
            {presentation.summary}
          </p>
        ) : null}
        {"count" in presentation && typeof presentation.count === "number" ? (
          <MetaLine label="results" value={String(presentation.count)} />
        ) : null}
        {body ? (
          <div>
            <div className="mb-1 text-[9px] font-semibold uppercase tracking-wide text-gray-400">
              {bodyLabel}
            </div>
            <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-words rounded bg-gray-950 p-2 font-mono text-[10px] leading-5 text-gray-100 custom-scrollbar">
              <code>{body}</code>
            </pre>
          </div>
        ) : null}
        {"items" in presentation && presentation.items?.length ? (
          <ul className="max-h-48 space-y-1 overflow-auto custom-scrollbar">
            {presentation.items.map((item, index) => (
              <li
                key={`${item.label}-${index}`}
                className="rounded bg-gray-50 px-2 py-1 dark:bg-muted/45"
              >
                <div className="font-mono text-[10px] text-gray-700 dark:text-foreground/80">
                  {item.label}
                </div>
                {item.detail ? (
                  <div className="mt-0.5 text-gray-500 dark:text-muted-foreground">
                    {item.detail}
                  </div>
                ) : null}
              </li>
            ))}
          </ul>
        ) : null}
        {transcript?.length ? <Transcript entries={transcript} /> : null}
        {"retry" in presentation && presentation.retry ? (
          <RetryControls eventId={presentation.retry.eventId} />
        ) : null}
        {"configuration" in presentation && presentation.configuration ? (
          <ResourceConfigurationControls
            configuration={presentation.configuration}
          />
        ) : null}
        {"approval" in presentation && presentation.approval ? (
          <ApprovalControls
            approval={presentation.approval}
            configuration={Boolean(
              "configuration" in presentation && presentation.configuration,
            )}
          />
        ) : null}
      </div>
    </details>
  );
}

function RetryControls({ eventId }: { eventId: string }) {
  const t = useTranslations("Content");
  const retryAgentTool = useChatStore((state) => state.retryAgentTool);
  const [busy, setBusy] = useState(false);
  const [outcome, setOutcome] = useState<"idle" | "succeeded" | "failed">(
    "idle",
  );
  const retry = async () => {
    if (busy || outcome === "succeeded") return;
    setBusy(true);
    setOutcome("idle");
    const succeeded = await retryAgentTool(eventId);
    setOutcome(succeeded ? "succeeded" : "failed");
    setBusy(false);
  };
  return (
    <div className="border-t border-gray-200 pt-2 dark:border-border">
      <button
        type="button"
        disabled={busy || outcome === "succeeded"}
        onClick={() => void retry()}
        className="inline-flex items-center gap-1 rounded bg-blue-50 px-2 py-1 text-[10px] font-medium text-blue-700 transition-colors hover:bg-blue-100 disabled:cursor-wait disabled:opacity-60 dark:bg-blue-950/40 dark:text-blue-300 dark:hover:bg-blue-950/60"
      >
        {busy ? (
          <LoaderCircle size={10} className="motion-safe:animate-spin" />
        ) : (
          <RotateCcw size={10} aria-hidden="true" />
        )}
        {outcome === "succeeded" ? t("processRetryCreated") : t("processRetry")}
      </button>
      {outcome === "failed" ? (
        <div className="mt-1.5 text-[10px] text-rose-500" role="alert">
          {t("processRetryFailed")}
        </div>
      ) : null}
    </div>
  );
}

function PresentationCopyButton({
  text,
  dark = false,
}: {
  text: string;
  dark?: boolean;
}) {
  const t = useTranslations("Content");
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      setCopied(false);
    }
  };
  return (
    <button
      type="button"
      onClick={copy}
      className={`inline-flex items-center gap-1 rounded px-1.5 py-1 text-[10px] transition-colors ${dark ? "text-slate-400 hover:bg-white/10 hover:text-slate-200" : "text-gray-400 hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-muted dark:hover:text-foreground"}`}
      aria-label={t("processCopy")}
    >
      {copied ? (
        <Check size={11} aria-hidden="true" />
      ) : (
        <Copy size={11} aria-hidden="true" />
      )}
      <span>{copied ? t("copied") : t("processCopy")}</span>
    </button>
  );
}

function terminalPresentationText(
  terminal: Extract<ProcessStepPresentation, { card: "terminal" }>,
) {
  return [
    terminal.cwd ? `cwd: ${terminal.cwd}` : "",
    `$ ${terminal.command}`,
    ...(terminal.transcript ?? []).map(
      (entry) => `[${entry.stream}] ${entry.content}`,
    ),
    typeof terminal.exitCode === "number" ? `exit: ${terminal.exitCode}` : "",
  ]
    .filter(Boolean)
    .join("\n");
}

function genericPresentationText(
  presentation: Exclude<ProcessStepPresentation, { card: "terminal" }>,
) {
  const values: string[] = [presentation.card];
  for (const key of [
    "title",
    "operation",
    "path",
    "query",
    "summary",
    "content",
    "diff",
    "jobId",
    "jobStatus",
    "jobStartedAt",
    "jobCompletedAt",
    "command",
    "cwd",
  ] as const) {
    if (key in presentation) {
      const value = presentation[key as keyof typeof presentation];
      if (typeof value === "string" && value) values.push(`${key}: ${value}`);
    }
  }
  if ("items" in presentation) {
    for (const item of presentation.items ?? []) {
      values.push(item.detail ? `${item.label}: ${item.detail}` : item.label);
    }
  }
  if ("transcript" in presentation) {
    for (const entry of presentation.transcript ?? []) {
      values.push(`[${entry.stream}] ${entry.content}`);
    }
  }
  return values.join("\n");
}

function Transcript({ entries }: { entries: ProcessTranscriptEntry[] }) {
  return (
    <div className="max-h-64 overflow-auto rounded bg-slate-950 p-2 font-mono text-[10px] leading-5 custom-scrollbar">
      {entries.map((entry) => (
        <div
          key={`${entry.sequence}-${entry.stream}`}
          className={
            entry.stream === "stderr" ? "text-rose-300" : "text-slate-100"
          }
        >
          <span className="mr-2 select-none text-[9px] uppercase text-slate-500">
            {entry.stream}
          </span>
          <span className="whitespace-pre-wrap break-words">
            {entry.content}
          </span>
        </div>
      ))}
    </div>
  );
}

function MetaLine({
  label,
  value,
  mono = false,
}: {
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <div className="flex min-w-0 gap-2">
      <span className="shrink-0 uppercase tracking-wide text-gray-400">
        {label}
      </span>
      <span
        className={`min-w-0 break-words text-gray-600 dark:text-foreground/80 ${mono ? "font-mono" : ""}`}
      >
        {value}
      </span>
    </div>
  );
}

function groupProcessStepsByRound(steps: ProcessStep[]) {
  const groups: { key: string; round?: number; steps: ProcessStep[] }[] = [];
  for (const step of steps) {
    const candidate = step.detail?.round;
    const round =
      typeof candidate === "number" &&
      Number.isInteger(candidate) &&
      candidate > 0
        ? candidate
        : undefined;
    const key = round === undefined ? "unscoped" : `round-${round}`;
    let group = groups.find((item) => item.key === key);
    if (!group) {
      group = { key, ...(round !== undefined ? { round } : {}), steps: [] };
      groups.push(group);
    }
    group.steps.push(step);
  }
  return groups;
}

function TerminalPill({
  children,
  tone,
}: {
  children: ReactNode;
  tone: "success" | "danger" | "warning" | "info";
}) {
  const tones = {
    success: "bg-emerald-400/15 text-emerald-300",
    danger: "bg-red-400/15 text-red-300",
    warning: "bg-amber-400/15 text-amber-200",
    info: "bg-violet-400/15 text-violet-200",
  } as const;
  return (
    <span
      className={`rounded px-1.5 py-0.5 text-[10px] font-medium ${tones[tone]}`}
    >
      {children}
    </span>
  );
}

function buildProcessSummary(
  steps: ProcessStep[],
  active: boolean,
  t: ReturnType<typeof useTranslations<"Content">>,
): string {
  if (active) {
    const current = [...steps].reverse().find(isProcessStepActive);
    return current
      ? t("processRunning", {
          stage:
            processToolLabelForDisplay(current) ||
            processKindLabel(current.kind, t),
        })
      : t("processRunningGeneric");
  }
  const route = summarizeProcessRoute(steps);
  switch (route.route) {
    case "direct":
      return route.toolCalls > 0
        ? t("processRouteTools", { count: route.toolCalls })
        : t("processRouteDirect");
    case "knowledge":
      return t("processRouteKnowledge", { count: route.knowledgeSources });
    case "web":
      return t("processRouteWeb", { count: route.webSources });
    case "both":
      return t("processRouteBoth", {
        knowledgeCount: route.knowledgeSources,
        webCount: route.webSources,
      });
  }
}

function processReasonLabel(
  reason: NonNullable<ReturnType<typeof processReasonCategoryForDisplay>>,
  t: ReturnType<typeof useTranslations<"Content">>,
): string {
  switch (reason) {
    case "knowledge_miss":
      return t("processReasonKnowledgeMiss");
    case "web_miss":
      return t("processReasonWebMiss");
    case "planner_failed":
      return t("processReasonPlannerFailed");
    case "provider_degraded":
      return t("processReasonProviderDegraded");
    case "memory_indexing":
      return t("processReasonMemoryIndexing");
    case "memory_unavailable":
      return t("processReasonMemoryUnavailable");
    case "stream_gap":
      return t("processReasonStreamGap");
  }
}

function processKindLabel(
  kind: ProcessStepKind,
  t: ReturnType<typeof useTranslations<"Content">>,
): string {
  switch (kind) {
    case "reasoning":
      return t("processReasoning");
    case "knowledge":
      return t("processKnowledge");
    case "web":
      return t("processWeb");
    case "tool":
      return t("processTool");
    case "generation":
      return t("processGeneration");
  }
}

function processStatusLabel(
  status: ProcessStep["status"],
  t: ReturnType<typeof useTranslations<"Content">>,
): string {
  switch (status) {
    case "pending":
      return t("statusPending");
    case "running":
      return t("statusRunning");
    case "awaiting_approval":
      return t("processAwaitingApproval");
    case "completed":
      return t("statusSuccess");
    case "failed":
      return t("statusError");
    case "skipped":
      return t("statusSkipped");
    case "cancelled":
      return t("processCancelled");
    case "outcome_unknown":
      return t("processOutcomeUnknown");
    case "interrupted":
      return t("processInterrupted");
  }
}

function formatDuration(durationMs: number): string {
  if (durationMs < 1000) return `${Math.round(durationMs)}ms`;
  return `${(durationMs / 1000).toFixed(durationMs < 10_000 ? 1 : 0)}s`;
}

function numberDetail(step: ProcessStep, key: string): number | undefined {
  const value = step.detail?.[key];
  return typeof value === "number" && Number.isFinite(value)
    ? value
    : undefined;
}
