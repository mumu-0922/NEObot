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
    createConversation: vi.fn(),
    listConversations: vi.fn(),
    appendUserMessage: vi.fn(),
    listMessages: vi.fn(),
  };
  const streamService = {
    streamEnabled: true,
    streamAssistantMessage: vi.fn(),
  };

  return {
    appDbMock,
    storedItems,
    deleteFromOPFSMock: vi.fn(() => Promise.resolve()),
    createChatCrudService: vi.fn(() => serverService),
    createChatStreamService: vi.fn(() => streamService),
    serverService,
    streamService,
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
  modelStringToModelRef: (model: string) => {
    const [providerId, modelId] = model.split(":");
    return providerId && modelId ? { providerId, modelId } : undefined;
  },
}));

vi.mock("../services/api/chatStreamService", () => ({
  createChatStreamService: mocks.createChatStreamService,
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
  generation: {
    status: "idle" as const,
    sessionId: null,
    userMessageId: null,
    assistantMessageId: null,
    activeServerRunId: null,
    error: null,
  },
  isLoading: false,
  error: null,
});

describe("chat store server read path", () => {
  beforeEach(() => {
    mocks.storedItems.clear();
    vi.clearAllMocks();
    mocks.createChatCrudService.mockReturnValue(mocks.serverService);
    mocks.createChatStreamService.mockReturnValue(mocks.streamService);
    mocks.serverService.serverEnabled = true;
    mocks.streamService.streamEnabled = true;
    mocks.serverService.listConversations.mockResolvedValue([
      makeServerSession("c1"),
      makeServerSession("c2"),
    ]);
    mocks.serverService.createConversation.mockResolvedValue(
      makeServerSession("c3"),
    );
    mocks.serverService.appendUserMessage.mockResolvedValue(
      makeMessage("m3", "user"),
    );
    mocks.streamService.streamAssistantMessage.mockResolvedValue({
      status: "completed",
      message: makeMessage("m4", "model"),
    });
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
    expect(mocks.serverService.createConversation).not.toHaveBeenCalled();
    expect(mocks.serverService.appendUserMessage).not.toHaveBeenCalled();
    expect(mocks.streamService.streamAssistantMessage).not.toHaveBeenCalled();
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
        generation: {
          status: "streaming",
          sessionId: "c1",
          userMessageId: "m1",
          assistantMessageId: "m2",
          activeServerRunId: "run-persist-guard",
          error: {
            code: "PROVIDER_ERROR",
            message: "provider failed",
            recoverable: true,
            requestId: "req-persist-guard",
          },
        },
      },
    });

    const partialize = useChatStore.persist.getOptions().partialize;
    expect(partialize).toEqual(expect.any(Function));
    if (!partialize) throw new Error("Expected chat store partialize option.");

    const persisted = partialize(useChatStore.getState()) as Partial<
      ReturnType<typeof useChatStore.getState>
    >;

    expect(persisted.serverReadState).toBeUndefined();
    expect(JSON.stringify(persisted)).not.toContain("run-persist-guard");
    expect(JSON.stringify(persisted)).not.toContain("req-persist-guard");
    expect(persisted.sessions).toEqual([]);
    expect(persisted.currentSessionId).toBeNull();
  });

  it("creates server conversations without changing local chat state", async () => {
    const localSession = makeServerSession("local");
    useChatStore.setState({
      sessions: [localSession],
      currentSessionId: "local",
      selectedModel: "openai:gpt-5.5",
    });

    await expect(
      useChatStore.getState().createServerSession({
        title: "  Server Draft  ",
        config: { useSearch: true },
      }),
    ).resolves.toBe("c3");

    const state = useChatStore.getState();
    expect(state.serverReadState.currentSessionId).toBe("c3");
    expect(state.serverReadState.sessions.map((session) => session.id)).toEqual(
      ["c3"],
    );
    expect(state.serverReadState.activeMessages).toEqual([]);
    expect(state.serverReadState.isLoading).toBe(false);
    expect(state.sessions).toEqual([localSession]);
    expect(state.currentSessionId).toBe("local");
    expect(mocks.serverService.createConversation).toHaveBeenCalledWith({
      title: "Server Draft",
      modelRef: { providerId: "openai", modelId: "gpt-5.5" },
      systemInstruction: undefined,
      config: { useSearch: true },
      idempotencyKey: "00000000-0000-7000-8000-000000000001",
    });
    expect(mocks.appDbMock.setItem).not.toHaveBeenCalled();
  });

  it("appends server user messages to the server snapshot only", async () => {
    const localMessage = makeMessage("local-m1", "user");
    useChatStore.setState({
      currentSessionId: "local",
      activeMessages: [localMessage],
      activeMessageTree: normalizeSessionMessageTree([localMessage]),
      serverReadState: {
        ...makeEmptyServerReadState(),
        sessions: [makeServerSession("c1")],
        currentSessionId: "c1",
      },
    });

    await expect(
      useChatStore.getState().appendServerUserMessage({
        sessionId: "c1",
        content: "hello",
        metadata: { source: "test" },
      }),
    ).resolves.toMatchObject({ id: "m3", role: "user" });

    const state = useChatStore.getState();
    expect(
      state.serverReadState.activeMessages.map((message) => message.id),
    ).toEqual(["m3"]);
    expect(state.serverReadState.sessions[0]?.messageCount).toBe(2);
    expect(state.currentSessionId).toBe("local");
    expect(state.activeMessages).toEqual([localMessage]);
    expect(mocks.serverService.appendUserMessage).toHaveBeenCalledWith({
      conversationId: "c1",
      content: "hello",
      parentMessageId: undefined,
      attachments: undefined,
      metadata: { source: "test" },
      idempotencyKey: "00000000-0000-7000-8000-000000000001",
    });
    expect(mocks.appDbMock.getItem).not.toHaveBeenCalled();
    expect(mocks.appDbMock.setItem).not.toHaveBeenCalled();
  });

  it("does not duplicate idempotent server append results", async () => {
    const existingMessage = makeMessage("m3", "user");
    mocks.serverService.appendUserMessage.mockResolvedValueOnce({
      ...existingMessage,
      content: "updated user content",
    });

    useChatStore.setState({
      serverReadState: {
        ...makeEmptyServerReadState(),
        sessions: [makeServerSession("c1")],
        currentSessionId: "c1",
        activeMessages: [existingMessage],
        activeMessageTree: normalizeSessionMessageTree([existingMessage]),
      },
    });

    await expect(
      useChatStore.getState().appendServerUserMessage({
        sessionId: "c1",
        content: "hello",
        idempotencyKey: "same-message-key",
      }),
    ).resolves.toMatchObject({
      id: "m3",
      content: "updated user content",
    });

    const state = useChatStore.getState();
    expect(
      state.serverReadState.activeMessages.map((message) => message.id),
    ).toEqual(["m3"]);
    expect(state.serverReadState.activeMessages[0]?.content).toBe(
      "updated user content",
    );
    expect(state.serverReadState.sessions[0]?.messageCount).toBe(1);
    expect(mocks.serverService.appendUserMessage).toHaveBeenCalledWith({
      conversationId: "c1",
      content: "hello",
      parentMessageId: undefined,
      attachments: undefined,
      metadata: undefined,
      idempotencyKey: "same-message-key",
    });
    expect(mocks.appDbMock.setItem).not.toHaveBeenCalled();
  });

  it("returns stale server-created ids without overwriting the latest snapshot", async () => {
    let resolveFirst!: (session: Session) => void;
    const firstCreate = new Promise<Session>((resolve) => {
      resolveFirst = resolve;
    });
    mocks.serverService.createConversation
      .mockReturnValueOnce(firstCreate)
      .mockResolvedValueOnce(makeServerSession("c-latest"));

    const first = useChatStore
      .getState()
      .createServerSession({ title: "first" });
    const second = useChatStore
      .getState()
      .createServerSession({ title: "second" });

    await expect(second).resolves.toBe("c-latest");
    resolveFirst(makeServerSession("c-stale"));
    await expect(first).resolves.toBe("c-stale");

    const state = useChatStore.getState().serverReadState;
    expect(state.currentSessionId).toBe("c-latest");
    expect(state.sessions.map((session) => session.id)).toEqual(["c-latest"]);
    expect(state.isLoading).toBe(false);
  });

  it("returns stale appended messages without overwriting the latest snapshot", async () => {
    let resolveFirst!: (message: Message) => void;
    const firstAppend = new Promise<Message>((resolve) => {
      resolveFirst = resolve;
    });
    mocks.serverService.appendUserMessage
      .mockReturnValueOnce(firstAppend)
      .mockResolvedValueOnce(makeMessage("m-latest", "user"));

    useChatStore.setState({
      serverReadState: {
        ...makeEmptyServerReadState(),
        sessions: [{ ...makeServerSession("c1"), messageCount: 0 }],
        currentSessionId: "c1",
      },
    });

    const first = useChatStore.getState().appendServerUserMessage({
      sessionId: "c1",
      content: "stale",
    });
    const second = useChatStore.getState().appendServerUserMessage({
      sessionId: "c1",
      content: "latest",
    });

    await expect(second).resolves.toMatchObject({ id: "m-latest" });
    resolveFirst(makeMessage("m-stale", "user"));
    await expect(first).resolves.toMatchObject({ id: "m-stale" });

    const state = useChatStore.getState().serverReadState;
    expect(state.activeMessages.map((message) => message.id)).toEqual([
      "m-latest",
    ]);
    expect(state.sessions[0]?.messageCount).toBe(1);
    expect(state.isLoading).toBe(false);
  });

  it("does not write server conversations or messages when server CRUD is disabled", async () => {
    mocks.serverService.serverEnabled = false;

    await expect(
      useChatStore.getState().createServerSession(),
    ).resolves.toBeNull();
    await expect(
      useChatStore.getState().appendServerUserMessage({
        sessionId: "c1",
        content: "hello",
      }),
    ).resolves.toBeNull();
    await expect(
      useChatStore.getState().sendServerMessageAndStream({
        sessionId: "c1",
        content: "hello",
      }),
    ).resolves.toBeNull();

    expect(useChatStore.getState().serverReadState).toEqual(
      makeEmptyServerReadState(),
    );
    expect(mocks.serverService.createConversation).not.toHaveBeenCalled();
    expect(mocks.serverService.appendUserMessage).not.toHaveBeenCalled();
    expect(mocks.streamService.streamAssistantMessage).not.toHaveBeenCalled();
    expect(mocks.appDbMock.getItem).not.toHaveBeenCalled();
    expect(mocks.appDbMock.setItem).not.toHaveBeenCalled();
  });

  it("sends a server user message and streams the assistant into the snapshot only", async () => {
    const localMessage = makeMessage("local-m1", "user");
    const generationSnapshots: unknown[] = [];
    mocks.streamService.streamAssistantMessage.mockImplementation(
      async (_input: unknown, handlers?: any) => {
        handlers?.onStarted?.({
          type: "message.started",
          runId: "run-1",
          conversationId: "c1",
          messageId: "m4",
          createdAt: "2026-07-08T00:00:02Z",
        });
        generationSnapshots.push(
          useChatStore.getState().serverReadState.generation,
        );
        handlers?.onDelta?.({
          type: "message.delta",
          runId: "run-1",
          conversationId: "c1",
          messageId: "m4",
          delta: "hel",
        });
        handlers?.onDelta?.({
          type: "message.delta",
          runId: "run-1",
          conversationId: "c1",
          messageId: "m4",
          delta: "lo",
        });
        return {
          status: "completed",
          message: {
            ...makeMessage("m4", "model"),
            content: "hello",
          },
        };
      },
    );

    useChatStore.setState({
      selectedModel: "openai:gpt-5.5",
      currentSessionId: "local",
      activeMessages: [localMessage],
      activeMessageTree: normalizeSessionMessageTree([localMessage]),
      serverReadState: {
        ...makeEmptyServerReadState(),
        sessions: [{ ...makeServerSession("c1"), messageCount: 0 }],
        currentSessionId: "c1",
      },
    });

    await expect(
      useChatStore.getState().sendServerMessageAndStream({
        sessionId: "c1",
        content: "hello user",
        metadata: { source: "input" },
        streamMetadata: { source: "stream" },
        config: { useSearch: true },
        userMessageIdempotencyKey: "user-key",
        streamIdempotencyKey: "stream-key",
      }),
    ).resolves.toMatchObject({ status: "completed" });

    const state = useChatStore.getState();
    expect(
      state.serverReadState.activeMessages.map((message) => [
        message.id,
        message.role,
        message.content,
      ]),
    ).toEqual([
      ["m3", "user", "user content"],
      ["m4", "model", "hello"],
    ]);
    expect(state.serverReadState.sessions[0]?.messageCount).toBe(2);
    expect(state.serverReadState.isLoading).toBe(false);
    expect(state.serverReadState.error).toBeNull();
    expect(generationSnapshots).toEqual([
      {
        status: "streaming",
        sessionId: "c1",
        userMessageId: "m3",
        assistantMessageId: "m4",
        activeServerRunId: "run-1",
        error: null,
      },
    ]);
    expect(state.serverReadState.generation).toEqual({
      status: "completed",
      sessionId: "c1",
      userMessageId: "m3",
      assistantMessageId: "m4",
      activeServerRunId: null,
      error: null,
    });
    expect(state.currentSessionId).toBe("local");
    expect(state.activeMessages).toEqual([localMessage]);
    expect(mocks.serverService.appendUserMessage).toHaveBeenCalledWith({
      conversationId: "c1",
      content: "hello user",
      parentMessageId: undefined,
      attachments: undefined,
      metadata: { source: "input" },
      idempotencyKey: "user-key",
    });
    expect(mocks.streamService.streamAssistantMessage).toHaveBeenCalledWith(
      {
        conversationId: "c1",
        userMessageId: "m3",
        modelRef: { providerId: "openai", modelId: "gpt-5.5" },
        config: { useSearch: true },
        systemInstruction: undefined,
        systemPrompt: undefined,
        metadata: { source: "stream" },
        idempotencyKey: "stream-key",
        signal: undefined,
      },
      expect.objectContaining({
        onStarted: expect.any(Function),
        onDelta: expect.any(Function),
      }),
    );
    expect(mocks.appDbMock.getItem).not.toHaveBeenCalled();
    expect(mocks.appDbMock.setItem).not.toHaveBeenCalled();
  });

  it("maps server stream provider errors to terminal generation state", async () => {
    mocks.streamService.streamAssistantMessage.mockImplementation(
      async (_input: unknown, handlers?: any) => {
        handlers?.onStarted?.({
          type: "message.started",
          runId: "run-error",
          conversationId: "c1",
          messageId: "m4",
        });
        handlers?.onDelta?.({
          type: "message.delta",
          runId: "run-error",
          conversationId: "c1",
          messageId: "m4",
          delta: "partial",
        });
        return {
          status: "failed",
          error: {
            code: "PROVIDER_ERROR",
            message: "provider failed",
            recoverable: true,
          },
        };
      },
    );

    useChatStore.setState({
      selectedModel: "openai:gpt-5.5",
      serverReadState: {
        ...makeEmptyServerReadState(),
        sessions: [{ ...makeServerSession("c1"), messageCount: 0 }],
        currentSessionId: "c1",
      },
    });

    await expect(
      useChatStore.getState().sendServerMessageAndStream({
        sessionId: "c1",
        content: "hello user",
      }),
    ).resolves.toMatchObject({
      status: "failed",
      error: {
        code: "PROVIDER_ERROR",
        message: "provider failed",
        recoverable: true,
      },
    });

    const state = useChatStore.getState().serverReadState;
    expect(state.activeMessages.map((message) => message.id)).toEqual([
      "m3",
      "m4",
    ]);
    expect(state.activeMessages[1]).toMatchObject({
      id: "m4",
      role: "model",
      content: "partial",
    });
    expect(state.generation).toEqual({
      status: "failed",
      sessionId: "c1",
      userMessageId: "m3",
      assistantMessageId: "m4",
      activeServerRunId: null,
      error: {
        code: "PROVIDER_ERROR",
        message: "provider failed",
        recoverable: true,
      },
    });
    expect(state.error).toBe("provider failed");
    expect(mocks.appDbMock.setItem).not.toHaveBeenCalled();
  });

  it("maps unsupported server stream results to failed generation state", async () => {
    mocks.streamService.streamAssistantMessage.mockImplementation(
      async (_input: unknown, handlers?: any) => {
        handlers?.onStarted?.({
          type: "message.started",
          runId: "run-unsupported",
          conversationId: "c1",
          messageId: "m4",
        });
        return { status: "unsupported" };
      },
    );

    useChatStore.setState({
      selectedModel: "openai:gpt-5.5",
      serverReadState: {
        ...makeEmptyServerReadState(),
        sessions: [{ ...makeServerSession("c1"), messageCount: 0 }],
        currentSessionId: "c1",
      },
    });

    await expect(
      useChatStore.getState().sendServerMessageAndStream({
        sessionId: "c1",
        content: "hello user",
      }),
    ).resolves.toMatchObject({ status: "unsupported" });

    const state = useChatStore.getState().serverReadState;
    expect(state.activeMessages.map((message) => message.id)).toEqual([
      "m3",
      "m4",
    ]);
    expect(state.generation).toEqual({
      status: "failed",
      sessionId: "c1",
      userMessageId: "m3",
      assistantMessageId: "m4",
      activeServerRunId: null,
      error: { message: "Server stream failed." },
    });
    expect(state.error).toBe("Server stream failed.");
    expect(mocks.appDbMock.setItem).not.toHaveBeenCalled();
  });

  it("maps server stream cancellation to terminal generation state", async () => {
    mocks.streamService.streamAssistantMessage.mockImplementation(
      async (_input: unknown, handlers?: any) => {
        handlers?.onStarted?.({
          type: "message.started",
          runId: "run-cancel",
          conversationId: "c1",
          messageId: "m4",
        });
        return { status: "cancelled" };
      },
    );

    useChatStore.setState({
      selectedModel: "openai:gpt-5.5",
      serverReadState: {
        ...makeEmptyServerReadState(),
        sessions: [{ ...makeServerSession("c1"), messageCount: 0 }],
        currentSessionId: "c1",
      },
    });

    await expect(
      useChatStore.getState().sendServerMessageAndStream({
        sessionId: "c1",
        content: "hello user",
      }),
    ).resolves.toMatchObject({ status: "cancelled" });

    const state = useChatStore.getState().serverReadState;
    expect(state.activeMessages.map((message) => message.id)).toEqual([
      "m3",
      "m4",
    ]);
    expect(state.generation).toEqual({
      status: "cancelled",
      sessionId: "c1",
      userMessageId: "m3",
      assistantMessageId: "m4",
      activeServerRunId: null,
      error: null,
    });
    expect(state.isLoading).toBe(false);
    expect(state.error).toBeNull();
    expect(mocks.appDbMock.setItem).not.toHaveBeenCalled();
  });

  it("does not let stale server stream terminal results overwrite the latest snapshot", async () => {
    let resolveStream!: (result: {
      status: string;
      message?: Message;
      error?: { code: string; message: string; recoverable?: boolean };
    }) => void;
    const streamResult = new Promise<{
      status: string;
      message?: Message;
      error?: { code: string; message: string; recoverable?: boolean };
    }>((resolve) => {
      resolveStream = resolve;
    });
    mocks.streamService.streamAssistantMessage.mockImplementation(
      async (_input: unknown, handlers?: any) => {
        handlers?.onStarted?.({
          type: "message.started",
          runId: "run-stale",
          conversationId: "c1",
          messageId: "m4",
        });
        return streamResult;
      },
    );

    useChatStore.setState({
      selectedModel: "openai:gpt-5.5",
      serverReadState: {
        ...makeEmptyServerReadState(),
        sessions: [{ ...makeServerSession("c1"), messageCount: 0 }],
        currentSessionId: "c1",
      },
    });

    const stream = useChatStore.getState().sendServerMessageAndStream({
      sessionId: "c1",
      content: "hello user",
    });
    await vi.waitFor(() => {
      expect(useChatStore.getState().serverReadState.generation).toMatchObject({
        status: "streaming",
        activeServerRunId: "run-stale",
      });
    });

    await expect(
      useChatStore.getState().selectServerSession("c1"),
    ).resolves.toBe(true);
    expect(useChatStore.getState().serverReadState.generation).toEqual(
      makeEmptyServerReadState().generation,
    );

    resolveStream({
      status: "completed",
      message: { ...makeMessage("m-stale", "model"), content: "stale" },
    });
    await expect(stream).resolves.toMatchObject({ status: "completed" });

    const state = useChatStore.getState().serverReadState;
    expect(state.activeMessages.map((message) => message.id)).toEqual([
      "m1",
      "m2",
    ]);
    expect(state.generation).toEqual(makeEmptyServerReadState().generation);
    expect(state.activeMessageTree.nodesById["m-stale"]).toBeUndefined();
    expect(mocks.appDbMock.setItem).not.toHaveBeenCalled();
  });

  it("does not overcount non-current assistant stream draft events", async () => {
    const activeServerMessage = makeMessage("c2-m1", "user");
    mocks.streamService.streamAssistantMessage.mockImplementation(
      async (_input: unknown, handlers?: any) => {
        handlers?.onStarted?.({
          type: "message.started",
          runId: "run-1",
          conversationId: "c1",
          messageId: "m4",
        });
        handlers?.onDelta?.({
          type: "message.delta",
          runId: "run-1",
          conversationId: "c1",
          messageId: "m4",
          delta: "draft",
        });
        return {
          status: "completed",
          message: makeMessage("m4", "model"),
        };
      },
    );

    useChatStore.setState({
      selectedModel: "openai:gpt-5.5",
      serverReadState: {
        ...makeEmptyServerReadState(),
        sessions: [
          { ...makeServerSession("c1"), messageCount: 0 },
          { ...makeServerSession("c2"), messageCount: 1 },
        ],
        currentSessionId: "c2",
        activeMessages: [activeServerMessage],
        activeMessageTree: normalizeSessionMessageTree([activeServerMessage]),
      },
    });

    await expect(
      useChatStore.getState().sendServerMessageAndStream({
        sessionId: "c1",
        content: "hello user",
      }),
    ).resolves.toMatchObject({ status: "completed" });

    const state = useChatStore.getState().serverReadState;
    expect(state.sessions.find((session) => session.id === "c1")).toMatchObject(
      {
        messageCount: 2,
      },
    );
    expect(state.activeMessages.map((message) => message.id)).toEqual([
      "c2-m1",
    ]);
    expect(state.activeMessageTree.nodesById.m4).toBeUndefined();
    expect(mocks.appDbMock.getItem).not.toHaveBeenCalled();
    expect(mocks.appDbMock.setItem).not.toHaveBeenCalled();
  });

  it("returns null without local writes when server stream is disabled", async () => {
    mocks.streamService.streamEnabled = false;

    await expect(
      useChatStore.getState().sendServerMessageAndStream({
        sessionId: "c1",
        content: "hello",
      }),
    ).resolves.toBeNull();

    expect(mocks.serverService.appendUserMessage).not.toHaveBeenCalled();
    expect(mocks.streamService.streamAssistantMessage).not.toHaveBeenCalled();
    expect(mocks.appDbMock.getItem).not.toHaveBeenCalled();
    expect(mocks.appDbMock.setItem).not.toHaveBeenCalled();
  });
});
