export type FixtureModel = {
  providerId: string;
  modelId: string;
};

export type FixtureConversation = {
  id: string;
  title: string;
  modelRef: FixtureModel;
  workspaceId?: string;
  activeGeneration?: {
    runId: string;
    messageId?: string;
    status: "pending" | "streaming";
  };
};

export type FixtureMessage = Record<string, unknown> & {
  id: string;
  conversationId: string;
  role: "user" | "assistant";
  status: "pending" | "streaming" | "completed" | "failed" | "cancelled";
  content: string;
  sequenceNo: number;
};

export type PendingRunResult =
  | { status: "completed"; content: string; agentEvents?: unknown[] }
  | { status: "failed"; code?: string; message?: string };

export type PendingRun = {
  runId: string;
  messageId: string;
  userMessageId: string;
  resolve: (result: PendingRunResult) => void;
  result: Promise<PendingRunResult>;
};
