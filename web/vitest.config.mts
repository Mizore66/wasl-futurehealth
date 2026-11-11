import { defineConfig } from "vitest/config";

export default defineConfig({
  resolve: {
    alias: [
      { find: "@", replacement: import.meta.dirname },
      // Next resolves `server-only` itself (its own empty module on the server); tests run as server code too.
      { find: /^server-only$/, replacement: "next/dist/compiled/server-only/empty.js" },
    ],
  },
  test: { include: ["tests/**/*.test.ts"], environment: "node" },
});
