import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { mkdirSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";

const webDist = resolve(__dirname, "../internal/webui/dist");

export default defineConfig({
  base: "./",
  plugins: [
    react(),
    {
      name: "preserve-go-embed-placeholder",
      apply: "build",
      closeBundle() {
        mkdirSync(webDist, { recursive: true });
        writeFileSync(
          resolve(webDist, ".keep"),
          "Kept so go:embed compiles before frontend assets are built.\n",
        );
      },
    },
  ],
  build: {
    outDir: webDist,
    emptyOutDir: true,
    sourcemap: false,
  },
  server: {
    host: "127.0.0.1",
    port: 34115,
    strictPort: true,
  },
  test: {
    environment: "jsdom",
    setupFiles: "./src/test-setup.ts",
  },
});
