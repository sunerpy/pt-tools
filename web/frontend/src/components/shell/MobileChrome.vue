<script setup lang="ts">
import { computed, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { MOBILE_TABS, NAV_GROUPS } from "../../config/navigation";
import { useRuntimeStore } from "../../stores/runtime";
import { useThemeStore } from "../../stores/theme";
import PtIcon from "../PtIcon";
import VersionChecker from "../VersionChecker.vue";
import ThemePrefs from "./ThemePrefs.vue";

/**
 * 移动端外壳（画板 30–35 与设计文档 §9）。
 *
 * 顶栏按画板是一行：左边「h1 19/700 + sub 11/400」两行文本，右边两个 30×30 的
 * 图标钮（search、bell）。六块移动画板的 topbar 骨架完全一样，只有文案不同。
 * sub 是每页自己的实时摘要（「14 个 · 12 正常 · 2 异常」这类），所以它跟桌面页头
 * 一样是个 Teleport 靶子；桌面那个 `#pt-head-sub` 在移动端整条被 display:none 藏掉，
 * 摘要就没地方落，于是这里给它一个移动端专用的靶子（见 PtHeadSub.vue）。
 *
 * 与画板的两处刻意不同（用户验收退回过的三条硬约束优先）：
 *   · 左边多一个菜单钮。画板的移动稿没有任何打开导航列的控件，而五个 tab 只覆盖
 *     五个路由，没有它就到不了其余十几个页面（约束③「导航列必须可收起」的反面：
 *     收起之后要能再打开）。
 *   · 右边多一个明暗切换钮（约束①「主题切换入口必须一眼看得见」）。折在「我的」
 *     面板里的三段选择器仍在，那是它的超集。
 *
 * 「我的」在画板 34 上是一个设置聚合页，产品里没有这个路由，所以做成上拉抽屉，
 * 把「系统」「ChatOps」两组和偏好折进去 —— 行为等价且不新增路由。
 */
const props = defineProps<{ activePath: string; title: string }>();
const emit = defineEmits<{ "open-nav": [] }>();

/** 画板顶栏右侧那两个钮的去处。都是真实路由，不是装饰 */
const SEARCH_PATH = "/search";
const BELL_PATH = "/chatops/notifications";

const route = useRoute();
const router = useRouter();
const runtimeStore = useRuntimeStore();
const themeStore = useThemeStore();

/** 图标表示「点了会变成什么」，与 rail 上那个保持一致 */
const themeIcon = computed(() => (themeStore.isDark ? "sun" : "moon"));
const themeLabel = computed(() => (themeStore.isDark ? "切换到明亮模式" : "切换到黑暗模式"));

const sheetOpen = ref(false);

/** 折进「我的」的两组：系统 + ChatOps */
const sheetGroups = computed(() =>
  NAV_GROUPS.filter((g) => g.title === "系统" || g.title === "ChatOps"),
);

const activeTab = computed(() => {
  const hit = MOBILE_TABS.find((t) => t.path && t.path === props.activePath);
  return hit?.key ?? "";
});

/** 没有 path 的 tab 只有「我的」，它打开上拉面板而不是跳路由 */
function onTab(path?: string) {
  if (!path) {
    sheetOpen.value = true;
    return;
  }
  go(path);
}

function go(path: string) {
  if (route.path !== path) void router.push(path);
}

function goFromSheet(path: string) {
  sheetOpen.value = false;
  if (route.path !== path) void router.push(path);
}
</script>

<template>
  <header class="pt-mchrome">
    <button
      type="button"
      class="pt-mchrome__icon"
      aria-label="打开导航"
      aria-controls="pt-nav-col"
      @click="emit('open-nav')">
      <PtIcon name="menu" :size="16" />
    </button>

    <div class="pt-mchrome__group">
      <h1 class="pt-mchrome__title">{{ title }}</h1>
      <!--
        画板 topbar 的 sub。靶子空着时 CSS 的 :empty 会把它收掉（display:none），
        所以 /userinfo、/search 这种自己画头的页面不会多出一条空行。
      -->
      <div id="pt-mhead-sub" class="pt-mchrome__sub" />
    </div>

    <button type="button" class="pt-mchrome__icon" aria-label="种子搜索" @click="go(SEARCH_PATH)">
      <PtIcon name="search" :size="16" />
    </button>
    <button type="button" class="pt-mchrome__icon" aria-label="消息通知" @click="go(BELL_PATH)">
      <PtIcon name="bell" :size="16" />
    </button>
    <!-- 明暗一键直切。折在「我的」面板里的三段选择器还在，那是它的超集 -->
    <button
      type="button"
      class="pt-mchrome__icon"
      :aria-label="themeLabel"
      @click="themeStore.toggle">
      <PtIcon :name="themeIcon" :size="16" />
    </button>
  </header>

  <nav class="pt-mnav" aria-label="底部导航">
    <!--
      桌面状态条折进底栏顶部（round-2 的「chrome 条 + 折进来的状态行」；这条现在跟主题走，明亮主题下是浅色）。
      之前这些数字只藏在「我的」抽屉里，手机上等于看不到调度器和速率。
    -->
    <div class="pt-mnav__status" :class="{ 'is-stale': runtimeStore.stale }">
      <span class="pt-mnav__status-cell">
        <i class="pt-mnav__dot" :class="`is-${runtimeStore.schedulerHint}`" aria-hidden="true" />
        <span>{{ runtimeStore.schedulerText }}</span>
      </span>
      <span class="pt-mnav__status-cell pt-mnav__status-cell--live">
        <PtIcon name="download" :size="11" />
        <span>{{ runtimeStore.downloadText }}</span>
      </span>
      <span class="pt-mnav__status-cell pt-mnav__status-cell--live">
        <PtIcon name="upload" :size="11" />
        <span>{{ runtimeStore.uploadText }}</span>
      </span>
      <span class="pt-mnav__status-cell pt-mnav__status-cell--live">
        <PtIcon name="hard-drive" :size="11" />
        <span>{{ runtimeStore.freeSpaceText }}</span>
      </span>
    </div>

    <div class="pt-mnav__tabs">
      <button
        v-for="tab in MOBILE_TABS"
        :key="tab.key"
        type="button"
        class="pt-mnav__tab"
        :class="{ 'is-active': activeTab === tab.key }"
        :aria-current="activeTab === tab.key ? 'page' : undefined"
        @click="onTab(tab.path)">
        <PtIcon :name="tab.icon" :size="20" />
        <span>{{ tab.label }}</span>
      </button>
    </div>
  </nav>

  <el-drawer v-model="sheetOpen" direction="btt" size="auto" :with-header="false" class="pt-msheet">
    <div class="pt-msheet__body">
      <!--
        版本与更新（画板 34 个人卡右侧的「v0.47.2 已最新」）。桌面上它挂在底部状态条，而状态条在 ≤768
        整条隐藏 —— 之前手机上没有任何检查更新 / 自升级 / 忽略版本的入口，启动时弹出的「发现新版本」通知
        也无处可点。main 上它挂在移动端顶栏，这次重构把它弄丢了。
      -->
      <div class="pt-msheet__ver">
        <span class="pt-msheet__title">版本与更新</span>
        <VersionChecker />
      </div>

      <section v-for="group in sheetGroups" :key="group.title" class="pt-msheet__group">
        <h2 class="pt-msheet__title">{{ group.title }}</h2>
        <div class="pt-msheet__items">
          <button
            v-for="item in group.items"
            :key="item.path"
            type="button"
            class="pt-msheet__item"
            :class="{ 'is-active': activePath === item.path }"
            @click="goFromSheet(item.path)">
            <PtIcon :name="item.icon" :size="16" />
            <span>{{ item.label }}</span>
          </button>
        </div>
      </section>

      <ThemePrefs />

      <p class="pt-msheet__foot">
        最后同步 {{ runtimeStore.lastSyncText }} · ↓ {{ runtimeStore.downloadText }} · ↑
        {{ runtimeStore.uploadText }}
      </p>
    </div>
  </el-drawer>
</template>
