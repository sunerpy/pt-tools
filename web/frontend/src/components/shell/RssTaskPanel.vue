<script setup lang="ts">
import { computed } from "vue";
import { useRuntimeStore } from "../../stores/runtime";
import { canStartRss, canStopRss, RSS_TASK_SCOPE } from "../../utils/rssTask";
import PtStatusPill from "../ui/PtStatusPill.vue";

/**
 * RSS 任务（原「调度器」）的控制面板：真实状态、作用范围、自动启动开关、启停。
 *
 * 桌面挂在状态栏的浮层里（画板 41 的 sched-pop，300 宽），手机挂在「我的」抽屉里 ——
 * 手机上原来没有任何启停入口。画板那行「最后同步 … · 间隔 N 分钟」说的是状态栏自己的刷新，
 * 跟 RSS 任务无关，换成了状态说明：在跑几个订阅，或者为什么没在跑。
 */
const runtimeStore = useRuntimeStore();
const view = computed(() => runtimeStore.rssView);
const canStop = computed(() => canStopRss(runtimeStore.rss));
const canStart = computed(() => canStartRss(runtimeStore.rss));

function onAutoStart(v: string | number | boolean) {
  void runtimeStore.setAutoStart(v === true);
}
</script>

<template>
  <div class="pt-status__menu" data-testid="rss-task-panel">
    <div class="pt-status__menu-head">
      <span class="pt-status__menu-t">RSS 任务</span>
      <PtStatusPill :tone="view.tone" size="sm">{{ view.pill }}</PtStatusPill>
    </div>
    <p class="pt-status__menu-detail" data-testid="rss-task-detail">{{ view.detail }}</p>
    <p class="pt-status__menu-scope">{{ RSS_TASK_SCOPE }}</p>
    <div class="pt-status__menu-auto">
      <span aria-hidden="true">程序启动时自动运行</span>
      <el-switch
        :model-value="runtimeStore.rss?.auto_start ?? false"
        :loading="runtimeStore.autoStartLoading"
        :disabled="!runtimeStore.rss"
        size="small"
        aria-label="程序启动时自动运行 RSS 任务"
        data-testid="rss-task-autostart"
        @change="onAutoStart" />
    </div>
    <div class="pt-status__menu-acts">
      <el-button
        size="small"
        type="danger"
        plain
        :disabled="!canStop"
        :loading="runtimeStore.stopLoading"
        @click="runtimeStore.stopAll()">
        停止
      </el-button>
      <el-button
        size="small"
        type="primary"
        plain
        :disabled="!canStart"
        :loading="runtimeStore.startLoading"
        @click="runtimeStore.startAll()">
        启动
      </el-button>
    </div>
  </div>
</template>
