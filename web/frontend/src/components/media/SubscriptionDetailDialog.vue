<script setup lang="ts">
/*
 * 订阅详情。剧集（追剧）：每一集的格子（已入库、下载中、缺、等版本、还没播、不追），点一集可以跳过或让它换版本；
 * 集号规则（标题里的集号对不上 TMDB 时）；运行记录（每次搜索找到了什么、为什么没要、推送了哪个）；
 * 以及订阅下载过的种子（站点、集、分数、状态）。
 */
import {
  type EpisodeRule,
  type EpisodeRuleInput,
  type SubscriptionDetail,
  type SubscriptionEpisode,
  type SubscriptionRun,
  subscribeApi,
} from "@/api";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { formatShortDateTime } from "@/utils/format";
import {
  EPISODE_STATES,
  RULE_MODES,
  episodeState,
  isFollow,
  lockText,
  progressText,
  ruleSource,
  ruleText,
  runStatus,
  runTrigger,
  scheduleText,
  sourceLabel,
  stepKind,
  subName,
  subStatus,
  torrentSpan,
  torrentStatus,
} from "@/utils/subscribe";
import { ElMessage } from "element-plus";
import { computed, ref, watch } from "vue";

const props = defineProps<{ modelValue: boolean; id: number | null }>();
const emit = defineEmits<{
  (e: "update:modelValue", v: boolean): void;
  /** 跳过、换版本、改集号规则以后：列表要重新读进度 */
  (e: "changed"): void;
}>();

const RUN_PAGE = 20;

const isMobile = useIsMobile();
const visible = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emit("update:modelValue", v),
});
const detail = ref<SubscriptionDetail | null>(null);
const ds = useDataState();
const isTV = computed(() => detail.value?.media_type === "tv");
const episodes = computed<SubscriptionEpisode[]>(() => detail.value?.progress?.episodes ?? []);
const legend = computed(() =>
  EPISODE_STATES.map((st) => ({
    state: st,
    label: episodeState(st).label,
    n: episodes.value.filter((e) => e.state === st).length,
  })).filter((l) => l.n > 0),
);
const followLine = computed(() => {
  const d = detail.value;
  if (!d || d.media_type !== "tv") return [];
  const out: string[] = [];
  if (isFollow(d)) out.push(scheduleText(d.schedule));
  const lock = lockText(d);
  if (lock) out.push(lock);
  if (d.start_episode && d.start_episode > 1) out.push(`从第 ${d.start_episode} 集开始`);
  if (d.follow_next_season) out.push("追完以后追下一季");
  return out;
});

async function load() {
  if (!props.id) return;
  const pending = ds.run(() => subscribeApi.get(props.id as number));
  const got = await pending;
  if (ds.isStale(pending)) return;
  detail.value = got;
}

// ---- 一集 ----
const selected = ref<number | null>(null);
const current = computed(() => episodes.value.find((e) => e.number === selected.value) ?? null);
const acting = ref(false);
const canUnlock = computed(() => {
  const e = current.value;
  return Boolean(
    e &&
    detail.value?.lock_version &&
    !e.manual_unlock &&
    (e.state === "missing" || e.state === "waiting_version"),
  );
});

function epNo(n: number) {
  return `E${String(n).padStart(2, "0")}`;
}

function epTitle(e: SubscriptionEpisode) {
  return [epNo(e.number), e.name, e.air_date, episodeState(e.state).label]
    .filter(Boolean)
    .join(" · ");
}

async function act(run: () => Promise<unknown>, ok: string) {
  acting.value = true;
  try {
    await run();
    ElMessage.success(ok);
    await load();
    emit("changed");
  } catch (e) {
    ElMessage.error((e as Error)?.message || "操作失败");
  } finally {
    acting.value = false;
  }
}

function skip(e: SubscriptionEpisode, on: boolean) {
  if (!props.id) return;
  const id = props.id;
  void act(
    () => subscribeApi.skipEpisode(id, e.number, on),
    on ? `已跳过第 ${e.number} 集` : `会接着找第 ${e.number} 集`,
  );
}

