"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  AlertTriangle,
  Check,
  KeyRound,
  Loader2,
  Plus,
  RefreshCw,
  Trash2,
  Wrench,
  X,
} from "lucide-react";
import { useTranslations } from "next-intl";

import { Dialog } from "@/components/ui/primitives";
import Tooltip from "@/components/ui/Tooltip";
import type {
  McpConversationSelection,
  McpSelectionServer,
  McpServer,
  McpServerRef,
} from "@/lib/mcp/types";
import { ApiClientError, createNeoChatApiClient } from "@/services/api/client";

import McpServerIcon from "./McpServerIcon";

interface McpToolsControlProps {
  conversationId?: string;
  enabled: boolean;
  disabled?: boolean;
  className?: string;
  variant?: "composer" | "page" | "embedded";
  onClose?: () => void;
  attention?: { nonce: number; message: string } | null;
  onAttentionHandled?: () => void;
  onDisableAllAndContinue?: () => void;
}

type PrivateServerDraft = {
  name: string;
  endpointUrl: string;
  authType: "none" | "header" | "oauth";
  headerName: string;
  clientId: string;
};

const emptyPrivateServerDraft: PrivateServerDraft = {
  name: "",
  endpointUrl: "",
  authType: "none",
  headerName: "Authorization",
  clientId: "",
};

