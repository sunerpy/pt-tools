<script setup lang="ts">
import type { InputInstance } from "element-plus";
import { computed, nextTick, ref, watch } from "vue";
import { useRouter } from "vue-router";
import { NAV_GROUPS, NAV_ITEMS, type NavItem } from "../../config/navigation";
import { useRuntimeStore } from "../../stores/runtime";
import { useThemeStore } from "../../stores/theme";
import { useVersionStore } from "../../stores/version";
import PtIcon from "../PtIcon";
import PtLogo from "../PtLogo";
import NavLink from "./NavLink.vue";
import ThemePrefs from "./ThemePrefs.vue";

/**
 * 导航列（设计稿 G 的 `D.nav`，宽 264，浅色）：品牌行 + 跳转框 + 6 组 19 项。
 * 计数徽标来自 runtimeStore；拿不到数或数为 0 就不画徽标，不用 0 占位。
 */
const props = defineProps<{
  activePath: string;
  /** 浮起态（抽屉形态）：多一个关闭按钮，钉在栅格里时不需要 */
  drawer?: boolean;
  /** 此刻是否真的看得见。收起后清空跳转框，下次打开是干净的 */
  visible?: boolean;
}>();
/** navigate = 点了某个链接（路由自己会动焦点）；close = 抽屉的关闭钮（焦点要交回开关） */
const emit = defineEmits<{ navigate: []; close: []; "toggle-nav": [] }>();

const router = useRouter();
const runtimeStore = useRuntimeStore();
const themeStore = useThemeStore();
const versionStore = useVersionStore();

/**
 * 画板 ver「v0.47.2 · 已是最新」：版本贴在品牌下面。
 * 版本号还没拿到时只显示前半段，不用占位符撑出一个假的「已是最新」。
 */
const versionLine = computed(() => {
  const v = versionStore.currentVersion;
  // store 在拿到数据前返回字面量 "unknown"，那不是版本号，别显示成「vunknown」
  if (!v || v === "unknown") return "";
  /*
   * 不能无条件加 "v"：`version.Version` 是 ldflags 从 `git describe` 灌进来的，
   * 而本仓库的 tag 本身就带 v（v0.47.2），加了就成了「vv0.47.2」——
   * 这一行在每个页面的导航列顶部，一直这么显示着。
   * 两种形状都接受：带 v 原样用，不带 v 才补。
   */
  const label = v.startsWith("v") ? v : `v${v}`;
  return versionStore.hasUpdate ? `${label} · 有新版本` : `${label} · 已是最新`;
});

function logout() {
  window.location.href = "/logout";
}

/*
 * 明暗开关。导航列钉住时 rail 整块淡出，rail 上那枚明暗钮就跟着不见了 ——
 * 而「主题入口必须一眼看得见」是用户定的硬约束，所以账号行里再放一枚，同一份 store。
 */
const themeIcon = computed(() => (themeStore.isDark ? "sun" : "moon"));
const themeLabel = computed(() => (themeStore.isDark ? "切换到明亮模式" : "切换到黑暗模式"));

const query = ref("");
const inputRef = ref<InputInstance>();
const toggleRef = ref<HTMLButtonElement>();

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
  /**
   * 把焦点交到头部那枚钮上（钉住时是「收起」，抽屉时是「关闭」—— 同一个 ref，v-if/v-else 只挂一个）。
   * 收起→钉住：开关自己长在会淡出的那一块里，不交接的话点完开关焦点就掉到 body。
   * 抽屉打开：打开它的开关（rail 上的）跟着 rail 一起淡出了，焦点同样会掉，所以进抽屉先落在关闭钮上。
   */
  focusToggle() {
    toggleRef.value?.focus();
  },
});

// 看不见了就清空查询，下次露出来是干净的
watch(
  () => props.visible,
  (v) => {
    if (!v) query.value = "";
  },
);
</script>

<template>
  <div class="pt-nav">
    <!--
      画板：品牌 + 版本行 + 跳转框是一个 98 高的整块，分隔线在它们之后。
      logo 放在和 rail 品牌**同一个** 32×32 盒子里（x=16, y=14）：侧栏收起/展开是同一块
      在两个宽度之间过渡，logo 钉在原位不动，只有右边的文字淡入淡出。
      导航列现在坐在 chrome（深色）上，所以和 rail 一样用单色版标志（brand.md：深色表面用 mono）。
    -->
    <div class="pt-nav__head">
      <div class="pt-nav__brand">
        <router-link to="/userinfo" class="pt-nav__logo" aria-label="pt-tools 首页">
          <PtLogo variant="mono" :size="28" />
        </router-link>
        <span class="pt-nav__brand-text">
          <span class="pt-nav__brand-name">pt-tools</span>
          <span class="pt-nav__brand-ver">{{ versionLine }}</span>
        </span>
        <!-- 抽屉里是「关闭」，钉住时是「收起」：同一个位置、一个按钮，语义随形态变 -->
        <button
          v-if="drawer"
          ref="toggleRef"
          type="button"
          class="pt-nav__close"
          aria-label="关闭导航"
          @click="emit('close')">
          <PtIcon name="x" :size="16" />
        </button>
        <button
          v-else
          ref="toggleRef"
          type="button"
          class="pt-nav__close"
          aria-label="收起导航"
          aria-controls="pt-nav-col"
          @click="emit('toggle-nav')">
          <PtIcon name="panel-left-close" :size="16" />
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

    <!-- 画板 nav 底部的账号页脚：头像 + 名称/角色 + 登出 -->
    <!--
      账号行。头像与 rail 底部的头像同一个位置（中心 x=32、离底 28），点开的也是同一份
      「偏好与账户」浮层（画板 41 的 user-pop）—— 钉住时 rail 不在，这里就是唯一入口。
      明暗钮同理（见 themeIcon 上面的注释）。
    -->
    <footer class="pt-nav__account">
      <el-popover
        placement="right-end"
        trigger="click"
        :width="280"
        popper-class="pt-prefs-popper"
        :offset="12">
        <template #reference>
          <!--
            可访问名就是看得见的那两行 + 一段只读给读屏的后缀，不用 aria-label 盖掉可见文字
            （WCAG 2.5.3 Label in Name：说「admin」的语音用户要能命中它）。
          -->
          <button type="button" class="pt-nav__account-who">
            <span class="pt-nav__account-av" aria-hidden="true">
              <PtIcon name="user" :size="16" />
            </span>
            <span class="pt-nav__account-text">
              <span class="pt-nav__account-name">admin</span>
              <span class="pt-nav__account-role">管理员 · 本地账号</span>
            </span>
            <span class="pt-sr-only">，偏好与账户</span>
          </button>
        </template>
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
      <button
        type="button"
        class="pt-nav__account-btn pt-nav__account-btn--theme"
        :aria-label="themeLabel"
        @click="themeStore.toggle">
        <PtIcon :name="themeIcon" :size="15" />
      </button>
      <button
        type="button"
        class="pt-nav__account-btn pt-nav__account-btn--out"
        aria-label="退出登录"
        @click="logout">
        <PtIcon name="log-out" :size="15" />
      </button>
    </footer>
  </div>
</template>
