"use client";

import React, { useEffect, useMemo, useState } from "react";
import { createNeoChatApiClient } from "@/services/api/client";
import {
  clearServerAuthSession,
  getServerAuthSession,
} from "@/services/api/client/authSession";
import AccessPasswordPage from "./AccessPasswordPage";
import ChatApp from "./ChatApp";

type GateState = "checking" | "authenticated" | "unauthenticated";

export default function ServerAuthGate() {
  const apiClient = useMemo(() => createNeoChatApiClient(), []);
  const [state, setState] = useState<GateState>("checking");

  useEffect(() => {
    let active = true;
    const session = getServerAuthSession();
    if (!session) {
      queueMicrotask(() => {
        if (active) setState("unauthenticated");
      });
      return;
    }

    apiClient.auth
      .getCurrentUser()
      .then(() => {
        if (active) setState("authenticated");
      })
      .catch(() => {
        clearServerAuthSession();
        if (active) setState("unauthenticated");
      });

    return () => {
      active = false;
    };
  }, [apiClient]);

  if (state === "checking") {
    return (
      <main className="min-h-dvh bg-background text-foreground">
        <div className="mx-auto flex min-h-dvh w-full max-w-md flex-col justify-center px-5 py-10 text-sm text-muted-foreground">
          Checking session…
        </div>
      </main>
    );
  }

  if (state === "authenticated") {
    return <ChatApp />;
  }

  return (
    <AccessPasswordPage
      mode="server-auth"
      onServerAuthSuccess={() => setState("authenticated")}
    />
  );
}
