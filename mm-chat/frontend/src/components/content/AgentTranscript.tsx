"use client";

import { useMemo } from "react";
import {
  Brain,
  Check,
  ChevronDown,
  DatabaseZap,
  LoaderCircle,
} from "lucide-react";
import { useTranslations } from "next-intl";

import { projectAgentTranscript } from "@/lib/chat/agentTranscript";
import type {
  AgentTranscriptContextNode,
  AgentTranscriptReasoningNode,
} from "@/lib/chat/agentTranscript";
import type { ChatAgentEvent } from "@/types";
import MarkdownRenderer from "./MarkdownRenderer";
import { ProcessStepRow } from "./ProcessTracePanel";

interface AgentTranscriptProps {
  events: ChatAgentEvent[];
}

export default function AgentTranscript({ events }: AgentTranscriptProps) {
  const t = useTranslations("Content");
  const nodes = useMemo(() => projectAgentTranscript(events), [events]);
  if (nodes.length === 0) return null;

  return (
    <section
      className="mb-3"
      aria-label={t("agentTranscript")}
      data-testid="agent-transcript"
    >
      <ol className="space-y-2 border-l border-gray-200 pl-2.5 dark:border-border">
        {nodes.map((node) => {
          if (node.type === "tool") {
            return <ProcessStepRow key={node.id} step={node.step} />;
          }
          if (node.type === "context") {
            return <ContextInjectionRow key={node.id} node={node} />;
          }
          return <ReasoningRow key={node.id} node={node} />;
        })}
      </ol>
    </section>
  );
}

function ContextInjectionRow({ node }: { node: AgentTranscriptContextNode }) {
  const t = useTranslations("Content");
  return (
    <li className="flex min-w-0 items-start gap-2 text-xs text-gray-600 dark:text-muted-foreground">
      <span className="mt-0.5 rounded bg-sky-100 p-1 text-sky-700 dark:bg-sky-950/40 dark:text-sky-300">
        <DatabaseZap size={12} aria-hidden="true" />
      </span>
      <details className="group/transcript min-w-0 flex-1">
        <summary className="flex min-h-6 cursor-pointer list-none items-center gap-2 marker:content-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/50 [&::-webkit-details-marker]:hidden">
          <span className="shrink-0 font-medium text-gray-700 dark:text-foreground/85">
            {t("agentContextInjection")}
          </span>
          <span className="min-w-0 flex-1 truncate text-[11px] text-gray-400 dark:text-muted-foreground/70">
            · {node.label}
          </span>
          {node.truncated ? (
            <span className="shrink-0 text-[10px] text-amber-600 dark:text-amber-300">
              {t("agentTranscriptTruncated")}
            </span>
          ) : null}
          <ChevronDown
            size={13}
            aria-hidden="true"
            className="shrink-0 text-gray-400 transition-transform group-open/transcript:rotate-180 motion-reduce:transition-none"
          />
        </summary>
        <pre className="mt-1.5 max-h-72 overflow-auto whitespace-pre-wrap break-words rounded-md border border-gray-200 bg-gray-50 px-3 py-2 font-mono text-[11px] leading-5 text-gray-700 custom-scrollbar dark:border-border dark:bg-muted/40 dark:text-foreground/80">
          <code>{node.content}</code>
        </pre>
      </details>
    </li>
  );
}

function ReasoningRow({ node }: { node: AgentTranscriptReasoningNode }) {
  const t = useTranslations("Content");
  const active = node.status === "running";
  const summary = reasoningSummary(node.content, t("thinking"));
  return (
    <li className="flex min-w-0 items-start gap-2 text-xs text-gray-600 dark:text-muted-foreground">
      <span
        className={`mt-0.5 rounded p-1 ${active ? "bg-violet-100 text-violet-700 dark:bg-violet-950/40 dark:text-violet-300" : "bg-gray-100 text-gray-500 dark:bg-muted dark:text-muted-foreground"}`}
      >
        {active ? (
          <LoaderCircle
            size={12}
            className="motion-safe:animate-spin"
            aria-hidden="true"
          />
        ) : node.status === "completed" ? (
          <Check size={12} aria-hidden="true" />
        ) : (
          <Brain size={12} aria-hidden="true" />
        )}
      </span>
      <details
        open={active || undefined}
        className="group/transcript min-w-0 flex-1"
      >
        <summary className="flex min-h-6 cursor-pointer list-none items-center gap-2 marker:content-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/50 [&::-webkit-details-marker]:hidden">
          <span className="shrink-0 font-medium text-gray-700 dark:text-foreground/85">
            {t("agentThink")}
          </span>
          <span className="min-w-0 flex-1 truncate text-[11px] text-gray-400 dark:text-muted-foreground/70">
            · {summary}
          </span>
          {node.round ? (
            <span className="shrink-0 text-[10px] text-gray-400 dark:text-muted-foreground/70">
              {t("processRound", { round: node.round })}
            </span>
          ) : null}
          <ChevronDown
            size={13}
            aria-hidden="true"
            className="shrink-0 text-gray-400 transition-transform group-open/transcript:rotate-180 motion-reduce:transition-none"
          />
        </summary>
        <div className="mt-1.5 rounded-md border border-gray-200 bg-white/75 px-3 py-2 dark:border-border dark:bg-card/70">
          {node.content ? (
            <MarkdownRenderer
              content={node.content}
              className="text-xs! text-gray-600 md:text-sm! dark:text-foreground/85"
            />
          ) : (
            <span className="text-[11px] text-gray-400">{t("thinking")}</span>
          )}
        </div>
      </details>
    </li>
  );
}

function reasoningSummary(content: string, fallback: string): string {
  const summary = content
    .replace(/[#*_`>\[\]()]/g, " ")
    .replace(/\s+/g, " ")
    .trim();
  if (!summary) return fallback;
  return summary.length > 96 ? `${summary.slice(0, 96)}…` : summary;
}
