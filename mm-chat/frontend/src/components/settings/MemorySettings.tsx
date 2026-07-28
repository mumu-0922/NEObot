"use client";

import { useMemo } from "react";
import { createNeoChatApiClient } from "@/services/api/client";
import LocalMemorySettings from "./LocalMemorySettings";
import ServerMemoryGovernance from "./ServerMemoryGovernance";

const MemorySettings = () => {
  const apiClient = useMemo(() => createNeoChatApiClient(), []);

  if (apiClient.mode === "server" && apiClient.capabilities.memories) {
    return <ServerMemoryGovernance apiClient={apiClient} />;
  }

  return <LocalMemorySettings />;
};

export default MemorySettings;
