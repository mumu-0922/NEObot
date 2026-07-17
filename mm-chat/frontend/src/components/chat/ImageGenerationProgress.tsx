"use client";

import { useEffect, useState } from "react";
import { Image as ImageIcon, LoaderCircle } from "lucide-react";
import { useTranslations } from "next-intl";

interface ImageGenerationProgressProps {
  startedAt: number;
}

export function getImageGenerationElapsedSeconds(
  startedAt: number,
  now = Date.now(),
): number {
  if (!Number.isFinite(startedAt) || !Number.isFinite(now)) return 0;
  return Math.max(0, Math.floor((now - startedAt) / 1000));
}

const ImageGenerationProgress = ({
  startedAt,
}: ImageGenerationProgressProps) => {
  const t = useTranslations("Message");
  const [elapsedSeconds, setElapsedSeconds] = useState(() =>
    getImageGenerationElapsedSeconds(startedAt),
  );

  useEffect(() => {
    const updateElapsed = () => {
      setElapsedSeconds(getImageGenerationElapsedSeconds(startedAt));
    };
    updateElapsed();
    const interval = window.setInterval(updateElapsed, 1000);
    return () => window.clearInterval(interval);
  }, [startedAt]);

  return (
    <div
      role="status"
      aria-live="polite"
      aria-label={t("generatingImage")}
      className="w-full max-w-sm overflow-hidden rounded-xl border border-red-100 bg-red-50/40 shadow-sm dark:border-red-950/70 dark:bg-red-950/20"
    >
      <div className="flex items-center gap-3 px-4 py-3.5">
        <div className="relative flex size-11 shrink-0 items-center justify-center rounded-xl bg-white text-red-400 shadow-sm ring-1 ring-red-100 dark:bg-red-950/60 dark:ring-red-900/70">
          <ImageIcon size={21} aria-hidden="true" />
          <LoaderCircle
            size={15}
            aria-hidden="true"
            className="absolute -right-1 -bottom-1 animate-spin rounded-full bg-white text-red-500 dark:bg-red-950"
          />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center justify-between gap-3">
            <p className="truncate text-sm font-medium text-gray-800 dark:text-gray-100">
              {t("generatingImage")}
            </p>
            <span className="shrink-0 font-mono text-xs tabular-nums text-gray-500 dark:text-gray-400">
              {t("imageGenerationElapsed", { seconds: elapsedSeconds })}
            </span>
          </div>
          <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {t("imageGenerationHint")}
          </p>
        </div>
      </div>
      <div className="h-1 w-full overflow-hidden bg-red-100 dark:bg-red-950/80">
        <div className="h-full w-full animate-pulse bg-red-400 dark:bg-red-500" />
      </div>
    </div>
  );
};

export default ImageGenerationProgress;