function unlock(e: SubscriptionEpisode, on: boolean) {
  if (!props.id) return;
  const id = props.id;
  void act(
    () => subscribeApi.unlockEpisode(id, e.number, on),
    on ? `第 ${e.number} 集不再受版本锁限制，会尽快搜一次` : `第 ${e.number} 集恢复版本锁`,
  );
}

// ---- 集号规则 ----
const rules = ref<EpisodeRule[]>([]);
const ruleForm = ref<EpisodeRuleInput>({ group: "", mode: "period_part", offset: 1, note: "" });
const savingRule = ref(false);
const ruleTip = computed(() => RULE_MODES.find((m) => m.value === ruleForm.value.mode)?.tip ?? "");

async function loadRules() {
  if (!props.id) return;
  try {
    rules.value = (await subscribeApi.episodeRules(props.id)) ?? [];
  } catch {
    rules.value = [];
  }
}

async function saveRule() {
  if (!props.id) return;
  savingRule.value = true;
  try {
    const f = ruleForm.value;
    rules.value =
      (await subscribeApi.saveEpisodeRule(props.id, {
        group: f.group.trim(),
        mode: f.mode,
        offset: f.mode === "offset" ? f.offset : 0,
        note: f.note.trim(),
      })) ?? [];
    ElMessage.success("已保存集号规则，会尽快按它搜一次");
    ruleForm.value = { group: "", mode: "period_part", offset: 1, note: "" };
    await load();
    emit("changed");
  } catch (e) {
    ElMessage.error((e as Error)?.message || "保存失败");
  } finally {
    savingRule.value = false;
  }
}

async function removeRule(r: EpisodeRule) {
  if (!props.id) return;
  try {
    rules.value = (await subscribeApi.deleteEpisodeRule(props.id, r.group)) ?? [];
    ElMessage.success("已删除集号规则");
    await load();
    emit("changed");
  } catch (e) {
    ElMessage.error((e as Error)?.message || "删除失败");
  }
}

// ---- 运行记录 ----
const runs = ref<SubscriptionRun[]>([]);
const runTotal = ref(0);
const runsFailed = ref("");
const loadingRuns = ref(false);
const openRun = ref<number | null>(null);

async function loadRuns(more = false) {
  if (!props.id) return;
  loadingRuns.value = true;
  try {
    const page = await subscribeApi.runs(props.id, {
      limit: RUN_PAGE,
      offset: more ? runs.value.length : 0,
    });
    runs.value = more ? [...runs.value, ...(page?.items ?? [])] : (page?.items ?? []);
    runTotal.value = page?.total ?? 0;
    runsFailed.value = "";
  } catch (e) {
    runsFailed.value = (e as Error)?.message || "读取运行记录失败";
  } finally {
    loadingRuns.value = false;
  }
}

watch(
  () => [props.modelValue, props.id] as const,
  ([open]) => {
    if (open) {
      detail.value = null;
      selected.value = null;
      rules.value = [];
      runs.value = [];
      runTotal.value = 0;
      openRun.value = null;
      void load().then(() => {
        if (isTV.value) void loadRules();
      });
      void loadRuns();
    }
  },
  { immediate: true },
);
</script>

