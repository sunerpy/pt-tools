<script setup lang="ts">
/*
 * 辅种一轮的详情（2026-10-30 用户：「辅种每个轮次的任务也应支持暂停、停止和继续等」）。
 * 上面是这一轮的结果与计数，然后是步骤（读了哪些下载器、每一批查到什么、加入与失败、为什么停下），
 * 最后是这一轮的每一次尝试（只追加，重试记录不会改掉它）。样式沿用刷流的运行记录抽屉（BrushRunsDrawer）。
 */
import { type ReseedAttempt, type ReseedRunDetail, reseedApi } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useIsMobile } from "@/composables/useIsMobile";
import { formatShortDateTime } from "@/utils/format";
import {
  reseedAttemptResult,
  reseedRunDuration,
  reseedRunStatus,
  reseedRunTrigger,
  reseedStepKind,
} from "@/utils/reseed";
import { computed, ref, watch } from "vue";

const props = defineProps<{ runId: number | null }>();
const emit = defineEmits<{ (e: "close"): void; (e: "continue", id: number): void }>();

const PAGE = 50;
const isMobile = useIsMobile();

const run = ref<ReseedRunDetail | null>(null);
const attempts = ref<ReseedAttempt[]>([]);
const total = ref(0);
const page = ref(1);
const loading = ref(false);
const failed = ref("");

/** 请求的序号：换了一轮时旧请求晚到不覆盖 */
let seq = 0;

async function load(more = false) {
  const id = props.runId;
  if (!id) return;
  const token = ++seq;
  loading.value = true;
  try {
    const next = more ? page.value + 1 : 1;
    const [detail, list] = await Promise.all([
      more ? Promise.resolve(run.value) : reseedApi.runDetail(id),
      reseedApi.runAttempts(id, next, PAGE),
    ]);
    if (token !== seq) return;
    run.value = detail ?? null;
    const items = list?.items ?? [];
    attempts.value = more ? [...attempts.value, ...items] : items;
    total.value = list?.total ?? 0;
    page.value = next;
    failed.value = "";
  } catch (e) {
    if (token !== seq) return;
    failed.value = (e as Error)?.message || "读取这一轮失败";
  } finally {
    if (token === seq) loading.value = false;
  }
}

watch(
  () => props.runId,
  (id) => {
    seq++;
    run.value = null;
    attempts.value = [];
    total.value = 0;
    page.value = 1;
    failed.value = "";
    loading.value = false;
    if (id) void load();
  },
  { immediate: true },
);

const counts = computed(() => {
  const r = run.value;
  if (!r) return [];
  const out: { label: string; value: string }[] = [];
  if (r.trigger === "retry") out.push({ label: "重试的记录", value: `${r.hashes} 条` });
  else {
    out.push({ label: "查询的种子", value: `${r.hashes} 个` });
    out.push({ label: "IYUU 批数", value: `${r.batches_done}/${r.batches}` });
    out.push({ label: "可辅种", value: `${r.candidates} 个` });
  }
  out.push({ label: "加入", value: `${r.created} 个` });
  out.push({ label: "失败", value: `${r.failed} 个` });
  out.push({ label: "站点已删除", value: `${r.gone} 个` });
  out.push({ label: "跳过", value: `${r.skipped} 个` });
  if (r.interrupted) out.push({ label: "被打断", value: `${r.interrupted} 个` });
  return out;
});

const skippedDetail = computed(() =>
  Object.entries(run.value?.skipped_detail ?? {})
    .sort((a, b) => b[1] - a[1])
    .map(([why, n]) => `${why} ${n} 个`)
    .join("、"),
);

defineExpose({ reload: () => load() });
</script>

