"use client";
import React, {
  useState,
  useEffect,
  useMemo,
  useRef,
  useCallback,
} from "react";
import { useLocale, useTranslations } from "next-intl";
import dynamic from "next/dynamic";
import {
  Bot,
  MessageSquarePlus,
  PanelLeftClose,
  PanelLeftOpen,
} from "lucide-react";
import { v7 as uuidv7 } from "uuid";

import Sidebar from "@/components/layout/Sidebar";
import ChatGenerationProgress from "@/components/chat/ChatGenerationProgress";
import MessageItem from "@/components/chat/MessageItem";
import ImageGenerationProgress from "@/components/chat/ImageGenerationProgress";
import MessageInput, { MessageInputRef } from "@/components/chat/MessageInput";
import AssistantHeader from "@/components/assistant/AssistantHeader";
import Tooltip from "@/components/ui/Tooltip";
import FollowUpQuestions from "@/components/chat/FollowUpQuestions";
import { Logo } from "@/components/ui/Icons";
import type { ModelInfo } from "@/services/api/chatService";
import { createNeoChatApiClient } from "@/services/api/client";
import { uploadMessageAttachmentsForServer } from "@/services/api/fileService";
import { resolveSkillsForMessage } from "@/services/api/skillService";
import { orchestrateServerPlugins } from "@/services/api/serverPluginOrchestration";
import { buildProviderRuntimeConfig } from "@/lib/byok/client";
import { getAgentDetail } from "@/services/api/agentService";
import {
  Message,
  Attachment,
  LobeAgent,
  ReasoningEffort,
  SessionMessageTree,
} from "@/types";
import { useChatStore } from "@/store/core/chatStore";
import { useMemoryStore } from "@/store/core/memoryStore";
import { appDb } from "@/store/storage/storageConfig";
import { formatModelName } from "@/store/core/settingsStore";
import { handleTokenUsageUpdate } from "@/lib/utils/message";
import {
  buildAvailableModels,
  findRecentImageGenerationModel,
  isImageGenerationModel,
  resolveImageGenerationRoute,
  resolveSelectedModel,
} from "@/lib/utils/models";
import {
  processMessageForSending,
  createBotMessagePlaceholder,
  getModelDisplayName,
} from "@/lib/chat/messageProcessor";
import {
  createSessionPostGenerationSnapshot,
  shouldAbortActiveGenerationForSessionDelete,
  shouldApplyCompressionUpdate,
  shouldApplyGeneratedTitle,
  shouldApplyRequestedTitle,
  shouldApplySuggestedQuestions,
} from "@/lib/chat/postGenerationGuards";
import {
  useChatGenerationController,
  useChatShellState,
  useChatThemeEffects,
} from "@/features/chat";
import { resolveEffectiveChatContext } from "@/lib/chat/effectiveChatContext";
import { buildDirectMemoryPromptContext } from "@/lib/memory/entities";
import { appendContextToChatInput } from "@/lib/utils/chatInput";
import {
  getActiveMessagePath,
  getMessageBranchInfo,
  normalizeSessionMessageTree,
} from "@/lib/chat/messageTree";
import { normalizeActivePluginIds } from "@/lib/plugin/config";
import { parseModelString } from "@/lib/utils/model";
import { logDevError } from "@/lib/utils/devLogger";
import { SERVER_DEFAULT_PROVIDER_ID } from "@/lib/defaultConfig/shared";
import { normalizeServerManagedProviderConfigs } from "@/lib/providers/config";
import {
  getSessionPluginPresetSyncKey,
  shouldApplySessionPluginPreset,
  shouldResolveSelectedModelAfterBootstrap,
  shouldRunSettingsStartupEffects,
} from "@/lib/app/startupEffects";
import {
  ChatPanel,
  SettingsTabId,
  parseChatPanelUrlState,
  setChatPanelUrlState,
} from "@/lib/chat/panelUrlState";
import { buildSearchUpdate } from "@/lib/chat/searchUpdate";
import { inferPendingChatProgressStage } from "@/lib/chat/generationProgress";
import {
  getChatComposerClearance,
  getChatScrollDistanceFromBottom,
  resolveChatScrollFollowOnScroll,
  resolveChatScrollFollowOnWheel,
} from "@/lib/chat/scrollFollow";
import { toServerMessageAttachments } from "@/lib/utils/serverAttachments";
import {
  getKnowledgeAttachmentCollectionIds,
  isKnowledgeAttachment,
  MAX_CONVERSATION_KNOWLEDGE_COLLECTIONS,
  normalizeKnowledgeCollectionIds,
} from "@/lib/utils/knowledgeAttachments";

const ImagePreview = dynamic(() => import("@/components/media/ImagePreview"), {
  ssr: false,
});
const PluginMarket = dynamic(() => import("@/components/plugin/PluginMarket"), {
  ssr: false,
});
const SkillMarket = dynamic(() => import("@/components/skill/SkillMarket"), {
  ssr: false,
});
const AssistantHub = dynamic(
  () => import("@/components/assistant/AssistantHub"),
  {
    ssr: false,
  },
);
const KnowledgeBase = dynamic(
  () => import("@/components/knowledge/KnowledgeBase"),
  {
    ssr: false,
  },
);
const SettingsPage = dynamic(
  () => import("@/components/settings/SettingsPage"),
  {
    ssr: false,
  },
);

function getLegacyKnowledgeSelectionIdsForMigration(
  message: Pick<Message, "attachments" | "metadata">,
): string[] {
  const rawMetadataIds = message.metadata?.selectedKnowledgeCollectionIds;
  const metadataIds = Array.isArray(rawMetadataIds)
    ? rawMetadataIds.filter(
        (collectionId): collectionId is string =>
          typeof collectionId === "string",
      )
    : [];
  return normalizeKnowledgeCollectionIds([
    ...metadataIds,
    ...getKnowledgeAttachmentCollectionIds(message.attachments ?? []),
  ]).slice(0, MAX_CONVERSATION_KNOWLEDGE_COLLECTIONS);
}

const logChatAppError = logDevError;
const EMPTY_MESSAGES: Message[] = [];
const loadChatService = () => import("@/services/api/chatService");

