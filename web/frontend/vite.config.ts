import vue from "@vitejs/plugin-vue";
import { resolve } from "node:path";
// 用 vitest/config 的 defineConfig：它就是 vite 的那个，只是额外认得下面的 test 段
import { defineConfig } from "vitest/config";

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
            return "vendor";
          }
        },
      },
    },
  },
  test: {
    // 见 src/test-setup.ts：happy-dom 的 storage 被 Node 25 的原生同名全局遮住了
    setupFiles: ["./src/test-setup.ts"],
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
