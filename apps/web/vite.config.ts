import tailwindcss from "@tailwindcss/vite";
import { tanstackRouter } from "@tanstack/router-plugin/vite";
import react from "@vitejs/plugin-react";
import { defineConfig, loadEnv } from "vite";

export default defineConfig(({ mode }) => {
  // in development Vite forwards API calls so cookies and SSE match the Go server
  const apiTarget = loadEnv(mode, ".", "FORGEDECK_").FORGEDECK_API_URL ?? "http://127.0.0.1:8080";

  return {
    // the router plugin must run before React
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
