export const CHAT_PANEL_VALUES = [
  "chat",
  "skill-store",
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

export interface ChatPanelUrlState {
  panel: ChatPanel;
  settingsTab: SettingsTabId | null;
  skillId: string | null;
  knowledgeCollectionId: string | null;
  needsReplace: boolean;
  normalizedSearchParams: URLSearchParams;
}

const QUERY_PANEL_VALUES: readonly ChatPanel[] = [
  "skill-store",
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

const isSkillId = (value: string | null): value is string =>
  value !== null && /^[a-z][a-z0-9_]*_[a-z0-9]{16,64}$/.test(value);

const isKnowledgeCollectionId = (value: string | null): value is string =>
  value !== null &&
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value);

const cloneSearchParams = (input: URLSearchParams | string) =>
  new URLSearchParams(input);

export const parseChatPanelUrlState = (
  input: URLSearchParams | string,
): ChatPanelUrlState => {
  const originalParams = cloneSearchParams(input);
  const normalizedSearchParams = cloneSearchParams(input);
  const rawPanel = originalParams.get("panel");
  const rawSettingsTab = originalParams.get("settingsTab");
  const rawSkillId = originalParams.get("skillId");
  const rawKnowledgeCollectionId = originalParams.get("collectionId");
  const legacyAgentTab = originalParams.get("agentTab");
  const legacyAgentId = originalParams.get("agentId");
  let panel: ChatPanel = "chat";
  let settingsTab: SettingsTabId | null = null;
  let skillId: string | null = null;
  let knowledgeCollectionId: string | null = null;
  let needsReplace = false;

  if (rawPanel === "agent-center") {
    panel = "skill-store";
    normalizedSearchParams.set("panel", "skill-store");
    needsReplace = true;
  } else if (isQueryPanel(rawPanel)) {
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

  if (panel === "skill-store") {
    const migratedSkillId =
      rawPanel === "agent-center" && legacyAgentTab === "skills"
        ? legacyAgentId
        : null;
    const candidate = rawSkillId ?? migratedSkillId;
    if (isSkillId(candidate)) {
      skillId = candidate;
      if (rawSkillId !== candidate) {
        normalizedSearchParams.set("skillId", candidate);
        needsReplace = true;
      }
    } else if (rawSkillId !== null) {
      normalizedSearchParams.delete("skillId");
      needsReplace = true;
    }
  } else if (rawSkillId !== null) {
    normalizedSearchParams.delete("skillId");
    needsReplace = true;
  }

  if (panel === "knowledge") {
    if (isKnowledgeCollectionId(rawKnowledgeCollectionId)) {
      knowledgeCollectionId = rawKnowledgeCollectionId;
    } else if (rawKnowledgeCollectionId !== null) {
      normalizedSearchParams.delete("collectionId");
      needsReplace = true;
    }
  } else if (rawKnowledgeCollectionId !== null) {
    normalizedSearchParams.delete("collectionId");
    needsReplace = true;
  }

  for (const key of ["agentTab", "agentId"]) {
    if (originalParams.has(key)) {
      normalizedSearchParams.delete(key);
      needsReplace = true;
    }
  }

  return {
    panel,
    settingsTab,
    skillId,
    knowledgeCollectionId,
    needsReplace,
    normalizedSearchParams,
  };
};

export const setChatPanelUrlState = (
  input: URLSearchParams | string,
  state: {
    panel: ChatPanel;
    settingsTab?: SettingsTabId | null;
    skillId?: string | null;
    knowledgeCollectionId?: string | null;
  },
): URLSearchParams => {
  const params = cloneSearchParams(input);

  params.delete("panel");
  params.delete("settingsTab");
  params.delete("skillId");
  params.delete("collectionId");
  params.delete("agentTab");
  params.delete("agentId");

  if (state.panel === "chat") {
    return params;
  }

  params.set("panel", state.panel);

  if (state.panel === "settings") {
    params.set("settingsTab", state.settingsTab ?? "providers");
  }

  if (state.panel === "skill-store" && state.skillId) {
    params.set("skillId", state.skillId);
  }

  if (state.panel === "knowledge" && state.knowledgeCollectionId) {
    params.set("collectionId", state.knowledgeCollectionId);
  }

  return params;
};
