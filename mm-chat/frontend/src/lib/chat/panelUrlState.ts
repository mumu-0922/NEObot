export const CHAT_PANEL_VALUES = [
  "chat",
  "agent-center",
  "assistants",
  "knowledge",
  "tools",
  "settings",
] as const;

export type ChatPanel = (typeof CHAT_PANEL_VALUES)[number];

export const SETTINGS_TAB_VALUES = [
  "providers",
  "defaults",
  "search",
  "rag",
  "voice",
  "memory",
  "health",
  "system",
] as const;

export type SettingsTabId = (typeof SETTINGS_TAB_VALUES)[number];

export const AGENT_CENTER_TAB_VALUES = [
  "skills",
  "runs",
  "schedules",
  "learning",
] as const;

export type AgentCenterTabId = (typeof AGENT_CENTER_TAB_VALUES)[number];

export interface ChatPanelUrlState {
  panel: ChatPanel;
  settingsTab: SettingsTabId | null;
  agentTab: AgentCenterTabId | null;
  agentId: string | null;
  needsReplace: boolean;
  normalizedSearchParams: URLSearchParams;
}

const QUERY_PANEL_VALUES: readonly ChatPanel[] = [
  "agent-center",
  "assistants",
  "knowledge",
  "tools",
  "settings",
];

const isChatPanel = (value: string | null): value is ChatPanel =>
  value !== null && CHAT_PANEL_VALUES.includes(value as ChatPanel);

const isQueryPanel = (value: string | null): value is ChatPanel =>
  value !== null && QUERY_PANEL_VALUES.includes(value as ChatPanel);

const isSettingsTab = (value: string | null): value is SettingsTabId =>
  value !== null && SETTINGS_TAB_VALUES.includes(value as SettingsTabId);

const isAgentCenterTab = (value: string | null): value is AgentCenterTabId =>
  value !== null && AGENT_CENTER_TAB_VALUES.includes(value as AgentCenterTabId);

const isAgentCenterId = (value: string | null): value is string =>
  value !== null && /^[a-z][a-z0-9_]*_[a-z0-9]{16,64}$/.test(value);

const cloneSearchParams = (input: URLSearchParams | string) =>
  new URLSearchParams(input);

export const parseChatPanelUrlState = (
  input: URLSearchParams | string,
): ChatPanelUrlState => {
  const originalParams = cloneSearchParams(input);
  const normalizedSearchParams = cloneSearchParams(input);
  const rawPanel = originalParams.get("panel");
  const rawSettingsTab = originalParams.get("settingsTab");
  const rawAgentTab = originalParams.get("agentTab");
  const rawAgentId = originalParams.get("agentId");
  let panel: ChatPanel = "chat";
  let settingsTab: SettingsTabId | null = null;
  let agentTab: AgentCenterTabId | null = null;
  let agentId: string | null = null;
  let needsReplace = false;

  if (isQueryPanel(rawPanel)) {
    panel = rawPanel;
  } else if (isChatPanel(rawPanel)) {
    normalizedSearchParams.delete("panel");
    needsReplace = true;
  } else if (rawPanel !== null) {
    normalizedSearchParams.delete("panel");
    needsReplace = true;
  }

  if (panel === "settings") {
    if (isSettingsTab(rawSettingsTab)) {
      settingsTab = rawSettingsTab;
    } else if (rawSettingsTab !== null) {
      normalizedSearchParams.delete("settingsTab");
      needsReplace = true;
    }
  } else if (rawSettingsTab !== null) {
    normalizedSearchParams.delete("settingsTab");
    needsReplace = true;
  }

  if (panel === "agent-center") {
    agentTab = isAgentCenterTab(rawAgentTab) ? rawAgentTab : "skills";
    if (rawAgentTab !== null && !isAgentCenterTab(rawAgentTab)) {
      normalizedSearchParams.delete("agentTab");
      needsReplace = true;
    }
    if (isAgentCenterId(rawAgentId)) {
      agentId = rawAgentId;
    } else if (rawAgentId !== null) {
      normalizedSearchParams.delete("agentId");
      needsReplace = true;
    }
  } else {
    for (const key of ["agentTab", "agentId"]) {
      if (originalParams.has(key)) {
        normalizedSearchParams.delete(key);
        needsReplace = true;
      }
    }
  }

  return {
    panel,
    settingsTab,
    agentTab,
    agentId,
    needsReplace,
    normalizedSearchParams,
  };
};

export const setChatPanelUrlState = (
  input: URLSearchParams | string,
  state: {
    panel: ChatPanel;
    settingsTab?: SettingsTabId | null;
    agentTab?: AgentCenterTabId | null;
    agentId?: string | null;
  },
): URLSearchParams => {
  const params = cloneSearchParams(input);

  params.delete("panel");
  params.delete("settingsTab");
  params.delete("agentTab");
  params.delete("agentId");

  if (state.panel === "chat") {
    return params;
  }

  params.set("panel", state.panel);

  if (state.panel === "settings") {
    params.set("settingsTab", state.settingsTab ?? "providers");
  }

  if (state.panel === "agent-center") {
    params.set("agentTab", state.agentTab ?? "skills");
    if (state.agentId) params.set("agentId", state.agentId);
  }

  return params;
};