<template>
  <el-dialog
    v-model="visible"
    class="pt-dialog"
    :title="isTV ? '追剧详情' : '订阅详情'"
    :width="isMobile ? '94%' : '760px'"
    append-to-body
    align-center
    data-testid="sub-detail">
    <PtDataState
      v-if="!detail"
      :state="ds.state.value"
      :sub="ds.errorText.value"
      data-testid="sub-detail-state" />
    <div v-else class="sdd">
      <div class="sdd__head">
        <div class="sdd__name" data-testid="sub-detail-name">{{ subName(detail) }}</div>
        <div class="sdd__sub">
          <PtStatusPill :tone="subStatus(detail.status).tone" size="sm">{{
            subStatus(detail.status).label
          }}</PtStatusPill>
          <span>{{ sourceLabel(detail.source) }}</span>
          <span v-if="detail.upgrade">洗版</span>
          <span>{{ progressText(detail.media_type, detail.progress) }}</span>
        </div>
        <div v-if="followLine.length" class="sdd__sub" data-testid="sub-detail-follow">
          <span v-for="t in followLine" :key="t">{{ t }}</span>
        </div>
        <div v-if="detail.message" class="sdd__sub" data-testid="sub-detail-message">
          {{ detail.message }}
        </div>
        <div class="sdd__sub">
          <template v-if="detail.last_search_at"
            >上次搜索 {{ formatShortDateTime(detail.last_search_at) }}</template
          >
          <template v-if="detail.next_search_at && detail.status === 'active'">
            · 下次 {{ formatShortDateTime(detail.next_search_at) }}</template
          >
        </div>
      </div>

      <div v-if="episodes.length" class="sdd__section">
        <div class="sdd__label">第 {{ detail.season }} 季的分集（点一集看详情）</div>
        <div class="sdd__grid" data-testid="sub-detail-episodes">
          <div class="sdd__cells">
            <button
              v-for="e in episodes"
              :key="e.number"
              type="button"
              class="sdd__cell"
              :class="[`sdd__cell--${e.state}`, { 'is-selected': selected === e.number }]"
              :title="epTitle(e)"
              :aria-label="epTitle(e)"
              :aria-pressed="selected === e.number"
              :data-testid="`sub-ep-${e.number}`"
              @click="selected = selected === e.number ? null : e.number">
              {{ e.number }}
            </button>
          </div>
          <div class="sdd__legend">
            <span v-for="l in legend" :key="l.state" class="sdd__legend-item">
              <i class="sdd__dot" :class="`sdd__cell--${l.state}`" />{{ l.label }} {{ l.n }}
            </span>
          </div>
        </div>
        <div v-if="current" class="sdd__ep" data-testid="sub-ep-detail">
          <div class="sdd__ep-title">
            <span class="sdd__epno">{{ epNo(current.number) }}</span>
            <span v-if="current.name">{{ current.name }}</span>
            <PtStatusPill :tone="episodeState(current.state).tone" size="sm">{{
              episodeState(current.state).label
            }}</PtStatusPill>
          </div>
          <div class="sdd__sub">
            <span v-if="current.air_date">{{ current.air_date }} 播出</span>
            <span v-if="current.version">版本 {{ current.version }}</span>
            <span v-if="current.source === 'downloader'">下载器里已有的种子</span>
            <span v-if="current.manual_unlock">这一集不受版本锁限制</span>
          </div>
          <div class="sdd__ep-acts">
            <el-button
              v-if="current.manual_skip"
              size="small"
              :loading="acting"
              data-testid="sub-ep-unskip"
              @click="skip(current, false)"
              >重新要这一集</el-button
            >
            <el-button
              v-else-if="current.state !== 'library'"
              size="small"
              :loading="acting"
              data-testid="sub-ep-skip"
              @click="skip(current, true)"
              >跳过这一集</el-button
            >
            <el-button
              v-if="canUnlock"
              size="small"
              type="primary"
              plain
              :loading="acting"
              data-testid="sub-ep-unlock"
              @click="unlock(current, true)"
              >这一集换版本</el-button
            >
            <el-button
              v-if="current.manual_unlock"
              size="small"
              :loading="acting"
              data-testid="sub-ep-relock"
              @click="unlock(current, false)"
              >恢复版本锁</el-button
            >
          </div>
        </div>
      </div>

      <div v-if="isTV" class="sdd__section" data-testid="sub-detail-rules">
        <div class="sdd__label">集号规则</div>
        <p class="sdd__sub">
          种子标题里的集号对不上 TMDB
          时用（比如综艺按期号分上下）：每个制作组一条，组名不填的对所有组
        </p>
        <div v-for="r in rules" :key="r.id" class="sdd__rule">
          <span class="sdd__rule-group">{{ r.group || "所有制作组" }}</span>
          <span>{{ ruleText(r) }}</span>
          <span class="sdd__muted">{{ ruleSource(r.source) }}</span>
          <span v-if="r.note" class="sdd__muted">{{ r.note }}</span>
          <el-button
            link
            type="danger"
            size="small"
            :data-testid="`sub-rule-del-${r.id}`"
            @click="removeRule(r)"
            >删除</el-button
          >
        </div>
        <div class="sdd__rule-form">
          <el-input
            v-model="ruleForm.group"
            size="small"
            placeholder="制作组（不填是所有组）"
            data-testid="sub-rule-group" />
          <el-select v-model="ruleForm.mode" size="small" data-testid="sub-rule-mode">
            <el-option v-for="m in RULE_MODES" :key="m.value" :value="m.value" :label="m.label" />
          </el-select>
          <el-input-number
            v-if="ruleForm.mode === 'offset'"
            v-model="ruleForm.offset"
            size="small"
            :min="-2000"
            :max="2000"
            controls-position="right"
            data-testid="sub-rule-offset" />
          <el-input
            v-model="ruleForm.note"
            size="small"
            placeholder="说明（可以不填）"
            data-testid="sub-rule-note" />
          <el-button
            size="small"
            type="primary"
            :loading="savingRule"
            data-testid="sub-rule-save"
            @click="saveRule"
            >保存规则</el-button
          >
        </div>
        <div class="field-tip">{{ ruleTip }}</div>
      </div>

      <div class="sdd__section">
        <div class="sdd__label">运行记录</div>
        <p v-if="runsFailed" class="sdd__sub">{{ runsFailed }}</p>
        <p v-else-if="!runs.length && !loadingRuns" class="sdd__sub">还没有运行记录</p>
        <div v-else class="sdd__runs" data-testid="sub-detail-runs">
          <div v-for="r in runs" :key="r.id" class="sdd__run">
            <button
              type="button"
              class="sdd__run-head"
              :aria-expanded="openRun === r.id"
              :data-testid="`sub-run-${r.id}`"
              @click="openRun = openRun === r.id ? null : r.id">
              <span class="sdd__run-time">{{ formatShortDateTime(r.started_at) }}</span>
              <span class="sdd__muted">{{ runTrigger(r.trigger) }}</span>
              <PtStatusPill :tone="runStatus(r.status).tone" size="sm">{{
                runStatus(r.status).label
              }}</PtStatusPill>
              <span class="sdd__run-summary">{{ r.summary }}</span>
            </button>
            <ul v-if="openRun === r.id" class="sdd__steps">
              <li v-for="(st, i) in r.steps" :key="i">
                <span class="sdd__step-kind">{{ stepKind(st.kind) }}</span>
                <span>{{ st.text }}</span>
              </li>
              <li v-if="!r.steps.length" class="sdd__muted">没有记下步骤</li>
              <li v-if="r.steps_truncated" class="sdd__muted">步骤太多，后面的没有记</li>
            </ul>
          </div>
          <el-button
            v-if="runs.length < runTotal"
            link
            :loading="loadingRuns"
            data-testid="sub-runs-more"
            @click="loadRuns(true)"
            >再看 {{ Math.min(RUN_PAGE, runTotal - runs.length) }} 条</el-button
          >
        </div>
      </div>

      <div class="sdd__section">
        <div class="sdd__label">下载过的种子</div>
        <p v-if="!detail.torrent_list.length" class="sdd__sub">还没有下载过</p>
        <div v-else class="sdd__torrents" data-testid="sub-detail-torrents">
          <div v-for="t in detail.torrent_list" :key="t.id" class="sdd__torrent">
            <div class="sdd__ttitle">{{ t.title }}</div>
            <div class="sdd__sub">
              <PtStatusPill :tone="torrentStatus(t.status).tone" size="sm">{{
                torrentStatus(t.status).label
              }}</PtStatusPill>
              <span>{{ t.adopted ? "下载器里已有的" : t.site_name }}</span>
              <span v-if="torrentSpan(t)">{{ torrentSpan(t) }}</span>
              <span v-if="!t.adopted">{{ t.score }} 分</span>
              <span>{{ formatShortDateTime(t.created_at) }}</span>
            </div>
            <div v-if="t.message" class="sdd__sub">{{ t.message }}</div>
          </div>
        </div>
      </div>
    </div>
    <template #footer>
      <el-button @click="visible = false">关闭</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.sdd {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
  max-height: min(70vh, 720px);
  overflow-y: auto;
  padding-right: 2px;
}