const ChatApp = () => {
  // --- Global Store ---
  const {
    chat: {
      _hasHydrated: chatHasHydrated,
      sessions,
      workspaces,
      currentSessionId,
      activeMessages,
      activeMessageTree,
      serverReadState,
      selectedModel,
      chatConfig,
      createSession,
      selectSession,
      refreshServerSessions,
      selectServerSession,
      createServerSession,
      sendServerMessageAndStream,
      regenerateServerAssistantMessage,
      switchServerMessageVersion,
      updateServerSessionTitle,
      updateServerSessionInstruction,
      updateServerSessionConfig,
      toggleServerSessionPin,
      deleteServerSession,
      duplicateServerSession,
      generateServerConversationTitle,
      updateServerMessageContent,
      deleteServerMessage,
      retractServerMessage,
      deleteSession,
      updateSessionTitle,
      updateSessionInstruction,
      updateSessionCompression,
      updateSessionMemoryContext,
      toggleSessionPin,
      duplicateSession,
      addMessage,
      updateMessageContent,
      updateMessage,
      addMessageVersion,
      createEditedUserMessageBranch,
      switchMessageVersion,
      deleteMessage,
      deleteMessageAndSubsequent,
      setSuggestedQuestions,
      setModel,
      setChatConfig,
      getCurrentSession,
      syncActiveSession,
    },
    settings: {
      _hasHydrated,
      modelMetadata,
      customModelMetadata,
      fetchModelMetadata,
      ensureBuiltInPlugins,
      system,
      rag,
      search,
      activePlugins,
      installedPlugins,
      pluginConfigs,
      installedSkills,
      activeSkillIds,
      skillAutoSelect,
      setActiveSkillIds,
      setActivePlugins,
      applyServerConfig: applySettingsServerConfig,
    },
    core: {
      _hasHydrated: coreHasHydrated,
      theme,
      providers,
      updateProvider,
      replaceServerManagedProviders,
      applyServerConfig: applyCoreServerConfig,
    },
    knowledgeCollections,
  } = useChatShellState();

  const t = useTranslations("ChatApp");
  const locale = useLocale();
  const apiClientSnapshot = useMemo(() => createNeoChatApiClient(), []);
  const serverModeEnabled =
    apiClientSnapshot.mode === "server" &&
    apiClientSnapshot.capabilities.chatCrud &&
    apiClientSnapshot.capabilities.chatStream;
  const serverFilesEnabled =
    serverModeEnabled && apiClientSnapshot.capabilities.files;

  // --- Local UI State ---
  const [isSidebarOpen, setIsSidebarOpen] = useState(false);
  const [isMobileViewport, setIsMobileViewport] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [activeImageGeneration, setActiveImageGeneration] = useState<{
    startedAt: number;
  } | null>(null);
  const [composerClearance, setComposerClearance] = useState(() =>
    getChatComposerClearance(0),
  );
  const [composerAreaElement, setComposerAreaElement] =
    useState<HTMLDivElement | null>(null);
  const {
    isGenerating,
    beginActiveGeneration,
    isGenerationRunActive,
    finishActiveGeneration,
    abortActiveGeneration,
    stopActiveGeneration,
  } = useChatGenerationController();

  const queueMemoryExtraction = useCallback(
    (
      sessionId: string,
      userMessage: Pick<Message, "id" | "content">,
      assistantMessage: Pick<Message, "id" | "content">,
    ) => {
      loadChatService()
        .then(({ performBackgroundMemoryExtraction }) =>
          performBackgroundMemoryExtraction({
            sessionId,
            userMessage,
            assistantMessage,
          }),
        )
        .catch((err) => {
          logChatAppError("Memory extraction failed:", err);
        });
    },
    [],
  );

  const [viewMode, setViewMode] = useState<ChatPanel>("chat");
  const [settingsTab, setSettingsTab] = useState<SettingsTabId>("providers");

  const [serverConfigResolved, setServerConfigResolved] = useState(false);
  const [serverModelBootstrapReady, setServerModelBootstrapReady] =
    useState(false);

  const availableModels = useMemo<ModelInfo[]>(() => {
    if (!_hasHydrated || !coreHasHydrated) return [];

    return buildAvailableModels(
      providers,
      modelMetadata,
      customModelMetadata,
      formatModelName,
    );
  }, [
    _hasHydrated,
    coreHasHydrated,
    providers,
    modelMetadata,
    customModelMetadata,
  ]);

  const messagesScrollRef = useRef<HTMLDivElement>(null);
  const shouldFollowMessageBottomRef = useRef(true);
  const lastMessageScrollTopRef = useRef(0);
  const hasWheelMessageScrollIntentRef = useRef(false);
  const hasPointerMessageScrollIntentRef = useRef(false);
  const messageInputRef = useRef<MessageInputRef>(null);
  const actionErrorTimerRef = useRef<ReturnType<typeof setTimeout> | null>(
    null,
  );
  const assistantSelectRequestRef = useRef(0);
  const defaultProviderFetchRef = useRef(false);
  const serverBootstrapRef = useRef(false);

  const visibleSessions = serverModeEnabled
    ? serverReadState.sessions
    : sessions;
  const visibleCurrentSessionId = serverModeEnabled
    ? serverReadState.currentSessionId
    : currentSessionId;
  const visibleActiveMessages = serverModeEnabled
    ? serverReadState.activeMessages
    : activeMessages;
  const visibleActiveMessageTree = serverModeEnabled
    ? serverReadState.activeMessageTree
    : activeMessageTree;
  const currentSession = serverModeEnabled
    ? (visibleSessions.find(
        (session) => session.id === visibleCurrentSessionId,
      ) ?? null)
    : getCurrentSession(); // This is just metadata now
  const messages = visibleActiveMessages ?? EMPTY_MESSAGES; // Use activeMessages from store
  const lastVisibleMessage = messages.at(-1);
  const hasVisibleImageGenerationMessage = Boolean(
    activeImageGeneration &&
    lastVisibleMessage?.role === "model" &&
    isImageGenerationModel(lastVisibleMessage.model || "") &&
    (serverReadState.generation.assistantMessageId === lastVisibleMessage.id ||
      (!lastVisibleMessage.content && !lastVisibleMessage.attachments?.length)),
  );
  const currentSessionConfig = currentSession?.config;
  const pendingServerProgressStage =
    serverModeEnabled &&
    isGenerating &&
    !activeImageGeneration &&
    serverReadState.generation.status === "pending" &&
    serverReadState.generation.userMessageId === lastVisibleMessage?.id &&
    lastVisibleMessage?.role === "user"
      ? inferPendingChatProgressStage({
          question: lastVisibleMessage.content,
          searchEnabled: currentSessionConfig?.useSearch ?? false,
          knowledgeCollectionIds:
            currentSessionConfig?.selectedKnowledgeCollectionIds,
        })
      : null;
  const currentSessionWorkspaceId = currentSession?.workspaceId;
  const serverSessionChatConfig = {
    useSearch: currentSessionConfig?.useSearch ?? false,
    searchResultsLimit: search.resultsLimit,
    useReasoning: currentSessionConfig?.useReasoning ?? chatConfig.useReasoning,
    reasoningEffort:
      currentSessionConfig?.reasoningEffort ?? chatConfig.reasoningEffort,
    activePlugins,
    activeSkills: activeSkillIds,
  };
  const composerChatConfig = serverModeEnabled
    ? {
        ...chatConfig,
        ...serverSessionChatConfig,
      }
    : chatConfig;
  useChatThemeEffects(theme, system.fontSize);

  const updateBrowserSearch = useCallback(
    (params: URLSearchParams, historyMode: "push" | "replace") => {
      if (typeof window === "undefined") return;

      const search = params.toString();
      const nextUrl = `${window.location.pathname}${
        search ? `?${search}` : ""
      }${window.location.hash}`;
      const currentUrl = `${window.location.pathname}${window.location.search}${window.location.hash}`;
      if (nextUrl === currentUrl) return;

      if (historyMode === "replace") {
        window.history.replaceState(null, "", nextUrl);
      } else {
        window.history.pushState(null, "", nextUrl);
      }
    },
    [],
  );

  const updatePanelUrl = useCallback(
    (
      panel: ChatPanel,
      nextSettingsTab?: SettingsTabId | null,
      historyMode: "push" | "replace" = "push",
    ) => {
      if (typeof window === "undefined") return;

      const nextParams = setChatPanelUrlState(
        new URLSearchParams(window.location.search),
        { panel, settingsTab: nextSettingsTab },
      );
      updateBrowserSearch(nextParams, historyMode);
    },
    [updateBrowserSearch],
  );

  const navigateToPanel = useCallback(
    (
      panel: ChatPanel,
      nextSettingsTab?: SettingsTabId | null,
      historyMode: "push" | "replace" = "push",
    ) => {
      const resolvedSettingsTab =
        panel === "settings" ? (nextSettingsTab ?? settingsTab) : null;

      setViewMode(panel);
      if (resolvedSettingsTab) {
        setSettingsTab(resolvedSettingsTab);
      }
      updatePanelUrl(panel, resolvedSettingsTab, historyMode);
      if (isMobileViewport) {
        setIsSidebarOpen(false);
      }
    },
    [isMobileViewport, settingsTab, updatePanelUrl],
  );

  const handleSettingsTabChange = useCallback(
    (tab: SettingsTabId) => {
      setSettingsTab(tab);
      if (viewMode === "settings") {
        updatePanelUrl("settings", tab);
      }
    },
    [updatePanelUrl, viewMode],
  );

  useEffect(() => {
    if (typeof window === "undefined") return;

    const syncPanelFromUrl = () => {
      const parsed = parseChatPanelUrlState(
        new URLSearchParams(window.location.search),
      );
      setViewMode(parsed.panel);
      setSettingsTab(parsed.settingsTab ?? "providers");
      if (parsed.needsReplace) {
        updateBrowserSearch(parsed.normalizedSearchParams, "replace");
      }
    };

    syncPanelFromUrl();
    window.addEventListener("popstate", syncPanelFromUrl);
    return () => window.removeEventListener("popstate", syncPanelFromUrl);
  }, [updateBrowserSearch]);

  useEffect(() => {
    if (typeof window === "undefined") return;

    const updateViewport = () => {
      setIsMobileViewport(window.innerWidth < 768);
    };

    updateViewport();
    window.addEventListener("resize", updateViewport);
    return () => window.removeEventListener("resize", updateViewport);
  }, []);

  const isMobileSidebarModalOpen = isSidebarOpen && isMobileViewport;

  useEffect(() => {
    if (!isMobileSidebarModalOpen) return;

    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";

    return () => {
      document.body.style.overflow = previousOverflow;
    };
  }, [isMobileSidebarModalOpen]);

  const mainInertProps = useMemo<
    React.HTMLAttributes<HTMLElement> & { inert?: boolean }
  >(
    () =>
      isMobileSidebarModalOpen ? { inert: true, "aria-hidden": true } : {},
    [isMobileSidebarModalOpen],
  );

  // Logic for Assistant List Animation
  const isChatEmpty =
    messages.length === 0 && !currentSession?.systemInstruction;
  const [welcomeState, setWelcomeState] = useState<
    "visible" | "exiting" | "hidden"
  >("hidden");
  const messageInputVariant = welcomeState === "visible" ? "hero" : "default";
  const shouldShowChatTitleBar = welcomeState === "hidden";
  const prevSessionIdRef = useRef(visibleCurrentSessionId);
  const inputSessionRef = useRef(visibleCurrentSessionId);
  const workspaceAttachmentHydratedSessionRef = useRef<string | null>(null);
  const syncedSessionPluginPresetRef = useRef<string | null>(null);

  // Sync welcomeState with chat emptiness, handling animations only within the same session
  useEffect(() => {
    // If session ID changed, snap to correct state immediately (no animation)
    if (prevSessionIdRef.current !== visibleCurrentSessionId) {
      setWelcomeState(isChatEmpty ? "visible" : "hidden");
      prevSessionIdRef.current = visibleCurrentSessionId;
      return;
    }

    // Same session transitions
    if (!isChatEmpty && welcomeState === "visible") {
      // Messages appeared -> animate out
      setWelcomeState("exiting");
    } else if (isChatEmpty && welcomeState !== "visible") {
      // Chat cleared -> snap back (or animate in? standard is snap for clear)
      setWelcomeState("visible");
    }
  }, [visibleCurrentSessionId, isChatEmpty, welcomeState]);

  // Handle Exiting Timer
  useEffect(() => {
    if (welcomeState === "exiting") {
      const timer = setTimeout(() => {
        setWelcomeState("hidden");
      }, 300); // Duration matches CSS transition
      return () => clearTimeout(timer);
    }
  }, [welcomeState]);

  // --- Effects ---

  // Sync Global Plugins from Session Config
  useEffect(() => {
    if (serverModeEnabled) return;

    const sessionPlugins = normalizeActivePluginIds(
      currentSessionConfig?.activePlugins,
      installedPlugins,
      pluginConfigs,
      { unauthenticatedAllowedPluginIds: ["unsplash"] },
    );
    const presetSyncKey = getSessionPluginPresetSyncKey(
      currentSessionId,
      sessionPlugins,
    );

    if (
      !shouldApplySessionPluginPreset(
        _hasHydrated,
        chatHasHydrated,
        sessionPlugins,
        syncedSessionPluginPresetRef.current,
        presetSyncKey,
      )
    ) {
      return;
    }

    const sortedSession = [...sessionPlugins].sort();
    const sortedActive = [...activePlugins].sort();

    if (JSON.stringify(sortedSession) !== JSON.stringify(sortedActive)) {
      setActivePlugins(sessionPlugins);
    }
    syncedSessionPluginPresetRef.current = presetSyncKey;
  }, [
    activePlugins,
    chatHasHydrated,
    currentSessionId,
    currentSessionConfig,
    _hasHydrated,
    installedPlugins,
    pluginConfigs,
    serverModeEnabled,
    setActivePlugins,
  ]);

  // Hydrate workspace preset files once when entering an empty workspace chat.
  useEffect(() => {
    if (serverModeEnabled) return;

    const inputSessionChanged = inputSessionRef.current !== currentSessionId;
    if (inputSessionChanged) {
      inputSessionRef.current = currentSessionId;
      workspaceAttachmentHydratedSessionRef.current = null;
    }

    const input = messageInputRef.current;
    if (!input) return;

    if (!currentSessionId || activeMessages.length > 0) {
      workspaceAttachmentHydratedSessionRef.current = null;
      if (inputSessionChanged) {
        input.setAttachments([]);
      }
      return;
    }

    if (workspaceAttachmentHydratedSessionRef.current === currentSessionId) {
      return;
    }

    const workspaceFiles = currentSessionWorkspaceId
      ? workspaces.find(
          (workspace) => workspace.id === currentSessionWorkspaceId,
        )?.files || []
      : [];
    input.setAttachments(workspaceFiles);
    workspaceAttachmentHydratedSessionRef.current = currentSessionId;
  }, [
    activeMessages.length,
    currentSessionId,
    currentSessionWorkspaceId,
    serverModeEnabled,
    workspaces,
  ]);

  // Fetch Metadata & Ensure Plugins on mount
  useEffect(() => {
    if (!shouldRunSettingsStartupEffects(_hasHydrated)) return;
    fetchModelMetadata();
    ensureBuiltInPlugins();
  }, [_hasHydrated, fetchModelMetadata, ensureBuiltInPlugins]);

  useEffect(() => {
    if (!coreHasHydrated || !_hasHydrated) return;

    let active = true;
    defaultProviderFetchRef.current = false;
    setServerConfigResolved(false);
    setServerModelBootstrapReady(false);

    const loadServerConfig = async () => {
      try {
        const [config, adminProviders] = await Promise.all([
          apiClientSnapshot.settings.getRuntimeConfig(),
          serverModeEnabled
            ? apiClientSnapshot.providers.listAdminProviderConfigs()
            : Promise.resolve(null),
        ]);
        if (!active) return;

        applyCoreServerConfig(config);
        applySettingsServerConfig(config);
        const normalizedServerProviders = adminProviders
          ? normalizeServerManagedProviderConfigs(adminProviders.providers)
          : null;
        if (normalizedServerProviders) {
          replaceServerManagedProviders(normalizedServerProviders);
        }
        setServerConfigResolved(true);
        const hasEnabledServerModels = normalizedServerProviders?.some(
          (provider) => provider.enabled && provider.models.length > 0,
        );
        const needsDefaultModelBootstrap = normalizedServerProviders
          ? !hasEnabledServerModels &&
            normalizedServerProviders.some(
              (provider) =>
                provider.isServerDefault &&
                provider.enabled &&
                provider.models.length === 0,
            )
          : config.modelProvider.available &&
            config.modelProvider.models.length === 0;
        if (!needsDefaultModelBootstrap) {
          setServerModelBootstrapReady(true);
        }
      } catch (error) {
        logChatAppError("Failed to load server config", error);
        if (!active) return;
        setServerConfigResolved(true);
        setServerModelBootstrapReady(true);
      }
    };

    loadServerConfig();

    return () => {
      active = false;
    };
  }, [
    _hasHydrated,
    applyCoreServerConfig,
    apiClientSnapshot,
    applySettingsServerConfig,
    coreHasHydrated,
    replaceServerManagedProviders,
    serverModeEnabled,
  ]);

  useEffect(() => {
    if (
      !coreHasHydrated ||
      !serverConfigResolved ||
      serverModelBootstrapReady
    ) {
      return;
    }

    const defaultProvider = providers.find(
      (provider) =>
        provider.id === SERVER_DEFAULT_PROVIDER_ID && provider.isServerDefault,
    );
    if (!defaultProvider) {
      setServerModelBootstrapReady(true);
      return;
    }
    if (
      defaultProvider.modelsList?.length ||
      defaultProvider.models.length > 0
    ) {
      setServerModelBootstrapReady(true);
      return;
    }
    if (defaultProviderFetchRef.current) return;

    let active = true;
    defaultProviderFetchRef.current = true;
    const providerSnapshot = defaultProvider;

    Promise.resolve()
      .then(async () =>
        apiClientSnapshot.providers.listModels({
          provider: await buildProviderRuntimeConfig(providerSnapshot),
        }),
      )
      .then((data) => {
        const models = data.models || [];
        updateProvider(providerSnapshot.id, {
          models,
          modelsList: models,
        });
        if (active) {
          setServerModelBootstrapReady(true);
        }
      })
      .catch((error) => {
        logChatAppError("Failed to fetch default provider models", error);
        if (active) {
          setServerModelBootstrapReady(true);
        }
      });

    return () => {
      active = false;
    };
  }, [
    apiClientSnapshot,
    coreHasHydrated,
    providers,
    serverConfigResolved,
    serverModelBootstrapReady,
    updateProvider,
  ]);

  useEffect(() => {
    if (
      !shouldResolveSelectedModelAfterBootstrap({
        chatHydrated: chatHasHydrated,
        settingsHydrated: _hasHydrated,
        coreHydrated: coreHasHydrated,
        serverModelBootstrapReady,
      })
    ) {
      return;
    }

    const nextModel = resolveSelectedModel(
      availableModels,
      selectedModel,
      SERVER_DEFAULT_PROVIDER_ID,
    );

    if (selectedModel === nextModel) {
      return;
    }

    setModel(nextModel);
  }, [
    chatHasHydrated,
    _hasHydrated,
    coreHasHydrated,
    serverModelBootstrapReady,
    availableModels,
    selectedModel,
    setModel,
  ]);

  // Check screen size on mount
  useEffect(() => {
    if (typeof window !== "undefined" && window.innerWidth > 768) {
      setIsSidebarOpen(true);
    }
  }, []);

  useEffect(() => {
    return () => {
      assistantSelectRequestRef.current += 1;
      if (actionErrorTimerRef.current) {
        clearTimeout(actionErrorTimerRef.current);
        actionErrorTimerRef.current = null;
      }
    };
  }, []);

  // Ensure a session exists on mount
  useEffect(() => {
    if (!serverModeEnabled || !chatHasHydrated || !serverModelBootstrapReady) {
      return;
    }
    if (serverBootstrapRef.current) return;
    serverBootstrapRef.current = true;

    refreshServerSessions()
      .then((loaded) => {
        if (!loaded) return;
        const state = useChatStore.getState().serverReadState;
        if (state.sessions.length === 0) {
          void createServerSession();
        }
      })
      .catch((error) => {
        logChatAppError("Failed to bootstrap server chat sessions", error);
        showActionError("Failed to load server chat sessions.");
      });
  }, [
    chatHasHydrated,
    createServerSession,
    refreshServerSessions,
    serverModeEnabled,
    serverModelBootstrapReady,
  ]);

  useEffect(() => {
    // Wait for chat store to hydrate before creating/selecting sessions
    if (!chatHasHydrated || serverModeEnabled) return;

    const timer = setTimeout(() => {
      if (sessions.length === 0) {
        createSession();
      } else if (!currentSessionId) {
        selectSession(sessions[0].id);
      }
    }, 100);
    return () => clearTimeout(timer);
  }, [
    chatHasHydrated,
    sessions,
    currentSessionId,
    createSession,
    selectSession,
    serverModeEnabled,
  ]);

  const updateMessageScrollFollow = useCallback(() => {
    const container = messagesScrollRef.current;
    if (!container) return;

    if (
      hasWheelMessageScrollIntentRef.current ||
      hasPointerMessageScrollIntentRef.current ||
      !shouldFollowMessageBottomRef.current
    ) {
      shouldFollowMessageBottomRef.current = resolveChatScrollFollowOnScroll({
        isFollowing: shouldFollowMessageBottomRef.current,
        previousScrollTop: lastMessageScrollTopRef.current,
        scrollTop: container.scrollTop,
        distanceFromBottom: getChatScrollDistanceFromBottom(container),
      });
    }
    lastMessageScrollTopRef.current = container.scrollTop;
    hasWheelMessageScrollIntentRef.current = false;
  }, []);

  const handleMessageWheel = useCallback(
    (event: React.WheelEvent<HTMLDivElement>) => {
      hasWheelMessageScrollIntentRef.current = true;
      shouldFollowMessageBottomRef.current = resolveChatScrollFollowOnWheel(
        shouldFollowMessageBottomRef.current,
        event.deltaY,
      );
    },
    [],
  );

  useEffect(() => {
    const composer = composerAreaElement;
    if (!composer) return;

    const updateComposerClearance = () => {
      setComposerClearance(
        getChatComposerClearance(composer.getBoundingClientRect().height),
      );
    };
    updateComposerClearance();

    if (typeof ResizeObserver === "undefined") {
      window.addEventListener("resize", updateComposerClearance);
      return () =>
        window.removeEventListener("resize", updateComposerClearance);
    }

    const observer = new ResizeObserver(updateComposerClearance);
    observer.observe(composer);
    return () => observer.disconnect();
  }, [composerAreaElement]);

  useEffect(() => {
    shouldFollowMessageBottomRef.current = true;
    lastMessageScrollTopRef.current = 0;
    hasWheelMessageScrollIntentRef.current = false;
    hasPointerMessageScrollIntentRef.current = false;
  }, [visibleCurrentSessionId]);

  // Scroll to bottom when the user is already following the live stream.
  useEffect(() => {
    const container = messagesScrollRef.current;
    if (
      container &&
      welcomeState === "hidden" &&
      (isGenerating || messages.length > 0) &&
      shouldFollowMessageBottomRef.current
    ) {
      container.scrollTop = container.scrollHeight;
      lastMessageScrollTopRef.current = container.scrollTop;
    }
  }, [
    activeImageGeneration,
    composerClearance,
    isGenerating,
    messages,
    visibleCurrentSessionId,
    welcomeState,
  ]);

  // --- Handlers ---

  const showActionError = (message: string) => {
    if (actionErrorTimerRef.current) {
      clearTimeout(actionErrorTimerRef.current);
    }
    setActionError(message);
    actionErrorTimerRef.current = setTimeout(() => {
      actionErrorTimerRef.current = null;
      setActionError(null);
    }, 5000);
  };

  const showServerUnsupportedAction = (action: string) => {
    showActionError(`Server mode does not support ${action} yet.`);
  };

  const toggleServerSearch = async () => {
    const useSearch = !(currentSession?.config?.useSearch ?? false);
    if (!visibleCurrentSessionId) {
      const sessionId = await createServerSession({ config: { useSearch } });
      if (!sessionId) {
        throw new Error("Server conversation could not be created.");
      }
      return;
    }
    const updated = await updateServerSessionConfig(visibleCurrentSessionId, {
      ...(currentSession?.config ?? {}),
      useSearch,
    });
    if (!updated) {
      throw new Error("Search selection could not be saved.");
    }
  };

  const persistServerReasoningSelection = async (
    useReasoning: boolean,
    reasoningEffort: ReasoningEffort,
  ) => {
    setChatConfig({ useReasoning, reasoningEffort });
    if (!visibleCurrentSessionId) {
      const sessionId = await createServerSession({
        config: { useReasoning, reasoningEffort },
      });
      if (!sessionId) {
        throw new Error("Server conversation could not be created.");
      }
      return;
    }
    const updated = await updateServerSessionConfig(visibleCurrentSessionId, {
      ...(currentSession?.config ?? {}),
      useReasoning,
      reasoningEffort,
    });
    if (!updated) {
      throw new Error("Reasoning selection could not be saved.");
    }
  };

  const persistConversationKnowledgeSelection = async (
    collectionIds: string[],
  ) => {
    let sessionId = visibleCurrentSessionId;
    if (!sessionId) {
      sessionId = await createServerSession();
    }
    if (!sessionId) {
      throw new Error("Server conversation could not be created.");
    }
    const normalizedIds = normalizeKnowledgeCollectionIds(collectionIds).slice(
      0,
      MAX_CONVERSATION_KNOWLEDGE_COLLECTIONS,
    );
    const updated = await updateServerSessionConfig(sessionId, {
      selectedKnowledgeCollectionIds: normalizedIds,
    });
    if (!updated) {
      throw new Error("Knowledge selection could not be saved.");
    }
  };

  const syncActiveSessionWithNotice = async (
    sessionId: string,
    logMessage: string,
  ) => {
    try {
      await syncActiveSession(sessionId);
    } catch (error) {
      logChatAppError(logMessage, error);
      showActionError(t("errSaveChanges"));
    }
  };

  const stopActiveGenerationWithFeedback = async () => {
    try {
      await stopActiveGeneration();
    } catch (error) {
      logChatAppError("Failed to persist stopped generation", error);
      showActionError(t("errSaveStopped"));
    }
  };

  const handleStopGeneration = () => {
    if (serverModeEnabled) {
      abortActiveGeneration();
      return;
    }
    void stopActiveGenerationWithFeedback();
  };

  const getEffectiveContextForSession = (
    session?: typeof currentSession | null,
  ) => {
    const { providerId } = parseModelString(selectedModel);
    const provider = providerId
      ? providers.find((item) => item.id === providerId)
      : providers.find((item) => item.enabled);
    const workspace = session?.workspaceId
      ? workspaces.find((item) => item.id === session.workspaceId)
      : null;

    return resolveEffectiveChatContext({
      session,
      workspace,
      systemPrompt: system.systemPrompt,
      enableHtmlVisualPrompt: system.enableHtmlVisualPrompt,
      selectedModel,
      provider,
      modelMetadata,
      customModelMetadata,
      chatConfig: composerChatConfig,
      search: {
        provider: search.provider,
        configs: search.configs,
      },
      rag,
      installedPlugins,
      pluginConfigs,
      activePlugins,
      activePluginIdsOverride: serverModeEnabled ? activePlugins : undefined,
      installedSkills,
      activeSkillIds: serverModeEnabled ? activeSkillIds : [],
      activeSkillIdsOverride: serverModeEnabled ? activeSkillIds : undefined,
    });
  };

  const buildRuntimeProviderConfigForModel = async (model: string) => {
    const { providerId } = parseModelString(model);
    const provider = providerId
      ? providers.find((item) => item.id === providerId)
      : providers.find((item) => item.enabled);

    if (!provider) return undefined;
    return buildProviderRuntimeConfig(provider);
  };

  const processPromptForModel = async (
    session: typeof currentSession | null | undefined,
    text: string,
    attachments: Attachment[],
  ) => {
    const effectiveContext = getEffectiveContextForSession(session);
    const processedData = await processMessageForSending({
      text,
      attachments,
      selectedModel,
      modelMetadata,
      customModelMetadata,
      ragConfig: rag,
      knowledgeCollections,
      workspaceKnowledgeCollectionIds:
        effectiveContext.workspaceKnowledgeCollectionIds,
    });

    const memoryState = useMemoryStore.getState();
    const directMemoryContext =
      !serverModeEnabled &&
      memoryState._hasHydrated &&
      memoryState.settings.enabled &&
      memoryState.settings.searchEnabled
        ? buildDirectMemoryPromptContext({
            memories: memoryState.memories,
            query: text,
            alreadyInjectedMemoryIds:
              session?.memoryContext?.injectedMemoryIds || [],
          })
        : { text: "", injectedMemoryIds: [] };

    return {
      ...processedData,
      finalText: directMemoryContext.text
        ? appendContextToChatInput(
            processedData.finalText,
            directMemoryContext.text,
            {
              separator: "\n\n",
            },
          )
        : processedData.finalText,
      effectiveContext,
      injectedMemoryIds: directMemoryContext.injectedMemoryIds,
    };
  };

  const commitInjectedMemoryContext = (
    sessionId: string,
    session: typeof currentSession | null | undefined,
    injectedMemoryIds: string[],
  ) => {
    if (injectedMemoryIds.length === 0) return;
    const merged = Array.from(
      new Set([
        ...(session?.memoryContext?.injectedMemoryIds || []),
        ...injectedMemoryIds,
      ]),
    );
    updateSessionMemoryContext(sessionId, {
      injectedMemoryIds: merged,
      updatedAt: Date.now(),
    });
  };

  const handleSendServerMessage = async (
    text: string,
    attachments: Attachment[],
  ) => {
    if ((!text.trim() && attachments.length === 0) || isGenerating) return;
    if (!text.trim()) {
      showActionError("Server mode requires message text with attachments.");
      return;
    }
    if (
      attachments.some((attachment) => !isKnowledgeAttachment(attachment)) &&
      !serverFilesEnabled
    ) {
      showActionError("Server file uploads are not enabled.");
      return;
    }

    const generation = beginActiveGeneration();

    try {
      const routedModel = resolveImageGenerationRoute({
        selectedModel,
        availableModels,
        prompt: text,
        hasAttachments: attachments.length > 0,
        recentImageGenerationModel: findRecentImageGenerationModel(
          serverReadState.activeMessages,
        ),
      });
      const routesToImageGeneration = isImageGenerationModel(routedModel);
      setActiveImageGeneration(
        routesToImageGeneration ? { startedAt: Date.now() } : null,
      );
      let targetSessionId = serverReadState.currentSessionId;
      if (!targetSessionId) {
        targetSessionId = await createServerSession();
      }
      if (!targetSessionId) {
        throw new Error("Server conversation could not be created.");
      }

      const serverSessionForTitle =
        useChatStore
          .getState()
          .serverReadState.sessions.find((s) => s.id === targetSessionId) ||
        currentSession;
      const shouldAutoRename =
        system.enableAutoTitle &&
        serverSessionForTitle?.messageCount === 0 &&
        serverSessionForTitle.title === "New Chat";
      const titleSnapshot = createSessionPostGenerationSnapshot(
        serverSessionForTitle,
      );

      const sessionForProcessing =
        useChatStore
          .getState()
          .serverReadState.sessions.find((s) => s.id === targetSessionId) ||
        currentSession;
      const effectiveContext =
        getEffectiveContextForSession(sessionForProcessing);
      const legacyKnowledgeCollectionIds =
        getKnowledgeAttachmentCollectionIds(attachments);
      const sessionKnowledgeBinding = useChatStore
        .getState()
        .serverReadState.sessions.find((s) => s.id === targetSessionId)
        ?.config?.selectedKnowledgeCollectionIds;
      if (
        legacyKnowledgeCollectionIds.length > 0 &&
        sessionKnowledgeBinding === undefined
      ) {
        const migrated = await updateServerSessionConfig(targetSessionId, {
          selectedKnowledgeCollectionIds: legacyKnowledgeCollectionIds.slice(
            0,
            MAX_CONVERSATION_KNOWLEDGE_COLLECTIONS,
          ),
        });
        if (!migrated) {
          throw new Error("Knowledge selection could not be migrated.");
        }
      }
      const uploadableAttachments = attachments.filter(
        (attachment) => !isKnowledgeAttachment(attachment),
      );
      const uploadedAttachments =
        uploadableAttachments.length > 0
          ? await uploadMessageAttachmentsForServer({
              attachments: uploadableAttachments,
              conversationId: targetSessionId,
              signal: generation.controller.signal,
            })
          : [];
      if (!isGenerationRunActive(generation)) return;
      let skillContext = "";
      let pluginContext = "";
      if (!routesToImageGeneration) {
        const skillResolution = await resolveSkillsForMessage({
          message: text,
          selectedModel,
          locale,
          installedSkills,
          activeSkillIds: effectiveContext.activeSkillIds,
          autoSelect: false,
          signal: generation.controller.signal,
        });
        if (!isGenerationRunActive(generation)) return;
        skillContext = skillResolution.context;

        const pluginResolution = await orchestrateServerPlugins({
          message: text,
          selectedModel,
          installedPlugins,
          pluginConfigs,
          activePluginIds: effectiveContext.activePluginIds,
          signal: generation.controller.signal,
        });
        if (!isGenerationRunActive(generation)) return;
        pluginContext = pluginResolution.context;
      }
      const systemInstruction = [
        effectiveContext.systemInstruction,
        skillContext,
        pluginContext,
      ]
        .filter((section): section is string => Boolean(section?.trim()))
        .join("\n\n");
      const runtimeProvider =
        await buildRuntimeProviderConfigForModel(routedModel);
      const latestServerState = useChatStore.getState().serverReadState;
      const parentMessageId =
        latestServerState.currentSessionId === targetSessionId
          ? getActiveMessagePath(latestServerState.activeMessageTree).at(-1)?.id
          : undefined;

      await sendServerMessageAndStream({
        sessionId: targetSessionId,
        content: text,
        parentMessageId,
        attachments: toServerMessageAttachments(uploadedAttachments),
        model: routedModel,
        config: serverSessionChatConfig,
        provider: runtimeProvider,
        systemInstruction,
        signal: generation.controller.signal,
      });

      if (shouldAutoRename) {
        generateServerConversationTitle(targetSessionId, selectedModel)
          .then((newTitle) => {
            const session = useChatStore
              .getState()
              .serverReadState.sessions.find(
                (item) => item.id === targetSessionId,
              );
            if (
              newTitle &&
              session &&
              titleSnapshot &&
              session.id === titleSnapshot.id &&
              titleSnapshot.title === "New Chat" &&
              session.title === "New Chat"
            ) {
              void updateServerSessionTitle(targetSessionId!, newTitle);
            }
          })
          .catch((error) => {
            logChatAppError("Server chat title generation failed:", error);
          });
      }
    } catch (error: any) {
      if (error.name === "AbortError" || generation.controller.signal.aborted) {
        return;
      }
      logChatAppError("Server message generation failed:", error);
      showActionError(
        error instanceof Error ? error.message : "Server message failed.",
      );
    } finally {
      setActiveImageGeneration(null);
      finishActiveGeneration(generation);
    }
  };

  const handleSendMessage = async (text: string, attachments: Attachment[]) => {
    shouldFollowMessageBottomRef.current = true;
    hasWheelMessageScrollIntentRef.current = false;
    hasPointerMessageScrollIntentRef.current = false;
    if (serverModeEnabled) {
      await handleSendServerMessage(text, attachments);
      return;
    }

    if ((!text.trim() && attachments.length === 0) || isGenerating) return;

    let targetSessionId = currentSessionId;

    if (!targetSessionId) {
      targetSessionId = createSession();
    }

    if (!targetSessionId) return;

    // Auto-rename check
    let shouldAutoRename = false;
    let sessionForCheck = sessions.find((s) => s.id === targetSessionId);

    if (!sessionForCheck) {
      sessionForCheck = useChatStore
        .getState()
        .sessions.find((s) => s.id === targetSessionId);
    }

    if (
      system.enableAutoTitle &&
      sessionForCheck &&
      sessionForCheck.messageCount === 0 &&
      sessionForCheck.title === "New Chat"
    ) {
      shouldAutoRename = true;
    }

    const generation = beginActiveGeneration();

    const modelDisplayName = getModelDisplayName(
      selectedModel,
      availableModels,
    );

    let botMsgId: string | null = null;
    let userMessageAdded = false;
    let startTime = Date.now();

    try {
      // Process message and attachments
      const sessionForProcessing =
        useChatStore
          .getState()
          .sessions.find((s) => s.id === targetSessionId) || sessionForCheck;
      const processedData = await processPromptForModel(
        sessionForProcessing,
        text,
        attachments,
      );

      const {
        finalText,
        finalAttachments,
        ragSources,
        userMessage,
        injectedMemoryIds,
      } = processedData;

      if (!isGenerationRunActive(generation)) return;
      commitInjectedMemoryContext(
        targetSessionId,
        sessionForProcessing,
        injectedMemoryIds,
      );

      // Add User Message
      await addMessage(targetSessionId, userMessage);
      userMessageAdded = true;
      if (!isGenerationRunActive(generation)) return;

      // Add Placeholder Bot Message
      const botMsg = createBotMessagePlaceholder(modelDisplayName, ragSources);
      const currentBotMsgId = botMsg.id;
      botMsgId = currentBotMsgId;
      startTime = botMsg.timestamp;

      await addMessage(targetSessionId, botMsg);
      if (!isGenerationRunActive(generation)) return;

      // Get fresh session data
      const historyMessages = useChatStore.getState().activeMessages;
      const freshSession = useChatStore
        .getState()
        .sessions.find((s) => s.id === targetSessionId);

      if (!freshSession) throw new Error("Session not found");
      const effectiveContext = processedData.effectiveContext;

      // Prepare History for LLM (excluding the just-added user message)
      // Filter out the user message we just added since it will be sent separately
      const historyWithoutCurrentUser = historyMessages.filter(
        (m) => m.id !== userMessage.id,
      );

      const { prepareHistoryForLLM, streamChatResponse } =
        await loadChatService();
      const historyForLLM = await prepareHistoryForLLM(
        historyWithoutCurrentUser,
        freshSession.compression,
        selectedModel,
      );
      if (!isGenerationRunActive(generation)) return;

      const effectiveConfig = { ...chatConfig };
      const skillResolution = await resolveSkillsForMessage({
        message: text,
        selectedModel,
        locale,
        installedSkills,
        activeSkillIds: effectiveContext.activeSkillIds,
        autoSelect: skillAutoSelect,
        signal: generation.controller.signal,
      });
      if (!isGenerationRunActive(generation)) return;

      if (skillResolution.invocations.length > 0) {
        updateMessage(targetSessionId, currentBotMsgId, {
          skillInvocations: skillResolution.invocations,
        });
      }

      let latestStreamText = "";
      let latestStreamReasoning = "";

      await streamChatResponse(
        targetSessionId,
        selectedModel,
        historyForLLM,
        finalText, // Injected context included here
        finalAttachments, // Injected files included here (excluding original KB refs)
        effectiveConfig,
        (streamText, streamReasoning, outputBlocks) => {
          if (!isGenerationRunActive(generation)) return;
          latestStreamText = streamText;
          if (streamReasoning !== undefined) {
            latestStreamReasoning = streamReasoning;
          }
          // Update active state in memory only
          updateMessageContent(
            targetSessionId!,
            currentBotMsgId,
            streamText,
            streamReasoning,
            outputBlocks,
          );
        },
        effectiveContext.systemInstruction,
        (isSearching, results) => {
          if (!isGenerationRunActive(generation)) return;
          const currentMessage = useChatStore
            .getState()
            .activeMessages.find((message) => message.id === currentBotMsgId);
          const updates = buildSearchUpdate(
            currentMessage,
            isSearching,
            results,
          );
          updateMessage(targetSessionId!, currentBotMsgId, updates);
        },
        (toolCalls) => {
          if (!isGenerationRunActive(generation)) return;
          updateMessage(targetSessionId!, currentBotMsgId, { toolCalls });
        },
        (images) => {
          if (!isGenerationRunActive(generation)) return;
          const currentActiveMsgs = useChatStore.getState().activeMessages;
          const msg = currentActiveMsgs.find((m) => m.id === currentBotMsgId);
          const currentAttachments = msg?.attachments || [];

          updateMessage(targetSessionId!, currentBotMsgId, {
            attachments: [...currentAttachments, ...images],
          });
        },
        (usage) => {
          if (!isGenerationRunActive(generation)) return;
          const currentMessages = useChatStore.getState().activeMessages;
          handleTokenUsageUpdate(
            usage,
            currentMessages,
            userMessage.id,
            currentBotMsgId,
            targetSessionId!,
            updateMessage,
          );
        },
        generation.controller.signal,
        effectiveContext.activePluginIds,
        skillResolution.context,
        (outputBlocks) => {
          if (!isGenerationRunActive(generation)) return;
          updateMessageContent(
            targetSessionId!,
            currentBotMsgId,
            latestStreamText,
            latestStreamReasoning || undefined,
            outputBlocks,
          );
        },
      );

      if (!isGenerationRunActive(generation)) return;
      const endTime = Date.now();
      updateMessage(targetSessionId, currentBotMsgId, {
        timing: {
          startTime,
          endTime,
          duration: endTime - startTime,
        },
      });

      // --- Post-Generation ---
      // Force sync active messages to storage at end of generation
      await syncActiveSession(targetSessionId);

      const postGenerationState = useChatStore.getState();
      const postGenerationSession = postGenerationState.sessions.find(
        (session) => session.id === targetSessionId,
      );
      const postGenerationSnapshot = createSessionPostGenerationSnapshot(
        postGenerationSession,
      );
      const isTargetSessionActive =
        postGenerationState.currentSessionId === targetSessionId;
      const updatedHistory = isTargetSessionActive
        ? postGenerationState.activeMessages
        : [];
      const completedBotMessage = isTargetSessionActive
        ? updatedHistory.find((message) => message.id === currentBotMsgId)
        : undefined;
      const suggestedQuestionSnapshot = completedBotMessage
        ? {
            id: completedBotMessage.id,
            content: completedBotMessage.content,
          }
        : null;

      if (completedBotMessage) {
        queueMemoryExtraction(targetSessionId, userMessage, {
          id: completedBotMessage.id,
          content: completedBotMessage.content,
        });
      }

      // 1. Follow-up Questions
      if (system.enableRelatedQuestions && updatedHistory.length > 0) {
        loadChatService()
          .then(({ generateRelatedQuestions }) =>
            generateRelatedQuestions(updatedHistory, {
              conversationId: targetSessionId,
            }),
          )
          .then((questions) => {
            const state = useChatStore.getState();
            const currentMessage =
              state.currentSessionId === targetSessionId
                ? state.activeMessages.find(
                    (message) => message.id === currentBotMsgId,
                  )
                : undefined;
            if (
              questions &&
              questions.length > 0 &&
              shouldApplySuggestedQuestions(
                currentMessage,
                suggestedQuestionSnapshot,
              )
            ) {
              setSuggestedQuestions(
                targetSessionId!,
                currentBotMsgId,
                questions,
              );
            }
          })
          .catch((err) => {
            logChatAppError("Related question generation failed:", err);
          });
      }

      // 2. Auto-Rename
      if (shouldAutoRename && updatedHistory.length > 0) {
        loadChatService()
          .then(({ generateChatTitle }) => generateChatTitle(updatedHistory))
          .then((newTitle) => {
            const currentSession = useChatStore
              .getState()
              .sessions.find((session) => session.id === targetSessionId);
            if (
              newTitle &&
              shouldApplyGeneratedTitle(currentSession, postGenerationSnapshot)
            ) {
              updateSessionTitle(targetSessionId!, newTitle);
            }
          })
          .catch((err) => {
            logChatAppError("Chat title generation failed:", err);
          });
      }

      // 3. Auto-Compress
      if (
        system.enableAutoCompression &&
        postGenerationSession &&
        updatedHistory.length > 0
      ) {
        loadChatService()
          .then(({ performBackgroundCompression }) =>
            performBackgroundCompression(
              updatedHistory,
              postGenerationSession.compression,
              selectedModel,
            ),
          )
          .then((newCompression) => {
            const currentSession = useChatStore
              .getState()
              .sessions.find((session) => session.id === targetSessionId);
            if (
              newCompression &&
              shouldApplyCompressionUpdate(
                currentSession,
                postGenerationSnapshot,
              )
            ) {
              updateSessionCompression(targetSessionId!, newCompression);
            }
          })
          .catch((err) => {
            logChatAppError("Context compression failed:", err);
          });
      }
    } catch (error: any) {
      if (error.name === "AbortError" || generation.controller.signal.aborted) {
        return;
      } else {
        logChatAppError("Generating content failed:", error);
        let errorMessage =
          error instanceof Error ? error.message : "An unknown error occurred.";
        if (typeof error === "object" && error !== null && "message" in error) {
          errorMessage = error.message;
        } else if (typeof error === "string") {
          errorMessage = error;
        }

        if (!userMessageAdded) {
          const fallbackUserMessage: Message = {
            id: uuidv7(),
            role: "user",
            content: text,
            timestamp: Date.now(),
            attachments,
          };
          await addMessage(targetSessionId, fallbackUserMessage);
          userMessageAdded = true;
        }

        if (botMsgId) {
          updateMessage(targetSessionId, botMsgId, {
            generationError: {
              message: errorMessage,
              recoverable: true,
            },
            timing: {
              startTime,
              endTime: Date.now(),
              duration: Date.now() - startTime,
            },
          });
        } else {
          const errorBotMsg = createBotMessagePlaceholder(modelDisplayName, []);
          errorBotMsg.content = "";
          errorBotMsg.generationError = {
            message: errorMessage,
            recoverable: true,
          };
          errorBotMsg.timing = {
            startTime,
            endTime: Date.now(),
            duration: Date.now() - startTime,
          };
          await addMessage(targetSessionId, errorBotMsg);
        }

        await syncActiveSession(targetSessionId); // Sync error message too
      }
    } finally {
      finishActiveGeneration(generation);
    }
  };

  const generateModelResponseBranch = async (
    messageId: string,
    {
      errorMessage,
      logPrefix,
    }: {
      errorMessage: string;
      logPrefix: string;
    },
  ) => {
    if (isGenerating || !currentSessionId) return;

    const sessionMessages = activeMessages;
    if (!sessionMessages) return;

    const msgIndex = sessionMessages.findIndex((m) => m.id === messageId);
    if (msgIndex === -1) return;

    const historyContext = sessionMessages.slice(0, msgIndex);

    const lastUserMsg = historyContext[historyContext.length - 1];
    if (!lastUserMsg || lastUserMsg.role !== "user") {
      logChatAppError(`${logPrefix}: preceding message is not a user message.`);
      showActionError(errorMessage);
      return;
    }

    const promptText = lastUserMsg.content;
    const promptAttachments = lastUserMsg.attachments || [];

    const currentModelInfo = availableModels.find(
      (m) => m.name === selectedModel,
    );
    const modelDisplayName = currentModelInfo?.displayName || selectedModel;

    const branchMessageId = addMessageVersion(
      currentSessionId,
      messageId,
      modelDisplayName,
    );
    if (!branchMessageId) {
      showActionError(errorMessage);
      return;
    }
    const generation = beginActiveGeneration();
    const startTime = Date.now();

    try {
      const sessionMeta = getCurrentSession();
      const {
        finalText,
        finalAttachments,
        ragSources,
        effectiveContext,
        injectedMemoryIds,
      } = await processPromptForModel(
        sessionMeta,
        promptText,
        promptAttachments,
      );
      commitInjectedMemoryContext(
        currentSessionId,
        sessionMeta,
        injectedMemoryIds,
      );
      const skillResolution = await resolveSkillsForMessage({
        message: promptText,
        selectedModel,
        locale,
        installedSkills,
        activeSkillIds: effectiveContext.activeSkillIds,
        autoSelect: skillAutoSelect,
        signal: generation.controller.signal,
      });
      if (ragSources.length > 0) {
        updateMessage(currentSessionId, branchMessageId, {
          ragSources,
        });
      }
      if (skillResolution.invocations.length > 0) {
        updateMessage(currentSessionId, branchMessageId, {
          skillInvocations: skillResolution.invocations,
        });
      }
      const historyBeforeUser = historyContext.slice(0, -1);
      const { prepareHistoryForLLM, streamChatResponse } =
        await loadChatService();
      const historyForApi = await prepareHistoryForLLM(
        historyBeforeUser,
        sessionMeta?.compression,
        selectedModel,
      );
      if (!isGenerationRunActive(generation)) return;

      let latestStreamText = "";
      let latestStreamReasoning = "";

      await streamChatResponse(
        currentSessionId,
        selectedModel,
        historyForApi, // Don't include lastUserMsg here, it's sent as newMessage
        finalText,
        finalAttachments,
        chatConfig,
        (streamText, streamReasoning, outputBlocks) => {
          if (!isGenerationRunActive(generation)) return;
          latestStreamText = streamText;
          if (streamReasoning !== undefined) {
            latestStreamReasoning = streamReasoning;
          }
          updateMessageContent(
            currentSessionId,
            branchMessageId,
            streamText,
            streamReasoning,
            outputBlocks,
          );
        },
        effectiveContext.systemInstruction,
        (isSearching, results) => {
          if (!isGenerationRunActive(generation)) return;
          const currentMessage = useChatStore
            .getState()
            .activeMessages.find((message) => message.id === branchMessageId);
          const updates = buildSearchUpdate(
            currentMessage,
            isSearching,
            results,
          );
          updateMessage(currentSessionId, branchMessageId, updates);
        },
        (toolCalls) => {
          if (!isGenerationRunActive(generation)) return;
          updateMessage(currentSessionId, branchMessageId, { toolCalls });
        },
        (images) => {
          if (!isGenerationRunActive(generation)) return;
          const currentActiveMsgs = useChatStore.getState().activeMessages;
          const msg = currentActiveMsgs.find((m) => m.id === branchMessageId);
          const currentAttachments = msg?.attachments || [];
          updateMessage(currentSessionId, branchMessageId, {
            attachments: [...currentAttachments, ...images],
          });
        },
        (usage) => {
          if (!isGenerationRunActive(generation)) return;
          const currentMessages = useChatStore.getState().activeMessages;
          handleTokenUsageUpdate(
            usage,
            currentMessages,
            lastUserMsg.id,
            branchMessageId,
            currentSessionId,
            updateMessage,
          );
        },
        generation.controller.signal,
        effectiveContext.activePluginIds,
        skillResolution.context,
        (outputBlocks) => {
          if (!isGenerationRunActive(generation)) return;
          updateMessageContent(
            currentSessionId,
            branchMessageId,
            latestStreamText,
            latestStreamReasoning || undefined,
            outputBlocks,
          );
        },
      );

      if (!isGenerationRunActive(generation)) return;
      const endTime = Date.now();
      updateMessage(currentSessionId, branchMessageId, {
        timing: {
          startTime,
          endTime,
          duration: endTime - startTime,
        },
      });

      await syncActiveSession(currentSessionId);
      const completedBranchMessage = useChatStore
        .getState()
        .activeMessages.find((message) => message.id === branchMessageId);
      if (completedBranchMessage) {
        queueMemoryExtraction(currentSessionId, lastUserMsg, {
          id: completedBranchMessage.id,
          content: completedBranchMessage.content,
        });
      }
    } catch (error: any) {
      if (error.name === "AbortError" || generation.controller.signal.aborted) {
        return;
      } else {
        logChatAppError(`${logPrefix} generation failed:`, error);
        const errorMessage =
          error instanceof Error ? error.message : "An unknown error occurred.";
        updateMessage(currentSessionId, branchMessageId, {
          generationError: {
            message: errorMessage,
            recoverable: true,
          },
          timing: {
            startTime,
            endTime: Date.now(),
            duration: Date.now() - startTime,
          },
        });
        await syncActiveSessionWithNotice(
          currentSessionId,
          `Failed to persist ${logPrefix.toLowerCase()} error message`,
        );
      }
    } finally {
      finishActiveGeneration(generation);
    }
  };

  const handleRegenerate = async (messageId: string) => {
    if (serverModeEnabled) {
      const sessionId = visibleCurrentSessionId;
      if (!sessionId || isGenerating) return;
      const targetNode = visibleActiveMessageTree.nodesById[messageId];
      const userMessageId = targetNode?.parentMessageId;
      const userMessage = userMessageId
        ? visibleActiveMessageTree.nodesById[userMessageId]?.message
        : undefined;
      if (!userMessage || userMessage.role !== "user") {
        showActionError(t("errRegenerate"));
        return;
      }

      const generation = beginActiveGeneration();
      try {
        const sessionForProcessing =
          useChatStore
            .getState()
            .serverReadState.sessions.find((s) => s.id === sessionId) ||
          currentSession;
        const effectiveContext =
          getEffectiveContextForSession(sessionForProcessing);
        const skillResolution = await resolveSkillsForMessage({
          message: userMessage.content,
          selectedModel,
          locale,
          installedSkills,
          activeSkillIds: effectiveContext.activeSkillIds,
          autoSelect: false,
          signal: generation.controller.signal,
        });
        if (!isGenerationRunActive(generation)) return;

        const pluginResolution = await orchestrateServerPlugins({
          message: userMessage.content,
          selectedModel,
          installedPlugins,
          pluginConfigs,
          activePluginIds: effectiveContext.activePluginIds,
          signal: generation.controller.signal,
        });
        if (!isGenerationRunActive(generation)) return;

        const systemInstruction = [
          effectiveContext.systemInstruction,
          skillResolution.context,
          pluginResolution.context,
        ]
          .filter((section): section is string => Boolean(section?.trim()))
          .join("\n\n");
        const runtimeProvider =
          await buildRuntimeProviderConfigForModel(selectedModel);
        if (!isGenerationRunActive(generation)) return;

        await regenerateServerAssistantMessage({
          sessionId,
          assistantMessageId: messageId,
          model: selectedModel,
          provider: runtimeProvider,
          config: serverSessionChatConfig,
          systemInstruction,
          signal: generation.controller.signal,
        });
      } catch (error: any) {
        if (
          error.name === "AbortError" ||
          generation.controller.signal.aborted
        ) {
          return;
        }
        logChatAppError("Server regeneration failed:", error);
        showActionError(
          error instanceof Error ? error.message : t("errRegenerate"),
        );
      } finally {
        finishActiveGeneration(generation);
      }
      return;
    }
    await generateModelResponseBranch(messageId, {
      errorMessage: t("errRegenerate"),
      logPrefix: "Regeneration",
    });
  };

  const handleVersionChange = (msgId: string, direction: "prev" | "next") => {
    if (serverModeEnabled) {
      if (visibleCurrentSessionId) {
        switchServerMessageVersion(visibleCurrentSessionId, msgId, direction);
      }
      return;
    }
    if (currentSessionId) {
      switchMessageVersion(currentSessionId, msgId, direction);
    }
  };

  const handleAssistantSelect = async (agent: LobeAgent) => {
    const requestId = assistantSelectRequestRef.current + 1;
    assistantSelectRequestRef.current = requestId;

    if (isGenerating) {
      if (serverModeEnabled) {
        abortActiveGeneration();
      } else {
        void stopActiveGenerationWithFeedback();
      }
    }

    if (viewMode === "assistants") {
      navigateToPanel("chat");
    }

    let instruction = agent.meta.systemRole;

    if (!instruction && !agent.isCustom) {
      try {
        const detail = await getAgentDetail(agent.identifier, locale);
        if (requestId !== assistantSelectRequestRef.current) return;
        instruction = detail.config?.systemRole;
      } catch (e) {
        if (requestId !== assistantSelectRequestRef.current) return;
        logChatAppError("Failed to fetch agent details for instruction", e);
      }
    }

    if (requestId !== assistantSelectRequestRef.current) return;

    if (!instruction) {
      instruction = `You are ${agent.meta.title}. ${agent.meta.description}`;
    }

    if (serverModeEnabled) {
      try {
        if (
          visibleCurrentSessionId &&
          currentSession &&
          currentSession.messageCount === 0 &&
          currentSession.title === "New Chat"
        ) {
          await updateServerSessionInstruction(
            visibleCurrentSessionId,
            instruction,
          );
          await updateServerSessionTitle(
            visibleCurrentSessionId,
            agent.meta.title,
          );
          return;
        }

        await createServerSession({
          systemInstruction: instruction,
          title: agent.meta.title,
        });
      } catch (error) {
        logChatAppError("Failed to apply server assistant preset", error);
        showActionError(t("errSaveChanges"));
      }
      return;
    }

    if (currentSessionId) {
      const session = getCurrentSession();
      if (
        session &&
        session.messageCount === 0 &&
        session.title === "New Chat"
      ) {
        updateSessionInstruction(currentSessionId, instruction);
        updateSessionTitle(currentSessionId, agent.meta.title);
        return;
      }
    }

    createSession(instruction, agent.meta.title);
  };

  const handleEditMessage = async (msgId: string, newContent: string) => {
    if (serverModeEnabled) {
      const sessionId = visibleCurrentSessionId;
      if (!sessionId) return;
      try {
        await updateServerMessageContent(sessionId, msgId, newContent);
      } catch (error) {
        logChatAppError("Failed to edit server message", error);
        showActionError(t("errSaveChanges"));
      }
      return;
    }
    if (currentSessionId) {
      updateMessageContent(currentSessionId, msgId, newContent);
      void syncActiveSessionWithNotice(
        currentSessionId,
        "Failed to persist edited message",
      );
    }
  };

  const handleSubmitUserMessageEdit = async (
    msgId: string,
    newContent: string,
  ) => {
    if (serverModeEnabled) {
      const sessionId = visibleCurrentSessionId;
      if (!sessionId || isGenerating || !newContent.trim()) return;

      const sessionMessages = visibleActiveMessages;
      const sourceMessage = sessionMessages.find(
        (message) => message.id === msgId,
      );
      if (!sourceMessage || sourceMessage.role !== "user") {
        showActionError(t("errEditUserMessage"));
        return;
      }
      if (newContent === sourceMessage.content) return;
      if (
        (sourceMessage.attachments ?? []).some(
          (attachment) => !isKnowledgeAttachment(attachment),
        ) &&
        !serverFilesEnabled
      ) {
        showActionError("Server file uploads are not enabled.");
        return;
      }

      const generation = beginActiveGeneration();
      try {
        const sessionForProcessing =
          useChatStore
            .getState()
            .serverReadState.sessions.find((s) => s.id === sessionId) ||
          currentSession;
        const effectiveContext =
          getEffectiveContextForSession(sessionForProcessing);
        const sessionKnowledgeBinding =
          sessionForProcessing?.config?.selectedKnowledgeCollectionIds;
        const legacyKnowledgeCollectionIds =
          getLegacyKnowledgeSelectionIdsForMigration(sourceMessage);
        if (
          sessionKnowledgeBinding === undefined &&
          legacyKnowledgeCollectionIds.length > 0
        ) {
          const migrated = await updateServerSessionConfig(sessionId, {
            selectedKnowledgeCollectionIds: legacyKnowledgeCollectionIds,
          });
          if (!migrated) {
            throw new Error("Knowledge selection could not be migrated.");
          }
        }
        const uploadableAttachments = (sourceMessage.attachments ?? []).filter(
          (attachment) => !isKnowledgeAttachment(attachment),
        );
        const uploadedAttachments =
          uploadableAttachments.length > 0
            ? await uploadMessageAttachmentsForServer({
                attachments: uploadableAttachments,
                conversationId: sessionId,
                signal: generation.controller.signal,
              })
            : [];
        if (!isGenerationRunActive(generation)) return;

        const skillResolution = await resolveSkillsForMessage({
          message: newContent,
          selectedModel,
          locale,
          installedSkills,
          activeSkillIds: effectiveContext.activeSkillIds,
          autoSelect: false,
          signal: generation.controller.signal,
        });
        if (!isGenerationRunActive(generation)) return;

        const pluginResolution = await orchestrateServerPlugins({
          message: newContent,
          selectedModel,
          installedPlugins,
          pluginConfigs,
          activePluginIds: effectiveContext.activePluginIds,
          signal: generation.controller.signal,
        });
        if (!isGenerationRunActive(generation)) return;

        const sourceParentId =
          visibleActiveMessageTree.nodesById[msgId]?.parentMessageId ??
          sourceMessage.parentMessageId ??
          null;
        const systemInstruction = [
          effectiveContext.systemInstruction,
          skillResolution.context,
          pluginResolution.context,
        ]
          .filter((section): section is string => Boolean(section?.trim()))
          .join("\n\n");
        const runtimeProvider =
          await buildRuntimeProviderConfigForModel(selectedModel);
        if (!isGenerationRunActive(generation)) return;

        await sendServerMessageAndStream({
          sessionId,
          content: newContent,
          parentMessageId: sourceParentId ?? undefined,
          attachments: toServerMessageAttachments(uploadedAttachments),
          model: selectedModel,
          config: serverSessionChatConfig,
          provider: runtimeProvider,
          systemInstruction,
          metadata: {
            branchSourceMessageId: msgId,
            treeParentMessageId: sourceParentId,
          },
          signal: generation.controller.signal,
        });
      } catch (error: any) {
        if (
          error.name === "AbortError" ||
          generation.controller.signal.aborted
        ) {
          return;
        }
        logChatAppError("Server user message edit branch failed:", error);
        showActionError(
          error instanceof Error ? error.message : t("errEditUserMessage"),
        );
      } finally {
        finishActiveGeneration(generation);
      }
      return;
    }

    const sessionId = currentSessionId;
    if (!sessionId || isGenerating || !newContent.trim()) return;

    const sessionMessages = activeMessages;
    const msgIndex = sessionMessages.findIndex(
      (message) => message.id === msgId,
    );
    const sourceMessage = sessionMessages[msgIndex];
    if (!sourceMessage || sourceMessage.role !== "user") {
      showActionError(t("errEditUserMessage"));
      return;
    }
    if (newContent === sourceMessage.content) return;

    const generation = beginActiveGeneration();
    let modelMessageId: string | null = null;
    let editedUserMessageId: string | null = null;
    let startTime = Date.now();

    try {
      const sessionMeta = getCurrentSession();
      const {
        finalText,
        finalAttachments,
        ragSources,
        userMessage,
        effectiveContext,
        injectedMemoryIds,
      } = await processPromptForModel(
        sessionMeta,
        newContent,
        sourceMessage.attachments || [],
      );
      if (!isGenerationRunActive(generation)) return;
      commitInjectedMemoryContext(sessionId, sessionMeta, injectedMemoryIds);

      const skillResolution = await resolveSkillsForMessage({
        message: newContent,
        selectedModel,
        locale,
        installedSkills,
        activeSkillIds: effectiveContext.activeSkillIds,
        autoSelect: skillAutoSelect,
        signal: generation.controller.signal,
      });
      if (!isGenerationRunActive(generation)) return;

      const modelDisplayName = getModelDisplayName(
        selectedModel,
        availableModels,
      );
      const modelPlaceholder = createBotMessagePlaceholder(
        modelDisplayName,
        ragSources,
      );
      startTime = modelPlaceholder.timestamp;

      const branchIds = createEditedUserMessageBranch(
        sessionId,
        msgId,
        userMessage,
        modelPlaceholder,
      );
      if (!branchIds) {
        showActionError(t("errEditUserMessage"));
        return;
      }

      editedUserMessageId = branchIds.userMessageId;
      modelMessageId = branchIds.modelMessageId;
      if (skillResolution.invocations.length > 0) {
        updateMessage(sessionId, modelMessageId, {
          skillInvocations: skillResolution.invocations,
        });
      }

      const historyBeforeUser = sessionMessages.slice(0, msgIndex);
      const { prepareHistoryForLLM, streamChatResponse } =
        await loadChatService();
      const historyForApi = await prepareHistoryForLLM(
        historyBeforeUser,
        sessionMeta?.compression,
        selectedModel,
      );
      if (!isGenerationRunActive(generation)) return;

      let latestStreamText = "";
      let latestStreamReasoning = "";

      await streamChatResponse(
        sessionId,
        selectedModel,
        historyForApi,
        finalText,
        finalAttachments,
        chatConfig,
        (streamText, streamReasoning, outputBlocks) => {
          if (!isGenerationRunActive(generation) || !modelMessageId) return;
          latestStreamText = streamText;
          if (streamReasoning !== undefined) {
            latestStreamReasoning = streamReasoning;
          }
          updateMessageContent(
            sessionId,
            modelMessageId,
            streamText,
            streamReasoning,
            outputBlocks,
          );
        },
        effectiveContext.systemInstruction,
        (isSearching, results) => {
          if (!isGenerationRunActive(generation) || !modelMessageId) return;
          const currentMessage = useChatStore
            .getState()
            .activeMessages.find((message) => message.id === modelMessageId);
          const updates = buildSearchUpdate(
            currentMessage,
            isSearching,
            results,
          );
          updateMessage(sessionId, modelMessageId, updates);
        },
        (toolCalls) => {
          if (!isGenerationRunActive(generation) || !modelMessageId) return;
          updateMessage(sessionId, modelMessageId, { toolCalls });
        },
        (images) => {
          if (!isGenerationRunActive(generation) || !modelMessageId) return;
          const currentActiveMsgs = useChatStore.getState().activeMessages;
          const msg = currentActiveMsgs.find(
            (message) => message.id === modelMessageId,
          );
          const currentAttachments = msg?.attachments || [];

          updateMessage(sessionId, modelMessageId, {
            attachments: [...currentAttachments, ...images],
          });
        },
        (usage) => {
          if (
            !isGenerationRunActive(generation) ||
            !modelMessageId ||
            !editedUserMessageId
          ) {
            return;
          }
          const currentMessages = useChatStore.getState().activeMessages;
          handleTokenUsageUpdate(
            usage,
            currentMessages,
            editedUserMessageId,
            modelMessageId,
            sessionId,
            updateMessage,
          );
        },
        generation.controller.signal,
        effectiveContext.activePluginIds,
        skillResolution.context,
        (outputBlocks) => {
          if (!isGenerationRunActive(generation) || !modelMessageId) return;
          updateMessageContent(
            sessionId,
            modelMessageId,
            latestStreamText,
            latestStreamReasoning || undefined,
            outputBlocks,
          );
        },
      );

      if (!isGenerationRunActive(generation) || !modelMessageId) return;
      const endTime = Date.now();
      updateMessage(sessionId, modelMessageId, {
        timing: {
          startTime,
          endTime,
          duration: endTime - startTime,
        },
      });

      await syncActiveSession(sessionId);
      const completedModelMessage = useChatStore
        .getState()
        .activeMessages.find((message) => message.id === modelMessageId);
      if (completedModelMessage && editedUserMessageId) {
        queueMemoryExtraction(
          sessionId,
          { id: editedUserMessageId, content: newContent },
          {
            id: completedModelMessage.id,
            content: completedModelMessage.content,
          },
        );
      }
    } catch (error: any) {
      if (error.name === "AbortError" || generation.controller.signal.aborted) {
        return;
      }

      logChatAppError("User message edit branch generation failed:", error);
      const errorMessage =
        error instanceof Error ? error.message : "An unknown error occurred.";
      if (modelMessageId) {
        updateMessage(sessionId, modelMessageId, {
          generationError: {
            message: errorMessage,
            recoverable: true,
          },
          timing: {
            startTime,
            endTime: Date.now(),
            duration: Date.now() - startTime,
          },
        });
        await syncActiveSessionWithNotice(
          sessionId,
          "Failed to persist edited user message branch error",
        );
      } else {
        showActionError(t("errEditUserMessage"));
      }
    } finally {
      finishActiveGeneration(generation);
    }
  };

  const handleDeleteMessage = async (msgId: string) => {
    if (serverModeEnabled) {
      const sessionId = visibleCurrentSessionId;
      if (!sessionId) return;
      try {
        await deleteServerMessage(sessionId, msgId);
      } catch (error) {
        logChatAppError("Failed to delete server message", error);
        showActionError(t("errDeleteMessage"));
      }
      return;
    }

    const sessionId = currentSessionId;
    if (!sessionId) return;

    try {
      await deleteMessage(sessionId, msgId);
    } catch (error) {
      logChatAppError("Failed to delete message", error);
      showActionError(t("errDeleteMessage"));
    }
  };

  const handleDeleteSession = async (sessionId: string) => {
    if (serverModeEnabled) {
      try {
        if (
          shouldAbortActiveGenerationForSessionDelete({
            currentSessionId: visibleCurrentSessionId,
            deletingSessionId: sessionId,
            isGenerating,
          })
        ) {
          abortActiveGeneration();
        }
        await deleteServerSession(sessionId);
      } catch (error) {
        logChatAppError("Failed to delete server session", error);
        showActionError(t("errDeleteChat"));
      }
      return;
    }

    try {
      if (
        shouldAbortActiveGenerationForSessionDelete({
          currentSessionId,
          deletingSessionId: sessionId,
          isGenerating,
        })
      ) {
        await stopActiveGeneration();
      }

      await deleteSession(sessionId);
    } catch (error) {
      logChatAppError("Failed to delete session", error);
      showActionError(t("errDeleteChat"));
    }
  };

  const handleDuplicateSession = async (sessionId: string) => {
    if (serverModeEnabled) {
      try {
        await duplicateServerSession(sessionId);
      } catch (error) {
        logChatAppError("Failed to duplicate server session", error);
        showActionError(t("errDuplicateChat"));
      }
      return;
    }

    try {
      await duplicateSession(sessionId);
    } catch (error) {
      logChatAppError("Failed to duplicate session", error);
      showActionError(t("errDuplicateChat"));
    }
  };

  const handleRetractMessage = async (msg: Message) => {
    if (serverModeEnabled) {
      const sessionId = visibleCurrentSessionId;
      if (!sessionId) return;
      try {
        await retractServerMessage(sessionId, msg.id);
        if (messageInputRef.current) {
          messageInputRef.current.setValue(msg.content);
          messageInputRef.current.focus();
        }
      } catch (error) {
        logChatAppError("Failed to retract server message", error);
        showActionError(t("errRetractMessage"));
      }
      return;
    }

    const sessionId = currentSessionId;
    if (!sessionId) return;

    try {
      await deleteMessageAndSubsequent(sessionId, msg.id);

      if (messageInputRef.current) {
        messageInputRef.current.setValue(msg.content);
        messageInputRef.current.focus();
      }
    } catch (error) {
      logChatAppError("Failed to retract message", error);
      showActionError(t("errRetractMessage"));
    }
  };

  const handleSmartRename = async (sessionId: string) => {
    if (serverModeEnabled) {
      const snapshot = createSessionPostGenerationSnapshot(
        useChatStore
          .getState()
          .serverReadState.sessions.find((session) => session.id === sessionId),
      );
      if (!snapshot) return;
      try {
        const newTitle = await generateServerConversationTitle(
          sessionId,
          selectedModel,
        );
        const currentSession = useChatStore
          .getState()
          .serverReadState.sessions.find((session) => session.id === sessionId);
        if (newTitle && shouldApplyRequestedTitle(currentSession, snapshot)) {
          await updateServerSessionTitle(sessionId, newTitle);
        }
      } catch (error) {
        logChatAppError("Server smart rename failed:", error);
        showActionError(t("errRenameChat"));
      }
      return;
    }

    const snapshot = createSessionPostGenerationSnapshot(
      useChatStore
        .getState()
        .sessions.find((session) => session.id === sessionId),
    );
    if (!snapshot) return;

    // Need messages for rename, if active session, use state, else load
    let msgs: Message[];
    try {
      const state = useChatStore.getState();
      if (state.currentSessionId === sessionId) {
        msgs = state.activeMessages;
      } else {
        const storedMessages = await appDb.getItem<
          Message[] | SessionMessageTree
        >(`session_messages_${sessionId}`);
        msgs = getActiveMessagePath(
          normalizeSessionMessageTree(storedMessages),
        );
      }
    } catch (error) {
      logChatAppError("Failed to load messages for smart rename", error);
      showActionError(t("errRenameChat"));
      return;
    }

    if (msgs.length === 0) return;

    const { generateChatTitle } = await loadChatService();
    const newTitle = await generateChatTitle(msgs);
    const currentSession = useChatStore
      .getState()
      .sessions.find((session) => session.id === sessionId);
    if (shouldApplyRequestedTitle(currentSession, snapshot)) {
      updateSessionTitle(sessionId, newTitle);
    }
  };

  const handleNewChat = () => {
    if (serverModeEnabled) {
      if (isGenerating) {
        abortActiveGeneration();
      }
      createServerSession()
        .then(() => navigateToPanel("chat"))
        .catch((error) => {
          logChatAppError("Failed to create server chat", error);
          showActionError("Failed to create server chat.");
        });
      return;
    }

    if (isGenerating) {
      void stopActiveGenerationWithFeedback();
    }

    createSession();
    navigateToPanel("chat");
  };

  const handleSuggestionClick = (question: string) => {
    handleSendMessage(question, []);
  };

  // --- Render ---

  return (
    <div className="relative flex h-dvh w-full overflow-hidden bg-background font-sans text-foreground transition-colors duration-300">
      <a className="skip-link" href="#main-chat">
        {t("skipToChat")}
      </a>
      <ImagePreview />

      {/* Mobile Sidebar Overlay Mask */}
      {isSidebarOpen && (
        <div
          className="fixed inset-0 z-30 bg-black/10 dark:bg-black/50 backdrop-blur-[1px] md:hidden transition-opacity duration-300"
          onClick={() => setIsSidebarOpen(false)}
          aria-hidden="true"
        />
      )}

      {/* Sidebar */}
      <Sidebar
        sessions={visibleSessions}
        currentSessionId={visibleCurrentSessionId}
        onSelectSession={(id) => {
          if (serverModeEnabled) {
            if (isGenerating) {
              abortActiveGeneration();
            }
            void selectServerSession(id);
          } else {
            if (isGenerating) {
              void stopActiveGenerationWithFeedback();
            }
            selectSession(id);
          }
          navigateToPanel("chat");
        }}
        onNewChat={handleNewChat}
        onDeleteSession={handleDeleteSession}
        onRenameSession={(id, title) => {
          if (serverModeEnabled) {
            void updateServerSessionTitle(id, title).catch((error) => {
              logChatAppError("Failed to rename server session", error);
              showActionError(t("errRenameChat"));
            });
            return;
          }
          updateSessionTitle(id, title);
        }}
        onTogglePin={(id) => {
          if (serverModeEnabled) {
            void toggleServerSessionPin(id).catch((error) => {
              logChatAppError("Failed to pin server session", error);
              showActionError(t("errSaveChanges"));
            });
            return;
          }
          toggleSessionPin(id);
        }}
        onDuplicate={handleDuplicateSession}
        onSmartRename={handleSmartRename}
        isOpen={isSidebarOpen}
        toggleSidebar={() => setIsSidebarOpen((open) => !open)}
        isModal={isMobileSidebarModalOpen}
        onRequestClose={() => setIsSidebarOpen(false)}
        onOpenPluginMarket={() => navigateToPanel("plugins")}
        isPluginMarketOpen={viewMode === "plugins"}
        onOpenSkillMarket={() => navigateToPanel("skills")}
        isSkillMarketOpen={viewMode === "skills"}
        onOpenAssistantHub={() => navigateToPanel("assistants")}
        isAssistantHubOpen={viewMode === "assistants"}
        onOpenKnowledgeBase={() => navigateToPanel("knowledge")}
        isKnowledgeBaseOpen={viewMode === "knowledge"}
        onOpenSettings={() => navigateToPanel("settings")}
        isSettingsOpen={viewMode === "settings"}
        onLogoClick={() => navigateToPanel("chat")}
      />

      {/* Main Chat Area */}
      <main
        {...mainInertProps}
        id="main-chat"
        tabIndex={-1}
        className="flex-1 flex flex-col h-full relative z-0 min-w-0 overflow-hidden"
      >
        {actionError && (
          <div
            role="alert"
            className="absolute top-16 left-4 right-4 z-30 pointer-events-none"
          >
            <div className="mx-auto max-w-3xl rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 shadow-sm dark:border-red-900/60 dark:bg-red-950/90 dark:text-red-100">
              {actionError}
            </div>
          </div>
        )}
        {viewMode === "plugins" ? (
          <PluginMarket onClose={() => navigateToPanel("chat")} />
        ) : viewMode === "skills" ? (
          <SkillMarket onClose={() => navigateToPanel("chat")} />
        ) : viewMode === "assistants" ? (
          <AssistantHub
            onClose={() => navigateToPanel("chat")}
            onSelect={handleAssistantSelect}
          />
        ) : viewMode === "knowledge" ? (
          <KnowledgeBase onClose={() => navigateToPanel("chat")} />
        ) : viewMode === "settings" ? (
          <SettingsPage
            activeTab={settingsTab}
            onTabChange={handleSettingsTabChange}
            onClose={() => navigateToPanel("chat")}
          />
        ) : (
          <>
            {/* Header */}
            <header className="relative z-10 flex h-14 items-center justify-between px-4 md:px-6">
              <div className="flex min-w-10 items-center">
                <Tooltip
                  content={isSidebarOpen ? t("closeSidebar") : t("openSidebar")}
                  position="right"
                  className="md:hidden"
                >
                  <button
                    type="button"
                    aria-label={
                      isSidebarOpen
                        ? t("closeSidebarAria")
                        : t("openSidebarAria")
                    }
                    onClick={() => setIsSidebarOpen((open) => !open)}
                    className="p-2 -ml-2 rounded-lg text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
                  >
                    {isSidebarOpen ? (
                      <PanelLeftClose size={16} aria-hidden="true" />
                    ) : (
                      <PanelLeftOpen size={16} aria-hidden="true" />
                    )}
                  </button>
                </Tooltip>
              </div>

              {shouldShowChatTitleBar && (
                <div className="absolute left-1/2 top-1/2 max-w-[50%] -translate-x-1/2 -translate-y-1/2 truncate text-center font-bold text-foreground">
                  {currentSession?.title || t("newChat")}
                </div>
              )}

              <div className="flex items-center justify-end min-w-10">
                {!isSidebarOpen && (
                  <Tooltip content={t("newChat")} position="left">
                    <button
                      type="button"
                      aria-label={t("newChatAria")}
                      onClick={handleNewChat}
                      className="p-2 -mr-2 rounded-lg text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
                    >
                      <MessageSquarePlus size={16} />
                    </button>
                  </Tooltip>
                )}
              </div>
            </header>

            {/* Content */}
            <div
              ref={messagesScrollRef}
              onScroll={updateMessageScrollFollow}
              onWheel={handleMessageWheel}
              onPointerDown={() => {
                hasPointerMessageScrollIntentRef.current = true;
              }}
              onPointerUp={() => {
                hasPointerMessageScrollIntentRef.current = false;
              }}
              onPointerCancel={() => {
                hasPointerMessageScrollIntentRef.current = false;
              }}
              style={
                welcomeState === "hidden"
                  ? { paddingBottom: `${composerClearance}px` }
                  : undefined
              }
              className="flex-1 px-4 md:px-8 pt-4 md:pt-6 pb-[calc(8rem+env(safe-area-inset-bottom))] relative scrollbar-overlay"
            >
              <div className="w-full max-w-3xl mx-auto min-h-full flex flex-col">
                {/* Assistant / System Instruction Header */}
                {currentSession &&
                  (messages.length > 0 ||
                    !!currentSession.systemInstruction) && (
                    <AssistantHeader
                      instruction={currentSession.systemInstruction || ""}
                      onUpdate={(newInst) => {
                        if (serverModeEnabled) {
                          void updateServerSessionInstruction(
                            currentSession.id,
                            newInst,
                          ).catch((error) => {
                            logChatAppError(
                              "Failed to update server system instruction",
                              error,
                            );
                            showActionError(t("errSaveChanges"));
                          });
                          return;
                        }
                        updateSessionInstruction(currentSession.id, newInst);
                      }}
                      onDelete={
                        currentSession.systemInstruction
                          ? () => {
                              if (serverModeEnabled) {
                                void updateServerSessionInstruction(
                                  currentSession.id,
                                  "",
                                ).catch((error) => {
                                  logChatAppError(
                                    "Failed to clear server system instruction",
                                    error,
                                  );
                                  showActionError(t("errSaveChanges"));
                                });
                                return;
                              }
                              updateSessionInstruction(currentSession.id, "");
                            }
                          : undefined
                      }
                    />
                  )}

                {/* Empty State */}
                {(welcomeState === "visible" || welcomeState === "exiting") && (
                  <div
                    className={`emptyChatSurface flex-1 motion-safe:transition-[opacity,transform] motion-safe:duration-300 motion-safe:transform origin-center ${
                      welcomeState === "exiting"
                        ? "opacity-0 scale-95 pointer-events-none"
                        : "opacity-100 scale-100"
                    }`}
                  />
                )}

                {/* Message Stream */}
                {welcomeState === "hidden" && (
                  <div className="space-y-1 motion-safe:animate-in motion-safe:fade-in motion-safe:duration-500 fill-mode-forwards">
                    {messages.map((msg, idx) => {
                      const isLastUserMessage =
                        msg.role === "user" &&
                        !messages.slice(idx + 1).some((m) => m.role === "user");
                      const isLastMessage = idx === messages.length - 1;

                      return (
                        <React.Fragment key={msg.id}>
                          <div className="[content-visibility:auto] [contain-intrinsic-size:0_240px]">
                            <MessageItem
                              message={msg}
                              branchInfo={getMessageBranchInfo(
                                visibleActiveMessageTree,
                                msg.id,
                              )}
                              onEdit={handleEditMessage}
                              onDelete={handleDeleteMessage}
                              canEditUserMessage={
                                msg.role === "user" && !isLastUserMessage
                              }
                              onSubmitUserEdit={handleSubmitUserMessageEdit}
                              onRetract={
                                isLastUserMessage
                                  ? () => handleRetractMessage(msg)
                                  : undefined
                              }
                              isLast={isLastMessage}
                              isTyping={isGenerating && isLastMessage}
                              onRegenerate={() => handleRegenerate(msg.id)}
                              onVersionChange={handleVersionChange}
                            />
                          </div>
                          {msg.role === "model" &&
                            isLastMessage &&
                            !isGenerating &&
                            msg.suggestedQuestions &&
                            msg.suggestedQuestions.length > 0 && (
                              <FollowUpQuestions
                                questions={msg.suggestedQuestions}
                                onClick={handleSuggestionClick}
                              />
                            )}
                        </React.Fragment>
                      );
                    })}

                    {isGenerating &&
                      activeImageGeneration &&
                      !hasVisibleImageGenerationMessage && (
                        <div className="message-item flex items-start gap-3 rounded-md border border-transparent px-3 py-3">
                          <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-xl border border-white bg-red-300 text-white shadow-sm dark:border-border">
                            <Bot size={18} aria-hidden="true" />
                          </div>
                          <div className="min-w-0 flex-1">
                            <ImageGenerationProgress
                              startedAt={activeImageGeneration.startedAt}
                            />
                          </div>
                        </div>
                      )}

                    {pendingServerProgressStage && (
                      <div className="message-item flex items-start gap-2.5 rounded-md border border-transparent px-3 py-3 md:gap-3">
                        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-xl border border-white bg-red-300 text-white shadow-sm dark:border-border">
                          <Bot size={18} aria-hidden="true" />
                        </div>
                        <div className="min-w-0 max-w-[84%] rounded-2xl rounded-tl-sm border border-gray-100 bg-gray-50 px-3.5 py-2.5 shadow-sm shadow-gray-900/5 dark:border-border dark:bg-muted/70 md:max-w-[88%] md:px-4 md:py-3">
                          <ChatGenerationProgress
                            stage={pendingServerProgressStage}
                            label={
                              pendingServerProgressStage === "knowledge"
                                ? t("progressSearchingKnowledge")
                                : pendingServerProgressStage === "search"
                                  ? t("progressSearchingWeb")
                                  : t("progressGeneratingAnswer")
                            }
                          />
                        </div>
                      </div>
                    )}
                  </div>
                )}
              </div>
            </div>

            <div className="w-full h-4 md:h-6"></div>

            {/* Input Area */}
            <div
              ref={setComposerAreaElement}
              className={`absolute left-0 right-0 z-20 px-4 pointer-events-none md:px-8 motion-safe:transition-[bottom,padding-bottom] motion-safe:duration-300 ${
                welcomeState === "visible"
                  ? "bottom-[40vh] pb-0 md:bottom-[32vh] md:pb-0"
                  : "bottom-0 pb-[calc(1rem+env(safe-area-inset-bottom))] md:pb-6"
              }`}
            >
              <div
                className={`flex w-full mx-auto pointer-events-auto flex-col items-center motion-safe:transition-[max-width] motion-safe:duration-300 ${
                  welcomeState === "visible" ? "max-w-2xl" : "max-w-3xl"
                }`}
              >
                {(welcomeState === "visible" || welcomeState === "exiting") && (
                  <div
                    className={`mb-3 md:mb-5 flex items-center gap-3 text-center motion-safe:transition-[opacity,transform] motion-safe:duration-300 ${
                      welcomeState === "exiting"
                        ? "pointer-events-none opacity-0 scale-95"
                        : "opacity-100 scale-100"
                    }`}
                  >
                    <div className="flex h-10 w-10 shrink-0 items-center justify-center md:h-11 md:w-11">
                      <Logo className="h-10 w-10 md:h-11 md:w-11" />
                    </div>
                    <h1 className="neoChatWordmark bg-clip-text text-[1.75rem] font-bold leading-none tracking-[0.01em] text-transparent bg-[linear-gradient(to_right,#00DEB9,#03B2DE,#1D88E1)]">
                      {t("productName")}
                    </h1>
                  </div>
                )}
                <MessageInput
                  ref={messageInputRef}
                  variant={messageInputVariant}
                  onSend={handleSendMessage}
                  onStop={isGenerating ? handleStopGeneration : undefined}
                  disabled={
                    isGenerating ||
                    availableModels.length === 0 ||
                    (serverModeEnabled && serverReadState.isLoading)
                  }
                  availableModels={availableModels}
                  selectedModel={selectedModel}
                  onSelectModel={setModel}
                  isSearchEnabled={composerChatConfig.useSearch}
                  onToggleSearch={() => {
                    if (serverModeEnabled) {
                      void toggleServerSearch().catch((error) =>
                        showActionError(
                          error instanceof Error
                            ? error.message
                            : "Search selection could not be saved.",
                        ),
                      );
                      return;
                    }
                    setChatConfig({ useSearch: !chatConfig.useSearch });
                  }}
                  isReasoningEnabled={composerChatConfig.useReasoning}
                  reasoningEffort={composerChatConfig.reasoningEffort}
                  onReasoningChange={(enabled, effort) => {
                    if (serverModeEnabled) {
                      void persistServerReasoningSelection(
                        enabled,
                        effort,
                      ).catch((error) =>
                        showActionError(
                          error instanceof Error
                            ? error.message
                            : "Reasoning selection could not be saved.",
                        ),
                      );
                      return;
                    }
                    setChatConfig({
                      useReasoning: enabled,
                      reasoningEffort: effort,
                    });
                  }}
                  localSessionToolsDisabled={serverModeEnabled}
                  allowSearchWhenSessionToolsDisabled={serverModeEnabled}
                  allowReasoningWhenSessionToolsDisabled={serverModeEnabled}
                  allowSkillsWhenSessionToolsDisabled={serverModeEnabled}
                  allowPluginsWhenSessionToolsDisabled={serverModeEnabled}
                  activeSkillIdsOverride={
                    serverModeEnabled ? activeSkillIds : undefined
                  }
                  onActiveSkillIdsChange={
                    serverModeEnabled ? setActiveSkillIds : undefined
                  }
                  onLocalSessionToolUnavailable={showServerUnsupportedAction}
                  knowledgeCollectionIds={
                    serverModeEnabled
                      ? currentSessionConfig?.selectedKnowledgeCollectionIds
                      : undefined
                  }
                  onKnowledgeCollectionIdsChange={
                    serverModeEnabled
                      ? persistConversationKnowledgeSelection
                      : undefined
                  }
                />
              </div>
            </div>
          </>
        )}
      </main>
    </div>
  );
};

export default ChatApp;