<template>
  <el-drawer
    :model-value="runId !== null"
    :size="isMobile ? '100%' : '760px'"
    :title="run ? `第 ${run.id} 轮 · ${reseedRunTrigger(run.trigger)}` : '辅种轮次'"
    @update:model-value="(v: boolean) => !v && emit('close')">
    <div class="rrd-head">
      <template v-if="run">
        <span class="rrd-muted">{{ formatShortDateTime(run.started_at) }}</span>
        <PtStatusPill :tone="reseedRunStatus(run.status).tone" size="sm">{{
          reseedRunStatus(run.status).label
        }}</PtStatusPill>
        <span v-if="reseedRunDuration(run)" class="rrd-muted"
          >用时 {{ reseedRunDuration(run) }}</span
        >
        <span v-if="run.continued_from" class="rrd-muted">接着第 {{ run.continued_from }} 轮</span>
      </template>
      <span class="rrd-spacer" />
      <el-button
        v-if="run?.continuable"
        size="small"
        type="primary"
        data-testid="rs-run-continue"
        @click="emit('continue', run.id)">
        <PtIcon name="play" :size="14" /><span>继续</span>
      </el-button>
      <el-button size="small" :loading="loading" data-testid="rs-run-refresh" @click="load()">
        <PtIcon v-if="!loading" name="refresh-cw" :size="14" /><span>刷新</span>
      </el-button>
    </div>
    <div v-loading="loading && !run" class="rrd">
      <div v-if="failed" class="pt-note pt-note--warn">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" /><span>{{ failed }}</span>
      </div>
      <template v-if="run">
        <p class="rrd-summary" data-testid="rs-run-summary">
          {{ run.summary || (run.status === "running" ? "这一轮还在跑" : "—") }}
        </p>
        <p v-if="run.error" class="rrd-error">{{ run.error }}</p>
        <dl class="rrd-counts" data-testid="rs-run-counts">
          <div v-for="c in counts" :key="c.label" class="rrd-count">
            <dt>{{ c.label }}</dt>
            <dd>{{ c.value }}</dd>
          </div>
        </dl>
        <p v-if="skippedDetail" class="rrd-muted">跳过：{{ skippedDetail }}</p>

        <h3 class="rrd-title">步骤</h3>
        <ul class="rrd-steps" data-testid="rs-run-steps">
          <li v-for="(st, i) in run.steps" :key="i" :class="['rrd-step', `rrd-step--${st.kind}`]">
            <span class="rrd-step__kind">{{ reseedStepKind(st.kind) }}</span>
            <span class="rrd-step__text">{{ st.text }}</span>
          </li>
          <li v-if="!run.steps.length" class="rrd-muted">
            {{
              run.status === "running" || run.status === "paused"
                ? "这一轮跑完以后列出每一步"
                : "没有记下步骤"
            }}
          </li>
          <li v-if="run.steps_truncated" class="rrd-muted">步骤太多，后面的没有记</li>
        </ul>

        <h3 class="rrd-title">尝试（{{ total }}）</h3>
        <PtDataState
          v-if="!attempts.length"
          state="empty"
          dense
          sub="这一轮没有下载过种子：候选都被跳过了，或者还没查到" />
        <ul v-else class="rrd-attempts" data-testid="rs-run-attempts">
          <li v-for="a in attempts" :key="a.id" class="rrd-attempt">
            <div class="rrd-attempt__main">
              <span class="rrd-attempt__name">{{ a.name || a.info_hash }}</span>
              <span class="rrd-muted">{{ a.site_name }} · {{ a.torrent_id }}</span>
            </div>
            <PtStatusPill :tone="reseedAttemptResult(a).tone" size="sm">{{
              reseedAttemptResult(a).label
            }}</PtStatusPill>
            <span v-if="a.message || a.job_message" class="rrd-attempt__msg">{{
              a.message || a.job_message
            }}</span>
          </li>
        </ul>
        <el-button
          v-if="attempts.length && attempts.length < total"
          link
          type="primary"
          :loading="loading"
          data-testid="rs-run-attempts-more"
          @click="load(true)"
          >再看 {{ Math.min(PAGE, total - attempts.length) }} 条</el-button
        >
      </template>
    </div>
  </el-drawer>
</template>

<style scoped>
.rrd-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--pt-space-2);
  margin-bottom: var(--pt-space-3);
}

.rrd-spacer {
  flex: 1;
}

.rrd {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: var(--pt-space-2);
  min-height: 120px;
}

.rrd-muted {
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

.rrd-summary {
  margin: 0;
  font-size: 13px;
  color: var(--pt-t1);
  word-break: break-all;
}

.rrd-error {
  margin: 0;
  font-size: 12px;
  color: var(--pt-dang);
  word-break: break-all;
}

.rrd-counts {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(120px, 1fr));
  gap: 8px;
  margin: 0;
}

.rrd-count {
  padding: 8px 10px;
  border: 1px solid var(--pt-border);
  border-radius: 6px;
}

.rrd-count dt {
  font-size: 12px;
  color: var(--pt-t3);
}

.rrd-count dd {
  margin: 2px 0 0;
  font-size: 14px;
  font-variant-numeric: tabular-nums;
  color: var(--pt-t1);
}

.rrd-title {
  margin: var(--pt-space-3) 0 0;
  font-size: 13px;
  font-weight: 600;
  color: var(--pt-t1);
}

.rrd-steps {
  display: flex;
  flex-direction: column;
  gap: 3px;
  margin: 0;
  padding: 0;
  list-style: none;
  font-size: 12px;
  color: var(--pt-t2);
  word-break: break-all;
}

.rrd-step {
  display: flex;
  gap: 6px;
}

.rrd-step__kind {
  flex: none;
  min-width: 44px;
  color: var(--pt-t3);
}

.rrd-step__text {
  min-width: 0;
}

.rrd-step--add .rrd-step__kind {
  color: var(--pt-ok);
}

.rrd-step--error .rrd-step__kind,
.rrd-step--error .rrd-step__text,
.rrd-step--fail .rrd-step__kind {
  color: var(--pt-dang);
}

.rrd-step--stop .rrd-step__kind {
  color: var(--pt-warn);
}

.rrd-attempts {
  margin: 0;
  padding: 0;
  list-style: none;
  border: 1px solid var(--pt-border);
  border-radius: 6px;
}

.rrd-attempt {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 10px;
  padding: 8px 10px;
  font-size: 13px;
}

.rrd-attempt + .rrd-attempt {
  border-top: 1px solid var(--pt-border);
}

.rrd-attempt__main {
  display: flex;
  flex: 1 1 260px;
  flex-direction: column;
  min-width: 0;
}

.rrd-attempt__name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--pt-t1);
}

.rrd-attempt__msg {
  flex-basis: 100%;
  font-size: 12px;
  color: var(--pt-t3);
  word-break: break-all;
}
</style>
