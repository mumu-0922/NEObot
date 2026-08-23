"use client";

import { useShallow } from "zustand/react/shallow";

import { useChatStore } from "@/store/core/chatStore";
import { useCoreSettingsStore } from "@/store/core/coreSettingsStore";
import { useSettingsStore } from "@/store/core/settingsStore";

export function useChatShellState() {
  const chat = useChatStore(
    useShallow((state) => ({
      _hasHydrated: state._hasHydrated,
      sessions: state.sessions,
      workspaces: state.workspaces,
      currentSessionId: state.currentSessionId,
      activeMessages: state.activeMessages,
      activeMessageTree: state.activeMessageTree,
      serverReadState: state.serverReadState,
      selectedModel: state.selectedModel,
      chatConfig: state.chatConfig,
      createSession: state.createSession,
      selectSession: state.selectSession,
      refreshServerSessions: state.refreshServerSessions,
      refreshServerWorkspaces: state.refreshServerWorkspaces,
      selectServerSession: state.selectServerSession,
      createServerSession: state.createServerSession,
      sendServerMessageAndStream: state.sendServerMessageAndStream,
      regenerateServerAssistantMessage: state.regenerateServerAssistantMessage,
      switchServerMessageVersion: state.switchServerMessageVersion,
      updateServerSessionTitle: state.updateServerSessionTitle,
      updateServerSessionInstruction: state.updateServerSessionInstruction,
      updateServerSessionConfig: state.updateServerSessionConfig,
      updateServerSessionModel: state.updateServerSessionModel,
      updateServerSessionPermission: state.updateServerSessionPermission,
      toggleServerSessionPin: state.toggleServerSessionPin,
      deleteServerSession: state.deleteServerSession,
      duplicateServerSession: state.duplicateServerSession,
      generateServerConversationTitle: state.generateServerConversationTitle,
      updateServerMessageContent: state.updateServerMessageContent,
      deleteServerMessage: state.deleteServerMessage,
      retractServerMessage: state.retractServerMessage,
      deleteSession: state.deleteSession,
      updateSessionTitle: state.updateSessionTitle,
      updateSessionInstruction: state.updateSessionInstruction,
      updateSessionConfig: state.updateSessionConfig,
      updateSessionModel: state.updateSessionModel,
      updateSessionCompression: state.updateSessionCompression,
      updateSessionMemoryContext: state.updateSessionMemoryContext,
      toggleSessionPin: state.toggleSessionPin,
      duplicateSession: state.duplicateSession,
      addMessage: state.addMessage,
      updateMessageContent: state.updateMessageContent,
      updateMessage: state.updateMessage,
      addMessageVersion: state.addMessageVersion,
      createEditedUserMessageBranch: state.createEditedUserMessageBranch,
      switchMessageVersion: state.switchMessageVersion,
      deleteMessage: state.deleteMessage,
      deleteMessageAndSubsequent: state.deleteMessageAndSubsequent,
      setSuggestedQuestions: state.setSuggestedQuestions,
      setModel: state.setModel,
      setChatConfig: state.setChatConfig,
      getCurrentSession: state.getCurrentSession,
      syncActiveSession: state.syncActiveSession,
    })),
  );

  const settings = useSettingsStore(
    useShallow((state) => ({
      _hasHydrated: state._hasHydrated,
      serverConfig: state.serverConfig,
      modelMetadata: state.modelMetadata,
      customModelMetadata: state.customModelMetadata,
      fetchModelMetadata: state.fetchModelMetadata,
      system: state.system,
      search: state.search,
      applyServerConfig: state.applyServerConfig,
    })),
  );

  const core = useCoreSettingsStore(
    useShallow((state) => ({
      _hasHydrated: state._hasHydrated,
      theme: state.theme,
      selectedChatModel: state.selectedChatModel,
      providers: state.providers,
      setSelectedChatModel: state.setSelectedChatModel,
      updateProvider: state.updateProvider,
      replaceServerManagedProviders: state.replaceServerManagedProviders,
      applyServerConfig: state.applyServerConfig,
    })),
  );

  return {
    chat,
    settings,
    core,
  };
}
