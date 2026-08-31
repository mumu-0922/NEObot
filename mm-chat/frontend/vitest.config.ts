import { fileURLToPath } from "node:url";
import { defineConfig } from "vitest/config";

export default defineConfig({
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  test: {
    // Keep wall-clock acceptance budgets reproducible on shared CI runners.
    fileParallelism: false,
    include: ["src/__tests__/**/*.{test,spec}.{ts,tsx}"],
  },
});
