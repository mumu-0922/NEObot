import type { AuthUserDTO, LoginResult } from "./types";

const SERVER_AUTH_SESSION_STORAGE_KEY = "mm-chat.server-auth-session.v1";

export interface ServerAuthSession {
  token: string;
  user: AuthUserDTO;
  expiresAt?: string;
}

function getSessionStorage(): Storage | null {
  if (typeof window === "undefined") return null;
  try {
    return window.sessionStorage;
  } catch {
    return null;
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function parseSession(value: string | null): ServerAuthSession | null {
  if (!value) return null;

  try {
    const parsed = JSON.parse(value) as unknown;
    if (!isRecord(parsed) || typeof parsed.token !== "string") return null;
    if (!isRecord(parsed.user)) return null;
    const user = parsed.user;
    if (
      typeof user.id !== "string" ||
      typeof user.displayName !== "string" ||
      typeof user.role !== "string"
    ) {
      return null;
    }
    if (
      parsed.expiresAt !== undefined &&
      typeof parsed.expiresAt !== "string"
    ) {
      return null;
    }

    return {
      token: parsed.token,
      user: {
        id: user.id,
        displayName: user.displayName,
        role:
          user.role === "owner" || user.role === "viewer" ? user.role : "user",
      },
      ...(parsed.expiresAt ? { expiresAt: parsed.expiresAt } : {}),
    };
  } catch {
    return null;
  }
}

function isExpired(session: ServerAuthSession, now = Date.now()): boolean {
  if (!session.expiresAt) return false;
  const expiresAt = Date.parse(session.expiresAt);
  return Number.isFinite(expiresAt) && expiresAt <= now;
}

export function getServerAuthSession(): ServerAuthSession | null {
  const storage = getSessionStorage();
  const session = parseSession(
    storage?.getItem(SERVER_AUTH_SESSION_STORAGE_KEY) ?? null,
  );
  if (!session) return null;
  if (isExpired(session)) {
    clearServerAuthSession();
    return null;
  }
  return session;
}

export function getServerAuthToken(): string | null {
  return getServerAuthSession()?.token ?? null;
}

export function setServerAuthSession(result: LoginResult): ServerAuthSession {
  if (!result.token) {
    throw new Error("Server auth login did not return a session token");
  }
  const session: ServerAuthSession = {
    token: result.token,
    user: result.user,
    ...(result.expiresAt ? { expiresAt: result.expiresAt } : {}),
  };
  getSessionStorage()?.setItem(
    SERVER_AUTH_SESSION_STORAGE_KEY,
    JSON.stringify(session),
  );
  return session;
}

export function clearServerAuthSession(): void {
  getSessionStorage()?.removeItem(SERVER_AUTH_SESSION_STORAGE_KEY);
}

export const serverAuthSessionStorageKey = SERVER_AUTH_SESSION_STORAGE_KEY;
