<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRoute } from "vue-router";
import PtIcon from "./components/PtIcon";
import AppNav from "./components/shell/AppNav.vue";
import AppRail from "./components/shell/AppRail.vue";
import AppStatusBar from "./components/shell/AppStatusBar.vue";
import MobileChrome from "./components/shell/MobileChrome.vue";
import V2DeprecationBanner from "./components/V2DeprecationBanner.vue";
import { activeNavPath, NAV_GROUPS, NAV_ITEMS } from "./config/navigation";
import { useLogLevelStore } from "./stores/logLevel";
import { useRuntimeStore } from "./stores/runtime";
import { useVersionStore } from "./stores/version";

/**
 * 应用外壳 —— Penpot 页面 G Cockpit 板 41（全局外壳）。
 *
 * 旧顶栏的七个全局控件按板 41 的落位表分散到三处，没有一个丢掉：
 *   停止/启动任务、版本与更新  → 底部状态条（AppStatusBar）
 *   明暗、配色、日志级别、退出登录 → rail 头像 popover（ThemePrefs）
 *   标题 + 面包屑                  → 本文件的页头
 */
const route = useRoute();
const logLevelStore = useLogLevelStore();
const versionStore = useVersionStore();
const runtimeStore = useRuntimeStore();

const navOpen = ref(false);
const navRef = ref<InstanceType<typeof AppNav> | null>(null);

/** 下载器 Web UI 是独立控制台，整屏让给它（沿用旧的 is-immersive 行为） */
const isImmersive = computed(() => route.name === "downloader-hub");

const activePath = computed(() => activeNavPath(route.name));

const navItem = computed(() => NAV_ITEMS.find((i) => i.path === activePath.value));

const groupTitle = computed(
  () => NAV_GROUPS.find((g) => g.items.some((i) => i.path === activePath.value))?.title ?? "",
);

const pageTitle = computed(() => {
  const metaTitle = route.meta.title;
  if (typeof metaTitle === "string" && metaTitle) return metaTitle;
  return navItem.value?.label ?? "pt-tools";
});

const crumbs = computed(() => {
  const out: { label: string; to?: string }[] = [{ label: "首页", to: "/userinfo" }];
  if (groupTitle.value) out.push({ label: groupTitle.value });
  if (navItem.value && navItem.value.label !== pageTitle.value) {
    out.push({ label: navItem.value.label, to: navItem.value.path });
  }
  out.push({ label: pageTitle.value });
  return out;
});

function onKeydown(e: KeyboardEvent) {
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
    e.preventDefault();
    navOpen.value = true;
    navRef.value?.focusSearch();
    return;
  }
  if (e.key === "Escape" && navOpen.value) navOpen.value = false;
}

onMounted(() => {
  logLevelStore.fetchLogLevel();
  versionStore.fetchVersionInfo();
  versionStore.checkForUpdates(undefined, true);
  runtimeStore.startPolling();
  window.addEventListener("keydown", onKeydown);
});

onBeforeUnmount(() => {
  runtimeStore.stopPolling();
  window.removeEventListener("keydown", onKeydown);
});

// 标签页标题跟着路由走，多开几个页签时能分清
watch(
  pageTitle,
  (t) => {
    document.title = t === "pt-tools" ? "pt-tools" : `${t} · pt-tools`;
  },
  { immediate: true },
);

// 路由一变就收起抽屉，免得移动端点完还盖着内容
watch(
  () => route.fullPath,
  () => {
    navOpen.value = false;
  },
);
</script>

<template>
  <div class="pt-shell" :class="{ 'is-immersive': isImmersive, 'is-nav-open': navOpen }">
    <AppRail :active-path="activePath" />

    <div class="pt-shell__nav-col">
      <AppNav
        ref="navRef"
        :active-path="activePath"
        :drawer="navOpen"
        @navigate="navOpen = false" />
    </div>

    <main class="pt-shell__main">
      <MobileChrome :active-path="activePath" :title="pageTitle" @open-nav="navOpen = true" />

      <header class="pt-head pt-head--crumbs">
        <button
          type="button"
          class="pt-head__toggle"
          aria-label="打开导航"
          @click="navOpen = !navOpen">
          <PtIcon name="menu" :size="18" />
        </button>
        <div class="pt-head__group">
          <h1 class="pt-head__title">{{ pageTitle }}</h1>
          <nav class="pt-head__crumbs" aria-label="面包屑">
            <template v-for="(c, i) in crumbs" :key="`${c.label}-${i}`">
              <PtIcon v-if="i > 0" name="chevron-right" :size="12" />
              <router-link v-if="c.to" :to="c.to">{{ c.label }}</router-link>
              <span v-else>{{ c.label }}</span>
            </template>
          </nav>
        </div>
      </header>

      <div class="pt-shell__content">
        <div class="pt-shell__inner">
          <V2DeprecationBanner />
          <router-view v-slot="{ Component }">
            <transition name="fade" mode="out-in">
              <component :is="Component" />
            </transition>
          </router-view>
        </div>
      </div>
    </main>

    <AppStatusBar />

    <button
      v-if="navOpen"
      type="button"
      class="pt-shell__scrim"
      aria-label="关闭导航"
      @click="navOpen = false" />
  </div>
</template>
