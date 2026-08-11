"use client";

import McpToolsControl from "./McpToolsControl";

interface McpToolsPageProps {
  conversationId?: string;
  enabled: boolean;
  onClose: () => void;
}

export default function McpToolsPage({
  conversationId,
  enabled,
  onClose,
}: McpToolsPageProps) {
  return (
    <McpToolsControl
      conversationId={conversationId}
      enabled={enabled}
      variant="page"
      onClose={onClose}
    />
  );
}
