<script setup lang="ts">
import type { InputInstance } from "element-plus";
import { computed, nextTick, ref, watch } from "vue";
import { useRouter } from "vue-router";
import { NAV_GROUPS, NAV_ITEMS, type NavItem } from "../../config/navigation";
import { useRuntimeStore } from "../../stores/runtime";
import { useThemeStore } from "../../stores/theme";
import PtIcon from "../PtIcon";
import PtLogo from "../PtLogo";
import NavLink from "./NavLink.vue";

/**
 * 导航列（设计稿 G 的 `D.nav`，宽 264，浅色）：品牌行 + 跳转框 + 6 组 19 项。
 * 计数徽标来自 runtimeStore；拿不到数或数为 0 就不画徽标，不用 0 占位。
 */
const props = defineProps<{ activePath: string; drawer?: boolean }>();
const emit = defineEmits<{ navigate: [] }>();

const router = useRouter();
const runtimeStore = useRuntimeStore();
const themeStore = useThemeStore();

/*
 * 导航列坐在 --pt-surface 上，明暗随模式变，所以标志变体也得跟着换表面：
 * brand.md 的规则是浅色表面用自带底板的彩色版、深色表面用单色版。
 * 这不是改色，改色是被禁的那条；这里换的是变体。
 */
const logoVariant = computed(() => (themeStore.isDark ? "mono" : "plated"));

const query = ref("");
const inputRef = ref<InputInstance>();

/** 跳转框（⌘K）：按中文文案或路径过滤，空查询时显示完整分组 */
const matches = computed<NavItem[]>(() => {
  const q = query.value.trim().toLowerCase();
  if (!q) return [];
  return NAV_ITEMS.filter(
    (i) => i.label.toLowerCase().includes(q) || i.path.toLowerCase().includes(q),
  );
});

const searching = computed(() => query.value.trim().length > 0);

function badgeOf(item: NavItem): number | null {
  if (!item.badge) return null;
  const n = runtimeStore.badges[item.badge];
  return typeof n === "number" && n > 0 ? n : null;
}

/** 回车跳到第一个命中项；外部项照样开新标签页 */
function jumpFirst() {
  const first = matches.value[0];
  if (!first) return;
  query.value = "";
  if (first.external) {
    window.open(router.resolve(first.path).href, "_blank");
    return;
  }
  void router.push(first.path);
  emit("navigate");
}

defineExpose({
  focusSearch: async () => {
    await nextTick();
    inputRef.value?.focus();
  },
});

// 抽屉关掉时清空查询，下次打开是干净的
watch(
  () => props.drawer,
  (open) => {
    if (!open) query.value = "";
  },
);
</script>

<template>
  <div class="pt-nav">
    <div class="pt-nav__brand">
      <PtLogo :variant="logoVariant" :size="22" />
      <span class="pt-nav__brand-name">pt-tools</span>
      <button
        v-if="drawer"
        type="button"
        class="pt-nav__close"
        aria-label="关闭导航"
        @click="emit('navigate')">
        <PtIcon name="x" :size="16" />
      </button>
    </div>

    <div class="pt-nav__jump">
      <el-input
        ref="inputRef"
        v-model="query"
        placeholder="跳转到页面…"
        size="small"
        clearable
        @keyup.enter="jumpFirst">
        <template #prefix>
          <PtIcon name="search" :size="14" />
        </template>
      </el-input>
    </div>

    <nav class="pt-nav__scroll" aria-label="主导航">
      <template v-if="searching">
        <p v-if="!matches.length" class="pt-nav__empty">没有匹配的页面</p>
        <NavLink
          v-for="item in matches"
          :key="item.path"
          :item="item"
          :active="activePath === item.path"
          :badge="badgeOf(item)"
          @navigate="emit('navigate')" />
      </template>

      <template v-else>
        <section v-for="group in NAV_GROUPS" :key="group.title" class="pt-nav__group">
          <h2 class="pt-nav__group-title">{{ group.title }}</h2>
          <NavLink
            v-for="item in group.items"
            :key="item.path"
            :item="item"
            :active="activePath === item.path"
            :badge="badgeOf(item)"
            @navigate="emit('navigate')" />
        </section>
      </template>
    </nav>
  </div>
</template>
