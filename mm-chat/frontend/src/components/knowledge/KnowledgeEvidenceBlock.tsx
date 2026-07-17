"use client";
import React, { useId, useState } from "react";
import { BookText, ChevronDown, Library, TriangleAlert } from "lucide-react";
import { useTranslations } from "next-intl";
import type { KnowledgeCitation, MessageKnowledgeMetadata } from "@/types";

interface KnowledgeEvidenceBlockProps {
  knowledge?: MessageKnowledgeMetadata;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}

function formatShortId(id: string | undefined): string | null {
  if (!id) return null;
  return id.length > 12 ? `${id.slice(0, 8)}…${id.slice(-4)}` : id;
}

function formatLocator(locator: unknown): string | null {
  if (!isRecord(locator)) return null;
  const page = locator.page;
  if (typeof page === "number" && Number.isFinite(page)) {
    return `p. ${page}`;
  }
  const sheet = typeof locator.sheet === "string" ? locator.sheet : null;
  const cell = typeof locator.cell === "string" ? locator.cell : null;
  if (sheet && cell) return `${sheet} ${cell}`;
  if (sheet) return sheet;
  const section =
    typeof locator.section === "string" ? locator.section.trim() : "";
  if (section) return section;
  try {
    return JSON.stringify(locator);
  } catch {
    return null;
  }
}

function citationTitle(citation: KnowledgeCitation): string {
  const source =
    formatShortId(citation.documentId) ??
    formatShortId(citation.collectionId) ??
    citation.id;
  const locator = formatLocator(citation.locator);
  return locator
    ? `${citation.marker} · ${source} · ${locator}`
    : `${citation.marker} · ${source}`;
}

const KnowledgeEvidenceBlock: React.FC<KnowledgeEvidenceBlockProps> = ({
  knowledge,
}) => {
  const t = useTranslations("Knowledge");
  const [isExpanded, setIsExpanded] = useState(false);
  const contentId = useId();
  const buttonId = useId();

  if (!knowledge) return null;

  const citations = knowledge.citations || [];
  const hasCitations = citations.length > 0;
  const isDegraded =
    knowledge.outcome === "dependency_unavailable" ||
    knowledge.outcome === "answer_governance_required";
  if (!hasCitations && !isDegraded) return null;

  if (!hasCitations) {
    return (
      <div
        role="status"
        className="mb-3 flex items-center gap-2 rounded-md border border-amber-200/80 bg-amber-50/70 px-3 py-2 text-xs text-amber-800 dark:border-amber-800/50 dark:bg-amber-950/20 dark:text-amber-200"
      >
        <TriangleAlert size={14} aria-hidden="true" className="shrink-0" />
        <span>{t("knowledgeTemporarilyUnavailable")}</span>
      </div>
    );
  }

  const heading = t("citationsHeading", { count: citations.length });
  const statusText = t("verifiedEvidenceUsed");

  return (
    <div className="mb-3 overflow-hidden rounded-lg border border-purple-200 bg-purple-50/50 transition-colors duration-300 dark:border-purple-800/60 dark:bg-purple-900/10">
      <button
        id={buttonId}
        type="button"
        aria-expanded={isExpanded}
        aria-controls={contentId}
        onClick={() => setIsExpanded(!isExpanded)}
        className="flex w-full items-center gap-2 px-3 py-2 text-xs font-medium text-purple-700 transition-colors hover:bg-purple-100/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-purple-500/60 dark:text-purple-300 dark:hover:bg-purple-900/20"
      >
        <Library
          size={14}
          className="text-purple-600 dark:text-purple-400"
          aria-hidden="true"
        />
        <span className="min-w-0 flex-1 truncate text-left">{heading}</span>
        <span className="shrink-0 rounded-full bg-white/70 px-2 py-0.5 text-[10px] uppercase tracking-wide text-purple-700 dark:bg-card/70 dark:text-purple-200">
          {knowledge.mode}
        </span>
        <ChevronDown
          size={14}
          aria-hidden="true"
          className={`transition-transform duration-200 ${isExpanded ? "rotate-180" : ""}`}
        />
      </button>

      <div
        id={contentId}
        role="region"
        aria-labelledby={buttonId}
        className={`grid transition-[grid-template-rows,opacity] duration-300 ease-in-out ${isExpanded ? "grid-rows-[1fr] opacity-100" : "grid-rows-[0fr] opacity-0"}`}
      >
        <div className="overflow-hidden">
          {isExpanded && (
            <div className="border-t border-purple-200/50 bg-white/40 px-3 py-3 dark:border-purple-800/50 dark:bg-card/40">
              <p className="mb-2 text-[11px] leading-relaxed text-gray-600 dark:text-foreground/80">
                {statusText}
              </p>
              <div className="space-y-2">
                {citations.map((citation) => (
                  <div
                    key={citation.id}
                    className="rounded-lg border border-purple-100 bg-white/60 p-3 dark:border-purple-900/30 dark:bg-muted/60"
                  >
                    <div className="mb-1.5 flex items-center gap-2">
                      <div className="flex h-4 w-4 shrink-0 items-center justify-center overflow-hidden rounded-sm bg-purple-100 text-purple-600 dark:bg-purple-900/30 dark:text-purple-400">
                        <BookText size={10} aria-hidden="true" />
                      </div>
                      <div className="line-clamp-1 text-xs font-bold text-gray-800 dark:text-foreground">
                        {citationTitle(citation)}
                      </div>
                    </div>
                    <div className="font-mono text-[11px] leading-relaxed text-gray-600 opacity-90 line-clamp-3 dark:text-foreground/85">
                      {citation.snippet}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

export default KnowledgeEvidenceBlock;
