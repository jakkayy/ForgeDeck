import tailwindcss from "@tailwindcss/vite";
import { tanstackRouter } from "@tanstack/router-plugin/vite";
import react from "@vitejs/plugin-react";
import { defineConfig, loadEnv } from "vite";

export default defineConfig(({ mode }) => {
  // In development the browser talks only to Vite, which forwards API calls,
  // so cookies and SSE behave exactly as they do behind the Go server.
  const apiTarget = loadEnv(mode, ".", "FORGEDECK_").FORGEDECK_API_URL ?? "http://127.0.0.1:8080";

  return {
    // The router plugin must run before React. autoCodeSplitting gives every
    // route its own chunk, so a page only downloads what it renders.
    plugins: [tanstackRouter({ target: "react", autoCodeSplitting: true }), react(), tailwindcss()],
    server: {
      proxy: {
        "/api": { target: apiTarget, changeOrigin: false },
        "/healthz": { target: apiTarget },
      },
    },
    build: {
      outDir: "dist",
      sourcemap: false,
    },
  };
});
