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

/** 单独打包成一个 IIFE 文件：内容脚本与一键采集时注册进页面（MAIN world）的请求记录器都不能用模块 */
async function buildIife(entry: string, name: string, fileName: string): Promise<void> {
  await build({
    configFile: false,
    build: {
      outDir: "dist",
      emptyOutDir: false,
      lib: {
        entry: resolve(__dirname, entry),
        name,
        formats: ["iife"],
        fileName: () => fileName,
      },
      rollupOptions: {
        output: {
          inlineDynamicImports: true,
        },
      },
    },
  });
}

function contentScriptPlugin(): Plugin {
  return {
    name: "content-script-iife",
    apply: "build",
    async closeBundle() {
      await buildIife("src/content/index.ts", "ptToolsContent", "content.js");
      await buildIife("src/recorder/entry.ts", "ptToolsRequestRecorder", "request-recorder.js");
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