export default function McpToolsControl({
  conversationId,
  enabled,
  disabled = false,
  className = "",
  variant = "composer",
  onClose,
  attention,
  onAttentionHandled,
  onDisableAllAndContinue,
}: McpToolsControlProps) {
  const t = useTranslations("Mcp");
  const [open, setOpen] = useState(false);
  const [servers, setServers] = useState<McpServer[]>([]);
  const [selection, setSelection] = useState<McpConversationSelection | null>(
    null,
  );
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [attentionMessage, setAttentionMessage] = useState("");
  const [credentialRef, setCredentialRef] = useState<McpServerRef | null>(null);
  const [credentialValue, setCredentialValue] = useState("");
  const [showCreate, setShowCreate] = useState(false);
  const [draft, setDraft] = useState<PrivateServerDraft>(
    emptyPrivateServerDraft,
  );
  const client = useMemo(() => createNeoChatApiClient(), []);

  const load = useCallback(
    async (signal?: AbortSignal) => {
      if (!enabled) {
        setServers([]);
        setSelection(null);
        return;
      }
      setLoading(true);
      setError("");
      try {
        const nextServersPromise = client.mcp.listServers({
          conversationId,
          signal,
        });
        const [nextServers, nextSelection] = conversationId
          ? await Promise.all([
              nextServersPromise,
              client.mcp.getConversationSelection(conversationId, { signal }),
            ])
          : [await nextServersPromise, null];
        if (signal?.aborted) return;
        setServers(nextServers);
        setSelection(nextSelection);
      } catch (loadError) {
        if (signal?.aborted) return;
        setError(formatError(loadError, t("loadFailed")));
      } finally {
        if (!signal?.aborted) setLoading(false);
      }
    },
    [client.mcp, conversationId, enabled, t],
  );

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  useEffect(() => {
    if (
      variant !== "composer" ||
      !attention?.nonce ||
      !enabled ||
      !conversationId
    ) {
      return;
    }
    setAttentionMessage(attention.message);
    setOpen(true);
  }, [attention?.message, attention?.nonce, conversationId, enabled, variant]);

  const selectedByKey = useMemo(
    () =>
      new Map(
        (selection?.servers ?? []).map((selected) => [
          serverKey(selected.ref),
          selected,
        ]),
      ),
    [selection?.servers],
  );
  const enabledServers =
    selection?.mode === "custom"
      ? servers.filter((server) => selectedByKey.has(serverKey(server.ref)))
      : [];
  const enabledToolCount = enabledServers.reduce((total, server) => {
    const disabledTools = new Set(
      selectedByKey.get(serverKey(server.ref))?.disabledTools ?? [],
    );
    return (
      total +
      server.tools.filter(
        (tool) => tool.supported && !disabledTools.has(tool.name),
      ).length
    );
  }, 0);

  const saveSelection = useCallback(
    async (
      mode: McpConversationSelection["mode"],
      nextServers: McpSelectionServer[],
    ): Promise<boolean> => {
      if (!conversationId || !selection || saving) return false;
      setSaving(true);
      setError("");
      try {
        const next = await client.mcp.replaceConversationSelection({
          conversationId,
          mode,
          revision: selection.revision,
          servers: mode === "inherit" ? [] : nextServers,
        });
        setSelection(next);
        return true;
      } catch (saveError) {
        setError(formatError(saveError, t("saveFailed")));
        await load();
        return false;
      } finally {
        setSaving(false);
      }
    },
    [client.mcp, conversationId, load, saving, selection, t],
  );

  const toggleServer = useCallback(
    (server: McpServer) => {
      if (!selection || server.status !== "ready") return;
      const key = serverKey(server.ref);
      const current =
        selection.mode === "custom" ? selection.servers : ([] as const);
      const next = selectedByKey.has(key)
        ? current.filter((item) => serverKey(item.ref) !== key)
        : [...current, { ref: server.ref, disabledTools: [] }];
      void saveSelection("custom", next);
    },
    [saveSelection, selectedByKey, selection],
  );

  const toggleTool = useCallback(
    (server: McpServer, toolName: string) => {
      if (!selection || selection.mode !== "custom") return;
      const key = serverKey(server.ref);
      const next = selection.servers.map((item) => {
        if (serverKey(item.ref) !== key) return item;
        const disabledTools = new Set(item.disabledTools);
        if (disabledTools.has(toolName)) disabledTools.delete(toolName);
        else disabledTools.add(toolName);
        return { ...item, disabledTools: [...disabledTools].sort() };
      });
      void saveSelection("custom", next);
    },
    [saveSelection, selection],
  );

  const authorize = useCallback(
    async (server: McpServer) => {
      if (server.authType === "header") {
        setCredentialRef(server.ref);
        setCredentialValue("");
        return;
      }
      if (server.authType !== "oauth") return;
      setSaving(true);
      setError("");
      try {
        const result = await client.mcp.startOAuth({
          serverRef: server.ref,
          conversationId,
          returnUrl: `${window.location.pathname}${window.location.search}${window.location.hash}`,
        });
        const authorizationUrl = validHttpsURL(result.authorizationUrl);
        if (!authorizationUrl) {
          throw new ApiClientError(
            "INVALID_SERVER_RESPONSE",
            t("oauthUrlInvalid"),
          );
        }
        window.location.assign(authorizationUrl);
      } catch (oauthError) {
        setError(formatError(oauthError, t("authorizeFailed")));
        setSaving(false);
      }
    },
    [client.mcp, conversationId, t],
  );

  const saveCredential = useCallback(async () => {
    if (!credentialRef || !credentialValue) return;
    setSaving(true);
    setError("");
    try {
      await client.mcp.setCredential({
        serverRef: credentialRef,
        conversationId,
        value: credentialValue,
      });
      setCredentialRef(null);
      setCredentialValue("");
      await load();
    } catch (credentialError) {
      setError(formatError(credentialError, t("authorizeFailed")));
    } finally {
      setSaving(false);
    }
  }, [client.mcp, conversationId, credentialRef, credentialValue, load, t]);

  const createPrivateServer = useCallback(async () => {
    if (
      !draft.name.trim() ||
      !draft.endpointUrl.trim() ||
      (draft.authType === "oauth" && !draft.clientId.trim())
    ) {
      return;
    }
    setSaving(true);
    setError("");
    try {
      const server = await client.mcp.createPrivateServer({
        name: draft.name,
        endpointUrl: draft.endpointUrl,
        authType: draft.authType,
        ...(draft.authType === "header"
          ? { headerName: draft.headerName }
          : {}),
        ...(draft.authType === "oauth" ? { clientId: draft.clientId } : {}),
      });
      await client.mcp.validatePrivateServer(server.ref.id);
      setDraft(emptyPrivateServerDraft);
      setShowCreate(false);
      await load();
    } catch (createError) {
      const message = formatError(createError, t("createFailed"));
      await load();
      setError(message);
    } finally {
      setSaving(false);
    }
  }, [client.mcp, draft, load, t]);

  const validatePrivateServer = useCallback(
    async (serverId: string) => {
      if (saving) return;
      setSaving(true);
      setError("");
      try {
        await client.mcp.validatePrivateServer(serverId);
        await load();
      } catch (validateError) {
        setError(formatError(validateError, t("validateFailed")));
      } finally {
        setSaving(false);
      }
    },
    [client.mcp, load, saving, t],
  );

  const disableAllAndContinue = useCallback(async () => {
    const saved = await saveSelection("custom", []);
    if (!saved) return;
    setAttentionMessage("");
    setOpen(false);
    onAttentionHandled?.();
    onDisableAllAndContinue?.();
  }, [onAttentionHandled, onDisableAllAndContinue, saveSelection]);

  const close = useCallback(() => {
    setOpen(false);
    if (attentionMessage) {
      setAttentionMessage("");
      onAttentionHandled?.();
    }
  }, [attentionMessage, onAttentionHandled]);

  const deletePrivateServer = useCallback(
    async (serverId: string) => {
      setSaving(true);
      setError("");
      try {
        await client.mcp.deletePrivateServer(serverId);
        await load();
      } catch (deleteError) {
        setError(formatError(deleteError, t("deleteFailed")));
      } finally {
        setSaving(false);
      }
    },
    [client.mcp, load, t],
  );

  const unavailable = !enabled || !conversationId;
  const statusLabel = !enabled
    ? t("serverModeOnly")
    : !conversationId
      ? t("serverSummary", { servers: servers.length })
      : selection?.mode === "inherit"
        ? t("inherited")
        : t("enabledSummary", {
            servers: enabledServers.length,
            tools: enabledToolCount,
          });

  const panel = (
    <div
      className={`flex min-h-0 flex-col ${
        variant !== "composer" ? "h-full" : "max-h-[min(650px,80vh)]"
      }`}
    >
      <div className="flex items-center justify-between gap-3 border-b border-gray-200 px-4 py-2 dark:border-border">
        <div className="min-w-0 text-xs text-gray-500 dark:text-muted-foreground">
          {statusLabel}
        </div>
        <div className="flex shrink-0 items-center gap-1">
          <button
            type="button"
            disabled={!enabled || saving || loading}
            onClick={() => void load()}
            aria-label={t("refresh")}
            className="rounded-md p-2 text-gray-500 hover:bg-gray-100 disabled:opacity-50 dark:hover:bg-accent"
          >
            <RefreshCw
              size={14}
              className={loading ? "animate-spin" : ""}
              aria-hidden="true"
            />
          </button>
          <button
            type="button"
            disabled={saving || !selection}
            onClick={() => void saveSelection("inherit", [])}
            className="rounded-md px-2 py-1.5 text-xs text-gray-600 hover:bg-gray-100 disabled:opacity-50 dark:text-muted-foreground dark:hover:bg-accent"
          >
            {t("useWorkspaceDefaults")}
          </button>
          <button
            type="button"
            disabled={saving || !selection}
            onClick={() => void saveSelection("custom", [])}
            className="rounded-md px-2 py-1.5 text-xs text-red-600 hover:bg-red-50 disabled:opacity-50 dark:text-red-300 dark:hover:bg-red-950/30"
          >
            {t("disableAll")}
          </button>
          {variant === "composer" ? (
            <button
              type="button"
              onClick={close}
              aria-label={t("close")}
              className="rounded-md p-2 text-gray-500 hover:bg-gray-100 dark:hover:bg-accent"
            >
              <X size={15} aria-hidden="true" />
            </button>
          ) : null}
        </div>
      </div>

      {error ? (
        <div
          role="alert"
          className="mx-4 mt-3 flex items-start gap-2 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-200"
        >
          <AlertTriangle size={14} className="mt-0.5 shrink-0" />
          <span>{error}</span>
        </div>
      ) : null}

      {attentionMessage ? (
        <div
          role="alert"
          className="mx-4 mt-3 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-100"
        >
          <div className="flex items-start gap-2">
            <AlertTriangle size={14} className="mt-0.5 shrink-0" />
            <span>{attentionMessage}</span>
          </div>
          <button
            type="button"
            disabled={saving || !selection}
            onClick={() => void disableAllAndContinue()}
            className="mt-2 rounded-md bg-amber-700 px-2.5 py-1.5 font-medium text-white disabled:opacity-50 dark:bg-amber-600"
          >
            {t("disableAllAndContinue")}
          </button>
        </div>
      ) : null}

      <div className="min-h-0 flex-1 overflow-y-auto px-4 py-3 custom-scrollbar">
        {loading && servers.length === 0 ? (
          <div className="flex items-center justify-center gap-2 py-10 text-sm text-gray-500">
            <Loader2 size={16} className="animate-spin" /> {t("loading")}
          </div>
        ) : servers.length === 0 ? (
          <div className="py-10 text-center text-sm text-gray-500">
            {t("empty")}
          </div>
        ) : (
          <div className="space-y-3">
            {servers.map((server) => {
              const selected = selectedByKey.get(serverKey(server.ref));
              const isSelected = Boolean(
                selection?.mode === "custom" && selected,
              );
              const ready = server.status === "ready";
              const needsAuth =
                server.status === "needs_auth" ||
                (server.authType !== "none" && !server.hasCredential);
              const disabledTools = new Set(selected?.disabledTools ?? []);

              return (
                <section
                  key={serverKey(server.ref)}
                  className="rounded-lg border border-gray-200 bg-gray-50/50 p-3 dark:border-border dark:bg-muted/20"
                >
                  <div className="flex items-start gap-3">
                    <button
                      type="button"
                      aria-label={
                        isSelected
                          ? t("disableServer", { name: server.name })
                          : t("enableServer", { name: server.name })
                      }
                      aria-pressed={isSelected}
                      disabled={saving || !ready || !selection}
                      onClick={() => toggleServer(server)}
                      className={`mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded border disabled:cursor-not-allowed disabled:opacity-45 ${isSelected ? "border-cyan-500 bg-cyan-500 text-white" : "border-gray-300 bg-white text-transparent dark:border-border dark:bg-background"}`}
                    >
                      <Check size={13} aria-hidden="true" />
                    </button>
                    <McpServerIcon icon={server.icon} />
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="truncate text-sm font-semibold text-gray-800 dark:text-foreground">
                          {server.name}
                        </span>
                        <ServerStatus status={server.status} />
                        <span className="text-[10px] uppercase text-gray-400">
                          {server.transport === "stdio" ? "stdio" : "HTTP"}
                        </span>
                      </div>
                      {server.description ? (
                        <p className="mt-1 line-clamp-2 text-xs text-gray-500 dark:text-muted-foreground">
                          {server.description}
                        </p>
                      ) : null}

                      <div className="mt-2 flex flex-wrap items-center gap-2">
                        {needsAuth ? (
                          <button
                            type="button"
                            disabled={saving}
                            onClick={() => void authorize(server)}
                            className="inline-flex items-center gap-1 rounded-md bg-amber-100 px-2 py-1 text-[11px] font-medium text-amber-800 hover:bg-amber-200 disabled:opacity-50 dark:bg-amber-950/40 dark:text-amber-200"
                          >
                            <KeyRound size={12} /> {t("authorize")}
                          </button>
                        ) : null}
                        {server.ref.source === "private" &&
                        server.status === "draft" ? (
                          <button
                            type="button"
                            disabled={saving}
                            onClick={() =>
                              void validatePrivateServer(server.ref.id)
                            }
                            className="rounded-md bg-blue-100 px-2 py-1 text-[11px] font-medium text-blue-700 hover:bg-blue-200 dark:bg-blue-950/40 dark:text-blue-200"
                          >
                            {t("validate")}
                          </button>
                        ) : null}
                        {server.ref.source === "private" ? (
                          <button
                            type="button"
                            disabled={saving}
                            aria-label={t("deleteServer", {
                              name: server.name,
                            })}
                            onClick={() =>
                              void deletePrivateServer(server.ref.id)
                            }
                            className="rounded-md p-1 text-red-500 hover:bg-red-100 disabled:opacity-50 dark:hover:bg-red-950/40"
                          >
                            <Trash2 size={13} />
                          </button>
                        ) : null}
                      </div>

                      {credentialRef &&
                      serverKey(credentialRef) === serverKey(server.ref) ? (
                        <div className="mt-2 flex gap-2">
                          <input
                            type="password"
                            autoComplete="new-password"
                            value={credentialValue}
                            onChange={(event) =>
                              setCredentialValue(event.target.value)
                            }
                            placeholder={t("credentialPlaceholder")}
                            className="min-w-0 flex-1 rounded-md border border-gray-300 bg-white px-2 py-1.5 text-xs outline-none focus:border-cyan-500 dark:border-border dark:bg-background"
                          />
                          <button
                            type="button"
                            disabled={saving || !credentialValue}
                            onClick={() => void saveCredential()}
                            className="rounded-md bg-cyan-600 px-2 py-1.5 text-xs font-medium text-white disabled:opacity-50"
                          >
                            {t("save")}
                          </button>
                        </div>
                      ) : null}

                      {isSelected && server.tools.length > 0 ? (
                        <div className="mt-3 border-t border-gray-200 pt-2 dark:border-border">
                          <div className="mb-1 text-[11px] font-medium text-gray-500">
                            {t("tools")}
                          </div>
                          <div className="space-y-1">
                            {server.tools.map((tool) => {
                              const toolEnabled =
                                tool.supported && !disabledTools.has(tool.name);
                              return (
                                <button
                                  key={tool.name}
                                  type="button"
                                  disabled={saving || !tool.supported}
                                  aria-pressed={toolEnabled}
                                  onClick={() => toggleTool(server, tool.name)}
                                  className="flex w-full items-start gap-2 rounded-md px-2 py-1.5 text-left hover:bg-white disabled:cursor-not-allowed disabled:opacity-50 dark:hover:bg-background"
                                >
                                  <span
                                    className={`mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center rounded border ${toolEnabled ? "border-cyan-500 bg-cyan-500 text-white" : "border-gray-300 text-transparent dark:border-border"}`}
                                  >
                                    <Check size={11} />
                                  </span>
                                  <span className="min-w-0 flex-1">
                                    <span className="flex flex-wrap items-center gap-1.5 text-xs font-medium text-gray-700 dark:text-foreground/85">
                                      <span className="truncate">
                                        {tool.title || tool.name}
                                      </span>
                                      <span className="rounded bg-gray-100 px-1 py-0.5 text-[9px] uppercase text-gray-500 dark:bg-muted">
                                        {tool.classification}
                                      </span>
                                    </span>
                                    {!tool.supported ? (
                                      <span className="block text-[10px] text-amber-600 dark:text-amber-300">
                                        {tool.unsupportedReason ||
                                          t("unsupported")}
                                      </span>
                                    ) : null}
                                  </span>
                                </button>
                              );
                            })}
                          </div>
                        </div>
                      ) : null}
                    </div>
                  </div>
                </section>
              );
            })}
          </div>
        )}

        <div className="mt-4 border-t border-gray-200 pt-3 dark:border-border">
          <button
            type="button"
            disabled={!enabled || saving}
            onClick={() => setShowCreate((value) => !value)}
            className="inline-flex items-center gap-1.5 rounded-md px-2 py-1.5 text-xs font-medium text-cyan-700 hover:bg-cyan-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-cyan-300 dark:hover:bg-cyan-950/30"
          >
            <Plus size={13} /> {t("addRemoteServer")}
          </button>
          {showCreate ? (
            <div className="mt-2 space-y-2 rounded-lg border border-gray-200 p-3 dark:border-border">
              <input
                value={draft.name}
                onChange={(event) =>
                  setDraft((current) => ({
                    ...current,
                    name: event.target.value,
                  }))
                }
                placeholder={t("serverName")}
                className="w-full rounded-md border border-gray-300 bg-white px-2 py-1.5 text-xs outline-none focus:border-cyan-500 dark:border-border dark:bg-background"
              />
              <input
                type="url"
                value={draft.endpointUrl}
                onChange={(event) =>
                  setDraft((current) => ({
                    ...current,
                    endpointUrl: event.target.value,
                  }))
                }
                placeholder="https://mcp.example.com/mcp"
                className="w-full rounded-md border border-gray-300 bg-white px-2 py-1.5 text-xs outline-none focus:border-cyan-500 dark:border-border dark:bg-background"
              />
              <select
                value={draft.authType}
                onChange={(event) =>
                  setDraft((current) => ({
                    ...current,
                    authType: event.target
                      .value as PrivateServerDraft["authType"],
                  }))
                }
                className="w-full rounded-md border border-gray-300 bg-white px-2 py-1.5 text-xs dark:border-border dark:bg-background"
              >
                <option value="none">{t("authNone")}</option>
                <option value="header">{t("authHeader")}</option>
                <option value="oauth">OAuth</option>
              </select>
              {draft.authType === "header" ? (
                <input
                  value={draft.headerName}
                  onChange={(event) =>
                    setDraft((current) => ({
                      ...current,
                      headerName: event.target.value,
                    }))
                  }
                  placeholder="Authorization"
                  className="w-full rounded-md border border-gray-300 bg-white px-2 py-1.5 text-xs dark:border-border dark:bg-background"
                />
              ) : null}
              {draft.authType === "oauth" ? (
                <input
                  value={draft.clientId}
                  onChange={(event) =>
                    setDraft((current) => ({
                      ...current,
                      clientId: event.target.value,
                    }))
                  }
                  placeholder={t("clientId")}
                  className="w-full rounded-md border border-gray-300 bg-white px-2 py-1.5 text-xs dark:border-border dark:bg-background"
                />
              ) : null}
              <button
                type="button"
                disabled={
                  !enabled ||
                  saving ||
                  !draft.name.trim() ||
                  !draft.endpointUrl.trim() ||
                  (draft.authType === "oauth" && !draft.clientId.trim())
                }
                onClick={() => void createPrivateServer()}
                className="rounded-md bg-cyan-600 px-3 py-1.5 text-xs font-medium text-white disabled:opacity-50"
              >
                {saving ? t("saving") : t("createAndValidate")}
              </button>
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );

  if (variant === "embedded") return panel;

  if (variant === "page") {
    return (
      <div className="flex h-full w-full flex-col overflow-hidden bg-gray-50/50 dark:bg-background">
        <header className="flex shrink-0 items-center justify-between gap-3 border-b border-gray-200/50 bg-white/95 px-6 py-4 dark:border-border dark:bg-card/95">
          <div className="flex min-w-0 items-center gap-3">
            <div
              className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-linear-to-tr from-cyan-600 to-blue-600 text-white shadow-lg shadow-cyan-500/20"
              aria-hidden="true"
            >
              <Wrench size={20} />
            </div>
            <div className="min-w-0">
              <h1 className="truncate text-lg font-bold text-gray-800 dark:text-foreground">
                {t("title")}
              </h1>
              <p className="mt-0.5 truncate text-xs text-gray-500 dark:text-muted-foreground">
                {t("pageSubtitle")}
              </p>
            </div>
          </div>
          {onClose ? (
            <button
              type="button"
              onClick={onClose}
              aria-label={t("close")}
              className="shrink-0 rounded-full p-2 text-gray-500 transition-colors hover:bg-gray-200/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/60 dark:text-muted-foreground dark:hover:bg-accent/50"
            >
              <X size={20} aria-hidden="true" />
            </button>
          ) : null}
        </header>
        <div className="mx-auto min-h-0 w-full max-w-5xl flex-1 p-4 md:p-6">
          <section
            aria-label={t("serverList")}
            className="h-full overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-border dark:bg-card"
          >
            {panel}
          </section>
        </div>
      </div>
    );
  }

  return (
    <div className={`flex min-w-0 items-center gap-1 ${className}`}>
      <Tooltip content={statusLabel} position="top">
        <button
          type="button"
          aria-label={t("open")}
          aria-pressed={enabledServers.length > 0}
          disabled={disabled || unavailable}
          onClick={() => setOpen(true)}
          className={`inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-lg transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-400/40 disabled:cursor-not-allowed disabled:opacity-45 ${enabledServers.length > 0 ? "bg-cyan-50 text-cyan-600 dark:bg-cyan-950/30 dark:text-cyan-300" : "text-gray-500 hover:bg-gray-100 dark:text-muted-foreground dark:hover:bg-accent/50"}`}
        >
          <Wrench size={16} aria-hidden="true" />
        </button>
      </Tooltip>

      <Dialog open={open} onClose={close} title={t("title")}>
        {panel}
      </Dialog>
    </div>
  );
}

function ServerStatus({ status }: { status: McpServer["status"] }) {
  const t = useTranslations("Mcp");
  const label = t(`status.${status}`);
  const classes =
    status === "ready"
      ? "bg-emerald-100 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-200"
      : status === "needs_auth"
        ? "bg-amber-100 text-amber-700 dark:bg-amber-950/40 dark:text-amber-200"
        : status === "unavailable"
          ? "bg-red-100 text-red-700 dark:bg-red-950/40 dark:text-red-200"
          : "bg-gray-100 text-gray-600 dark:bg-muted dark:text-muted-foreground";
  return (
    <span className={`rounded-full px-1.5 py-0.5 text-[9px] ${classes}`}>
      {label}
    </span>
  );
}

function serverKey(ref: McpServerRef): string {
  return `${ref.source}:${ref.id}`;
}

function formatError(error: unknown, fallback: string): string {
  if (error instanceof ApiClientError) return error.message || fallback;
  if (error instanceof Error) return error.message || fallback;
  return fallback;
}

function validHttpsURL(value: string): string | null {
  try {
    const parsed = new URL(value);
    if (
      parsed.protocol !== "https:" ||
      !parsed.hostname ||
      parsed.username ||
      parsed.password
    ) {
      return null;
    }
    return parsed.toString();
  } catch {
    return null;
  }
}
