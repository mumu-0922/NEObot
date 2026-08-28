"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Blocks,
  Cable,
  Check,
  ExternalLink,
  Loader2,
  Search,
} from "lucide-react";
import { useTranslations } from "next-intl";

import McpServerIcon from "@/components/mcp/McpServerIcon";
import { SkillIcon } from "@/components/skills/SkillIcon";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import type {
  McpConversationSelection,
  McpServer,
  McpSelectionServer,
} from "@/lib/mcp/types";
import type {
  AgentPackageInstallationDTO,
  SkillConversationSelectionDTO,
} from "@/services/api/client";
import { createNeoChatApiClient } from "@/services/api/client";

import Tooltip from "../ui/Tooltip";

interface ConversationResourcePickersProps {
  conversationId?: string;
  skillEnabled: boolean;
  mcpEnabled: boolean;
  runActive: boolean;
  disabled?: boolean;
  openPicker: "skill" | "mcp" | null;
  onOpenPickerChange: (picker: "skill" | "mcp", open: boolean) => void;
  onOpenSkillStore: () => void;
  onOpenMcpTools: () => void;
}

const iconButtonFocusClass =
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-400/40 focus-visible:ring-offset-2 focus-visible:ring-offset-white dark:focus-visible:ring-offset-background";

export default function ConversationResourcePickers({
  conversationId,
  skillEnabled,
  mcpEnabled,
  runActive,
  disabled = false,
  openPicker,
  onOpenPickerChange,
  onOpenSkillStore,
  onOpenMcpTools,
}: ConversationResourcePickersProps) {
  const t = useTranslations("MessageInput");
  const client = useMemo(() => createNeoChatApiClient(), []);
  const currentConversationIdRef = useRef(conversationId);
  currentConversationIdRef.current = conversationId;
  const [skillQuery, setSkillQuery] = useState("");
  const [mcpQuery, setMcpQuery] = useState("");
  const [skills, setSkills] = useState<AgentPackageInstallationDTO[]>([]);
  const [skillSelection, setSkillSelection] =
    useState<SkillConversationSelectionDTO | null>(null);
  const [mcpServers, setMcpServers] = useState<McpServer[]>([]);
  const [mcpSelection, setMcpSelection] =
    useState<McpConversationSelection | null>(null);
  const [skillLoading, setSkillLoading] = useState(false);
  const [mcpLoading, setMcpLoading] = useState(false);
  const [skillSaving, setSkillSaving] = useState(false);
  const [mcpSaving, setMcpSaving] = useState(false);
  const [skillError, setSkillError] = useState("");
  const [mcpError, setMcpError] = useState("");

  const loadSkills = useCallback(
    async (signal?: AbortSignal) => {
      const targetConversationId = conversationId;
      if (!skillEnabled || !targetConversationId) {
        if (currentConversationIdRef.current === targetConversationId) {
          setSkills([]);
          setSkillSelection(null);
          setSkillLoading(false);
        }
        return;
      }
      setSkillLoading(true);
      setSkillError("");
      try {
        const [library, selection] = await Promise.all([
          client.skillStore.listPackageLibrary({ signal }),
          client.skillStore.getConversationSelection(targetConversationId, {
            signal,
          }),
        ]);
        if (
          signal?.aborted ||
          currentConversationIdRef.current !== targetConversationId
        ) {
          return;
        }
        setSkills(library);
        setSkillSelection(selection);
      } catch {
        if (
          !signal?.aborted &&
          currentConversationIdRef.current === targetConversationId
        ) {
          setSkillError(t("resourceSelectionLoadFailed"));
        }
      } finally {
        if (
          !signal?.aborted &&
          currentConversationIdRef.current === targetConversationId
        ) {
          setSkillLoading(false);
        }
      }
    },
    [client.skillStore, conversationId, skillEnabled, t],
  );

  const loadMcp = useCallback(
    async (signal?: AbortSignal) => {
      const targetConversationId = conversationId;
      if (!mcpEnabled || !targetConversationId) {
        if (currentConversationIdRef.current === targetConversationId) {
          setMcpServers([]);
          setMcpSelection(null);
          setMcpLoading(false);
        }
        return;
      }
      setMcpLoading(true);
      setMcpError("");
      try {
        const [listed, selection] = await Promise.all([
          client.mcp.listServers({
            conversationId: targetConversationId,
            signal,
          }),
          client.mcp.getConversationSelection(targetConversationId, { signal }),
        ]);
        if (
          signal?.aborted ||
          currentConversationIdRef.current !== targetConversationId
        ) {
          return;
        }
        setMcpServers(listed.servers);
        setMcpSelection(selection);
      } catch {
        if (
          !signal?.aborted &&
          currentConversationIdRef.current === targetConversationId
        ) {
          setMcpError(t("resourceSelectionLoadFailed"));
        }
      } finally {
        if (
          !signal?.aborted &&
          currentConversationIdRef.current === targetConversationId
        ) {
          setMcpLoading(false);
        }
      }
    },
    [client.mcp, conversationId, mcpEnabled, t],
  );

  useEffect(() => {
    setSkillSaving(false);
    setMcpSaving(false);
    setSkillError("");
    setMcpError("");
    const controller = new AbortController();
    void loadSkills(controller.signal);
    void loadMcp(controller.signal);
    return () => controller.abort();
  }, [loadMcp, loadSkills]);

  const selectedSkillIds = useMemo(() => {
    if (!skillSelection || skillSelection.conversationId !== conversationId) {
      return new Set<string>();
    }
    return new Set(skillSelection.skills.map((skill) => skill.id));
  }, [conversationId, skillSelection]);
  const selectedMcpKeys = useMemo(() => {
    if (!mcpSelection || mcpSelection.conversationId !== conversationId) {
      return new Set<string>();
    }
    return new Set(
      mcpSelection.servers.map((server) =>
        mcpServerKey(server.ref.source, server.ref.id),
      ),
    );
  }, [conversationId, mcpSelection]);

  const filteredSkills = useMemo(() => {
    const query = skillQuery.trim().toLowerCase();
    if (!query) return skills;
    return skills.filter((skill) =>
      `${skill.name} ${skill.description} ${skill.version}`
        .toLowerCase()
        .includes(query),
    );
  }, [skillQuery, skills]);

  const filteredMcpServers = useMemo(() => {
    const query = mcpQuery.trim().toLowerCase();
    if (!query) return mcpServers;
    return mcpServers.filter((server) =>
      `${server.name} ${server.description ?? ""} ${server.ref.source}`
        .toLowerCase()
        .includes(query),
    );
  }, [mcpQuery, mcpServers]);

  const toggleSkill = useCallback(
    async (installationId: string) => {
      if (!conversationId || !skillSelection || skillSaving) return;
      const targetConversationId = conversationId;
      const nextIds = selectedSkillIds.has(installationId)
        ? [...selectedSkillIds].filter((id) => id !== installationId)
        : [...selectedSkillIds, installationId];
      setSkillSaving(true);
      setSkillError("");
      try {
        const next = await client.skillStore.replaceConversationSelection({
          conversationId: targetConversationId,
          revision: skillSelection.revision,
          installationIds: nextIds,
        });
        if (currentConversationIdRef.current === targetConversationId) {
          setSkillSelection(next);
        }
      } catch {
        if (currentConversationIdRef.current === targetConversationId) {
          setSkillError(t("resourceSelectionSaveFailed"));
          await loadSkills();
        }
      } finally {
        if (currentConversationIdRef.current === targetConversationId) {
          setSkillSaving(false);
        }
      }
    },
    [
      client.skillStore,
      conversationId,
      loadSkills,
      selectedSkillIds,
      skillSaving,
      skillSelection,
      t,
    ],
  );

  const toggleMcpServer = useCallback(
    async (server: McpServer) => {
      if (
        !conversationId ||
        !mcpSelection ||
        mcpSaving ||
        server.status !== "ready"
      ) {
        return;
      }
      const targetConversationId = conversationId;
      const key = mcpServerKey(server.ref.source, server.ref.id);
      const current: McpSelectionServer[] =
        mcpSelection.mode === "custom" ? mcpSelection.servers : [];
      const nextServers = selectedMcpKeys.has(key)
        ? current.filter(
            (item) => mcpServerKey(item.ref.source, item.ref.id) !== key,
          )
        : [...current, { ref: server.ref, disabledTools: [] }];
      setMcpSaving(true);
      setMcpError("");
      try {
        const next = await client.mcp.replaceConversationSelection({
          conversationId: targetConversationId,
          mode: "custom",
          revision: mcpSelection.revision,
          servers: nextServers,
        });
        if (currentConversationIdRef.current === targetConversationId) {
          setMcpSelection(next);
        }
      } catch {
        if (currentConversationIdRef.current === targetConversationId) {
          setMcpError(t("resourceSelectionSaveFailed"));
          await loadMcp();
        }
      } finally {
        if (currentConversationIdRef.current === targetConversationId) {
          setMcpSaving(false);
        }
      }
    },
    [
      client.mcp,
      conversationId,
      loadMcp,
      mcpSaving,
      mcpSelection,
      selectedMcpKeys,
      t,
    ],
  );

  const unavailable = disabled || !conversationId;
  return (
    <>
      {skillEnabled ? (
        <DropdownMenu
          open={openPicker === "skill"}
          onOpenChange={(open) => {
            onOpenPickerChange("skill", open);
            if (open) void loadSkills();
          }}
        >
          <Tooltip content={t("skillPicker")} position="top">
            <DropdownMenuTrigger asChild>
              <button
                type="button"
                aria-label={t("skillPickerAria")}
                aria-pressed={selectedSkillIds.size > 0}
                className={`relative inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-lg transition-colors ${iconButtonFocusClass} ${
                  selectedSkillIds.size > 0
                    ? "bg-fuchsia-50 text-fuchsia-700 dark:bg-fuchsia-950/35 dark:text-fuchsia-200"
                    : "text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-muted-foreground dark:hover:bg-accent/50 dark:hover:text-foreground"
                }`}
                disabled={unavailable}
              >
                {skillSaving ? (
                  <Loader2 size={16} className="animate-spin" />
                ) : (
                  <Blocks size={16} aria-hidden="true" />
                )}
                <SelectionCount count={selectedSkillIds.size} />
              </button>
            </DropdownMenuTrigger>
          </Tooltip>
          <ResourceMenu
            kind="skill"
            loading={skillLoading}
            saving={skillSaving}
            error={skillError}
            query={skillQuery}
            onQueryChange={setSkillQuery}
            runActive={runActive}
            empty={filteredSkills.length === 0}
            onManage={() => {
              onOpenPickerChange("skill", false);
              onOpenSkillStore();
            }}
          >
            {filteredSkills.map((skill) => (
              <DropdownMenuCheckboxItem
                key={skill.id}
                checked={selectedSkillIds.has(skill.id)}
                disabled={skillSaving}
                indicator={<Check size={13} />}
                className="h-auto min-h-10 items-center py-2"
                onSelect={(event) => event.preventDefault()}
                onCheckedChange={() => void toggleSkill(skill.id)}
              >
                <ResourceLabel
                  icon={<SkillIcon label={skill.name} compact />}
                  title={skill.name}
                  subtitle={skill.description}
                />
              </DropdownMenuCheckboxItem>
            ))}
          </ResourceMenu>
        </DropdownMenu>
      ) : null}

      {mcpEnabled ? (
        <DropdownMenu
          open={openPicker === "mcp"}
          onOpenChange={(open) => {
            onOpenPickerChange("mcp", open);
            if (open) void loadMcp();
          }}
        >
          <Tooltip content={t("mcpPicker")} position="top">
            <DropdownMenuTrigger asChild>
              <button
                type="button"
                aria-label={t("mcpPickerAria")}
                aria-pressed={selectedMcpKeys.size > 0}
                className={`relative inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-lg transition-colors ${iconButtonFocusClass} ${
                  selectedMcpKeys.size > 0
                    ? "bg-cyan-50 text-cyan-700 dark:bg-cyan-950/35 dark:text-cyan-200"
                    : "text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-muted-foreground dark:hover:bg-accent/50 dark:hover:text-foreground"
                }`}
                disabled={unavailable}
              >
                {mcpSaving ? (
                  <Loader2 size={16} className="animate-spin" />
                ) : (
                  <Cable size={16} aria-hidden="true" />
                )}
                <SelectionCount count={selectedMcpKeys.size} />
              </button>
            </DropdownMenuTrigger>
          </Tooltip>
          <ResourceMenu
            kind="mcp"
            loading={mcpLoading}
            saving={mcpSaving}
            error={mcpError}
            query={mcpQuery}
            onQueryChange={setMcpQuery}
            runActive={runActive}
            empty={filteredMcpServers.length === 0}
            onManage={() => {
              onOpenPickerChange("mcp", false);
              onOpenMcpTools();
            }}
          >
            {filteredMcpServers.map((server) => {
              const key = mcpServerKey(server.ref.source, server.ref.id);
              return (
                <DropdownMenuCheckboxItem
                  key={key}
                  checked={selectedMcpKeys.has(key)}
                  disabled={mcpSaving || server.status !== "ready"}
                  indicator={<Check size={13} />}
                  className="h-auto min-h-10 items-center py-2"
                  onSelect={(event) => event.preventDefault()}
                  onCheckedChange={() => void toggleMcpServer(server)}
                >
                  <ResourceLabel
                    icon={<McpServerIcon icon={server.icon} compact />}
                    title={server.name}
                    subtitle={`${server.status} · ${server.toolCount} Tools`}
                  />
                </DropdownMenuCheckboxItem>
              );
            })}
          </ResourceMenu>
        </DropdownMenu>
      ) : null}
    </>
  );
}

