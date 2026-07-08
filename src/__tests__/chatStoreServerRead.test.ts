import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Message, Session } from "../types";

const mocks = vi.hoisted(() => {
  const storedItems = new Map<string, unknown>();
  const appDbMock = {
    getItem: vi.fn((key: string) => Promise.resolve(storedItems.get(key))),
    setItem: vi.fn((key: string, value: unknown) => {
      storedItems.set(key, value);
      return Promise.resolve(value);
    }),
    removeItem: vi.fn((key: string) => {
      storedItems.delete(key);
      return Promise.resolve();
    }),
  };
  const serverService = {
    serverEnabled: true,
    listConversations: vi.fn(),
    listMessages: vi.fn(),
  };

  return {
    appDbMock,
    storedItems,
    deleteFromOPFSMock: vi.fn(() => Promise.resolve()),
    createChatCrudService: vi.fn(() => serverService),
    serverService,
  };
});

vi.mock("uuid", () => ({
  v7: vi.fn(() => "00000000-0000-7000-8000-000000000001"),
}));

vi.mock("zustand", () => ({
  create: () => (initializer: any) => {
    let state: any;
    const initialStateRef: { value: any } = { value: undefined };

    const store: any = (selector?: (value: any) => unknown) =>
      selector ? selector(state) : state;
    const setState = (partial: any) => {
      const patch =
        typeof partial === "function" ? partial(state) : (partial ?? {});
      state = { ...state, ...patch };
    };
    const getState = () => state;
    const api = {
      setState,
      getState,
      getInitialState: () => initialStateRef.value,
      persist: { getOptions: () => ({}) },
    };

    state = initializer(setState, getState, api);
    initialStateRef.value = state;

    store.setState = setState;
    store.getState = getState;
    store.getInitialState = api.getInitialState;
    store.persist = api.persist;

    return store;
  },
}));

vi.mock("zustand/middleware", () => ({
  createJSONStorage: () => ({}),
  persist:
    (initializer: any, options: any) => (set: any, get: any, api: any) => {
      const state = initializer(set, get, api);
      api.persist = { getOptions: () => options };
      return state;
    },
}));

vi.mock("@/utils/opfs", () => ({
  deleteFromOPFS: mocks.deleteFromOPFSMock,
}));

vi.mock("../store/storage/storageConfig", () => ({
  appDb: mocks.appDbMock,
  getAppDbStorage: () => ({
    getItem: () => null,
    setItem: () => undefined,
    removeItem: () => undefined,
  }),
  STORAGE_KEYS: {
    CHAT: "neo-chat-storage",
  },
  STORAGE_VERSION: 2,
}));

vi.mock("../services/api/chatCrudService", () => ({
  createChatCrudService: mocks.createChatCrudService,
}));

const { normalizeSessionMessageTree } = await import("../lib/chat/messageTree");
const { useChatStore } = await import("../store/core/chatStore");

const makeServerSession = (id: string): Session => ({
  id,
  title: `Server ${id}`,
  updatedAt: Date.parse("2026-07-08T00:00:00Z"),
  model: "openai:gpt-5.5",
  pinned: false,
  messageCount: 1,
});

const makeMessage = (id: string, role: Message["role"]): Message => ({
  id,
  role,
  content: `${role} content`,
  timestamp: Date.parse("2026-07-08T00:00:01Z"),
});

const makeEmptyServerReadState = () => ({
  sessions: [],
  currentSessionId: null,
  activeMessages: [],
  activeMessageTree: normalizeSessionMessageTree([]),
  isLoading: false,
  error: null,
});

