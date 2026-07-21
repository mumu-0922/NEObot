"use client";

import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useTranslations } from "next-intl";
import { ArrowDownToLine, ArrowUpToLine } from "lucide-react";

import type { Message } from "@/types";
import {
  CHAT_NAVIGATION_READING_LINE_RATIO,
  buildChatMessageNavigationItems,
  getChatMessageElementId,
  resolveActiveChatNavigationMessageId,
} from "@/lib/chat/messageNavigation";

interface ChatMessageNavigatorProps {
  messages: readonly Message[];
  scrollContainerRef: React.RefObject<HTMLDivElement | null>;
  renderStartIndex: number;
  onNavigateMessage: (messageId: string) => void;
  onJumpTop: () => void;
  onJumpBottom: () => void;
}

interface ChatNavigationPreview {
  key: string;
  label: string;
  top: number;
}

const EDGE_THRESHOLD_PX = 2;
const DESKTOP_NAVIGATION_QUERY = "(min-width: 1024px)";

const ChatMessageNavigator: React.FC<ChatMessageNavigatorProps> = ({
  messages,
  scrollContainerRef,
  renderStartIndex,
  onNavigateMessage,
  onJumpTop,
  onJumpBottom,
}) => {
  const t = useTranslations("ChatApp");
  const [activeMessageId, setActiveMessageId] = useState<string | null>(null);
  const [isAtTop, setIsAtTop] = useState(true);
  const [isAtBottom, setIsAtBottom] = useState(true);
  const [isDesktopNavigation, setIsDesktopNavigation] = useState(false);
  const [navigationPreview, setNavigationPreview] =
    useState<ChatNavigationPreview | null>(null);
  const frameRef = useRef<number | null>(null);
  const navigatorRef = useRef<HTMLElement>(null);
  const emptyLabel = t("userMessageFallback");
  const attachmentPrefix = t("attachmentMessagePrefix");
  const navigationItems = useMemo(
    () =>
      buildChatMessageNavigationItems(messages, {
        emptyLabel,
        attachmentPrefix,
      }),
    [attachmentPrefix, emptyLabel, messages],
  );

  const updateNavigationState = useCallback(() => {
    const container = scrollContainerRef.current;
    if (!container) return;

    const containerRect = container.getBoundingClientRect();
    const positions = navigationItems.flatMap((item) => {
      const element = document.getElementById(getChatMessageElementId(item.id));
      if (!element || !container.contains(element)) return [];
      const rect = element.getBoundingClientRect();
      return [{ id: item.id, top: rect.top, height: rect.height }];
    });
    const readingLine =
      containerRect.top +
      container.clientHeight * CHAT_NAVIGATION_READING_LINE_RATIO;
    const nextActiveId = resolveActiveChatNavigationMessageId(
      positions,
      readingLine,
    );
    const distanceFromBottom = Math.max(
      0,
      container.scrollHeight - container.scrollTop - container.clientHeight,
    );

    setActiveMessageId((current) =>
      current === nextActiveId ? current : nextActiveId,
    );
    setIsAtTop(container.scrollTop <= EDGE_THRESHOLD_PX);
    setIsAtBottom(distanceFromBottom <= EDGE_THRESHOLD_PX);
  }, [navigationItems, scrollContainerRef]);

  const scheduleNavigationUpdate = useCallback(() => {
    if (frameRef.current !== null) return;
    frameRef.current = window.requestAnimationFrame(() => {
      frameRef.current = null;
      updateNavigationState();
    });
  }, [updateNavigationState]);

  useEffect(() => {
    const mediaQuery = window.matchMedia(DESKTOP_NAVIGATION_QUERY);
    const updateDesktopNavigation = () =>
      setIsDesktopNavigation(mediaQuery.matches);
    updateDesktopNavigation();
    mediaQuery.addEventListener("change", updateDesktopNavigation);
    return () =>
      mediaQuery.removeEventListener("change", updateDesktopNavigation);
  }, []);

  useEffect(() => {
    if (!isDesktopNavigation) return;
    const container = scrollContainerRef.current;
    if (!container) return;

    scheduleNavigationUpdate();
    container.addEventListener("scroll", scheduleNavigationUpdate, {
      passive: true,
    });

    const content = container.firstElementChild;
    const resizeObserver =
      typeof ResizeObserver === "undefined"
        ? null
        : new ResizeObserver(scheduleNavigationUpdate);
    resizeObserver?.observe(container);
    if (content instanceof HTMLElement) resizeObserver?.observe(content);

    return () => {
      container.removeEventListener("scroll", scheduleNavigationUpdate);
      resizeObserver?.disconnect();
      if (frameRef.current !== null) {
        window.cancelAnimationFrame(frameRef.current);
        frameRef.current = null;
      }
    };
  }, [
    isDesktopNavigation,
    renderStartIndex,
    scheduleNavigationUpdate,
    scrollContainerRef,
  ]);

  const showNavigationPreview = useCallback(
    (element: HTMLButtonElement, key: string, label: string) => {
      const navigator = navigatorRef.current;
      if (!navigator) return;
      const navigatorRect = navigator.getBoundingClientRect();
      const elementRect = element.getBoundingClientRect();
      setNavigationPreview({
        key,
        label,
        top: elementRect.top - navigatorRect.top + elementRect.height / 2,
      });
    },
    [],
  );

  const hideNavigationPreview = useCallback((key: string) => {
    setNavigationPreview((current) => (current?.key === key ? null : current));
  }, []);

  if (!isDesktopNavigation || navigationItems.length === 0) return null;

  return (
    <nav
      ref={navigatorRef}
      aria-label={t("userMessageNavigation")}
      onMouseLeave={() => setNavigationPreview(null)}
      className="chat-message-navigator absolute right-4 top-1/2 z-20 hidden max-h-[70vh] w-11 -translate-y-1/2 flex-col overflow-visible rounded-2xl bg-transparent py-1 text-sm text-muted-foreground lg:flex"
    >
      {navigationPreview && (
        <div
          data-chat-navigation-preview
          aria-hidden="true"
          style={{ top: navigationPreview.top }}
          className="pointer-events-none absolute right-full z-10 mr-2 max-w-72 -translate-y-1/2 truncate whitespace-nowrap rounded-lg border border-border bg-background px-3 py-1.5 text-sm text-foreground shadow-md"
        >
          {navigationPreview.label}
        </div>
      )}

      <button
        type="button"
        aria-label={t("jumpToConversationTop")}
        onClick={onJumpTop}
        onMouseEnter={(event) =>
          showNavigationPreview(
            event.currentTarget,
            "top",
            t("conversationTop"),
          )
        }
        onMouseLeave={() => hideNavigationPreview("top")}
        onFocus={(event) =>
          showNavigationPreview(
            event.currentTarget,
            "top",
            t("conversationTop"),
          )
        }
        onBlur={() => hideNavigationPreview("top")}
        className={`flex h-9 w-full shrink-0 items-center justify-end gap-3 rounded-lg px-3 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60 ${
          isAtTop ? "text-blue-500" : "hover:bg-muted hover:text-foreground"
        }`}
      >
        <ArrowUpToLine size={17} className="shrink-0" aria-hidden="true" />
      </button>

      <div
        onScroll={() => setNavigationPreview(null)}
        className="chat-message-nav-list min-h-0 flex-1 overflow-y-auto overscroll-contain py-1"
      >
        {navigationItems.map((item, index) => {
          const isActive = item.id === activeMessageId;
          return (
            <button
              key={item.id}
              type="button"
              data-message-id={item.id}
              aria-label={t("jumpToUserMessage", {
                index: index + 1,
                message: item.label,
              })}
              aria-current={isActive ? "location" : undefined}
              onClick={() => onNavigateMessage(item.id)}
              onMouseEnter={(event) =>
                showNavigationPreview(event.currentTarget, item.id, item.label)
              }
              onMouseLeave={() => hideNavigationPreview(item.id)}
              onFocus={(event) =>
                showNavigationPreview(event.currentTarget, item.id, item.label)
              }
              onBlur={() => hideNavigationPreview(item.id)}
              className={`flex min-h-8 w-full items-center justify-end gap-3 rounded-lg px-3 py-1 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60 ${
                isActive
                  ? "text-blue-500"
                  : "hover:bg-muted hover:text-foreground"
              }`}
            >
              <span
                className={`h-1.5 shrink-0 rounded-full ${
                  isActive ? "w-7 bg-blue-500" : "w-4 bg-border"
                }`}
                aria-hidden="true"
              />
            </button>
          );
        })}
      </div>

      <button
        type="button"
        aria-label={t("jumpToConversationBottom")}
        onClick={onJumpBottom}
        onMouseEnter={(event) =>
          showNavigationPreview(
            event.currentTarget,
            "bottom",
            t("conversationBottom"),
          )
        }
        onMouseLeave={() => hideNavigationPreview("bottom")}
        onFocus={(event) =>
          showNavigationPreview(
            event.currentTarget,
            "bottom",
            t("conversationBottom"),
          )
        }
        onBlur={() => hideNavigationPreview("bottom")}
        className={`flex h-9 w-full shrink-0 items-center justify-end gap-3 rounded-lg px-3 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60 ${
          isAtBottom ? "text-blue-500" : "hover:bg-muted hover:text-foreground"
        }`}
      >
        <ArrowDownToLine size={17} className="shrink-0" aria-hidden="true" />
      </button>
    </nav>
  );
};

export default ChatMessageNavigator;
