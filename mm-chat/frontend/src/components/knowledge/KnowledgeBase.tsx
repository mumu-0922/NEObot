"use client";

import ServerKnowledgeBase from "./ServerKnowledgeBase";

interface KnowledgeBaseProps {
  onClose?: () => void;
}

const KnowledgeBase = ({ onClose }: KnowledgeBaseProps) => (
  <ServerKnowledgeBase onClose={onClose} />
);

export default KnowledgeBase;
