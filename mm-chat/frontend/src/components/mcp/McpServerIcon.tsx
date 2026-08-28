"use client";

import { Box } from "lucide-react";
import Image from "next/image";
import { useState } from "react";

interface McpServerIconProps {
  icon?: string;
  large?: boolean;
  compact?: boolean;
}

export default function McpServerIcon({
  icon,
  large = false,
  compact = false,
}: McpServerIconProps) {
  const [failedImageUrl, setFailedImageUrl] = useState("");
  const imageUrl = icon?.startsWith("https://") ? icon : "";
  const shortText =
    icon && !imageUrl && Array.from(icon).length <= 4 ? icon : "";
  let sizeClass = "h-10 w-10 text-lg";
  let glyphSize = 18;
  let imageSize = 40;
  if (large) {
    sizeClass = "h-12 w-12 text-xl";
    glyphSize = 21;
    imageSize = 48;
  } else if (compact) {
    sizeClass = "h-8 w-8 rounded-lg text-sm";
    glyphSize = 15;
    imageSize = 32;
  }

  return (
    <span
      className={`relative flex shrink-0 items-center justify-center overflow-hidden rounded-xl bg-cyan-50 text-cyan-700 dark:bg-cyan-950/30 dark:text-cyan-200 ${sizeClass}`}
      aria-hidden="true"
    >
      <span className="absolute inset-0 flex items-center justify-center">
        {shortText || <Box size={glyphSize} aria-hidden="true" />}
      </span>
      {imageUrl && failedImageUrl !== imageUrl ? (
        <Image
          src={imageUrl}
          alt=""
          width={imageSize}
          height={imageSize}
          unoptimized
          loading="lazy"
          referrerPolicy="no-referrer"
          onError={() => setFailedImageUrl(imageUrl)}
          className="relative z-10 h-full w-full object-cover"
        />
      ) : null}
    </span>
  );
}
