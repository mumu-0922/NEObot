"use client";
import React, { useState, useRef, useEffect, useId } from "react";
import { useTranslations } from "next-intl";
import { Session, Message, Workspace, SessionMessageTree } from "@/types";
import { Logo } from "../ui/Icons";
import { useChatStore } from "@/store/core/chatStore";
import { appDb } from "@/store/storage/storageConfig";
import Tooltip from "../ui/Tooltip";
import WorkspaceSettingsModal from "./WorkspaceSettingsModal";
import SidebarSearch from "./SidebarSearch";
import {
  MessageSquarePlus,
  MoreVertical,
  Pin,
  Copy,
  FileOutput,
  PenLine,
  Trash2,
  Sparkles,
  PinOff,
  Check,
  X,
  FolderOpen,
  Settings,
  BotMessageSquare,
  ChevronDown,
  Library,
  FolderPlus,
  EllipsisVertical,
  FolderCog,
  FolderInput,
  Folder,
  PanelLeftClose,
  PanelLeftOpen,
  Wrench,
  PackageCheck,
  MessageSquare,
  LoaderCircle,
} from "lucide-react";
import { CHAT_ENTITY_LIMITS } from "@/config/limits";
import { sanitizeDownloadFilename } from "@/lib/utils/filename";
import { createSessionExportPayload } from "@/lib/chat/sessionExport";
import { logDevError } from "@/lib/utils/devLogger";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

interface SidebarProps {
  sessions: Session[];
  currentSessionId: string | null;
  runningSessionIds: string[];
  unreadSessionIds: string[];
  onSelectSession: (id: string) => void;
  onNewChat: () => void;
  onNewTemporaryChat: () => void;
  onNewChatInWorkspace: (workspace: Workspace) => void;
  onDeleteSession: (id: string) => void | Promise<void>;
  onRenameSession: (id: string, newTitle: string) => void;
  onTogglePin?: (id: string) => void;
  onDuplicate?: (id: string) => void | Promise<void>;
  onSmartRename?: (id: string) => void;
  isOpen: boolean;
  toggleSidebar: () => void;
  isModal?: boolean;
  onRequestClose?: () => void;
  onOpenSkillStore: () => void;
  isSkillStoreOpen: boolean;
  onOpenAssistantHub: () => void;
  isAssistantHubOpen: boolean;
  onOpenKnowledgeBase: () => void;
  isKnowledgeBaseOpen: boolean;
  onOpenTools: () => void;
  isToolsOpen: boolean;
  onOpenSettings: () => void;
  isSettingsOpen: boolean;
  onLogoClick: () => void;
}

const WORKSPACE_COLOR_MAP: Record<string, string> = {
  blue: "text-blue-500",
  purple: "text-purple-500",
  green: "text-green-500",
  orange: "text-orange-500",
  red: "text-red-500",
  pink: "text-pink-500",
  cyan: "text-cyan-500",
  gray: "text-gray-500",
};

const WORKSPACE_SESSION_PREVIEW_LIMIT = 5;
const TEMPORARY_SESSION_PREVIEW_LIMIT = 5;
const TEMPORARY_SECTION_KEY = "temporary-chats";

const SIDEBAR_FOCUSABLE_SELECTOR = [
  "a[href]",
  "button:not([disabled])",
  "input:not([disabled])",
  "select:not([disabled])",
  "textarea:not([disabled])",
  '[tabindex]:not([tabindex="-1"])',
].join(",");

const getSidebarFocusableElements = (container: HTMLElement | null) => {
  if (!container) return [];

  return Array.from(
    container.querySelectorAll<HTMLElement>(SIDEBAR_FOCUSABLE_SELECTOR),
  ).filter((element) => !element.getAttribute("aria-hidden"));
};

interface SidebarNavTooltipProps {
  isOpen: boolean;
  content: React.ReactNode;
  children: React.ReactElement;
}

const SidebarNavTooltip: React.FC<SidebarNavTooltipProps> = ({
  isOpen,
  content,
  children,
}) => {
  if (isOpen) return children;

  return (
    <Tooltip
      content={content}
      position="right"
      className="w-full justify-center"
      motion="instant"
      surface="solid"
    >
      {children}
    </Tooltip>
  );
};

