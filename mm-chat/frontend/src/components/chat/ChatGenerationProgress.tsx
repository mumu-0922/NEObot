"use client";

import { BookOpenText, Globe2, LoaderCircle } from "lucide-react";

import type { ChatGenerationProgressStage } from "@/lib/chat/generationProgress";

interface ChatGenerationProgressProps {
  stage: ChatGenerationProgressStage;
  label: string;
}

const stageIcons = {
  knowledge: BookOpenText,
  search: Globe2,
  model: LoaderCircle,
} as const;

export default function ChatGenerationProgress({
  stage,
  label,
}: ChatGenerationProgressProps) {
  const Icon = stageIcons[stage];

  return (
    <div
      role="status"
      aria-live="polite"
      aria-busy="true"
      className="flex min-h-6 items-center gap-2 text-sm font-medium text-gray-600 dark:text-muted-foreground"
    >
      <Icon
        size={16}
        aria-hidden="true"
        className={
          stage === "model"
            ? "motion-safe:animate-spin"
            : "motion-safe:animate-pulse"
        }
      />
      <span>{label}</span>
    </div>
  );
}
