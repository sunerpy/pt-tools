<script setup lang="ts">
import { computed, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { MOBILE_TABS, NAV_GROUPS } from "../../config/navigation";
import { useRuntimeStore } from "../../stores/runtime";
import { useThemeStore } from "../../stores/theme";
import PtIcon from "../PtIcon";
import PtLogo from "../PtLogo";
import ThemePrefs from "./ThemePrefs.vue";

/**
 * 移动端外壳（设计稿板 34 / §9）：顶栏 88 + 底部 5 个 tab 72。
 * 「我的」在设计稿上是一个设置聚合页，产品里没有这个路由，所以做成上拉抽屉，
 * 把「系统」「ChatOps」两组和偏好折进去 —— 行为等价且不新增路由。
 */
const props = defineProps<{ activePath: string; title: string }>();
const emit = defineEmits<{ "open-nav": [] }>();

const route = useRoute();
const router = useRouter();
const runtimeStore = useRuntimeStore();
const themeStore = useThemeStore();

/** 顶栏坐在 --pt-surface 上，变体按表面明暗选（brand.md「用哪个变体」） */
const logoVariant = computed(() => (themeStore.isDark ? "mono" : "plated"));

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
  if (route.path !== path) void router.push(path);
}

function goFromSheet(path: string) {
  sheetOpen.value = false;
  if (route.path !== path) void router.push(path);
}
</script>

<template>
  <header class="pt-mchrome">
    <div class="pt-mchrome__row">
      <button
        type="button"
        class="pt-mchrome__icon"
        aria-label="打开导航"
        @click="emit('open-nav')">
        <PtIcon name="menu" :size="20" />
      </button>
      <span class="pt-mchrome__brand">
        <PtLogo :variant="logoVariant" :size="20" />
        <span>pt-tools</span>
      </span>
      <button
        type="button"
        class="pt-mchrome__icon"
        aria-label="偏好与账户"
        @click="sheetOpen = true">
        <PtIcon name="user" :size="20" />
      </button>
    </div>
    <h1 class="pt-mchrome__title">{{ title }}</h1>
  </header>

  <nav class="pt-mnav" aria-label="底部导航">
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
  </nav>

  <el-drawer v-model="sheetOpen" direction="btt" size="auto" :with-header="false" class="pt-msheet">
    <div class="pt-msheet__body">
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
