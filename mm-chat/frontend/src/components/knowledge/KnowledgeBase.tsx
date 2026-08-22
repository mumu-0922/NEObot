"use client";

import ServerKnowledgeBase from "./ServerKnowledgeBase";

interface KnowledgeBaseProps {
  onClose?: () => void;
  selectedCollectionId: string | null;
  onSelectedCollectionIdChange: (
    collectionId: string | null,
    historyMode?: "push" | "replace",
  ) => void;
}

const KnowledgeBase = ({
  onClose,
  selectedCollectionId,
  onSelectedCollectionIdChange,
}: KnowledgeBaseProps) => (
  <ServerKnowledgeBase
    onClose={onClose}
    selectedCollectionId={selectedCollectionId}
    onSelectedCollectionIdChange={onSelectedCollectionIdChange}
  />
);

export default KnowledgeBase;