describe("chat store server read path", () => {
  beforeEach(() => {
    mocks.storedItems.clear();
    vi.clearAllMocks();
    mocks.createChatCrudService.mockReturnValue(mocks.serverService);
    mocks.serverService.serverEnabled = true;
    mocks.serverService.listConversations.mockResolvedValue([
      makeServerSession("c1"),
      makeServerSession("c2"),
    ]);
    mocks.serverService.listMessages.mockResolvedValue([
      makeMessage("m1", "user"),
      makeMessage("m2", "model"),
    ]);

    useChatStore.setState({
      _hasHydrated: true,
      sessions: [],
      workspaces: [],
      currentSessionId: null,
      activeMessages: [],
      activeMessageTree: normalizeSessionMessageTree([]),
      isActiveSessionLoading: false,
      serverReadState: makeEmptyServerReadState(),
      selectedModel: "openai:gpt-5.5",
      chatConfig: {
        useSearch: false,
        useReasoning: false,
        temperature: 0.7,
      },
    });
  });

  it("loads server conversations into a non-persisted snapshot", async () => {
    const localSession = makeServerSession("local");
    const localMessage = makeMessage("local-m1", "user");
    useChatStore.setState({
      sessions: [localSession],
      currentSessionId: "local",
      activeMessages: [localMessage],
      activeMessageTree: normalizeSessionMessageTree([localMessage]),
    });

    await expect(useChatStore.getState().refreshServerSessions()).resolves.toBe(
      true,
    );

    const state = useChatStore.getState();
    expect(state.serverReadState.sessions.map((session) => session.id)).toEqual(
      ["c1", "c2"],
    );
    expect(state.serverReadState.currentSessionId).toBe("c1");
    expect(
      state.serverReadState.activeMessages.map((message) => message.id),
    ).toEqual(["m1", "m2"]);
    expect(state.serverReadState.isLoading).toBe(false);
    expect(state.serverReadState.error).toBeNull();

    expect(state.sessions).toEqual([localSession]);
    expect(state.currentSessionId).toBe("local");
    expect(state.activeMessages).toEqual([localMessage]);
    expect(state.isActiveSessionLoading).toBe(false);

    expect(mocks.serverService.listConversations).toHaveBeenCalledTimes(1);
    expect(mocks.serverService.listMessages).toHaveBeenCalledWith("c1");
    expect(mocks.appDbMock.getItem).not.toHaveBeenCalled();
    expect(mocks.appDbMock.setItem).not.toHaveBeenCalled();
  });

  it("selects server messages without changing local chat state", async () => {
    const localMessage = makeMessage("local-m1", "user");
    useChatStore.setState({
      sessions: [makeServerSession("local")],
      currentSessionId: "local",
      activeMessages: [localMessage],
      activeMessageTree: normalizeSessionMessageTree([localMessage]),
      serverReadState: {
        ...makeEmptyServerReadState(),
        sessions: [makeServerSession("c1")],
      },
    });

    await expect(
      useChatStore.getState().selectServerSession("c1"),
    ).resolves.toBe(true);

    const state = useChatStore.getState();
    expect(state.serverReadState.currentSessionId).toBe("c1");
    expect(state.serverReadState.activeMessages).toHaveLength(2);
    expect(state.serverReadState.sessions[0]?.messageCount).toBe(2);
    expect(state.currentSessionId).toBe("local");
    expect(state.activeMessages).toEqual([localMessage]);
    expect(mocks.serverService.listConversations).not.toHaveBeenCalled();
    expect(mocks.serverService.listMessages).toHaveBeenCalledWith("c1");
    expect(mocks.appDbMock.getItem).not.toHaveBeenCalled();
    expect(mocks.appDbMock.setItem).not.toHaveBeenCalled();
  });

  it("does not call server or local storage when server CRUD is disabled", async () => {
    mocks.serverService.serverEnabled = false;

    await expect(useChatStore.getState().refreshServerSessions()).resolves.toBe(
      false,
    );
    await expect(
      useChatStore.getState().selectServerSession("c1"),
    ).resolves.toBe(false);

    expect(useChatStore.getState().serverReadState).toEqual(
      makeEmptyServerReadState(),
    );
    expect(mocks.serverService.listConversations).not.toHaveBeenCalled();
    expect(mocks.serverService.listMessages).not.toHaveBeenCalled();
    expect(mocks.appDbMock.getItem).not.toHaveBeenCalled();
    expect(mocks.appDbMock.setItem).not.toHaveBeenCalled();
  });

  it("keeps the latest server selection when stale reads finish late", async () => {
    let resolveFirst!: (messages: Message[]) => void;
    const firstMessages = new Promise<Message[]>((resolve) => {
      resolveFirst = resolve;
    });
    mocks.serverService.listMessages
      .mockReturnValueOnce(firstMessages)
      .mockResolvedValueOnce([makeMessage("m3", "user")]);

    useChatStore.setState({
      serverReadState: {
        ...makeEmptyServerReadState(),
        sessions: [makeServerSession("c1"), makeServerSession("c2")],
      },
    });

    const first = useChatStore.getState().selectServerSession("c1");
    const second = useChatStore.getState().selectServerSession("c2");

    await expect(second).resolves.toBe(true);
    resolveFirst([makeMessage("m1", "user")]);
    await expect(first).resolves.toBe(false);

    const state = useChatStore.getState().serverReadState;
    expect(state.currentSessionId).toBe("c2");
    expect(state.activeMessages.map((message) => message.id)).toEqual(["m3"]);
    expect(state.isLoading).toBe(false);
    expect(mocks.serverService.listMessages).toHaveBeenNthCalledWith(1, "c1");
    expect(mocks.serverService.listMessages).toHaveBeenNthCalledWith(2, "c2");
  });

  it("excludes server read state from persisted chat metadata", () => {
    useChatStore.setState({
      serverReadState: {
        ...makeEmptyServerReadState(),
        sessions: [makeServerSession("c1")],
        currentSessionId: "c1",
        activeMessages: [makeMessage("m1", "user")],
      },
    });

    const partialize = useChatStore.persist.getOptions().partialize;
    expect(partialize).toEqual(expect.any(Function));
    if (!partialize) throw new Error("Expected chat store partialize option.");

    const persisted = partialize(useChatStore.getState()) as Partial<
      ReturnType<typeof useChatStore.getState>
    >;

    expect(persisted.serverReadState).toBeUndefined();
    expect(persisted.sessions).toEqual([]);
    expect(persisted.currentSessionId).toBeNull();
  });
});
