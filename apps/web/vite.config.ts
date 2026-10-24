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
      rollupOptions: {
        output: {
          // a few shared chunks instead of dozens of tiny ones: fewer round trips on a slow link
          manualChunks(id: string) {
            if (/node_modules\/(react|react-dom|scheduler)\//.test(id)) return "react";
            if (/node_modules\/@tanstack\//.test(id)) return "tanstack";
            if (/node_modules\/(@heroui|@react-aria|@react-stately|@react-types|@internationalized|react-aria|react-aria-components|react-stately|tailwind-variants|tailwind-merge)\//.test(id)) return "ui";
          },
        },
      },
    },
  };
});