function ResourceMenu({
  kind,
  loading,
  saving,
  error,
  query,
  onQueryChange,
  runActive,
  empty,
  onManage,
  children,
}: {
  kind: "skill" | "mcp";
  loading: boolean;
  saving: boolean;
  error: string;
  query: string;
  onQueryChange: (value: string) => void;
  runActive: boolean;
  empty: boolean;
  onManage: () => void;
  children: React.ReactNode;
}) {
  const t = useTranslations("MessageInput");
  return (
    <DropdownMenuContent side="top" align="start" className="w-80 p-1.5">
      <DropdownMenuLabel className="flex items-center justify-between">
        <span>
          {t(kind === "skill" ? "skillPickerTitle" : "mcpPickerTitle")}
        </span>
        {saving ? <Loader2 size={12} className="animate-spin" /> : null}
      </DropdownMenuLabel>
      <div className="relative px-1 pb-1">
        <Search
          size={14}
          className="pointer-events-none absolute left-3 top-2.5 text-muted-foreground"
        />
        <input
          value={query}
          onChange={(event) => onQueryChange(event.target.value)}
          onKeyDown={(event) => event.stopPropagation()}
          placeholder={t("resourcePickerSearch")}
          className="h-9 w-full rounded-lg border border-border bg-background pl-8 pr-2 text-sm outline-none focus:border-cyan-400"
        />
      </div>
      {runActive ? (
        <div className="mx-1 mb-1 rounded-md bg-amber-50 px-2 py-1.5 text-xs text-amber-700 dark:bg-amber-950/30 dark:text-amber-200">
          {t("resourceSelectionNextRun")}
        </div>
      ) : null}
      {error ? (
        <div className="px-2 py-2 text-xs text-red-600 dark:text-red-300">
          {error}
        </div>
      ) : loading ? (
        <div className="flex items-center gap-2 px-2 py-4 text-sm text-muted-foreground">
          <Loader2 size={14} className="animate-spin" />
          {t("resourcePickerLoading")}
        </div>
      ) : empty ? (
        <div className="px-2 py-4 text-sm text-muted-foreground">
          {t("resourcePickerEmpty")}
        </div>
      ) : (
        <div className="max-h-64 overflow-y-auto">{children}</div>
      )}
      <DropdownMenuSeparator />
      <DropdownMenuItem onSelect={onManage}>
        <ExternalLink size={14} aria-hidden="true" />
        {t(kind === "skill" ? "manageSkills" : "manageMcp")}
      </DropdownMenuItem>
    </DropdownMenuContent>
  );
}

function ResourceLabel({
  icon,
  title,
  subtitle,
}: {
  icon: React.ReactNode;
  title: string;
  subtitle: string;
}) {
  return (
    <span className="flex min-w-0 flex-1 items-center gap-2.5">
      {icon}
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-medium">{title}</span>
        <span className="block truncate text-xs text-muted-foreground">
          {subtitle}
        </span>
      </span>
    </span>
  );
}

function SelectionCount({ count }: { count: number }) {
  if (count < 1) return null;
  return (
    <span className="absolute -right-0.5 -top-0.5 flex h-3.5 min-w-3.5 items-center justify-center rounded-full bg-slate-700 px-0.5 text-[8px] font-bold text-white dark:bg-slate-200 dark:text-slate-950">
      {count > 9 ? "9+" : count}
    </span>
  );
}

function mcpServerKey(source: string, id: string): string {
  return `${source}:${id}`;
}
