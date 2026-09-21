<script setup lang="ts">
import { computed, ref } from "vue";
import { RAIL_FOOT_ITEM, RAIL_ITEMS } from "../../config/navigation";
import { useThemeStore } from "../../stores/theme";
import PtIcon from "../PtIcon";
import PtLogo from "../PtLogo";
import ThemePrefs from "./ThemePrefs.vue";

/**
 * 深色图标 rail（设计稿 G 的 `D.rail`，宽 64）。
 *
 * 结构是「品牌 / 快捷入口 / 运行日志 / 明暗开关 / 头像」。两点与板 41 不同，都是验收反馈：
 *   · 导航入口只在导航列收起时出现（CSS 在 shell.css 管），否则与导航列逐条重复。
 *     这条同样管住底部的运行日志 —— 它在导航列里叫「系统 › 运行日志」；
 *   · 明暗切换单独立一个按钮。板 41 把它塞进头像 popover 里，实测「看不到」——
 *     头像是个无字的人形图标，没人会为了换主题去点它。这里一键直切，
 *     popover 里的三段选择器（含跟随系统）保留，是它的超集。
 */
defineProps<{
  activePath: string;
  /** 导航收起开关的图标与说明由外壳算（它知道当前是钉住还是浮起） */
  navToggleIcon: string;
  navToggleLabel: string;
}>();

const emit = defineEmits<{ "toggle-nav": [] }>();

const toggleRef = ref<HTMLButtonElement>();
/** 钉住→收起之后把焦点交到这枚开关上；原因见 AppNav.focusToggle */
defineExpose({
  focusToggle() {
    toggleRef.value?.focus();
  },
});

const themeStore = useThemeStore();

/** 图标表示「点了会变成什么」：当前是深色就给太阳 */
const themeIcon = computed(() => (themeStore.isDark ? "sun" : "moon"));
const themeLabel = computed(() => (themeStore.isDark ? "切换到明亮模式" : "切换到黑暗模式"));
</script>

<template>
  <aside class="pt-rail" aria-label="快捷导航">
    <router-link to="/userinfo" class="pt-rail__brand" aria-label="pt-tools 首页">
      <!-- rail 在 8 套配色里都是深色 chrome，按 brand.md 取该表面主文字色的单色版 -->
      <PtLogo variant="mono" :size="28" />
    </router-link>

    <!--
      导航收起开关放在 rail 而不是页头：它控制的是导航列，属于外壳；
      而画板里总览页没有页头（KPI 带直接贴 y=0），开关留在页头就会随页头一起消失。
    -->
    <el-tooltip :content="navToggleLabel" placement="right" :offset="10" :show-after="240">
      <button
        ref="toggleRef"
        type="button"
        class="pt-rail__tool pt-rail__tool--nav"
        :aria-label="navToggleLabel"
        aria-controls="pt-nav-col"
        @click="emit('toggle-nav')">
        <PtIcon :name="navToggleIcon" :size="16" />
      </button>
    </el-tooltip>

    <nav class="pt-rail__items">
      <el-tooltip
        v-for="item in RAIL_ITEMS"
        :key="item.path"
        :content="item.label"
        placement="right"
        :offset="10"
        :show-after="240">
        <router-link
          :to="item.path"
          class="pt-rail__item"
          :class="{ 'is-active': activePath === item.path }"
          :aria-label="item.label"
          :aria-current="activePath === item.path ? 'page' : undefined">
          <PtIcon :name="item.icon" :size="20" />
        </router-link>
      </el-tooltip>
    </nav>

    <!--
      画板底部一组（foot-rule 之下）：运行日志、明暗开关、头像。
      画板只画了日志与头像，明暗开关是验收补的，排在日志之后 ——
      这样日志入口随导航列开合出现或消失时，明暗按钮到底边的距离都不变，
      不会换位置（它必须一眼能找到）。
    -->
    <div class="pt-rail__foot">
      <!--
        运行日志。它在导航列里就是「系统 › 运行日志」，所以必须和主入口区那 8 项同一档：
        带上 .pt-rail__item，验收脚本按这个类数「导航列钉住时 rail 上还剩几个入口」。
        （钉住时整条 rail 淡出，见 shell.css 网格骨架那段；这里不再有单独的 display:none。）
        .pt-rail__tool 给的是画板 rail-log 的 32×32。

        不加 is-active：那条选中条（3×40，left:-6px）的位置是按主入口区 52×40 的项盒
        算出来的，那里项盒起于 x=6，条正好贴着 rail 左沿。底部这个 32×32 的盒子起于
        x=16，同一条会落在 x=10，而且 40 高压在 32 高的盒子上还要下探 8px 到明暗按钮。
        当前页由 aria-current 表达。
      -->
      <el-tooltip
        v-if="RAIL_FOOT_ITEM"
        :content="RAIL_FOOT_ITEM.label"
        placement="right"
        :offset="10"
        :show-after="240">
        <router-link
          :to="RAIL_FOOT_ITEM.path"
          class="pt-rail__tool pt-rail__item"
          :aria-label="RAIL_FOOT_ITEM.label"
          :aria-current="activePath === RAIL_FOOT_ITEM.path ? 'page' : undefined">
          <PtIcon :name="RAIL_FOOT_ITEM.icon" :size="16" />
        </router-link>
      </el-tooltip>

      <el-tooltip :content="themeLabel" placement="right" :offset="10" :show-after="240">
        <button
          type="button"
          class="pt-rail__tool pt-rail__tool--theme"
          :aria-label="themeLabel"
          @click="themeStore.toggle">
          <PtIcon :name="themeIcon" :size="16" />
        </button>
      </el-tooltip>

      <!--
        头像不套 el-tooltip：tooltip 和 popover 都通过 ElOnlyChild 绑在同一个 DOM 节点上，
        会互相抢 click/hover/focus 与 aria 属性。这里只留 aria-label。
      -->
      <el-popover
        placement="right-end"
        trigger="click"
        :width="280"
        popper-class="pt-prefs-popper"
        :offset="12">
        <template #reference>
          <button type="button" class="pt-rail__avatar" aria-label="偏好与账户">
            <PtIcon name="user" :size="16" />
          </button>
        </template>
        <!--
          画板 41 的 user-pop（280 宽）顶部是身份：头像 + 用户名 + 「管理员 · 单用户模式」，
          下面才是偏好。导航列收起时这是唯一能看到「我是谁」的地方 ——
          账号页脚长在导航列里，收起来就一起不见了。
        -->
        <div class="pt-prefs__who">
          <span class="pt-prefs__who-av" aria-hidden="true">
            <PtIcon name="user" :size="16" />
          </span>
          <span class="pt-prefs__who-txt">
            <strong>admin</strong>
            <small>管理员 · 单用户模式</small>
          </span>
        </div>
        <ThemePrefs />
      </el-popover>
    </div>
  </aside>
</template>
