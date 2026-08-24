import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

describe("Sidebar composition", () => {
  it("keeps search/filter UI in a dedicated component", () => {
    const sidebar = readFileSync(
      resolve(process.cwd(), "src/components/layout/Sidebar.tsx"),
      "utf8",
    );
    const sidebarSearch = readFileSync(
      resolve(process.cwd(), "src/components/layout/SidebarSearch.tsx"),
      "utf8",
    );

    expect(sidebar).toContain("SidebarSearch");
    expect(sidebar).toContain("WORKSPACE_SESSION_PREVIEW_LIMIT = 5");
    expect(sidebar).toContain("TEMPORARY_SESSION_PREVIEW_LIMIT = 5");
    expect(sidebar).toContain("expandedWorkspaceSessionLists");
    expect(sidebar).toContain("temporarySessionListExpanded");
    expect(sidebar).not.toContain("expandedRootSessionLists");
    expect(sidebar).toContain("isSearchingChats");
    expect(sidebar).toContain("renderShowAllButton");
    expect(sidebar).toContain('t("temporaryChats")');
    expect(sidebar).toContain('t("newTemporaryChat")');
    expect(sidebar).toContain("onNewTemporaryChat: () => void");
    expect(sidebar).toContain(
      "onNewChatInWorkspace: (workspace: Workspace) => void",
    );
    expect(sidebar).toContain("onClick={onNewTemporaryChat}");
    expect(sidebar).toContain("if (ws) onNewChatInWorkspace(ws)");
    expect(sidebar).not.toContain('t("chatList")');
    expect(sidebar).not.toContain("rootSessions");
    expect(sidebar).toContain("PanelLeftOpen");
    expect(sidebar).toContain("PanelLeftClose");
    expect(sidebar).not.toContain('name="sidebar-chat-search"');
    expect(sidebarSearch).toContain('name="sidebar-chat-search"');
    expect(sidebarSearch).toContain("onCollapsedSearchClick");
  });

  it("shows the active Workspace and Conversation as one breadcrumb", () => {
    const chatApp = readFileSync(
      resolve(process.cwd(), "src/components/app/ChatApp.tsx"),
      "utf8",
    );

    expect(chatApp).toContain("currentHostWorkspace.name");
    expect(chatApp).toContain("<ChevronRight");
    expect(chatApp).toContain('currentSession?.title || t("newChat")');
    expect(chatApp).toContain("void createNewChat(currentHostWorkspace)");
    expect(chatApp).toContain("void createNewChat()");
    expect(chatApp).toContain("activeLocalTimingRef");
    expect(chatApp).toContain("botMsg.timestamp = startTime");
    expect(chatApp).toContain("modelPlaceholder.timestamp = startTime");
    expect(chatApp).toContain(
      "duration: Math.max(0, endTime - activeTiming.startTime)",
    );
  });

  it("defaults workspace chat lists to collapsed while search expands matches", () => {
    const source = readFileSync(
      resolve(process.cwd(), "src/components/layout/Sidebar.tsx"),
      "utf8",
    );

    expect(source).toContain("newExpanded[w.id] = false");
    expect(source).toContain("isSearchingChats || expandedSections[ws.id]");
    expect(source).not.toContain("newExpanded[w.id] = true");
  });

  it("keeps expanded navigation direct and collapsed tooltips immediate", () => {
    const source = readFileSync(
      resolve(process.cwd(), "src/components/layout/Sidebar.tsx"),
      "utf8",
    );

    expect(source).toContain("const SidebarNavTooltip");
    expect(source).toContain("if (isOpen) return children");
    expect(source).toContain('motion="instant"');
    expect(source).toContain('surface="solid"');
    expect(source.match(/^\s*<SidebarNavTooltip /gm)).toHaveLength(5);
    expect(source).toContain("onOpenSkillStore: () => void");
    expect(source).toContain("onOpenTools: () => void");
    expect(source).toContain('content={t("tools")}');
    expect(source).toContain('aria-current={isToolsOpen ? "page" : undefined}');
    expect(source).not.toContain(
      "text-sm font-medium transition-[color,background-color] focus-visible",
    );
  });

  it("keeps conversation hover and action reveal immediate", () => {
    const source = readFileSync(
      resolve(process.cwd(), "src/components/layout/Sidebar.tsx"),
      "utf8",
    );

    expect(source).not.toContain(
      "text-sm transition-[color,background-color] duration-200",
    );
    expect(source).not.toContain(
      "opacity-100 transition-[opacity,background-color]",
    );
  });
});
