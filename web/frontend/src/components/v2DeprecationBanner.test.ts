// @vitest-environment happy-dom
/**
 * 「pt-tools v2.0 升级完成」只给从 v1 升级上来的库看：全新安装没有要说明的升级（以前全新安装也显示）。
 */
import { createPinia, setActivePinia } from "pinia";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { nextTick } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";

import V2DeprecationBanner from "./V2DeprecationBanner.vue";
import { useVersionStore } from "@/stores/version";
import { type MountedView, mountView } from "@/test-mount";

const KEY = "pt_tools_v2_banner_dismissed_v1";
let view: MountedView | undefined;

async function render(upgraded: boolean | undefined) {
  view?.unmount();
  setActivePinia(createPinia());
  useVersionStore().versionInfo = {
    version: "v1.0.0-rc.22",
    build_time: "",
    commit_id: "",
    upgraded_from_v1: upgraded,
  };
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/:p(.*)*", component: { template: "<div />" } }],
  });
  view = mountView(V2DeprecationBanner, { router });
  await nextTick();
  return document.querySelector('[data-testid="v2-deprecation-banner"]');
}

describe("V2DeprecationBanner", () => {
  beforeEach(() => window.localStorage.clear());
  afterEach(() => {
    view?.unmount();
    view = undefined;
  });

  it("全新安装不显示", async () => {
    expect(await render(false)).toBeNull();
    expect(await render(undefined)).toBeNull();
  });

  it("从 v1 升级上来的显示，关掉以后不再显示", async () => {
    expect(await render(true)).not.toBeNull();
    window.localStorage.setItem(KEY, "1");
    expect(await render(true)).toBeNull();
  });
});
