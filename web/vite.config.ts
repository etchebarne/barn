import { resolve } from "node:path"

import tailwindcss from "@tailwindcss/vite"
import { tanstackRouter } from "@tanstack/router-plugin/vite"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vitest/config"

// The Go backend (openbotd) listens here during development. Override with OPENBOT_BACKEND_URL.
const BACKEND = process.env.OPENBOT_BACKEND_URL ?? "http://127.0.0.1:8080"

export default defineConfig({
  plugins: [
    // Must run before the React plugin.
    tanstackRouter({
      target: "react",
      autoCodeSplitting: true,
      routesDirectory: "./src/routes",
      generatedRouteTree: "./src/routeTree.gen.ts",
    }),
    react(),
    tailwindcss(),
  ],
  resolve: {
    alias: {
      "@": resolve(import.meta.dirname, "./src"),
    },
  },
  server: {
    proxy: {
      // Same-origin API and WebSocket in dev. The Host header is kept (no changeOrigin) so the
      // backend's Origin check sees a same-origin request.
      "/api": { target: BACKEND, ws: true },
      // Webhook URLs are built from the request's host, so they point here in dev.
      "/hooks": { target: BACKEND },
      // Services send the browser back here after a sign-in.
      "/oauth": { target: BACKEND },
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    css: false,
    restoreMocks: true,
  },
})
