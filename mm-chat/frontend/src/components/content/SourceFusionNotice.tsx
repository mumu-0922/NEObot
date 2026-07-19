"use client";

import React from "react";
import { Globe2 } from "lucide-react";
import { useTranslations } from "next-intl";
import { getWebSearchDegradationReason } from "@/lib/search/fusion";

interface SourceFusionNoticeProps {
  metadata?: Record<string, unknown>;
}

const SourceFusionNotice: React.FC<SourceFusionNoticeProps> = ({
  metadata,
}) => {
  const t = useTranslations("Content");
  if (!getWebSearchDegradationReason(metadata)) return null;

  return (
    <div
      role="status"
      className="mb-3 flex items-center gap-2 rounded-md border border-amber-200/80 bg-amber-50/70 px-3 py-2 text-xs text-amber-800 dark:border-amber-800/50 dark:bg-amber-950/20 dark:text-amber-200"
    >
      <Globe2 size={14} aria-hidden="true" className="shrink-0" />
      <span>{t("webSearchTemporarilyUnavailable")}</span>
    </div>
  );
};

export default SourceFusionNotice;
