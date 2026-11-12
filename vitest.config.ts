import { fileURLToPath } from "node:url";
import { defineConfig } from "vitest/config";

export default defineConfig({
  esbuild: { jsx: "automatic", jsxImportSource: "plugin-jsx" },
  resolve: {
    alias: {
      "plugin-jsx/jsx-runtime": fileURLToPath(new URL("./ui/src/jsx/jsx-runtime.ts", import.meta.url)),
      "plugin-jsx/jsx-dev-runtime": fileURLToPath(new URL("./ui/src/jsx/jsx-dev-runtime.ts", import.meta.url)),
    },
  },
  test: {
    include: ["ui/src/**/*.test.ts", "ui/src/**/*.test.tsx"],
    exclude: [".build/**", "node_modules/**"],
  },
});
