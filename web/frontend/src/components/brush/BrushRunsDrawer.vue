<script setup lang="ts">
/*
 * 刷流任务的运行记录（2026-10-27 用户：「需要有一个地方看每次任务的详细信息和日志，方便用户知晓和排查故障」）。
 * 每一轮一行：开始时间、定时还是手动、状态、用时、摘要与错误；点开看这一轮的步骤：读到的列表、不符合条件的种子与原因、
 * 加入与删除的种子、为什么停下。最新一条默认展开。记录由后端每轮写一条（scheduler/brush_runlog.go）。
 */
import { type BrushRun, type BrushTask, brushApi } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useIsMobile } from "@/composables/useIsMobile";
import { brushRunDuration, brushRunStatus, brushRunTrigger, brushStepKind } from "@/utils/brush";
import { formatShortDateTime } from "@/utils/format";
import { ref, watch } from "vue";

const props = defineProps<{ task: BrushTask | null }>();
const emit = defineEmits<{ (e: "close"): void }>();

const PAGE = 20;
const isMobile = useIsMobile();

const runs = ref<BrushRun[]>([]);
const total = ref(0);
const loading = ref(false);
const failed = ref("");
/** 展开的运行（可以同时展开几条对比） */
const expanded = ref(new Set<number>());

/** 请求的序号：只有最新一次能写列表、错误与 loading（换了任务时旧请求晚到不覆盖） */
let seq = 0;

async function load(more = false) {
  const task = props.task;
  if (!task) return;
  const token = ++seq;
  loading.value = true;
  try {
    const page = await brushApi.runs(task.id, {
      limit: PAGE,
      offset: more ? runs.value.length : 0,
    });
    if (token !== seq) return;
    const items = page?.items ?? [];
    if (more) {
      runs.value = [...runs.value, ...items];
    } else {
      // 刷新时保留还在的展开项；一条都没展开时展开最新一条
      const keep = [...expanded.value].filter((id) => items.some((r) => r.id === id));
      expanded.value = new Set(keep.length ? keep : items.slice(0, 1).map((r) => r.id));
      runs.value = items;
    }
    total.value = page?.total ?? 0;
    failed.value = "";
  } catch (e) {
    if (token !== seq) return;
    failed.value = (e as Error)?.message || "读取运行记录失败";
  } finally {
    if (token === seq) loading.value = false;
  }
}

function toggle(id: number) {
  const next = new Set(expanded.value);
  if (next.has(id)) next.delete(id);
  else next.add(id);
  expanded.value = next;
}

watch(
  () => props.task?.id,
  (id) => {
    seq++;
    runs.value = [];
    total.value = 0;
    failed.value = "";
    loading.value = false;
    expanded.value = new Set();
    if (id) void load();
  },
  { immediate: true },
);

defineExpose({ reload: () => load() });
</script>

