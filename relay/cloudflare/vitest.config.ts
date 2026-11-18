import { cloudflareTest } from "@cloudflare/vitest-pool-workers";
import { defineConfig } from "vitest/config";

// 测试跑在 workerd 里（Miniflare），用 wrangler.jsonc 的绑定；限额调小，单测能很快碰到
export default defineConfig({
  plugins: [
    cloudflareTest({
      wrangler: { configPath: "./wrangler.jsonc" },
      miniflare: {
        bindings: {
          MAX_STREAMS_PER_HOST: "2",
          DAILY_BYTES_PER_HOST: "4096",
          MAX_CONN_PER_IP_PER_MIN: "-1",
          RELAY_VERSION: "test",
        },
      },
    }),
  ],
});
