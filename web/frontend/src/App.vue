<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
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
const railRef = ref<InstanceType<typeof AppRail> | null>(null);

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

/**
 * 侧栏的开关。语义随形态变：钉住→收起，宽屏收起→展开，窄屏→开合抽屉。
 *
 * 开关自己长在会淡出的那一块里（收起钮在 nav 头部、展开钮在 rail 上），点完之后
 * 原来聚焦的按钮就 visibility:hidden 了，焦点会掉回 body —— 键盘用户得从页头重新 Tab。
 * 所以状态一变就把焦点交给对面那枚：露出态的规则是 `visibility 0s`，同一个 tick 就能 focus 上。
 */
async function toggleNav() {
  if (!wide.value) {
    if (navOpen.value) await closeDrawer();
    else await openDrawer();
    return;
  }
  setCollapsed(!navCollapsed.value);
  await nextTick();
  if (navDocked.value) navRef.value?.focusToggle();
  else railRef.value?.focusToggle();
}

/** 抽屉打开：焦点进抽屉、落在关闭钮上 —— 打开它的 rail 开关跟着 rail 一起淡出了，不接就掉到 body */
async function openDrawer() {
  if (navOpen.value) return;
  navOpen.value = true;
  await nextTick();
  navRef.value?.focusToggle();
}

/** 抽屉被 Esc / 遮罩 / 关闭钮收掉：焦点交回打开它的那枚开关（≤768 时 rail 不在，交给顶栏） */
async function closeDrawer() {
  if (!navOpen.value) return;
  navOpen.value = false;
  await nextTick();
  railRef.value?.focusToggle();
  /*
   * ≤768 时 rail 是 display:none，上面那一下 focus 是空操作；而此刻焦点还停在正在淡出的
   * 抽屉关闭钮上（visibility 要 240ms 后才变 hidden），不能用「焦点是否在 body」来判 ——
   * 直接看焦点有没有落到 rail 开关上，没有就交给顶栏。
   */
  if (!document.activeElement?.classList.contains("pt-rail__tool--nav")) {
    document.querySelector<HTMLElement>(".pt-mchrome__icon[aria-controls='pt-nav-col']")?.focus();
  }
}

const navToggleIcon = computed(() => {
  if (!wide.value) return "menu" as const;
  return navDocked.value ? ("panel-left-close" as const) : ("panel-left-open" as const);
});

const navToggleLabel = computed(() => {
  if (!wide.value) return navOpen.value ? "关闭导航" : "打开导航";
  return navDocked.value ? "收起导航" : "展开导航";
});

/**
 * 详情页 —— 画板里只有这两类页面的 head 是 88 高且带面包屑（13 站点详情 / 23 通道详情）。
 * 其余页面一律 64 高，标题下面那一行是实时摘要而不是面包屑。
 */
const DETAIL_ROUTES = new Set(["site-detail", "notification-detail"]);
const isDetail = computed(() => typeof route.name === "string" && DETAIL_ROUTES.has(route.name));

/**
 * 总览类页面没有页头 —— 画板 02 工作台与 10 用户统计的主区顶上是 KPI 带（328,0 1112×64），
 * 不是 head。页面身份由导航列的高亮项表达，KPI 带里的真实数字替代了摘要行。
 * 导航收起开关已经移到 rail，所以去掉页头不会把它一起带走。
 */
/*
 * 种子搜索同理：画板 15 的 search-head（328,0 1112×88）里没有标题也没有摘要，
 * 装的是 720 宽的查询框 + 搜索按钮 + 两个页头动作 + 一行 4 个筛选 chip。
 * 那是页面自己的控件，不是外壳的标题栏，所以页头交给页面画。
 */
const OWN_TOP_ROUTES = new Set(["userinfo", "search"]);
/*
 * 首次导航完成之前 route.matched 是空的、route.name 是 undefined —— 之前那一刻 hasHead 判成 true，
 * 冷启动打开 /userinfo 或 /search 时先画出一条 64px 的「pt-tools」页头，懒加载的页面到了再消失，
 * 内容整体往上跳 64。路由还没解析出来时先不画页头。
 */
const hasHead = computed(
  () =>
    route.matched.length > 0 && !(typeof route.name === "string" && OWN_TOP_ROUTES.has(route.name)),
);

/*
 * 沉浸模式（整屏让给页面、藏掉 rail / 导航列 / 页头 / 状态条）目前没有页面在用。
 *
 * 下载器 Web UI 原来走这条路，但画板 18 明确把它画在外壳里：
 * head 64 → bar-64 → 左列 276 / 右 788，rail 与导航列都在。
 * 规则与样式先留着（`.pt-shell.is-immersive`），将来真需要整屏时把路由名加回来即可。
 */
const isImmersive = computed(() => false);

const activePath = computed(() => activeNavPath(route.name));

const navItem = computed(() => NAV_ITEMS.find((i) => i.path === activePath.value));

const groupTitle = computed(
  () => NAV_GROUPS.find((g) => g.items.some((i) => i.path === activePath.value))?.title ?? "",
);

