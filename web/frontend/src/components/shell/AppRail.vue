<script setup lang="ts">
import { computed } from "vue";
import { RAIL_ITEMS } from "../../config/navigation";
import { useThemeStore } from "../../stores/theme";
import PtIcon from "../PtIcon";
import PtLogo from "../PtLogo";
import ThemePrefs from "./ThemePrefs.vue";

/**
 * 深色图标 rail（设计稿 G 的 `D.rail`，宽 64）。
 *
 * 结构是「品牌 / 快捷入口 / 明暗开关 / 头像」。两点与板 41 不同，都是验收反馈：
 *   · 快捷入口只在导航列收起时出现（CSS 在 shell.css 管），否则与导航列逐条重复；
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

    <!-- 画板底部一组：分隔线之下是日志入口与头像，和上面的导航入口分开 -->
    <div class="pt-rail__foot">
      <el-tooltip :content="themeLabel" placement="right" :offset="10" :show-after="240">
        <button
          type="button"
          class="pt-rail__tool"
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
        :width="268"
        popper-class="pt-prefs-popper"
        :offset="12">
        <template #reference>
          <button type="button" class="pt-rail__avatar" aria-label="偏好与账户">
            <PtIcon name="user" :size="16" />
          </button>
        </template>
        <ThemePrefs />
      </el-popover>
    </div>
  </aside>
</template>
