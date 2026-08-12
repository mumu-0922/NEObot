"use client";

import { Box } from "lucide-react";
import Image from "next/image";
import { useState } from "react";

interface McpServerIconProps {
  icon?: string;
  large?: boolean;
}

export default function McpServerIcon({
  icon,
  large = false,
}: McpServerIconProps) {
  const [failedImageUrl, setFailedImageUrl] = useState("");
  const imageUrl = icon?.startsWith("https://") ? icon : "";
  const shortText =
    icon && !imageUrl && Array.from(icon).length <= 4 ? icon : "";

  return (
    <span
      className={`flex shrink-0 items-center justify-center overflow-hidden rounded-xl bg-cyan-50 text-cyan-700 dark:bg-cyan-950/30 dark:text-cyan-200 ${
        large ? "h-12 w-12 text-xl" : "h-10 w-10 text-lg"
      }`}
      aria-hidden="true"
    >
      {imageUrl && failedImageUrl !== imageUrl ? (
        <Image
          src={imageUrl}
          alt=""
          width={large ? 48 : 40}
          height={large ? 48 : 40}
          unoptimized
          loading="lazy"
          referrerPolicy="no-referrer"
          onError={() => setFailedImageUrl(imageUrl)}
          className="h-full w-full object-cover"
        />
      ) : (
        shortText || <Box size={large ? 21 : 18} aria-hidden="true" />
      )}
    </span>
  );
}
