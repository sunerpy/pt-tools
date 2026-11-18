<script setup lang="ts">
import { computed, ref } from "vue";
import { useRouter } from "vue-router";
import { useRuntimeStore } from "../../stores/runtime";
import { rssBanner } from "../../utils/rssTask";
import PtIcon from "../PtIcon";

/**
 * 「RSS 任务没有运行」提示条：没开自动启动、有 RSS 订阅、RSS 任务又没在跑时出现在每页顶上。
 *
 * 用户反馈：重启容器后 RSS 任务不会自己运行，入口只是左下角状态栏里的一行小字，
 * 刚升级完找了半天。何时出现见 utils/rssTask.ts 的 rssBanner（手动停止的不提示）。
 *
 * 关掉只对这个浏览器会话里的同一种状态有效：状态变了（例如下载器又连不上）会再出现。
 */
const DISMISS_KEY = "pt_tools_rss_banner_dismissed";

const runtimeStore = useRuntimeStore();
const router = useRouter();

function readDismissed(): string {
  try {
    return window.sessionStorage.getItem(DISMISS_KEY) ?? "";
  } catch {
    return "";
  }
}

const dismissed = ref(readDismissed());
const banner = computed(() => rssBanner(runtimeStore.rss));
const visible = computed(
  () => banner.value !== null && dismissed.value !== (runtimeStore.rss?.state ?? ""),
);

function dismiss() {
  const state = runtimeStore.rss?.state ?? "";
  dismissed.value = state;
  try {
    window.sessionStorage.setItem(DISMISS_KEY, state);
  } catch {
    // 无痕模式拿不到 sessionStorage：这次关掉，刷新后会再出现
  }
}
</script>

<template>
  <div
    v-if="visible && banner"
    class="pt-note pt-note--warn rss-banner"
    role="status"
    aria-live="polite"
    data-testid="rss-task-banner">
    <PtIcon name="rss" :size="14" class="pt-note__icon" />
    <div class="rss-banner__body">
      <p class="rss-banner__text">
        <strong class="pt-note__title">RSS 任务没有运行</strong>
        <span class="rss-banner__sep">·</span>
        {{ banner.text }}
      </p>
      <div class="rss-banner__acts">
        <el-button
          v-if="banner.action === 'start'"
          size="small"
          type="primary"
          :loading="runtimeStore.startLoading"
          @click="runtimeStore.startAll({ confirm: false })">
          启动 RSS 任务
        </el-button>
        <el-button v-else size="small" type="primary" @click="router.push('/downloaders')">
          设置下载器
        </el-button>
        <el-button
          size="small"
          :loading="runtimeStore.autoStartLoading"
          @click="runtimeStore.setAutoStart(true)">
          打开自动启动
        </el-button>
      </div>
    </div>
    <button
      type="button"
      class="rss-banner__x"
      aria-label="暂时不提示"
      data-testid="rss-task-banner-dismiss"
      @click="dismiss">
      <PtIcon name="x" :size="14" />
    </button>
  </div>
</template>

<style scoped>
.rss-banner {
  align-items: flex-start;
  margin-bottom: var(--pt-space-4);
}

.rss-banner__body {
  display: flex;
  flex: 1 1 auto;
  flex-wrap: wrap;
  gap: var(--pt-space-2) var(--pt-space-3);
  align-items: center;
  min-width: 0;
}

.rss-banner__text {
  flex: 1 1 320px;
  min-width: 0;
  margin: 0;
}

.rss-banner__sep {
  margin: 0 4px;
  color: var(--pt-t4);
}

.rss-banner__acts {
  display: flex;
  flex: 0 0 auto;
  flex-wrap: wrap;
  gap: var(--pt-space-2);
}

.rss-banner__acts .el-button {
  margin: 0;
}

/* 关闭钮与其他通告条上的 × 同一套：24×24、hover 才显底 */
.rss-banner__x {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  padding: 0;
  color: var(--pt-t3);
  cursor: pointer;
  background: none;
  border: 0;
  border-radius: var(--pt-r-sm);
}

.rss-banner__x:hover {
  color: var(--pt-t1);
  background: var(--pt-hover);
}
</style>
