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
  LoaderCircle,
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
import type { ProcessStep, ProcessStepKind } from "@/types";
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
            <ol className="space-y-2" aria-label={t("processSteps")}>
              {visibleSteps.map((step) => (
                <ProcessStepRow key={step.id} step={step} />
              ))}
            </ol>

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

function ProcessStepRow({ step }: { step: ProcessStep }) {
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
  const terminal = step.presentation;
  const StepIcon = terminal ? SquareTerminal : Icon;

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
        {terminal ? <TerminalProcessCard terminal={terminal} /> : null}
      </div>
    </li>
  );
}

function TerminalProcessCard({
  terminal,
}: {
  terminal: NonNullable<ProcessStep["presentation"]>;
}) {
  const t = useTranslations("Content");
  const preview = terminal.command.replace(/\s+/g, " ").trim();
  const exitFailed =
    typeof terminal.exitCode === "number" && terminal.exitCode !== 0;

  return (
    <details className="group/terminal mt-1.5 overflow-hidden rounded-md border border-slate-800/70 bg-slate-950 text-slate-200 dark:border-slate-700">
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
      </div>
    </details>
  );
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
