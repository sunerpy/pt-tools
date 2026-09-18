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

/**
 * 导航列的两种形态。
 *
 * 钉住（docked）时它在栅格里占一列，rail 上的 8 个快捷入口是重复的，藏掉；
 * 收起后 rail 的图标就是唯一的导航，此时导航列变成抽屉，靠 navOpen 开合。
 * 窄屏（≤1180）没有「钉住」这回事，塞不下两列。
 *
 * 收起是用户偏好，落 localStorage，刷新后保持 —— 特意收窄过的人不该每次重开都被撑开。
 */
const NAV_COLLAPSED_KEY = "nav-collapsed";
const WIDE_QUERY = "(min-width: 1181px)";

const navCollapsed = ref(localStorage.getItem(NAV_COLLAPSED_KEY) === "1");
const wide = ref(window.matchMedia(WIDE_QUERY).matches);
const navDocked = computed(() => wide.value && !navCollapsed.value);

let wideMql: MediaQueryList | null = null;

function onWideChange(e: MediaQueryListEvent) {
  wide.value = e.matches;
}

function setCollapsed(v: boolean) {
  navCollapsed.value = v;
  localStorage.setItem(NAV_COLLAPSED_KEY, v ? "1" : "0");
}

/** 页头只有一个按钮，语义随形态变：钉住→收起，宽屏收起→展开，窄屏→开合抽屉 */
function toggleNav() {
  if (!wide.value) {
    navOpen.value = !navOpen.value;
    return;
  }
  setCollapsed(!navCollapsed.value);
}

const navToggleIcon = computed(() => {
  if (!wide.value) return "menu" as const;
  return navDocked.value ? ("panel-left-close" as const) : ("panel-left-open" as const);
});

const navToggleLabel = computed(() => {
  if (!wide.value) return navOpen.value ? "关闭导航" : "打开导航";
  return navDocked.value ? "收起导航" : "展开导航";
});

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

/** 面包屑的根。产品里没有独立首页，/userinfo（用户统计）就是首页 */
const HOME_PATH = "/userinfo";

/**
 * 面包屑，规则是「链上不出现指向当前页的链接」。
 *
 * 之前根节点恒为「首页 → /userinfo」，而 /userinfo 本身就是用户统计页，
 * 于是在这一页上点「首页」原地不动，看着像坏了。当前页就是首页时直接省掉根节点，
 * 链读作「概览 › 用户统计」。
 */
const crumbs = computed(() => {
  const out: { label: string; to?: string }[] = [];
  if (activePath.value !== HOME_PATH) out.push({ label: "首页", to: HOME_PATH });
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
    // 跳转框长在导航列里，先让它可见再送焦点：宽屏展开回栅格，窄屏拉出抽屉
    if (wide.value) setCollapsed(false);
    else navOpen.value = true;
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
  wideMql = window.matchMedia(WIDE_QUERY);
  wideMql.addEventListener("change", onWideChange);
});

onBeforeUnmount(() => {
  runtimeStore.stopPolling();
  window.removeEventListener("keydown", onKeydown);
  wideMql?.removeEventListener("change", onWideChange);
  wideMql = null;
});

/*
 * 宽屏不用抽屉这种形态：要么钉在栅格里，要么只留 rail。
 * 所以从窄屏拖宽时把抽屉连遮罩一起收掉，否则它会带着一层 z-40 留在 1600 宽的屏幕上。
 */
watch(wide, (w) => {
  if (w) navOpen.value = false;
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
  <div
    class="pt-shell"
    :class="{
      'is-immersive': isImmersive,
      'is-nav-docked': navDocked,
      'is-nav-open': navOpen,
    }">
    <AppRail :active-path="activePath" />

    <div id="pt-nav-col" class="pt-shell__nav-col">
      <AppNav
        ref="navRef"
        :active-path="activePath"
        :drawer="!navDocked"
        :visible="navDocked || navOpen"
        @navigate="navOpen = false" />
    </div>

    <main class="pt-shell__main">
      <MobileChrome :active-path="activePath" :title="pageTitle" @open-nav="navOpen = true" />

      <header class="pt-head pt-head--crumbs">
        <button
          type="button"
          class="pt-head__toggle"
          :aria-label="navToggleLabel"
          :aria-expanded="navDocked || navOpen"
          aria-controls="pt-nav-col"
          @click="toggleNav">
          <PtIcon :name="navToggleIcon" :size="18" />
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