const Sidebar: React.FC<SidebarProps> = ({
  sessions,
  currentSessionId,
  runningSessionIds,
  unreadSessionIds,
  onSelectSession,
  onNewChat,
  onNewTemporaryChat,
  onNewChatInWorkspace,
  onDeleteSession,
  onRenameSession,
  onTogglePin,
  onDuplicate,
  onSmartRename,
  isOpen,
  toggleSidebar,
  isModal = false,
  onRequestClose,
  onOpenSkillStore,
  isSkillStoreOpen,
  onOpenAssistantHub,
  isAssistantHubOpen,
  onOpenKnowledgeBase,
  isKnowledgeBaseOpen,
  onOpenTools,
  isToolsOpen,
  onOpenSettings,
  isSettingsOpen,
  onLogoClick,
}) => {
  const t = useTranslations("Sidebar");
  const chatT = useTranslations("ChatApp");
  const { workspaces, moveSessionToWorkspace } = useChatStore();
  const runningSessionIdSet = new Set(runningSessionIds);
  const unreadSessionIdSet = new Set(unreadSessionIds);

  const [searchTerm, setSearchTerm] = useState("");
  const [contextMenu, setContextMenu] = useState<{
    x: number;
    y: number;
    sessionId: string;
  } | null>(null);
  const [workspaceMenu, setWorkspaceMenu] = useState<{
    x: number;
    y: number;
    workspaceId: string;
  } | null>(null);
  const [pendingDeleteSessionId, setPendingDeleteSessionId] = useState<
    string | null
  >(null);
  const [exportError, setExportError] = useState<string | null>(null);
  const [expandedWorkspaceSessionLists, setExpandedWorkspaceSessionLists] =
    useState<Record<string, boolean>>({});
  const [temporarySessionListExpanded, setTemporarySessionListExpanded] =
    useState(false);

  const [renamingId, setRenamingId] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState("");

  // Section Expansion State
  const [expandedSections, setExpandedSections] = useState<{
    [key: string]: boolean;
  }>({
    [TEMPORARY_SECTION_KEY]: true,
  });

  const [editingWorkspace, setEditingWorkspace] = useState<
    Workspace | undefined
  >(undefined);
  const [showWorkspaceModal, setShowWorkspaceModal] = useState(false);

  const renameInputRef = useRef<HTMLInputElement>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const sidebarRef = useRef<HTMLDivElement>(null);
  const restoreFocusRef = useRef<HTMLElement | null>(null);
  const sidebarListRegionRef = useRef<HTMLDivElement>(null);
  const isMountedRef = useRef(true);
  const searchFocusTimerRef = useRef<ReturnType<typeof setTimeout> | null>(
    null,
  );
  const sidebarId = useId();
  const searchInputId = `${sidebarId}-search`;

  useEffect(() => {
    isMountedRef.current = true;

    return () => {
      isMountedRef.current = false;
      if (searchFocusTimerRef.current) {
        clearTimeout(searchFocusTimerRef.current);
        searchFocusTimerRef.current = null;
      }
    };
  }, []);

  useEffect(() => {
    if (renamingId && renameInputRef.current) {
      renameInputRef.current.focus();
    }
  }, [renamingId]);

  useEffect(() => {
    if (!isModal || !isOpen) return;

    restoreFocusRef.current =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;

    const frameId = requestAnimationFrame(() => {
      const firstFocusable = getSidebarFocusableElements(sidebarRef.current)[0];
      (firstFocusable ?? sidebarRef.current)?.focus({ preventScroll: true });
    });

    return () => {
      cancelAnimationFrame(frameId);
      if (restoreFocusRef.current?.isConnected) {
        restoreFocusRef.current.focus({ preventScroll: true });
      }
      restoreFocusRef.current = null;
    };
  }, [isModal, isOpen]);

  // Set default expanded state for workspaces
  useEffect(() => {
    const newExpanded = { ...expandedSections };
    let changed = false;
    workspaces.forEach((w) => {
      if (newExpanded[w.id] === undefined) {
        newExpanded[w.id] = false;
        changed = true;
      }
    });
    if (changed) {
      // Use queueMicrotask to defer state update
      queueMicrotask(() => {
        if (!isMountedRef.current) return;
        setExpandedSections(newExpanded);
      });
    }
  }, [workspaces, expandedSections]);

  useEffect(() => {
    if (!currentSessionId) return;
    const activeSession = sessions.find(
      (session) => session.id === currentSessionId,
    );
    if (!activeSession) return;
    const workspaceId = activeSession.workspaceId;
    const sectionKey = workspaceId ?? TEMPORARY_SECTION_KEY;
    const siblingSessions = sessions
      .filter(
        (session) =>
          (session.workspaceId ?? TEMPORARY_SECTION_KEY) === sectionKey,
      )
      .sort((left, right) =>
        sectionKey === TEMPORARY_SECTION_KEY
          ? Number(right.pinned) - Number(left.pinned) ||
            right.updatedAt - left.updatedAt
          : right.updatedAt - left.updatedAt,
      );
    const previewLimit = workspaceId
      ? WORKSPACE_SESSION_PREVIEW_LIMIT
      : TEMPORARY_SESSION_PREVIEW_LIMIT;
    const activeSessionNeedsExpandedList =
      siblingSessions.findIndex((session) => session.id === currentSessionId) >=
      previewLimit;
    queueMicrotask(() => {
      if (!isMountedRef.current) return;
      setExpandedSections((current) =>
        current[sectionKey] ? current : { ...current, [sectionKey]: true },
      );
      if (!activeSessionNeedsExpandedList) return;
      if (workspaceId) {
        setExpandedWorkspaceSessionLists((current) =>
          current[workspaceId] ? current : { ...current, [workspaceId]: true },
        );
        return;
      }
      setTemporarySessionListExpanded(true);
    });
  }, [currentSessionId, sessions]);

  const handleContextMenu = (e: React.MouseEvent, sessionId: string) => {
    e.preventDefault();
    e.stopPropagation();
    const x = e.clientX;
    const y = Math.min(e.clientY, window.innerHeight - 350);
    setContextMenu({ x, y, sessionId });
    setPendingDeleteSessionId(null);
  };

  const handleWorkspaceContextMenu = (
    e: React.MouseEvent,
    workspaceId: string,
  ) => {
    e.preventDefault();
    e.stopPropagation();
    const x = e.clientX;
    const y = Math.min(e.clientY, window.innerHeight - 200);
    setWorkspaceMenu({ x, y, workspaceId });
  };

  const handleExport = async (sessionId: string) => {
    const session = sessions.find((s) => s.id === sessionId);
    if (!session) return;
    setExportError(null);

    const chatState = useChatStore.getState();
    const activeMessages =
      chatState.currentSessionId === sessionId ? chatState.activeMessages : [];
    const activeMessageTree =
      chatState.currentSessionId === sessionId
        ? chatState.activeMessageTree
        : undefined;

    let fullSession;
    try {
      fullSession = await createSessionExportPayload({
        session,
        currentSessionId: chatState.currentSessionId,
        activeMessages,
        activeMessageTree,
        loadMessages: (id) =>
          appDb.getItem<Message[] | SessionMessageTree>(
            `session_messages_${id}`,
          ),
      });
    } catch (e) {
      logDevError("Failed to load messages for export", e);
      setExportError(t("exportError"));
      return;
    }

    const blob = new Blob([JSON.stringify(fullSession, null, 2)], {
      type: "application/json;charset=utf-8",
    });
    const url = URL.createObjectURL(blob);
    const downloadAnchorNode = document.createElement("a");
    downloadAnchorNode.setAttribute("href", url);
    downloadAnchorNode.setAttribute(
      "download",
      sanitizeDownloadFilename(`chat_export_${session.title}.json`),
    );
    document.body.appendChild(downloadAnchorNode);
    downloadAnchorNode.click();
    downloadAnchorNode.remove();
    URL.revokeObjectURL(url);
    setExportError(null);
  };

  const handleStartRename = (sessionId: string, currentTitle: string) => {
    setRenamingId(sessionId);
    setRenameValue(currentTitle);
    setContextMenu(null);
    setPendingDeleteSessionId(null);
  };

  const submitRename = () => {
    if (renamingId && renameValue.trim()) {
      onRenameSession(renamingId, renameValue.trim());
    }
    setRenamingId(null);
  };

  const handleKeyDownRename = (e: React.KeyboardEvent) => {
    if (e.key === "Enter") submitRename();
    if (e.key === "Escape") setRenamingId(null);
  };

  const handleSearchIconClick = () => {
    if (!isOpen) {
      toggleSidebar();
      if (searchFocusTimerRef.current) {
        clearTimeout(searchFocusTimerRef.current);
      }
      searchFocusTimerRef.current = setTimeout(() => {
        searchFocusTimerRef.current = null;
        searchInputRef.current?.focus();
      }, 300);
    }
  };

  const handleSidebarKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (!isModal) return;

    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      onRequestClose?.();
      return;
    }

    if (event.key !== "Tab") return;

    const focusableElements = getSidebarFocusableElements(sidebarRef.current);
    if (focusableElements.length === 0) {
      event.preventDefault();
      sidebarRef.current?.focus({ preventScroll: true });
      return;
    }

    const firstFocusable = focusableElements[0];
    const lastFocusable = focusableElements[focusableElements.length - 1];
    const activeElement = document.activeElement;

    if (event.shiftKey) {
      if (
        activeElement === firstFocusable ||
        !sidebarRef.current?.contains(activeElement)
      ) {
        event.preventDefault();
        lastFocusable?.focus({ preventScroll: true });
      }
      return;
    }

    if (activeElement === lastFocusable) {
      event.preventDefault();
      firstFocusable?.focus({ preventScroll: true });
    }
  };

  const toggleSection = (section: string) => {
    setExpandedSections((prev) => ({ ...prev, [section]: !prev[section] }));
  };

  // Filtering Logic
  const isSearchingChats = searchTerm.trim().length > 0;
  const filteredSessions = sessions.filter((s) =>
    s.title.toLowerCase().includes(searchTerm.toLowerCase()),
  );

  // Workspace membership is the single navigation authority. Unbound
  // Conversations remain durable and appear under the virtual Temporary group.
  const temporarySessions = filteredSessions
    .filter((session) => !session.workspaceId)
    .sort(
      (left, right) =>
        Number(right.pinned) - Number(left.pinned) ||
        right.updatedAt - left.updatedAt,
    );
  const workspaceSessionsMap = new Map<string, Session[]>();

  workspaces.forEach((w) => {
    workspaceSessionsMap.set(
      w.id,
      filteredSessions
        .filter((session) => session.workspaceId === w.id)
        .sort((left, right) => right.updatedAt - left.updatedAt),
    );
  });

  const visibleTemporarySessions =
    isSearchingChats || temporarySessionListExpanded
      ? temporarySessions
      : temporarySessions.slice(0, TEMPORARY_SESSION_PREVIEW_LIMIT);

  const getVisibleWorkspaceSessions = (
    workspaceId: string,
    items: Session[],
  ) => {
    if (isSearchingChats || expandedWorkspaceSessionLists[workspaceId]) {
      return items;
    }
    return items.slice(0, WORKSPACE_SESSION_PREVIEW_LIMIT);
  };

  const renderSessionItem = (session: Session) => {
    const isActive =
      currentSessionId === session.id &&
      !isSkillStoreOpen &&
      !isAssistantHubOpen &&
      !isKnowledgeBaseOpen &&
      !isToolsOpen &&
      !isSettingsOpen;
    const isRunning = runningSessionIdSet.has(session.id);
    const hasUnreadResult =
      !isActive && !isRunning && unreadSessionIdSet.has(session.id);

    return (
      <div
        key={session.id}
        className={`
          group relative flex items-center rounded-lg py-2 pl-3 pr-2 text-sm
          ${
            isActive
              ? "bg-gray-100/80 font-medium text-gray-800 dark:bg-accent/60 dark:text-foreground"
              : "text-gray-600 hover:bg-gray-100/80 dark:text-muted-foreground dark:hover:bg-muted/60"
          }
        `}
        onContextMenu={(e) => handleContextMenu(e, session.id)}
      >
        {renamingId === session.id ? (
          <div className="flex w-full items-center gap-1">
            <input
              ref={renameInputRef}
              aria-label={t("renameAria", { title: session.title })}
              name="session-title"
              value={renameValue}
              onChange={(e) => setRenameValue(e.target.value)}
              maxLength={CHAT_ENTITY_LIMITS.maxSessionTitleChars}
              onKeyDown={handleKeyDownRename}
              onBlur={submitRename}
              className="flex-1 rounded border border-blue-300 bg-white px-1 py-0.5 text-xs text-gray-800 focus:outline-none focus:ring-2 focus:ring-blue-500/40 dark:bg-muted dark:text-foreground"
              onClick={(e) => e.stopPropagation()}
            />
            <button
              type="button"
              aria-label={t("saveTitleAria", { title: session.title })}
              className="rounded p-0.5 text-green-600 transition-colors hover:bg-green-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-green-500/60 dark:hover:bg-green-900/30"
              onMouseDown={submitRename}
            >
              <Check size={12} aria-hidden="true" />
            </button>
            <button
              type="button"
              aria-label={t("cancelRenameAria", { title: session.title })}
              className="rounded p-0.5 text-red-500 transition-colors hover:bg-red-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-500/60 dark:hover:bg-red-900/30"
              onMouseDown={() => setRenamingId(null)}
            >
              <X size={12} aria-hidden="true" />
            </button>
          </div>
        ) : (
          <>
            <button
              type="button"
              aria-current={isActive ? "page" : undefined}
              className="flex min-w-0 flex-1 items-center gap-1.5 rounded-md pr-6 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60"
              onClick={() => {
                onSelectSession(session.id);
              }}
            >
              {session.pinned && (
                <Pin
                  size={12}
                  className="shrink-0 fill-current text-red-500"
                  aria-hidden="true"
                />
              )}
              <span className="min-w-0 flex-1 truncate">{session.title}</span>
              {isRunning ? (
                <span
                  role="status"
                  aria-label={t("chatRunningAria", { title: session.title })}
                  title={t("chatRunning", { title: session.title })}
                  className="mr-0.5 inline-flex shrink-0 text-blue-500 dark:text-blue-400"
                >
                  <LoaderCircle
                    size={13}
                    className="animate-spin motion-reduce:animate-none"
                    aria-hidden="true"
                  />
                </span>
              ) : hasUnreadResult ? (
                <span
                  role="status"
                  aria-label={t("chatUnreadAria", { title: session.title })}
                  title={t("chatUnread", { title: session.title })}
                  className="mr-1 size-2 shrink-0 rounded-full bg-blue-500 ring-2 ring-blue-500/15 dark:bg-blue-400"
                />
              ) : null}
            </button>

            <button
              type="button"
              aria-label={t("moreActionsAria", { title: session.title })}
              className={`absolute right-2 rounded-lg p-1 opacity-100 hover:bg-white/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60 md:opacity-0 md:group-hover:opacity-100 dark:hover:bg-accent ${contextMenu?.sessionId === session.id ? "opacity-100" : ""}`}
              onClick={(e) => handleContextMenu(e, session.id)}
            >
              <MoreVertical size={14} aria-hidden="true" />
            </button>
          </>
        )}
      </div>
    );
  };

  const renderShowAllButton = ({
    controlId,
    expanded,
    hiddenCount,
    onToggle,
  }: {
    controlId: string;
    expanded: boolean;
    hiddenCount: number;
    onToggle: () => void;
  }) => {
    if (hiddenCount <= 0) return null;

    return (
      <button
        type="button"
        aria-expanded={expanded}
        aria-controls={controlId}
        onClick={onToggle}
        className="mt-1 flex w-full items-center rounded-md px-3 py-1 text-left text-xs font-medium text-gray-400 transition-colors hover:bg-gray-100/70 hover:text-gray-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60 dark:text-muted-foreground/70 dark:hover:bg-muted/60 dark:hover:text-foreground/85"
      >
        {expanded ? t("showLess") : t("showAll", { count: hiddenCount })}
      </button>
    );
  };

  return (
    <div
      ref={sidebarRef}
      role={isModal ? "dialog" : undefined}
      aria-modal={isModal || undefined}
      aria-label={isModal ? "NeoBot" : undefined}
      tabIndex={isModal ? -1 : undefined}
      onKeyDown={handleSidebarKeyDown}
      className={`
      glass-shell border-r border-gray-200 dark:border-sidebar-border flex flex-col shrink-0 h-full pt-[env(safe-area-inset-top)] pb-[env(safe-area-inset-bottom)] transition-[width,transform] duration-300 ease-in-out md:pb-0 md:pt-0
      fixed inset-y-0 left-0 z-40
      ${isOpen ? "translate-x-0 w-72" : "-translate-x-full w-72"}
      md:translate-x-0 md:relative
      ${isOpen ? "md:w-72" : "md:w-16"}
    `}
    >
      {showWorkspaceModal && (
        <WorkspaceSettingsModal
          onClose={() => {
            setShowWorkspaceModal(false);
            setEditingWorkspace(undefined);
          }}
          workspace={editingWorkspace}
        />
      )}

      <div
        className={`px-3 py-3 flex shrink-0 transition-[height,padding] duration-300 ${
          isOpen ? "h-14 items-center gap-2" : "items-center justify-center"
        }`}
      >
        {isOpen ? (
          <>
            <button
              type="button"
              aria-label={t("goHomeAria")}
              className="flex min-w-0 flex-1 items-center gap-2 rounded-lg px-2 text-lg font-bold text-gray-700 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60 dark:text-foreground"
              onClick={onLogoClick}
            >
              <div className="w-8 h-8 flex items-center justify-center shrink-0">
                <Logo className="w-7 h-7" />
              </div>
              <span className="truncate whitespace-nowrap bg-clip-text text-transparent bg-[linear-gradient(to_right,#00DEB9,#03B2DE,#1D88E1)]">
                NeoBot
              </span>
            </button>
            <Tooltip content={chatT("closeSidebar")} position="left">
              <button
                type="button"
                aria-label={chatT("closeSidebarAria")}
                onClick={toggleSidebar}
                className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-gray-500 transition-[background-color,color,box-shadow] hover:bg-gray-200/70 hover:text-gray-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60 dark:text-muted-foreground dark:hover:bg-muted/80 dark:hover:text-foreground"
              >
                <PanelLeftClose size={18} aria-hidden="true" />
              </button>
            </Tooltip>
          </>
        ) : (
          <Tooltip content={chatT("openSidebar")} position="right">
            <button
              type="button"
              aria-label={chatT("openSidebarAria")}
              className="group relative flex h-10 w-10 items-center justify-center rounded-lg text-gray-700 transition-[background-color,color,box-shadow] hover:bg-gray-100/80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60 dark:text-foreground dark:hover:bg-muted/60"
              onClick={toggleSidebar}
            >
              <div className="absolute inset-0 flex items-center justify-center transition-[opacity,transform] duration-200 group-hover:scale-90 group-hover:opacity-0 group-focus-visible:scale-90 group-focus-visible:opacity-0">
                <Logo className="w-7 h-7" />
              </div>
              <PanelLeftOpen
                size={18}
                aria-hidden="true"
                className="scale-75 opacity-0 transition-[opacity,transform] duration-200 group-hover:scale-100 group-hover:opacity-100 group-focus-visible:scale-100 group-focus-visible:opacity-100"
              />
            </button>
          </Tooltip>
        )}
      </div>

      <div className="px-3 pb-2 space-y-1 shrink-0">
        <SidebarNavTooltip isOpen={isOpen} content={t("skillStore")}>
          <button
            type="button"
            aria-label={t("openSkillStore")}
            aria-current={isSkillStoreOpen ? "page" : undefined}
            onClick={onOpenSkillStore}
            className={`flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/60 ${
              isSkillStoreOpen
                ? "bg-cyan-50 text-cyan-700 dark:bg-cyan-900/30 dark:text-cyan-300"
                : "text-gray-600 hover:bg-gray-100/80 dark:text-muted-foreground dark:hover:bg-muted/60"
            } ${isOpen ? "w-full" : "w-10 justify-center px-0"}`}
          >
            <PackageCheck
              size={18}
              className={`shrink-0 ${isSkillStoreOpen ? "text-cyan-600" : "text-gray-500"}`}
              aria-hidden="true"
            />
            {isOpen && <span className="truncate">{t("skillStore")}</span>}
          </button>
        </SidebarNavTooltip>

        <SidebarNavTooltip isOpen={isOpen} content={t("assistantHub")}>
          <button
            type="button"
            aria-label={t("openAssistantHub")}
            aria-current={isAssistantHubOpen ? "page" : undefined}
            onClick={onOpenAssistantHub}
            className={`flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-rose-500/60 ${
              isAssistantHubOpen
                ? "bg-rose-50 text-rose-600 dark:bg-rose-900/30 dark:text-rose-400"
                : "text-gray-600 dark:text-muted-foreground hover:bg-gray-100/80 dark:hover:bg-muted/60"
            } ${isOpen ? "w-full" : "w-10 justify-center px-0"}`}
          >
            <BotMessageSquare
              size={18}
              className={`shrink-0 ${isAssistantHubOpen ? "text-rose-500" : "text-gray-500"}`}
              aria-hidden="true"
            />
            {isOpen && <span className="truncate">{t("assistantHub")}</span>}
          </button>
        </SidebarNavTooltip>

        <SidebarNavTooltip isOpen={isOpen} content={t("knowledgeBase")}>
          <button
            type="button"
            aria-label={t("openKnowledgeBase")}
            aria-current={isKnowledgeBaseOpen ? "page" : undefined}
            onClick={onOpenKnowledgeBase}
            className={`flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-purple-500/60 ${
              isKnowledgeBaseOpen
                ? "bg-purple-50 text-purple-600 dark:bg-purple-900/30 dark:text-purple-400"
                : "text-gray-600 dark:text-muted-foreground hover:bg-gray-100/80 dark:hover:bg-muted/60"
            } ${isOpen ? "w-full" : "w-10 justify-center px-0"}`}
          >
            <Library
              size={18}
              className={`shrink-0 ${isKnowledgeBaseOpen ? "text-purple-500" : "text-gray-500"}`}
              aria-hidden="true"
            />
            {isOpen && <span className="truncate">{t("knowledgeBase")}</span>}
          </button>
        </SidebarNavTooltip>

        <SidebarNavTooltip isOpen={isOpen} content={t("tools")}>
          <button
            type="button"
            aria-label={t("openTools")}
            aria-current={isToolsOpen ? "page" : undefined}
            onClick={onOpenTools}
            className={`flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/60 ${
              isToolsOpen
                ? "bg-cyan-50 text-cyan-700 dark:bg-cyan-950/30 dark:text-cyan-300"
                : "text-gray-600 dark:text-muted-foreground hover:bg-gray-100/80 dark:hover:bg-muted/60"
            } ${isOpen ? "w-full" : "w-10 justify-center px-0"}`}
          >
            <Wrench
              size={18}
              className={`shrink-0 ${isToolsOpen ? "text-cyan-500" : "text-gray-500"}`}
              aria-hidden="true"
            />
            {isOpen && <span className="truncate">{t("tools")}</span>}
          </button>
        </SidebarNavTooltip>
      </div>

      <div className="shrink-0">
        <SidebarSearch
          isOpen={isOpen}
          inputId={searchInputId}
          inputRef={searchInputRef}
          value={searchTerm}
          onChange={setSearchTerm}
          onCollapsedSearchClick={handleSearchIconClick}
        />
        {isOpen && exportError && (
          <div
            role="alert"
            aria-live="polite"
            className="mx-3 mb-2 mt-0 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-900/60 dark:bg-red-950/40 dark:text-red-200"
          >
            {exportError}
          </div>
        )}
      </div>

      <div className="min-h-0 flex-1 px-3 pb-2">
        {isOpen ? (
          <div
            ref={sidebarListRegionRef}
            className="flex h-full min-h-0 flex-col gap-2"
          >
            <section className="flex min-h-0 flex-1 flex-col">
              <div className="flex shrink-0 items-center justify-between pt-1 pl-3 pr-1 group">
                <span className="text-sm font-medium text-gray-600 dark:text-muted-foreground whitespace-nowrap">
                  {t("workspaces")}
                </span>
                <Tooltip content={t("newWorkspace")} position="left">
                  <button
                    type="button"
                    aria-label={t("createWorkspaceAria")}
                    onClick={() => {
                      setEditingWorkspace(undefined);
                      setShowWorkspaceModal(true);
                    }}
                    className="p-1.5 text-gray-500 dark:text-muted-foreground hover:bg-gray-200 dark:hover:bg-accent/80 rounded-md transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60"
                  >
                    <FolderPlus size={16} aria-hidden="true" />
                  </button>
                </Tooltip>
              </div>

              <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain pr-0.5 custom-scrollbar">
                <div className="space-y-1 pb-1">
                  {workspaces.map((ws) => {
                    const wsSessions = workspaceSessionsMap.get(ws.id) || [];
                    const visibleWorkspaceSessions =
                      getVisibleWorkspaceSessions(ws.id, wsSessions);
                    const workspaceListExpanded =
                      !!expandedWorkspaceSessionLists[ws.id] ||
                      isSearchingChats;
                    const hiddenWorkspaceSessionCount = Math.max(
                      wsSessions.length - WORKSPACE_SESSION_PREVIEW_LIMIT,
                      0,
                    );
                    const isExpanded =
                      isSearchingChats || expandedSections[ws.id];
                    const folderColorClass = ws.color
                      ? WORKSPACE_COLOR_MAP[ws.color]
                      : "text-blue-500";
                    const workspaceContentId = `${sidebarId}-workspace-${ws.id}`;

                    return (
                      <div key={ws.id}>
                        <div
                          className="group relative flex items-center justify-between rounded-lg py-1.5 pl-3 pr-2 transition-colors hover:bg-gray-100/50 dark:hover:bg-muted/30"
                          onContextMenu={(e) =>
                            handleWorkspaceContextMenu(e, ws.id)
                          }
                        >
                          <button
                            type="button"
                            aria-expanded={isExpanded}
                            aria-controls={workspaceContentId}
                            className="flex min-w-0 flex-1 items-center gap-2 rounded-md text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60"
                            onClick={() => toggleSection(ws.id)}
                          >
                            {isExpanded ? (
                              <FolderOpen
                                size={14}
                                className={`${folderColorClass} shrink-0`}
                                aria-hidden="true"
                              />
                            ) : (
                              <Folder
                                size={14}
                                className={`${folderColorClass} shrink-0`}
                                aria-hidden="true"
                              />
                            )}
                            <span className="min-w-0 flex-1">
                              <span className="block truncate text-sm font-medium text-gray-700 dark:text-foreground/85">
                                {ws.name}
                              </span>
                              <span
                                className={`block truncate text-[10px] ${
                                  ws.bindingStatus === "bound"
                                    ? "text-emerald-600 dark:text-emerald-400"
                                    : "text-amber-600 dark:text-amber-400"
                                }`}
                              >
                                {ws.bindingStatus === "bound"
                                  ? ws.displayPath || t("workspaceBound")
                                  : t("workspaceUnbound")}
                              </span>
                            </span>
                          </button>

                          <button
                            type="button"
                            aria-label={t("workspaceMoreActionsAria", {
                              name: ws.name,
                            })}
                            onClick={(e) =>
                              handleWorkspaceContextMenu(e, ws.id)
                            }
                            className={`p-1 text-gray-400 hover:text-gray-600 dark:hover:text-foreground/85 hover:bg-gray-200 dark:hover:bg-accent rounded transition-[opacity,color,background-color] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60 ${workspaceMenu?.workspaceId === ws.id ? "opacity-100" : "opacity-0 group-hover:opacity-100 focus-visible:opacity-100"}`}
                          >
                            <EllipsisVertical size={14} aria-hidden="true" />
                          </button>
                        </div>

                        {isExpanded && (
                          <div
                            id={workspaceContentId}
                            className="border-gray-200 dark:border-border space-y-0.5"
                          >
                            {wsSessions.length > 0 ? (
                              <>
                                {visibleWorkspaceSessions.map(
                                  renderSessionItem,
                                )}
                                {!isSearchingChats &&
                                  renderShowAllButton({
                                    controlId: workspaceContentId,
                                    expanded: workspaceListExpanded,
                                    hiddenCount: hiddenWorkspaceSessionCount,
                                    onToggle: () =>
                                      setExpandedWorkspaceSessionLists(
                                        (prev) => ({
                                          ...prev,
                                          [ws.id]: !prev[ws.id],
                                        }),
                                      ),
                                  })}
                              </>
                            ) : (
                              <div className="pl-3 pr-2 py-1.5 text-xs text-gray-400 italic">
                                {t("noChats")}
                              </div>
                            )}
                          </div>
                        )}
                      </div>
                    );
                  })}

                  <div className="mt-2 border-t border-gray-200/60 pt-2 dark:border-border/70">
                    <div className="group relative flex items-center justify-between rounded-lg py-1.5 pl-3 pr-2 transition-colors hover:bg-gray-100/50 dark:hover:bg-muted/30">
                      <button
                        type="button"
                        aria-expanded={
                          isSearchingChats ||
                          expandedSections[TEMPORARY_SECTION_KEY]
                        }
                        aria-controls={`${sidebarId}-temporary-sessions`}
                        className="flex min-w-0 flex-1 items-center gap-2 rounded-md text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60"
                        onClick={() => toggleSection(TEMPORARY_SECTION_KEY)}
                      >
                        <MessageSquare
                          size={14}
                          className="shrink-0 text-violet-500"
                          aria-hidden="true"
                        />
                        <span className="truncate text-sm font-medium text-gray-700 dark:text-foreground/85">
                          {t("temporaryChats")}
                        </span>
                      </button>
                      <Tooltip content={t("newTemporaryChat")} position="left">
                        <button
                          type="button"
                          aria-label={t("createTemporaryChatAria")}
                          onClick={onNewTemporaryChat}
                          className="rounded p-1 text-gray-400 opacity-100 transition-[opacity,color,background-color] hover:bg-gray-200 hover:text-gray-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60 md:opacity-0 md:group-hover:opacity-100 md:focus-visible:opacity-100 dark:hover:bg-accent dark:hover:text-foreground/85"
                        >
                          <MessageSquarePlus size={14} aria-hidden="true" />
                        </button>
                      </Tooltip>
                    </div>

                    {(isSearchingChats ||
                      expandedSections[TEMPORARY_SECTION_KEY]) && (
                      <div
                        id={`${sidebarId}-temporary-sessions`}
                        className="space-y-0.5"
                      >
                        {temporarySessions.length > 0 ? (
                          <>
                            {visibleTemporarySessions.map(renderSessionItem)}
                            {!isSearchingChats &&
                              renderShowAllButton({
                                controlId: `${sidebarId}-temporary-sessions`,
                                expanded: temporarySessionListExpanded,
                                hiddenCount: Math.max(
                                  temporarySessions.length -
                                    TEMPORARY_SESSION_PREVIEW_LIMIT,
                                  0,
                                ),
                                onToggle: () =>
                                  setTemporarySessionListExpanded(
                                    (expanded) => !expanded,
                                  ),
                              })}
                          </>
                        ) : (
                          <div className="pl-3 pr-2 py-1.5 text-xs text-gray-400 italic">
                            {t("noTemporaryChats")}
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                </div>
              </div>
            </section>
          </div>
        ) : (
          <div className="flex flex-col gap-1 items-center">
            {/* Collapsed New Workspace Button */}
            <Tooltip
              content={t("newWorkspace")}
              position="right"
              className="justify-center"
            >
              <button
                type="button"
                aria-label={t("createWorkspaceAria")}
                onClick={() => {
                  setEditingWorkspace(undefined);
                  setShowWorkspaceModal(true);
                }}
                className="p-2 text-gray-500 hover:bg-gray-100/80 dark:hover:bg-muted/60 rounded-lg transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60"
              >
                <FolderPlus size={18} aria-hidden="true" />
              </button>
            </Tooltip>

            <Tooltip
              content={t("newChat")}
              position="right"
              className="justify-center"
            >
              <button
                type="button"
                aria-label={t("createChatAria")}
                onClick={onNewChat}
                className="p-2 text-gray-500 hover:bg-gray-100/80 dark:hover:bg-muted/60 rounded-lg transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60"
              >
                <MessageSquarePlus size={18} aria-hidden="true" />
              </button>
            </Tooltip>
          </div>
        )}
      </div>

      <div className="shrink-0 border-t border-gray-200/50 p-3 dark:border-border">
        <SidebarNavTooltip isOpen={isOpen} content={t("settings")}>
          <button
            type="button"
            aria-label={t("openSettings")}
            aria-current={isSettingsOpen ? "page" : undefined}
            onClick={onOpenSettings}
            className={`flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60 ${
              isSettingsOpen
                ? "bg-slate-100 text-slate-700 dark:bg-sidebar-accent dark:text-sidebar-accent-foreground"
                : "text-gray-600 hover:bg-gray-100/80 dark:text-muted-foreground dark:hover:bg-muted/60"
            } ${isOpen ? "w-full" : "w-10 justify-center px-0"}`}
          >
            <Settings
              size={18}
              className={`shrink-0 ${isSettingsOpen ? "text-blue-500" : "text-gray-500"}`}
              aria-hidden="true"
            />
            {isOpen && <span className="truncate">{t("settings")}</span>}
          </button>
        </SidebarNavTooltip>
      </div>

      {/* Session Context Menu */}
      {contextMenu && (
        <DropdownMenu
          open
          onOpenChange={(open) => {
            if (open) return;
            setContextMenu(null);
            setPendingDeleteSessionId(null);
          }}
        >
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              aria-label={t("chatActions")}
              className="fixed z-50 h-px w-px opacity-0"
              style={{ top: contextMenu.y, left: contextMenu.x }}
            />
          </DropdownMenuTrigger>
          <DropdownMenuContent
            side="bottom"
            align="start"
            sideOffset={0}
            className="w-52 overflow-visible"
          >
            {(() => {
              const session = sessions.find(
                (s) => s.id === contextMenu.sessionId,
              );
              if (!session) return null;

              const hasMessages = session.messageCount > 0;
              const isConfirmingDelete = pendingDeleteSessionId === session.id;

              return (
                <>
                  {hasMessages && (
                    <>
                      <DropdownMenuItem
                        onSelect={() => {
                          onTogglePin?.(session.id);
                          setContextMenu(null);
                        }}
                      >
                        {session.pinned ? (
                          <PinOff size={14} aria-hidden="true" />
                        ) : (
                          <Pin size={14} aria-hidden="true" />
                        )}
                        {session.pinned ? t("unpin") : t("pin")}
                      </DropdownMenuItem>

                      <DropdownMenuItem
                        onSelect={() => {
                          if (onDuplicate) void onDuplicate(session.id);
                          setContextMenu(null);
                        }}
                      >
                        <Copy size={14} aria-hidden="true" /> {t("duplicate")}
                      </DropdownMenuItem>

                      <DropdownMenuItem
                        onSelect={() => {
                          void handleExport(session.id);
                          setContextMenu(null);
                        }}
                      >
                        <FileOutput size={14} aria-hidden="true" />
                        {t("export")}
                      </DropdownMenuItem>

                      <DropdownMenuSeparator />
                    </>
                  )}

                  <DropdownMenuItem
                    onSelect={() =>
                      handleStartRename(session.id, session.title)
                    }
                  >
                    <PenLine size={14} aria-hidden="true" /> {t("rename")}
                  </DropdownMenuItem>

                  {hasMessages && (
                    <DropdownMenuItem
                      className="text-purple-600 dark:text-purple-400"
                      onSelect={() => {
                        onSmartRename?.(session.id);
                        setContextMenu(null);
                      }}
                    >
                      <Sparkles size={14} aria-hidden="true" />
                      {t("aiRename")}
                    </DropdownMenuItem>
                  )}

                  <DropdownMenuSub>
                    <DropdownMenuSubTrigger>
                      <FolderInput size={14} aria-hidden="true" />
                      <span>{t("moveTo")}</span>
                      <ChevronDown
                        size={12}
                        className="ml-auto -rotate-90"
                        aria-hidden="true"
                      />
                    </DropdownMenuSubTrigger>
                    <DropdownMenuSubContent
                      className="max-h-60 w-52 overflow-y-auto custom-scrollbar"
                      aria-label={t("moveToWorkspaceAria")}
                    >
                      <DropdownMenuRadioGroup
                        value={session.workspaceId ?? ""}
                        onValueChange={(workspaceId) => {
                          void moveSessionToWorkspace(
                            session.id,
                            workspaceId || null,
                          ).catch((error) => {
                            logDevError(
                              "Failed to move conversation to Workspace",
                              error,
                            );
                            setExportError(t("moveWorkspaceFailed"));
                          });
                          setContextMenu(null);
                        }}
                      >
                        <DropdownMenuRadioItem value="">
                          <MessageSquare
                            size={14}
                            className="text-violet-500"
                            aria-hidden="true"
                          />
                          {t("temporaryChats")}
                        </DropdownMenuRadioItem>
                        <DropdownMenuSeparator />
                        {workspaces.map((ws) => (
                          <DropdownMenuRadioItem key={ws.id} value={ws.id}>
                            <Folder
                              size={14}
                              className="shrink-0 text-blue-500"
                              aria-hidden="true"
                            />
                            <span className="truncate">{ws.name}</span>
                          </DropdownMenuRadioItem>
                        ))}
                        {workspaces.length === 0 && (
                          <div className="px-4 py-2 text-xs italic text-gray-400">
                            {t("noWorkspaces")}
                          </div>
                        )}
                      </DropdownMenuRadioGroup>
                    </DropdownMenuSubContent>
                  </DropdownMenuSub>

                  <DropdownMenuSeparator />

                  <DropdownMenuItem
                    aria-label={
                      isConfirmingDelete
                        ? t("confirmDeleteAria", { title: session.title })
                        : t("deleteAria", { title: session.title })
                    }
                    variant="destructive"
                    className={
                      isConfirmingDelete
                        ? "bg-red-50 text-red-700 dark:bg-red-900/40 dark:text-red-200"
                        : undefined
                    }
                    onSelect={(event) => {
                      if (!isConfirmingDelete) {
                        event.preventDefault();
                        setPendingDeleteSessionId(session.id);
                        return;
                      }

                      void onDeleteSession(session.id);
                      setContextMenu(null);
                      setPendingDeleteSessionId(null);
                    }}
                  >
                    {isConfirmingDelete ? (
                      <Check size={14} aria-hidden="true" />
                    ) : (
                      <Trash2 size={14} aria-hidden="true" />
                    )}
                    {isConfirmingDelete ? t("confirmDelete") : t("delete")}
                  </DropdownMenuItem>
                </>
              );
            })()}
          </DropdownMenuContent>
        </DropdownMenu>
      )}

      {/* Workspace Context Menu */}
      {workspaceMenu && (
        <DropdownMenu
          open
          onOpenChange={(open) => {
            if (!open) setWorkspaceMenu(null);
          }}
        >
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              aria-label={t("workspaceActions")}
              className="fixed z-50 h-px w-px opacity-0"
              style={{ top: workspaceMenu.y, left: workspaceMenu.x }}
            />
          </DropdownMenuTrigger>
          <DropdownMenuContent
            side="bottom"
            align="start"
            sideOffset={0}
            className="w-48"
          >
            <DropdownMenuItem
              onSelect={() => {
                const ws = workspaces.find(
                  (w) => w.id === workspaceMenu.workspaceId,
                );
                if (ws) onNewChatInWorkspace(ws);
                setWorkspaceMenu(null);
              }}
            >
              <MessageSquarePlus size={14} aria-hidden="true" />
              {t("newChat")}
            </DropdownMenuItem>

            <DropdownMenuItem
              onSelect={() => {
                const ws = workspaces.find(
                  (w) => w.id === workspaceMenu.workspaceId,
                );
                setEditingWorkspace(ws);
                setShowWorkspaceModal(true);
                setWorkspaceMenu(null);
              }}
            >
              <FolderCog size={14} aria-hidden="true" />
              {t("editWorkspace")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      )}
    </div>
  );
};

export default Sidebar;