const pageTitle = computed(() => {
  /*
   * 详情页的标题是「这一条是谁」，不是页面类型名 —— 画板 13 的 h1 写的是
   * 「PTerClub」而不是「站点详情」，面包屑末节也是同一个名字。站点名就在路由参数里，
   * 不用页面再往上送一次。
   */
  if (route.name === "site-detail" && typeof route.params.name === "string") {
    return route.params.name;
  }
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
  /*
   * 详情页只有两节：所属列表页 + 这一条的名字（画板 13 的 crumb 就是
   * 「站点列表 › PTerClub」）。「首页」和分组名对回上一层没有帮助 —— 分组名根本
   * 不可点，而详情页唯一要回的地方就是它的列表页。
   */
  if (!isDetail.value) {
    if (activePath.value !== HOME_PATH) out.push({ label: "首页", to: HOME_PATH });
    if (groupTitle.value) out.push({ label: groupTitle.value });
  }
  if (navItem.value && navItem.value.label !== pageTitle.value) {
    out.push({ label: navItem.value.label, to: navItem.value.path });
  }
  out.push({ label: pageTitle.value });
  return out;
});

function onKeydown(e: KeyboardEvent) {
  /* Chrome 选自动填充建议时派发的 keydown 没有 key，直接 toLowerCase 会抛 TypeError */
  if ((e.metaKey || e.ctrlKey) && e.key?.toLowerCase() === "k") {
    e.preventDefault();
    /*
     * 跳转框长在导航列里，先让它可见再送焦点：宽屏展开回栅格，窄屏拉出抽屉。
     * 宽屏只在内存里展开、不落盘 —— 之前调的是 setCollapsed(false)，会把「收起」偏好写回 localStorage，
     * 收起过导航的用户用一次快捷跳转，之后每次刷新导航都是展开的。宽屏没有抽屉形态可用
     * （≥1181 的侧栏格子为了宽度过渡是裁切的），所以是展开而不是拉抽屉。
     */
    if (wide.value) navCollapsed.value = false;
    else navOpen.value = true;
    navRef.value?.focusSearch();
    return;
  }
  if (e.key === "Escape" && navOpen.value) void closeDrawer();
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
    <!--
      侧栏是**一块**，只有两个宽度：收起 64（rail：品牌 + 8 个快捷入口 + 日志 / 明暗 / 头像），
      展开 264（nav：品牌 + 跳转框 + 6 组 19 项 + 账号行）。两者叠在同一个格子里交叉淡入淡出，
      格子本身的宽度过渡 —— 主区跟着平移，而不是一步跳到位。
      此前是 rail 64 + nav 264 两列并排：钉住时 rail 除了品牌和底部两个钮全空，
      黑色空条贴着白色导航列，两枚 logo、两个头像；用户原话「应该是一体的」。
    -->
    <div class="pt-side">
      <AppRail
        ref="railRef"
        :active-path="activePath"
        :nav-toggle-icon="navToggleIcon"
        :nav-toggle-label="navToggleLabel"
        @toggle-nav="toggleNav" />

      <div id="pt-nav-col" class="pt-shell__nav-col">
        <AppNav
          ref="navRef"
          :active-path="activePath"
          :drawer="!navDocked"
          :visible="navDocked || navOpen"
          @navigate="navOpen = false"
          @close="closeDrawer"
          @toggle-nav="toggleNav" />
      </div>
    </div>

    <main class="pt-shell__main">
      <MobileChrome :active-path="activePath" :title="pageTitle" @open-nav="openDrawer" />

      <!--
        页头 —— 画板 head：列表页 64 高「标题 19/700 + 实时摘要 11.5/400」，
        详情页 88 高，标题上方多一行面包屑。

        摘要和操作按钮由各页 Teleport 送进来（#pt-head-sub / #pt-head-acts）：
        画板上每页的摘要都是真实数字（「37 个任务 · 12 下载中 · ↓93.8 MB/s」这类），
        只有页面自己知道，做不成外壳里的通用逻辑。

        之前这里给每一页都挂了 88 高的面包屑，稿子里面包屑只属于详情页 ——
        列表页那一行位置放的是摘要。
      -->
      <header v-if="hasHead" class="pt-head" :class="{ 'pt-head--crumbs': isDetail }">
        <div class="pt-head__group">
          <nav v-if="isDetail" class="pt-head__crumbs" aria-label="面包屑">
            <template v-for="(c, i) in crumbs" :key="`${c.label}-${i}`">
              <PtIcon v-if="i > 0" name="chevron-right" :size="13" />
              <router-link v-if="c.to" :to="c.to">{{ c.label }}</router-link>
              <span v-else>{{ c.label }}</span>
            </template>
          </nav>
          <h1 class="pt-head__title">{{ pageTitle }}</h1>
          <div id="pt-head-sub" class="pt-head__sub" />
        </div>
        <!--
          注意：≤768px 时整条页头被移动端外壳替掉（shell.css 里 .pt-head display:none），
          所以送进这两个靶子的东西在手机上是看不见的。页面把主操作 Teleport 进来时
          必须写成 `<Teleport to="#pt-head-acts" :disabled="isMobile">` ——
          窄屏就地留在工具栏里，否则手机上会连入口都没有。
        -->
        <div id="pt-head-acts" class="pt-head__acts" />
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
      @click="closeDrawer" />
  </div>
</template>
