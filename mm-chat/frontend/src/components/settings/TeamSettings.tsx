"use client";

import React, { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import {
  AlertCircle,
  CheckCircle2,
  LogOut,
  MailPlus,
  Plus,
  RefreshCw,
  ShieldCheck,
  Trash2,
  UsersRound,
} from "lucide-react";
import { createNeoChatApiClient } from "@/services/api/client";
import type {
  TeamDTO,
  TeamInviteDTO,
  TeamMemberDTO,
  TeamRole,
} from "@/services/api/client";
import { logDevError } from "@/lib/utils/devLogger";

const roleOptions: TeamRole[] = ["member", "admin"];

const newIdempotencyKey = (prefix: string): string => {
  const randomId = globalThis.crypto?.randomUUID?.();
  return `${prefix}-${randomId ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`}`;
};

const roleBadgeClass = (role: TeamRole) =>
  role === "admin"
    ? "border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900/60 dark:bg-emerald-950/40 dark:text-emerald-300"
    : "border-border bg-muted text-muted-foreground";

const statusBadgeClass = (status: string) => {
  switch (status) {
    case "sent":
    case "accepted":
    case "active":
      return "border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900/60 dark:bg-emerald-950/40 dark:text-emerald-300";
    case "failed":
    case "revoked":
    case "expired":
      return "border-red-200 bg-red-50 text-red-700 dark:border-red-900/60 dark:bg-red-950/40 dark:text-red-300";
    default:
      return "border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-900/60 dark:bg-amber-950/40 dark:text-amber-300";
  }
};

const formatDate = (value: string): string => {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
};

export default function TeamSettings() {
  const t = useTranslations("Team");
  const apiClient = useMemo(() => createNeoChatApiClient(), []);
  const teamsSupported =
    apiClient.mode === "server" && apiClient.capabilities.teams;

  const [teams, setTeams] = useState<TeamDTO[]>([]);
  const [selectedTeamId, setSelectedTeamId] = useState<string | null>(null);
  const [members, setMembers] = useState<TeamMemberDTO[]>([]);
  const [invites, setInvites] = useState<TeamInviteDTO[]>([]);
  const [newTeamName, setNewTeamName] = useState("");
  const [renameValue, setRenameValue] = useState("");
  const [inviteEmail, setInviteEmail] = useState("");
  const [inviteRole, setInviteRole] = useState<TeamRole>("member");
  const [loadingTeams, setLoadingTeams] = useState(false);
  const [loadingDetails, setLoadingDetails] = useState(false);
  const [actionBusy, setActionBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const selectedTeam = useMemo(
    () => teams.find((team) => team.id === selectedTeamId) ?? null,
    [selectedTeamId, teams],
  );
  const canManageTeam = selectedTeam?.myMembership.teamRole === "admin";
  const trimmedNewTeamName = newTeamName.trim();
  const trimmedRenameValue = renameValue.trim();
  const trimmedInviteEmail = inviteEmail.trim();

  const showError = useCallback((message: string, caught: unknown) => {
    logDevError(message, caught);
    setNotice(null);
    setError(message);
  }, []);

  const refreshTeams = useCallback(async () => {
    if (!teamsSupported) return;
    setLoadingTeams(true);
    setError(null);
    try {
      const page = await apiClient.teams.listTeams({ limit: 50 });
      setTeams(page.items);
      setSelectedTeamId((current) => {
        if (current && page.items.some((team) => team.id === current)) {
          return current;
        }
        return page.items[0]?.id ?? null;
      });
    } catch (caught) {
      showError(t("loadTeamsFailed"), caught);
    } finally {
      setLoadingTeams(false);
    }
  }, [apiClient, showError, t, teamsSupported]);

  const refreshTeamDetails = useCallback(async () => {
    if (!teamsSupported || !selectedTeamId) {
      setMembers([]);
      setInvites([]);
      return;
    }
    setLoadingDetails(true);
    setError(null);
    try {
      const [memberPage, invitePage] = await Promise.all([
        apiClient.teams.listMembers({ teamId: selectedTeamId, limit: 100 }),
        apiClient.teams.listInvites({ teamId: selectedTeamId, limit: 100 }),
      ]);
      setMembers(memberPage.items);
      setInvites(invitePage.items);
    } catch (caught) {
      showError(t("loadDetailsFailed"), caught);
    } finally {
      setLoadingDetails(false);
    }
  }, [apiClient, selectedTeamId, showError, t, teamsSupported]);

  useEffect(() => {
    void refreshTeams();
  }, [refreshTeams]);

  useEffect(() => {
    setRenameValue(selectedTeam?.name ?? "");
  }, [selectedTeam?.name]);

  useEffect(() => {
    void refreshTeamDetails();
  }, [refreshTeamDetails]);

  const runAction = async (key: string, action: () => Promise<void>) => {
    setActionBusy(key);
    setError(null);
    setNotice(null);
    try {
      await action();
    } finally {
      setActionBusy(null);
    }
  };

  const handleCreateTeam = () => {
    if (!trimmedNewTeamName) return;
    void runAction("create-team", async () => {
      try {
        const team = await apiClient.teams.createTeam({
          name: trimmedNewTeamName,
          idempotencyKey: newIdempotencyKey("team"),
        });
        setNewTeamName("");
        setSelectedTeamId(team.id);
        setNotice(t("teamCreated"));
        await refreshTeams();
      } catch (caught) {
        showError(t("createTeamFailed"), caught);
      }
    });
  };

  const handleRenameTeam = () => {
    if (!selectedTeam || !trimmedRenameValue) return;
    void runAction(`rename-${selectedTeam.id}`, async () => {
      try {
        const team = await apiClient.teams.updateTeam({
          teamId: selectedTeam.id,
          name: trimmedRenameValue,
        });
        setTeams((current) =>
          current.map((item) => (item.id === team.id ? team : item)),
        );
        setNotice(t("teamRenamed"));
      } catch (caught) {
        showError(t("renameTeamFailed"), caught);
      }
    });
  };

  const handleInviteMember = () => {
    if (!selectedTeam || !trimmedInviteEmail || !canManageTeam) return;
    void runAction(`invite-${selectedTeam.id}`, async () => {
      try {
        await apiClient.teams.createInvite({
          teamId: selectedTeam.id,
          email: trimmedInviteEmail,
          teamRole: inviteRole,
          idempotencyKey: newIdempotencyKey("invite"),
        });
        setInviteEmail("");
        setInviteRole("member");
        setNotice(t("inviteCreated"));
        await refreshTeamDetails();
      } catch (caught) {
        showError(t("inviteFailed"), caught);
      }
    });
  };

  const handleRoleChange = (member: TeamMemberDTO, teamRole: TeamRole) => {
    if (!selectedTeam || member.teamRole === teamRole || !canManageTeam) return;
    void runAction(`member-${member.userId}`, async () => {
      try {
        const updated = await apiClient.teams.updateMember({
          teamId: selectedTeam.id,
          userId: member.userId,
          teamRole,
        });
        setMembers((current) =>
          current.map((item) =>
            item.userId === updated.userId ? updated : item,
          ),
        );
        setNotice(t("memberUpdated"));
      } catch (caught) {
        showError(t("memberUpdateFailed"), caught);
      }
    });
  };

  const handleRevokeInvite = (invite: TeamInviteDTO) => {
    if (!selectedTeam || !canManageTeam) return;
    if (!window.confirm(t("confirmRevokeInvite"))) return;
    void runAction(`invite-${invite.id}`, async () => {
      try {
        await apiClient.teams.revokeInvite({
          teamId: selectedTeam.id,
          inviteId: invite.id,
        });
        setInvites((current) =>
          current.map((item) =>
            item.id === invite.id ? { ...item, status: "revoked" } : item,
          ),
        );
        setNotice(t("inviteRevoked"));
        await refreshTeamDetails();
      } catch (caught) {
        showError(t("revokeInviteFailed"), caught);
      }
    });
  };

  const handleLeaveTeam = () => {
    if (!selectedTeam) return;
    if (!window.confirm(t("confirmLeaveTeam", { name: selectedTeam.name }))) {
      return;
    }
    void runAction(`leave-${selectedTeam.id}`, async () => {
      try {
        await apiClient.teams.leaveTeam({ teamId: selectedTeam.id });
        setNotice(t("teamLeft"));
        await refreshTeams();
      } catch (caught) {
        showError(t("leaveTeamFailed"), caught);
      }
    });
  };

  if (!teamsSupported) {
    return (
      <section className="space-y-4">
        <div className="rounded-2xl border border-border bg-card p-5 shadow-sm">
          <div className="flex items-start gap-3">
            <span className="rounded-xl bg-muted p-2 text-muted-foreground">
              <UsersRound size={20} aria-hidden="true" />
            </span>
            <div>
              <h2 className="text-lg font-semibold text-foreground">
                {t("title")}
              </h2>
              <p className="mt-1 text-sm text-muted-foreground">
                {t("unsupportedDescription")}
              </p>
            </div>
          </div>
        </div>
      </section>
    );
  }

  return (
    <section className="space-y-5">
      <div className="rounded-2xl border border-border bg-card p-5 shadow-sm">
        <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
          <div className="min-w-0">
            <div className="flex items-center gap-2 text-sm font-medium text-brand">
              <UsersRound size={18} aria-hidden="true" />
              {t("eyebrow")}
            </div>
            <h2 className="mt-2 text-xl font-semibold text-foreground">
              {t("title")}
            </h2>
            <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
              {t("description")}
            </p>
          </div>
          <button
            type="button"
            onClick={() => void refreshTeams()}
            disabled={loadingTeams || actionBusy !== null}
            className="inline-flex items-center justify-center gap-2 rounded-lg border border-border bg-background px-3 py-2 text-sm font-medium text-foreground transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60"
          >
            <RefreshCw
              size={16}
              aria-hidden="true"
              className={loadingTeams ? "animate-spin" : ""}
            />
            {t("refresh")}
          </button>
        </div>

        <div className="mt-5 grid gap-3 md:grid-cols-[1fr_auto]">
          <label className="block">
            <span className="mb-1 block text-xs font-medium uppercase tracking-wide text-muted-foreground">
              {t("newTeamLabel")}
            </span>
            <input
              value={newTeamName}
              onChange={(event) => setNewTeamName(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter") handleCreateTeam();
              }}
              placeholder={t("newTeamPlaceholder")}
              className="w-full rounded-lg border border-input bg-background px-3 py-2 text-sm text-foreground shadow-sm outline-none transition focus-visible:ring-2 focus-visible:ring-ring"
            />
          </label>
          <button
            type="button"
            onClick={handleCreateTeam}
            disabled={!trimmedNewTeamName || actionBusy !== null}
            className="inline-flex items-center justify-center gap-2 self-end rounded-lg bg-brand px-4 py-2 text-sm font-semibold text-white transition hover:bg-brand/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60"
          >
            <Plus size={16} aria-hidden="true" />
            {t("createTeam")}
          </button>
        </div>

        <div className="mt-4 rounded-xl border border-border bg-muted/40 p-3 text-xs text-muted-foreground">
          <ShieldCheck
            size={14}
            className="mr-1 inline-block text-emerald-500"
            aria-hidden="true"
          />
          {t("identityBoundary")}
        </div>
      </div>

      {(error || notice) && (
        <div
          className={`flex items-start gap-2 rounded-xl border px-4 py-3 text-sm ${
            error
              ? "border-red-200 bg-red-50 text-red-700 dark:border-red-900/60 dark:bg-red-950/40 dark:text-red-300"
              : "border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900/60 dark:bg-emerald-950/40 dark:text-emerald-300"
          }`}
          role="status"
        >
          {error ? (
            <AlertCircle size={16} aria-hidden="true" />
          ) : (
            <CheckCircle2 size={16} aria-hidden="true" />
          )}
          <span>{error ?? notice}</span>
        </div>
      )}

      <div className="grid gap-5 lg:grid-cols-[minmax(220px,300px)_1fr]">
        <div className="rounded-2xl border border-border bg-card p-3 shadow-sm">
          <div className="mb-2 flex items-center justify-between px-2">
            <h3 className="text-sm font-semibold text-foreground">
              {t("teamList")}
            </h3>
            <span className="text-xs text-muted-foreground">
              {t("teamCount", { count: teams.length })}
            </span>
          </div>

          {loadingTeams ? (
            <div className="rounded-xl border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
              {t("loadingTeams")}
            </div>
          ) : teams.length === 0 ? (
            <div className="rounded-xl border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
              {t("noTeams")}
            </div>
          ) : (
            <div className="space-y-2">
              {teams.map((team) => {
                const selected = team.id === selectedTeamId;
                return (
                  <button
                    key={team.id}
                    type="button"
                    onClick={() => setSelectedTeamId(team.id)}
                    className={`w-full rounded-xl border p-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${
                      selected
                        ? "border-brand bg-brand/10 text-foreground"
                        : "border-border bg-background hover:bg-muted"
                    }`}
                  >
                    <span className="block truncate text-sm font-semibold">
                      {team.name}
                    </span>
                    <span className="mt-2 flex items-center justify-between gap-2 text-xs text-muted-foreground">
                      <span
                        className={`rounded-full border px-2 py-0.5 ${roleBadgeClass(
                          team.myMembership.teamRole,
                        )}`}
                      >
                        {t(`role.${team.myMembership.teamRole}`)}
                      </span>
                      <span>
                        {t("revision", {
                          revision: team.membershipRevision,
                        })}
                      </span>
                    </span>
                  </button>
                );
              })}
            </div>
          )}
        </div>

        <div className="min-w-0 rounded-2xl border border-border bg-card p-5 shadow-sm">
          {!selectedTeam ? (
            <div className="rounded-xl border border-dashed border-border p-8 text-center text-sm text-muted-foreground">
              {t("selectTeamEmpty")}
            </div>
          ) : (
            <div className="space-y-6">
              <div className="flex flex-col gap-4 border-b border-border pb-5 md:flex-row md:items-start md:justify-between">
                <div className="min-w-0 flex-1">
                  <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                    {t("selectedTeam")}
                  </p>
                  <h3 className="mt-1 truncate text-xl font-semibold text-foreground">
                    {selectedTeam.name}
                  </h3>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {t("updatedAt", {
                      value: formatDate(selectedTeam.updatedAt),
                    })}
                  </p>
                </div>
                <button
                  type="button"
                  onClick={handleLeaveTeam}
                  disabled={actionBusy !== null}
                  className="inline-flex items-center justify-center gap-2 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm font-medium text-red-700 transition-colors hover:bg-red-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300 dark:hover:bg-red-950/50"
                >
                  <LogOut size={16} aria-hidden="true" />
                  {t("leaveTeam")}
                </button>
              </div>

              <div className="grid gap-3 md:grid-cols-[1fr_auto]">
                <label className="block">
                  <span className="mb-1 block text-xs font-medium uppercase tracking-wide text-muted-foreground">
                    {t("renameLabel")}
                  </span>
                  <input
                    value={renameValue}
                    onChange={(event) => setRenameValue(event.target.value)}
                    disabled={!canManageTeam}
                    className="w-full rounded-lg border border-input bg-background px-3 py-2 text-sm text-foreground shadow-sm outline-none transition focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60"
                  />
                </label>
                <button
                  type="button"
                  onClick={handleRenameTeam}
                  disabled={
                    !canManageTeam ||
                    !trimmedRenameValue ||
                    trimmedRenameValue === selectedTeam.name ||
                    actionBusy !== null
                  }
                  className="inline-flex items-center justify-center gap-2 self-end rounded-lg border border-border bg-background px-4 py-2 text-sm font-medium text-foreground transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60"
                >
                  {t("saveName")}
                </button>
              </div>

              <div className="space-y-3">
                <div className="flex items-center justify-between gap-3">
                  <h4 className="text-sm font-semibold text-foreground">
                    {t("members")}
                  </h4>
                  {loadingDetails && (
                    <span className="text-xs text-muted-foreground">
                      {t("loadingDetails")}
                    </span>
                  )}
                </div>
                <div className="overflow-hidden rounded-xl border border-border">
                  {members.length === 0 ? (
                    <div className="p-5 text-sm text-muted-foreground">
                      {t("noMembers")}
                    </div>
                  ) : (
                    members.map((member) => (
                      <div
                        key={member.userId}
                        className="grid gap-3 border-b border-border p-3 last:border-b-0 md:grid-cols-[1fr_auto] md:items-center"
                      >
                        <div className="min-w-0">
                          <p className="truncate text-sm font-medium text-foreground">
                            {member.displayName || member.userId}
                          </p>
                          <p className="mt-1 text-xs text-muted-foreground">
                            {t("joinedAt", {
                              value: formatDate(member.joinedAt),
                            })}
                          </p>
                        </div>
                        <div className="flex flex-wrap items-center gap-2">
                          <span
                            className={`rounded-full border px-2 py-1 text-xs font-medium ${roleBadgeClass(
                              member.teamRole,
                            )}`}
                          >
                            {t(`role.${member.teamRole}`)}
                          </span>
                          {roleOptions.map((role) => (
                            <button
                              key={role}
                              type="button"
                              onClick={() => handleRoleChange(member, role)}
                              disabled={
                                !canManageTeam ||
                                member.teamRole === role ||
                                actionBusy !== null
                              }
                              className="rounded-lg border border-border bg-background px-2.5 py-1 text-xs font-medium text-foreground transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
                            >
                              {t(`setRole.${role}`)}
                            </button>
                          ))}
                        </div>
                      </div>
                    ))
                  )}
                </div>
              </div>

              <div className="space-y-3">
                <div>
                  <h4 className="text-sm font-semibold text-foreground">
                    {t("invites")}
                  </h4>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {canManageTeam
                      ? t("inviteDescription")
                      : t("adminOnlyDescription")}
                  </p>
                </div>

                <div className="grid gap-3 rounded-xl border border-border bg-muted/30 p-3 md:grid-cols-[1fr_140px_auto]">
                  <input
                    type="email"
                    value={inviteEmail}
                    onChange={(event) => setInviteEmail(event.target.value)}
                    disabled={!canManageTeam}
                    placeholder={t("inviteEmailPlaceholder")}
                    className="rounded-lg border border-input bg-background px-3 py-2 text-sm text-foreground outline-none transition focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60"
                  />
                  <select
                    value={inviteRole}
                    onChange={(event) =>
                      setInviteRole(event.target.value as TeamRole)
                    }
                    disabled={!canManageTeam}
                    className="rounded-lg border border-input bg-background px-3 py-2 text-sm text-foreground outline-none transition focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60"
                  >
                    {roleOptions.map((role) => (
                      <option key={role} value={role}>
                        {t(`role.${role}`)}
                      </option>
                    ))}
                  </select>
                  <button
                    type="button"
                    onClick={handleInviteMember}
                    disabled={
                      !canManageTeam ||
                      !trimmedInviteEmail ||
                      actionBusy !== null
                    }
                    className="inline-flex items-center justify-center gap-2 rounded-lg bg-brand px-4 py-2 text-sm font-semibold text-white transition hover:bg-brand/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60"
                  >
                    <MailPlus size={16} aria-hidden="true" />
                    {t("sendInvite")}
                  </button>
                </div>

                <div className="overflow-hidden rounded-xl border border-border">
                  {invites.length === 0 ? (
                    <div className="p-5 text-sm text-muted-foreground">
                      {t("noInvites")}
                    </div>
                  ) : (
                    invites.map((invite) => (
                      <div
                        key={invite.id}
                        className="grid gap-3 border-b border-border p-3 last:border-b-0 md:grid-cols-[1fr_auto] md:items-center"
                      >
                        <div className="min-w-0">
                          <p className="truncate text-sm font-medium text-foreground">
                            {invite.maskedEmail}
                          </p>
                          <p className="mt-1 text-xs text-muted-foreground">
                            {t("expiresAt", {
                              value: formatDate(invite.expiresAt),
                            })}
                          </p>
                        </div>
                        <div className="flex flex-wrap items-center gap-2">
                          <span
                            className={`rounded-full border px-2 py-1 text-xs font-medium ${roleBadgeClass(
                              invite.teamRole,
                            )}`}
                          >
                            {t(`role.${invite.teamRole}`)}
                          </span>
                          <span
                            className={`rounded-full border px-2 py-1 text-xs font-medium ${statusBadgeClass(
                              invite.status,
                            )}`}
                          >
                            {t(`inviteStatus.${invite.status}`)}
                          </span>
                          <span
                            className={`rounded-full border px-2 py-1 text-xs font-medium ${statusBadgeClass(
                              invite.deliveryStatus,
                            )}`}
                          >
                            {t(`deliveryStatus.${invite.deliveryStatus}`)}
                          </span>
                          <button
                            type="button"
                            onClick={() => handleRevokeInvite(invite)}
                            disabled={
                              !canManageTeam ||
                              invite.status !== "pending" ||
                              actionBusy !== null
                            }
                            className="inline-flex items-center gap-1 rounded-lg border border-border bg-background px-2.5 py-1 text-xs font-medium text-foreground transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
                          >
                            <Trash2 size={13} aria-hidden="true" />
                            {t("revokeInvite")}
                          </button>
                        </div>
                      </div>
                    ))
                  )}
                </div>
              </div>
            </div>
          )}
        </div>
      </div>
    </section>
  );
}
