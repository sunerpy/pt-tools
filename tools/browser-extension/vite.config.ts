import { resolve } from "node:path";
import { build, defineConfig, type Plugin } from "vite";
import vue from "@vitejs/plugin-vue";

import { manifest } from "./src/manifest";

function extensionManifestPlugin(): Plugin {
  return {
    name: "extension-manifest-plugin",
    apply: "build",
    generateBundle() {
      this.emitFile({
        type: "asset",
        fileName: "manifest.json",
        source: JSON.stringify(manifest, null, 2),
      });
    },
  };
}

function contentScriptPlugin(): Plugin {
  return {
    name: "content-script-iife",
    apply: "build",
    async closeBundle() {
      await build({
        configFile: false,
        build: {
          outDir: "dist",
          emptyOutDir: false,
          lib: {
            entry: resolve(__dirname, "src/content/index.ts"),
            name: "ptToolsContent",
            formats: ["iife"],
            fileName: () => "content.js",
          },
          rollupOptions: {
            output: {
              inlineDynamicImports: true,
            },
          },
        },
      });
    },
  };
}

export default defineConfig({
  publicDir: "public",
  plugins: [vue(), extensionManifestPlugin(), contentScriptPlugin()],
  base: "./",
  build: {
    outDir: "dist",
    emptyOutDir: true,
    rollupOptions: {
      input: {
        popup: resolve(__dirname, "src/popup/index.html"),
        background: resolve(__dirname, "src/background/index.ts"),
      },
      output: {
        entryFileNames: (chunkInfo): string => {
          if (chunkInfo.name === "background") {
            return "[name].js";
          }
          return "assets/[name]-[hash].js";
        },
        chunkFileNames: "assets/[name]-[hash].js",
        assetFileNames: "assets/[name]-[hash][extname]",
      },
    },
  },
});