<template>
  <el-drawer
    :model-value="task !== null"
    :size="isMobile ? '100%' : '760px'"
    :title="task ? `${task.name} 的运行记录` : ''"
    @update:model-value="(v: boolean) => !v && emit('close')">
    <div class="brs-head">
      <span class="brs-muted">每一轮一条，保留最近 500 条、14 天以内</span>
      <el-button size="small" :loading="loading" data-testid="brush-runs-refresh" @click="load()">
        <PtIcon v-if="!loading" name="refresh-cw" :size="14" /><span>刷新</span>
      </el-button>
    </div>
    <div v-loading="loading && !runs.length" class="brs">
      <div v-if="failed" class="pt-note pt-note--warn">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" /><span>{{ failed }}</span>
      </div>
      <PtDataState
        v-if="!runs.length && !loading && !failed"
        state="empty"
        dense
        sub="还没有运行记录：任务每运行一轮（定时或立即运行）记一条" />
      <ul v-if="runs.length" class="brs-list" data-testid="brush-runs">
        <li v-for="r in runs" :key="r.id" class="brs-run">
          <button
            type="button"
            class="brs-run__head"
            :aria-expanded="expanded.has(r.id)"
            :data-testid="`brush-run-${r.id}`"
            @click="toggle(r.id)">
            <PtIcon :name="expanded.has(r.id) ? 'chevron-down' : 'chevron-right'" :size="14" />
            <span class="brs-run__time">{{ formatShortDateTime(r.started_at) }}</span>
            <span class="brs-muted">{{ brushRunTrigger(r.trigger) }}</span>
            <PtStatusPill :tone="brushRunStatus(r.status).tone" size="sm">{{
              brushRunStatus(r.status).label
            }}</PtStatusPill>
            <span v-if="brushRunDuration(r)" class="brs-muted">用时 {{ brushRunDuration(r) }}</span>
            <span class="brs-run__summary">{{
              r.summary || (r.status === "running" ? "这一轮还在跑" : "")
            }}</span>
          </button>
          <p v-if="r.error" class="brs-run__error">{{ r.error }}</p>
          <ul v-if="expanded.has(r.id)" class="brs-steps" :data-testid="`brush-run-steps-${r.id}`">
            <li v-for="(st, i) in r.steps" :key="i" :class="['brs-step', `brs-step--${st.kind}`]">
              <span class="brs-step__kind">{{ brushStepKind(st.kind) }}</span>
              <span class="brs-step__text">{{ st.text }}</span>
            </li>
            <li v-if="!r.steps.length" class="brs-muted">
              {{ r.status === "running" ? "这一轮跑完以后列出每一步" : "没有记下步骤" }}
            </li>
            <li v-if="r.steps_truncated" class="brs-muted">步骤太多，后面的没有记</li>
          </ul>
        </li>
      </ul>
      <el-button
        v-if="runs.length && runs.length < total"
        link
        type="primary"
        :loading="loading"
        data-testid="brush-runs-more"
        @click="load(true)"
        >再看 {{ Math.min(PAGE, total - runs.length) }} 条</el-button
      >
    </div>
  </el-drawer>
</template>

<style scoped>
.brs-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--pt-space-2);
  margin-bottom: var(--pt-space-3);
}

.brs {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--pt-space-2);
  min-height: 120px;
}

.brs-muted {
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

.brs-list {
  width: 100%;
  margin: 0;
  padding: 0;
  list-style: none;
  border: 1px solid var(--pt-border);
  border-radius: 6px;
}

.brs-run + .brs-run {
  border-top: 1px solid var(--pt-border);
}

.brs-run__head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 10px;
  width: 100%;
  padding: 8px 10px;
  border: 0;
  background: transparent;
  font-size: 13px;
  color: var(--pt-t2);
  text-align: left;
  cursor: pointer;
}

.brs-run__head:hover {
  background: var(--pt-hover);
}

.brs-run__time {
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  color: var(--pt-t3);
}

.brs-run__summary {
  flex: 1 1 240px;
  min-width: 0;
  color: var(--pt-t1);
  word-break: break-all;
}

.brs-run__error {
  margin: 0;
  padding: 0 10px 8px 34px;
  font-size: 12px;
  color: var(--pt-dang);
  word-break: break-all;
}

.brs-steps {
  display: flex;
  flex-direction: column;
  gap: 3px;
  margin: 0;
  padding: 0 10px 10px 34px;
  list-style: none;
  font-size: 12px;
  color: var(--pt-t2);
  word-break: break-all;
}

.brs-step {
  display: flex;
  gap: 6px;
}

.brs-step__kind {
  flex: none;
  min-width: 44px;
  color: var(--pt-t3);
}

.brs-step__text {
  min-width: 0;
}

.brs-step--add .brs-step__kind {
  color: var(--pt-ok);
}

.brs-step--error .brs-step__kind,
.brs-step--error .brs-step__text {
  color: var(--pt-dang);
}

.brs-step--stop .brs-step__kind {
  color: var(--pt-warn);
}

.brs-step--reject .brs-step__text,
.brs-step--skip .brs-step__text {
  color: var(--pt-t3);
}
</style>