.sdd__head {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.sdd__name {
  font-size: 15px;
  font-weight: 600;
  color: var(--pt-t1);
}

.sdd__sub {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 10px;
  margin: 0;
  font-size: 12px;
  color: var(--pt-t3);
  word-break: break-all;
}

.sdd__muted {
  color: var(--pt-t3);
}

.sdd__section {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.sdd__label {
  font-size: 12px;
  font-weight: 600;
  color: var(--pt-t2);
}

.sdd__grid {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.sdd__cells {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

/* 一集一格：底色是状态色的淡染，字是同一个颜色 */
.sdd__cell {
  --c: var(--pt-t3);

  width: 34px;
  height: 28px;
  padding: 0;
  border: 1px solid color-mix(in srgb, var(--c) 40%, transparent);
  border-radius: 4px;
  background: color-mix(in srgb, var(--c) 14%, transparent);
  color: var(--c);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  cursor: pointer;
}

.sdd__cell.is-selected {
  outline: 2px solid var(--c);
  outline-offset: 1px;
}

.sdd__cell--library {
  --c: var(--pt-ok);
}

.sdd__cell--downloading {
  --c: var(--pt-info);
}

.sdd__cell--missing {
  --c: var(--pt-warn);
}

.sdd__cell--waiting_version {
  --c: var(--pt-p);
}

.sdd__cell--upcoming {
  --c: var(--pt-t3);
}

.sdd__cell--skipped {
  --c: var(--pt-t4, var(--pt-t3));

  border-style: dashed;
  background: transparent;
  text-decoration: line-through;
}

.sdd__legend {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
  font-size: 12px;
  color: var(--pt-t2);
}

.sdd__legend-item {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}

.sdd__dot {
  --c: var(--pt-t3);

  width: 8px;
  height: 8px;
  border-radius: 2px;
  background: var(--c);
}

.sdd__ep {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 12px;
  border: 1px solid var(--pt-border);
  border-radius: 6px;
}

.sdd__ep-title {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px 10px;
  font-size: 13px;
  color: var(--pt-t1);
}

.sdd__epno {
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

.sdd__ep-acts {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.sdd__ep-acts .el-button + .el-button {
  margin-left: 0;
}

.sdd__rule {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 10px;
  font-size: 13px;
  color: var(--pt-t2);
}

.sdd__rule-group {
  font-weight: 600;
  color: var(--pt-t1);
}

.sdd__rule-form {
  display: grid;
  grid-template-columns: minmax(0, 1.2fr) minmax(0, 1fr) auto minmax(0, 1fr) auto;
  align-items: center;
  gap: 8px;
}

.sdd__runs {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  border: 1px solid var(--pt-border);
  border-radius: 6px;
}

.sdd__run {
  width: 100%;
}

.sdd__run + .sdd__run {
  border-top: 1px solid var(--pt-border);
}

.sdd__run-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 10px;
  width: 100%;
  padding: 7px 10px;
  border: 0;
  background: transparent;
  font-size: 13px;
  color: var(--pt-t2);
  text-align: left;
  cursor: pointer;
}

.sdd__run-head:hover {
  background: var(--pt-hover);
}

.sdd__run-time {
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  color: var(--pt-t3);
}

.sdd__run-summary {
  flex: 1 1 240px;
  min-width: 0;
  color: var(--pt-t1);
  word-break: break-all;
}

.sdd__steps {
  display: flex;
  flex-direction: column;
  gap: 3px;
  margin: 0;
  padding: 0 10px 8px 22px;
  font-size: 12px;
  color: var(--pt-t2);
  word-break: break-all;
}

.sdd__step-kind {
  display: inline-block;
  min-width: 44px;
  margin-right: 6px;
  color: var(--pt-t3);
}

.sdd__torrents {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.sdd__torrent {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.sdd__ttitle {
  font-size: 13px;
  color: var(--pt-t1);
  word-break: break-all;
}

@media (max-width: 767px) {
  .sdd__rule-form {
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  }
}
</style>
