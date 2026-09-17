<script setup lang="ts">
import { RAIL_ITEMS } from "../../config/navigation";
import PtIcon from "../PtIcon";
import PtLogo from "../PtLogo";
import ThemePrefs from "./ThemePrefs.vue";

/**
 * 深色图标 rail（设计稿 G 的 `D.rail`，宽 64）。
 * 8 个高频入口 + 底部头像。头像 popover 装偏好与退出登录（板 41 的落位表）。
 */
defineProps<{ activePath: string }>();
</script>

<template>
  <aside class="pt-rail" aria-label="快捷导航">
    <router-link to="/userinfo" class="pt-rail__brand" aria-label="pt-tools 首页">
      <!-- rail 在 8 套配色里都是深色 chrome，按 brand.md 取该表面主文字色的单色版 -->
      <PtLogo variant="mono" :size="26" />
    </router-link>

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

    <el-popover
      placement="right-end"
      trigger="click"
      :width="268"
      popper-class="pt-prefs-popper"
      :offset="12">
      <template #reference>
        <button type="button" class="pt-rail__avatar" aria-label="偏好与账户">
          <PtIcon name="user" :size="18" />
        </button>
      </template>
      <ThemePrefs />
    </el-popover>
  </aside>
</template>
