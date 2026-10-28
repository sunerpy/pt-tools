import vue from "@vitejs/plugin-vue";
import { resolve } from "node:path";
import { defineConfig } from "vite";
export default defineConfig({
  base: "./",
  plugins: [vue()],
  resolve: { alias: { "@": resolve(import.meta.dirname, "src") } },
  build: {
    outDir: "../../apps/mobile/assets/mobile",
    emptyOutDir: true,
    rollupOptions: { input: resolve(import.meta.dirname, "mobile.html") },
  },
});
