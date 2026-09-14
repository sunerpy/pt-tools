import vue from "@vitejs/plugin-vue";
import { resolve } from "node:path";
import { defineConfig } from "vite";

// https://vite.dev/config/
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      "@": resolve(import.meta.dirname, "src"),
    },
  },
  build: {
    outDir: "../static/dist",
    emptyOutDir: true,
    chunkSizeWarningLimit: 1500,
    rollupOptions: {
      onLog(level, log, defaultHandler) {
        if (
          level === "warn" &&
          log.code === "INVALID_ANNOTATION" &&
          typeof log.id === "string" &&
          log.id.includes("/node_modules/@vueuse/core/")
        ) {
          return;
        }
        defaultHandler(level, log);
      },
      output: {
        manualChunks(id) {
          if (id.includes("node_modules")) {
            if (id.includes("element-plus")) {
              return "element-plus";
            }
            if (id.includes("vue") || id.includes("pinia") || id.includes("vue-router")) {
              return "vue-vendor";
            }
            if (id.includes("@element-plus/icons-vue")) {
              return "element-plus-icons";
            }
            return "vendor";
          }
        },
      },
    },
  },
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        changeOrigin: true,
      },
      "/logout": {
        target: "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
});
