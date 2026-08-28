"use client";

import { PackageOpen } from "lucide-react";
import Image from "next/image";
import { useState } from "react";

interface SkillIconProps {
  icon?: string;
  label: string;
  compact?: boolean;
}

export function SkillIcon({ icon, label, compact = false }: SkillIconProps) {
  const [failedImageUrl, setFailedImageUrl] = useState("");
  const imageUrl = getSkillIconImageURL(icon);
  const shortText =
    icon && !imageUrl && Array.from(icon).length <= 4 ? icon : "";
  const fallback = shortText || label.trim().slice(0, 1).toUpperCase();

  return (
    <span
      aria-hidden="true"
      className={`relative flex shrink-0 items-center justify-center overflow-hidden bg-cyan-50 font-bold text-cyan-700 dark:bg-cyan-950/30 dark:text-cyan-300 ${
        compact ? "h-8 w-8 rounded-lg text-xs" : "h-11 w-11 rounded-xl text-sm"
      }`}
    >
      <span className="absolute inset-0 flex items-center justify-center">
        {fallback || <PackageOpen size={18} aria-hidden="true" />}
      </span>
      {imageUrl && failedImageUrl !== imageUrl ? (
        <Image
          src={imageUrl}
          alt=""
          width={compact ? 32 : 44}
          height={compact ? 32 : 44}
          loading="lazy"
          referrerPolicy="no-referrer"
          onError={() => setFailedImageUrl(imageUrl)}
          className="relative z-10 h-full w-full object-cover"
        />
      ) : null}
    </span>
  );
}

export function getSkillIconImageURL(icon?: string): string {
  if (!icon) return "";
  try {
    const parsed = new URL(icon);
    if (
      parsed.protocol !== "https:" ||
      parsed.hostname !== "github.com" ||
      parsed.port ||
      parsed.username ||
      parsed.password ||
      parsed.search ||
      parsed.hash ||
      !/^\/[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})\.png$/.test(parsed.pathname)
    ) {
      return "";
    }
    return parsed.toString();
  } catch {
    return "";
  }
}
