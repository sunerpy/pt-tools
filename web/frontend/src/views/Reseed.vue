<script setup lang="ts">
/*
 * IYUU 辅种（路线图 M6）：用 IYUU 找出其他站点上数据相同的种子，下载后核对 info hash 与文件列表，
 * 暂停加入同一台下载器、校验到 100% 才开始做种（复用转移做种的任务）。默认关闭，token 加密保存、只写不读。
 * 画板没有这一页，沿用下载组列表页的样式：页头 + 带形工具栏 + 面板里的表单与表格，手机是行卡。
 */
import {
  type DownloaderSetting,
  downloadersApi,
  type ReseedRecord,
  type ReseedRecordResult,
  type ReseedRun,
  type ReseedSettings,
  type ReseedSiteMapItem,
  type ReseedSiteSelection,
  reseedApi,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import ReseedRunDrawer from "@/components/reseed/ReseedRunDrawer.vue";
import ReseedStats from "@/components/reseed/ReseedStats.vue";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtToolbar from "@/components/ui/PtToolbar.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { useSearchInput } from "@/composables/useSearchInput";
import { formatShortDateTime } from "@/utils/format";
import {
  applySiteSelection,
  RECORD_RESULT_OPTIONS,
  reseedRecordActive,
  reseedRecordCancelable,
  reseedRecordMessage,
  reseedRecordResult,
  reseedRecordRetryable,
  reseedIYUUCallsText,
  reseedIYUUEndpoint,
  reseedRunDuration,
  reseedRunProgress,
  reseedRunStatus,
  reseedRunTrigger,
  reseedSiteStatus,
} from "@/utils/reseed";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";

type Tab = "stats" | "settings" | "sites" | "runs" | "records";
const TABS: { label: string; value: Tab }[] = [
  { label: "统计", value: "stats" },
  { label: "设置", value: "settings" },
  { label: "站点", value: "sites" },
  { label: "轮次", value: "runs" },
  { label: "记录", value: "records" },
];
/** 轮次、记录列表每页几条 */
const RUNS_PAGE = 20;
const RECORDS_PAGE = 20;
/** 正在运行或有任务没结束时多久刷新一次 */
const REFRESH_MS = 5000;

const isMobile = useIsMobile();
const tab = ref<Tab>("settings");
/** 用户自己点过标签：读到设置以后不再替他换到「统计」 */
const tabPicked = ref(false);
const downloaders = ref<DownloaderSetting[]>([]);

// ---- 设置 ----
const settings = ref<ReseedSettings | null>(null);
const form = ref({
  enabled: false,
  token: "",
  interval_hours: 12,
  downloader_ids: [] as number[],
  max_per_site_per_day: 20,
  iyuu_min_interval_seconds: 3,
  iyuu_max_calls_per_day: 0,
});
const loadFailed = ref(false);
const saving = ref(false);
const starting = ref(false);

// ---- 站点 ----
const sitesDS = useDataState();
const sites = ref<ReseedSiteMapItem[]>([]);
const sitesLoaded = ref(false);
/** 正在保存开关的站点（site_name；全部开关时是 *） */
const siteSaving = ref<string | null>(null);
const siteMode = computed(() => settings.value?.site_mode ?? "all");

// ---- 记录 ----
const recordsDS = useDataState();
const records = ref<ReseedRecord[]>([]);
const recordsTotal = ref(0);
const recordsPage = ref(1);
const recordsLoaded = ref(false);
/** 记录里出现过的站点（筛选用） */
const recordSites = ref<string[]>([]);
const recordFilter = reactive({ result: "" as ReseedRecordResult | "", site: "", q: "" });
const recordFiltered = computed(() =>
  Boolean(recordFilter.result || recordFilter.site || recordFilter.q.trim()),
);
/** 勾选的记录：只在当前这一页里（换页、筛选时清掉；任务没结束的不能勾） */
const picked = ref<number[]>([]);
/** 正在重试、删除或取消 */
const recordBusy = ref(false);

// ---- 轮次 ----
const runsDS = useDataState();
const runs = ref<ReseedRun[]>([]);
const runsTotal = ref(0);
const runsPage = ref(1);
const runsLoaded = ref(false);
/** 抽屉里看的那一轮 */
const drawerRun = ref<number | null>(null);
const runDrawer = ref<InstanceType<typeof ReseedRunDrawer> | null>(null);
const statsView = ref<InstanceType<typeof ReseedStats> | null>(null);
/** 正在跑（或暂停中）的那一轮 */
const current = computed(() => settings.value?.current_run ?? null);
/** 正在暂停、继续或停止 */
const controlling = ref(false);
const stopDialog = ref(false);
const stopCancelPending = ref(false);

let timer: ReturnType<typeof setInterval> | undefined;
const busy = computed(
  () =>
    Boolean(settings.value?.running) ||
    Boolean(current.value) ||
    records.value.some((r) => reseedRecordActive(r)),
);

const headSub = computed(() => {
  if (loadFailed.value) return "设置没读到，点刷新重试";
  if (!settings.value?.has_token) return "填写 IYUU token 后才能辅种";
  return "用 IYUU 找出其他站点上数据相同的种子，暂停加入同一台下载器，校验到 100% 才开始做种";
});

/** resetForm 为 false 时只更新运行情况（轮询时不能把正在填的表单改回去） */
function fill(s: ReseedSettings, resetForm = true) {
  settings.value = s;
  if (!resetForm) return;
  form.value = {
    enabled: s.enabled,
    token: "",
    interval_hours: s.interval_hours,
    downloader_ids: [...(s.downloader_ids ?? [])],
    max_per_site_per_day: s.max_per_site_per_day,
    iyuu_min_interval_seconds: s.iyuu_min_interval_seconds ?? 3,
    iyuu_max_calls_per_day: s.iyuu_max_calls_per_day ?? 0,
  };
}

/** 今天请求 IYUU 的情况 */
const iyuuToday = computed(() => reseedIYUUCallsText(settings.value?.iyuu_today ?? []));

async function loadSettings(resetForm = true) {
  try {
    const s = await reseedApi.settings();
    if (s) fill(s, resetForm);
    loadFailed.value = false;
  } catch {
    loadFailed.value = true;
  }
}

async function loadDownloaders() {
  try {
    downloaders.value = ((await downloadersApi.list()) ?? []).filter((d) => d.enabled);
  } catch {
    downloaders.value = [];
  }
}

async function loadSites() {
  if (!settings.value?.has_token) return;
  const pending = sitesDS.run(() => reseedApi.sites());
  const data = await pending;
  if (sitesDS.isStale(pending)) return;
  if (data) {
    sites.value = data.items ?? [];
    sitesLoaded.value = true;
  }
}

async function loadRecords(page = recordsPage.value): Promise<void> {
  const filter = { ...recordFilter, q: recordFilter.q.trim() };
  const pending = recordsDS.run(() => reseedApi.records(filter, page, RECORDS_PAGE));
  const data = await pending;
  if (recordsDS.isStale(pending)) return;
  if (!data) return;
  const total = data.total ?? 0;
  const last = Math.max(1, Math.ceil(total / RECORDS_PAGE));
  // 删掉了最后一页的全部记录：回到现在的最后一页
  if (!data.items?.length && page > last) return loadRecords(last);
  records.value = data.items ?? [];
  recordsTotal.value = total;
  recordsPage.value = page;
  recordSites.value = data.sites ?? [];
  recordsLoaded.value = true;
  const can = new Set(records.value.filter((r) => !reseedRecordActive(r)).map((r) => r.id));
  picked.value = picked.value.filter((id) => can.has(id));
}

function applyRecordFilter() {
  picked.value = [];
  void loadRecords(1);
}
const recordSearch = useSearchInput(() => recordFilter.q, applyRecordFilter);

function changeRecordsPage(page: number) {
  picked.value = [];
  void loadRecords(page);
}

const recordsNote = computed(() => {
  const from = (recordsPage.value - 1) * RECORDS_PAGE + 1;
  const to = Math.min(recordsPage.value * RECORDS_PAGE, recordsTotal.value);
  return `第 ${from}–${to} 条，共 ${recordsTotal.value} 条`;
});

/** 这一页能勾的记录（任务还没结束的不能重试也不能删除） */
const pickable = computed(() => records.value.filter((r) => !reseedRecordActive(r)));
const allPicked = computed(
  () => pickable.value.length > 0 && pickable.value.every((r) => picked.value.includes(r.id)),
);
const pickedRecords = computed(() => records.value.filter((r) => picked.value.includes(r.id)));
const pickedRetryable = computed(() => pickedRecords.value.filter(reseedRecordRetryable));

function pickAll(on: boolean) {
  picked.value = on ? pickable.value.map((r) => r.id) : [];
}

function togglePick(id: number, on: boolean) {
  picked.value = on ? [...new Set([...picked.value, id])] : picked.value.filter((v) => v !== id);
}

/** 重试选中的记录（只发能重试的）：在后台跑一轮，进度在页头 */
async function retryRecords(rows: ReseedRecord[]) {
  const ids = rows.filter(reseedRecordRetryable).map((r) => r.id);
  if (!ids.length) return;
  recordBusy.value = true;
  try {
    await reseedApi.retryRecords(ids);
    ElMessage.success(`已开始重试 ${ids.length} 条，进度在页头，结果写在「轮次」里`);
    picked.value = [];
    await loadSettings(false);
    scheduleRefresh();
  } catch (e) {
    ElMessage.error((e as Error).message || "重试失败");
  } finally {
    recordBusy.value = false;
  }
}

async function deleteRecords(rows: ReseedRecord[]) {
  const ids = rows.filter((r) => !reseedRecordActive(r)).map((r) => r.id);
  if (!ids.length) return;
  try {
    await ElMessageBox.confirm(
      `删除 ${ids.length} 条记录？删除以后，这些种子在以后的轮次里会再试一次；今天已经用掉的名额和统计不变。`,
      "删除记录",
      { type: "warning", confirmButtonText: "删除", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  recordBusy.value = true;
  try {
    const res = await reseedApi.deleteRecords(ids);
    ElMessage.success(
      res.active
        ? `已删除 ${res.deleted} 条；${res.active} 条的任务还没结束，没有删`
        : `已删除 ${res.deleted} 条`,
    );
    picked.value = picked.value.filter((id) => !ids.includes(id));
    await loadRecords();
  } catch (e) {
    ElMessage.error((e as Error).message || "删除失败");
  } finally {
    recordBusy.value = false;
  }
}

/** 取消还在等待加入下载器的任务（取消以后可以再重试） */
async function cancelRecord(r: ReseedRecord) {
  recordBusy.value = true;
  try {
    await reseedApi.cancelRecord(r.id);
    ElMessage.success("已取消；要再试时点「重试」");
    await loadRecords();
  } catch (e) {
    ElMessage.error((e as Error).message || "取消失败");
  } finally {
    recordBusy.value = false;
  }
}

async function loadRuns(page = runsPage.value) {
  const pending = runsDS.run(() => reseedApi.runs(page, RUNS_PAGE));
  const data = await pending;
  if (runsDS.isStale(pending)) return;
  if (data) {
    runs.value = data.items ?? [];
    runsTotal.value = data.total ?? 0;
    runsPage.value = page;
    runsLoaded.value = true;
  }
}

/** 正在运行或有任务没结束时每 5 秒刷新；都停了就不刷 */
function scheduleRefresh() {
  if (busy.value) {
    if (!timer)
      timer = setInterval(() => {
        void loadSettings(false);
        void loadRecords();
        if (tab.value === "runs") void loadRuns();
      }, REFRESH_MS);
  } else if (timer) {
    clearInterval(timer);
    timer = undefined;
  }
}

async function save(clearToken = false) {
  saving.value = true;
  try {
    const token = form.value.token.trim();
    const s = await reseedApi.saveSettings({
      enabled: form.value.enabled,
      ...(clearToken ? { token: "" } : token ? { token } : {}),
      interval_hours: form.value.interval_hours,
      downloader_ids: form.value.downloader_ids,
      max_per_site_per_day: form.value.max_per_site_per_day,
      iyuu_min_interval_seconds: form.value.iyuu_min_interval_seconds,
      iyuu_max_calls_per_day: form.value.iyuu_max_calls_per_day,
    });
    if (s) fill(s);
    ElMessage.success("已保存");
    if (token || clearToken) {
      sites.value = [];
      sitesLoaded.value = false;
    }
  } catch (e) {
    ElMessage.error((e as Error).message || "保存失败");
  } finally {
    saving.value = false;
  }
}

function applySelection(sel: ReseedSiteSelection) {
  if (settings.value) {
    settings.value = { ...settings.value, site_mode: sel.site_mode, site_names: sel.site_names };
  }
  sites.value = applySiteSelection(sites.value, sel);
}

/** 打开或关掉一个站点（name 为 * 时是全部） */
async function toggleSite(name: string, selected: boolean) {
  siteSaving.value = name;
  try {
    const sel =
      name === "*"
        ? await reseedApi.setAllSites(selected)
        : await reseedApi.setSite(name, selected);
    if (sel) applySelection(sel);
  } catch (e) {
    ElMessage.error((e as Error).message || "保存失败");
  } finally {
    siteSaving.value = null;
  }
}

async function clearToken() {
  try {
    await ElMessageBox.confirm("清除 IYUU token？辅种会一并关闭。", "清除 token", {
      type: "warning",
      confirmButtonText: "清除",
      cancelButtonText: "取消",
    });
  } catch {
    return;
  }
  form.value.enabled = false;
  await save(true);
}

async function runNow() {
  starting.value = true;
  try {
    await reseedApi.run();
    ElMessage.success("已开始，进度在页头，结果写在「轮次」里");
    await loadSettings(false);
    scheduleRefresh();
  } catch (e) {
    ElMessage.error((e as Error).message || "运行失败");
  } finally {
    starting.value = false;
  }
}

/** 暂停或继续正在跑的那一轮 */
async function togglePause() {
  const cur = current.value;
  if (!cur) return;
  controlling.value = true;
  try {
    if (cur.status === "paused") {
      await reseedApi.resumeRun(cur.id);
      ElMessage.success("已继续");
    } else {
      await reseedApi.pauseRun(cur.id);
      ElMessage.success("已暂停：正在下载的那一个做完以后停下");
    }
  } catch (e) {
    ElMessage.error((e as Error).message || "操作失败");
  } finally {
    controlling.value = false;
    await loadSettings(false);
  }
}

function askStop() {
  stopCancelPending.value = false;
  stopDialog.value = true;
}

async function confirmStop() {
  const cur = current.value;
  stopDialog.value = false;
  if (!cur) return;
  controlling.value = true;
  try {
    await reseedApi.stopRun(cur.id, stopCancelPending.value);
    ElMessage.success("已停止；没查完的部分可以在「轮次」里继续");
  } catch (e) {
    ElMessage.error((e as Error).message || "停止失败");
  } finally {
    controlling.value = false;
    await loadSettings(false);
  }
}

/** 接着一轮提前停下的往下查 */
async function continueRun(id: number) {
  try {
    await reseedApi.continueRun(id);
    ElMessage.success("已开始，接着这一轮没查完的部分往下查");
    drawerRun.value = null;
    await loadSettings(false);
    void loadRuns(1);
    scheduleRefresh();
  } catch (e) {
    ElMessage.error((e as Error).message || "继续失败");
  }
}

function refresh() {
  void loadSettings();
  void loadDownloaders();
  if (tab.value === "stats") void statsView.value?.reload();
  if (tab.value === "sites") void loadSites();
  if (tab.value === "records") void loadRecords();
  if (tab.value === "runs") void loadRuns();
}

watch(tab, (t) => {
  if (t === "sites" && !sitesLoaded.value) void loadSites();
  if (t === "records") void loadRecords();
  if (t === "runs") void loadRuns();
});
watch(busy, () => scheduleRefresh());
/* 一轮结束（或开始）时刷新轮次列表、抽屉与统计 */
watch(
  () => current.value?.id,
  () => {
    if (tab.value === "runs") void loadRuns();
    void runDrawer.value?.reload();
    void statsView.value?.reload();
  },
);

onMounted(async () => {
  await Promise.all([loadSettings(), loadDownloaders()]);
  // 填了 token 以后先看统计；还没填时先看设置
  if (settings.value?.has_token && !tabPicked.value) tab.value = "stats";
  void loadRecords();
  if (settings.value?.has_token) void loadSites();
});
onBeforeUnmount(() => {
  if (timer) clearInterval(timer);
});
</script>

<template>
  <div class="pt-cards pt-cards--wide">
    <PtHeadSub>{{ headSub }}</PtHeadSub>
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button data-testid="rs-refresh" @click="refresh">
        <PtIcon name="refresh-cw" :size="15" /><span>刷新</span>
      </el-button>
      <template v-if="current">
        <el-button
          :type="current.status === 'paused' ? 'primary' : 'default'"
          :loading="controlling"
          :data-testid="current.status === 'paused' ? 'rs-resume' : 'rs-pause'"
          @click="togglePause">
          <PtIcon
            v-if="!controlling"
            :name="current.status === 'paused' ? 'play' : 'pause'"
            :size="15" /><span>{{ current.status === "paused" ? "继续" : "暂停" }}</span>
        </el-button>
        <el-button :disabled="controlling" data-testid="rs-stop" @click="askStop">
          <PtIcon name="square" :size="15" /><span>停止</span>
        </el-button>
      </template>
      <el-button
        v-else
        type="primary"
        :loading="starting"
        :disabled="!settings?.has_token || settings?.running"
        data-testid="rs-run"
        @click="runNow">
        <PtIcon v-if="!starting" name="play" :size="15" /><span>立即运行</span>
      </el-button>
    </Teleport>

    <div class="pt-note">
      <PtIcon name="info" :size="14" class="pt-note__icon" />
      <span
        >开启后，pt-tools 会把参与辅种的下载器里已下完种子的 info hash 发给
        IYUU（2025.iyuu.cn）查询，找出其他站点上数据相同的种子。 IYUU token
        加密保存在本机，页面上不显示。</span
      >
    </div>

    <PtToolbar band>
      <el-segmented
        v-model="tab"
        class="pt-seg"
        :options="TABS"
        :props="{ label: 'label', value: 'value' }"
        data-testid="rs-tabs"
        @change="tabPicked = true" />
      <template v-if="current" #note>
        <span data-testid="rs-progress"
          >{{ current.status === "paused" ? "已暂停" : "正在运行" }}：{{
            reseedRunProgress(current)
          }}</span
        >
      </template>
      <template v-else-if="settings?.running && !isMobile" #note>正在运行</template>
    </PtToolbar>

    <!-- 统计 -->
    <ReseedStats v-if="tab === 'stats'" ref="statsView" :settings="settings" />

    <!-- 设置 -->
    <PtPanel v-else-if="tab === 'settings'" title="辅种设置" icon="share-2">
      <div v-if="loadFailed" class="pt-note pt-note--warn">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>设置没读到，点刷新重试。</span>
      </div>
      <!-- 卡够宽时表单在左、运行情况在右（按卡宽判）；以前要视口 ≥1600 才分栏，1440 上右边空着半张卡 -->
      <div class="pt-formaside">
        <div class="pt-formaside__grid">
          <el-form class="pt-form" label-position="top" @submit.prevent>
            <el-form-item label="开启辅种">
              <el-switch v-model="form.enabled" data-testid="rs-enabled" />
            </el-form-item>
            <el-form-item label="IYUU token">
              <div class="rs-token">
                <el-input
                  v-model="form.token"
                  type="password"
                  show-password
                  autocomplete="off"
                  :placeholder="
                    settings?.has_token ? '已设置；要更换时填写新的' : '按 IYUU 的说明获取'
                  "
                  data-testid="rs-token" />
                <el-button
                  v-if="settings?.has_token"
                  data-testid="rs-clear-token"
                  @click="clearToken"
                  >清除</el-button
                >
              </div>
              <div class="field-tip" data-testid="rs-token-tip">
                在 IYUU 官网
                <a href="https://iyuu.cn/" target="_blank" rel="noopener noreferrer">iyuu.cn</a>
                点「开始使用」，用微信扫码登录后拿到 token。
              </div>
            </el-form-item>
            <div class="rs-row">
              <el-form-item label="运行间隔（小时）">
                <el-input-number
                  v-model="form.interval_hours"
                  :min="1"
                  :max="168"
                  :disabled="!form.enabled"
                  controls-position="right"
                  data-testid="rs-interval" />
                <div class="field-tip">开着时按这个间隔自动运行；关着时只能「立即运行」</div>
              </el-form-item>
              <el-form-item label="每站每天最多（个）">
                <el-input-number
                  v-model="form.max_per_site_per_day"
                  :min="1"
                  :max="500"
                  controls-position="right"
                  data-testid="rs-max" />
                <div class="field-tip">
                  每个站点每天最多尝试多少个辅种（下载失败、核对没通过的也算），一次不给站点太多请求
                </div>
              </el-form-item>
            </div>
            <el-form-item label="参与的下载器">
              <el-select
                v-model="form.downloader_ids"
                multiple
                clearable
                placeholder="全部启用的下载器"
                data-testid="rs-downloaders">
                <el-option v-for="d in downloaders" :key="d.id" :label="d.name" :value="d.id" />
              </el-select>
            </el-form-item>
            <div class="field-tip rs-sites-tip">参与辅种的站点在「站点」里用开关设置。</div>
            <h3 class="rs-form-title">IYUU 请求</h3>
            <div class="rs-row">
              <el-form-item label="两次请求至少隔（秒）">
                <el-input-number
                  v-model="form.iyuu_min_interval_seconds"
                  :min="0"
                  :max="300"
                  controls-position="right"
                  data-testid="rs-iyuu-gap" />
                <div class="field-tip">默认 3 秒；0 是不隔</div>
              </el-form-item>
              <el-form-item label="每天最多请求（次）">
                <el-input-number
                  v-model="form.iyuu_max_calls_per_day"
                  :min="0"
                  :max="10000"
                  controls-position="right"
                  data-testid="rs-iyuu-cap" />
                <div class="field-tip">0 是不限；到了上限，这一轮提前停下，第二天接着往下查</div>
              </el-form-item>
            </div>
            <el-button type="primary" :loading="saving" data-testid="rs-save" @click="save()">
              <PtIcon v-if="!saving" name="save" :size="14" /><span>保存</span>
            </el-button>
          </el-form>
          <aside class="pt-formaside__aside">
            <h3 class="pt-formaside__title">运行情况</h3>
            <ul class="rs-status" data-testid="rs-status">
              <li>
                <span class="rs-k">上次运行</span>
                <span v-if="settings?.last_run_at"
                  >{{ formatShortDateTime(settings.last_run_at) }}：{{ settings.last_result }}</span
                >
                <span v-else class="rs-sub">还没运行过</span>
              </li>
              <li>
                <span class="rs-k">下次运行</span>
                <span v-if="settings?.running">正在运行</span>
                <span v-else-if="settings?.next_run_at"
                  >大约 {{ formatShortDateTime(settings.next_run_at) }}</span
                >
                <span v-else class="rs-sub">{{
                  settings?.has_token
                    ? "没开启：只能「立即运行」"
                    : "填写 token 并开启以后按间隔自动运行"
                }}</span>
              </li>
            </ul>
            <ul class="rs-status" data-testid="rs-iyuu-status">
              <li>
                <span class="rs-k">今天请求</span>
                <span v-if="iyuuToday">{{ iyuuToday }}</span>
                <span v-else class="rs-sub">还没有请求 IYUU</span>
              </li>
              <li v-if="settings?.iyuu_cooldown_until" class="rs-warn">
                <span class="rs-k">限流冷却</span>
                <span data-testid="rs-iyuu-cooldown"
                  >{{ formatShortDateTime(settings.iyuu_cooldown_until) }} 以前不请求
                  IYUU，也不跑定时轮次</span
                >
              </li>
              <li v-if="settings?.iyuu_last_limit">
                <span class="rs-k">上次限流</span>
                <span
                  >{{ formatShortDateTime(settings.iyuu_last_limit.at) }}
                  {{ reseedIYUUEndpoint(settings.iyuu_last_limit.endpoint) }}：{{
                    settings.iyuu_last_limit.message || "请求太频繁"
                  }}</span
                >
              </li>
            </ul>
            <p class="rs-tip">
              只辅种文件完全一样（路径和大小逐个相同）的种子；核对没通过的不再尝试，下载不到种子的过
              7 天再试。
            </p>
            <p class="rs-tip">
              IYUU 推荐每天辅种一次，短时间内多次运行会被拒绝（辅种太频繁）。被限流以后按 IYUU
              给的时间冷却。
            </p>
          </aside>
        </div>
      </div>
    </PtPanel>

    <!-- 站点 -->
    <PtPanel
      v-else-if="tab === 'sites'"
      title="IYUU 站点"
      icon="globe"
      :count="sitesLoaded ? sites.length : undefined">
      <template v-if="sites.some((s) => s.configured)" #actions>
        <el-button
          link
          size="small"
          :disabled="siteSaving !== null"
          data-testid="rs-sites-all-on"
          @click="toggleSite('*', true)">
          全部开启
        </el-button>
        <el-button
          link
          size="small"
          :disabled="siteSaving !== null"
          data-testid="rs-sites-all-off"
          @click="toggleSite('*', false)">
          全部关闭
        </el-button>
      </template>
      <div
        v-if="sitesLoaded && siteMode === 'selected'"
        class="pt-note"
        data-testid="rs-sites-mode-note">
        <PtIcon name="info" :size="14" class="pt-note__icon" />
        <span
          >只辅种打开了开关的站点；以后新配置的站点默认不参与，要在这里打开。点「全部开启」恢复为全部站点（以后新配置的也自动参与）。</span
        >
      </div>
      <div
        v-if="sites.length && sitesDS.error.value"
        class="pt-note pt-note--warn"
        data-testid="rs-sites-stale">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>刷新失败：{{ sitesDS.errorText.value }}。下面是上次读到的。</span>
      </div>
      <PtDataState
        v-if="!sites.length"
        :state="settings?.has_token ? sitesDS.state.value : 'empty'"
        :title="settings?.has_token ? (sitesDS.error.value ? '' : '没有站点') : '还没有 token'"
        :sub="
          settings?.has_token
            ? sitesDS.errorText.value || 'IYUU 没有返回站点'
            : '在「设置」里填写 IYUU token 后查看'
        " />
      <el-table
        v-else-if="!isMobile"
        :data="sites"
        class="pt-grid"
        row-key="sid"
        data-testid="rs-site-table">
        <el-table-column label="IYUU 站点" min-width="180" class-name="pt-cell-strong">
          <template #default="{ row }">{{ row.nickname || row.iyuu_site }}</template>
        </el-table-column>
        <el-table-column label="地址" min-width="180">
          <template #default="{ row }">{{ row.host }}</template>
        </el-table-column>
        <el-table-column label="pt-tools 站点" min-width="140">
          <template #default="{ row }">{{ row.site_name || "—" }}</template>
        </el-table-column>
        <el-table-column label="参与辅种" width="130">
          <template #default="{ row }">
            <el-switch
              v-if="row.configured"
              :model-value="row.selected"
              :loading="siteSaving === row.site_name"
              :disabled="siteSaving !== null"
              :aria-label="`${row.nickname || row.iyuu_site}参与辅种`"
              :data-testid="`rs-site-switch-${row.site_name}`"
              @change="(v: string | number | boolean) => toggleSite(row.site_name, Boolean(v))" />
            <PtStatusPill v-else :tone="reseedSiteStatus(row).tone" size="sm">{{
              reseedSiteStatus(row).label
            }}</PtStatusPill>
          </template>
        </el-table-column>
      </el-table>
      <div v-else class="rs-cards">
        <PtRowCard v-for="s in sites" :key="s.sid">
          <template #title>{{ s.nickname || s.iyuu_site }}</template>
          <template #meta>
            <span
              >{{ s.host }}<template v-if="s.site_name"> · {{ s.site_name }}</template></span
            >
          </template>
          <template #status>
            <el-switch
              v-if="s.configured"
              :model-value="s.selected"
              :loading="siteSaving === s.site_name"
              :disabled="siteSaving !== null"
              :aria-label="`${s.nickname || s.iyuu_site}参与辅种`"
              :data-testid="`rs-site-switch-${s.site_name}`"
              @change="(v: string | number | boolean) => toggleSite(s.site_name!, Boolean(v))" />
            <PtStatusPill v-else :tone="reseedSiteStatus(s).tone" size="sm">{{
              reseedSiteStatus(s).label
            }}</PtStatusPill>
          </template>
        </PtRowCard>
      </div>
    </PtPanel>

    <!-- 轮次 -->
    <PtPanel
      v-else-if="tab === 'runs'"
      title="辅种轮次"
      icon="history"
      :count="runsLoaded ? runsTotal : undefined">
      <div
        v-if="runs.length && runsDS.error.value"
        class="pt-note pt-note--warn"
        data-testid="rs-runs-stale">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>刷新失败：{{ runsDS.errorText.value }}。下面是上次读到的。</span>
      </div>
      <PtDataState
        v-if="!runs.length"
        :state="runsDS.state.value"
        :title="runsDS.error.value ? '' : '还没有轮次'"
        :sub="
          runsDS.errorText.value || '每一轮（定时、立即运行、继续或重试）记一条，保留 180 天'
        " />
      <el-table
        v-else-if="!isMobile"
        :data="runs"
        class="pt-grid"
        row-key="id"
        data-testid="rs-runs"
        @row-click="(row: ReseedRun) => (drawerRun = row.id)">
        <el-table-column label="开始" width="130">
          <template #default="{ row }">{{ formatShortDateTime(row.started_at) }}</template>
        </el-table-column>
        <el-table-column label="方式" width="90">
          <template #default="{ row }">{{ reseedRunTrigger(row.trigger) }}</template>
        </el-table-column>
        <el-table-column label="状态" width="110">
          <template #default="{ row }">
            <PtStatusPill :tone="reseedRunStatus(row.status).tone" size="sm">{{
              reseedRunStatus(row.status).label
            }}</PtStatusPill>
          </template>
        </el-table-column>
        <el-table-column
          label="结果"
          min-width="260"
          class-name="pt-cell-1line"
          show-overflow-tooltip>
          <template #default="{ row }">{{ row.summary || row.error || "—" }}</template>
        </el-table-column>
        <el-table-column label="IYUU 请求" width="90" align="right">
          <template #default="{ row }">{{
            row.trigger === "retry" ? "—" : `${row.iyuu_calls} 次`
          }}</template>
        </el-table-column>
        <el-table-column label="用时" width="100">
          <template #default="{ row }">{{ reseedRunDuration(row) || "—" }}</template>
        </el-table-column>
        <el-table-column label="" width="120" align="right">
          <template #default="{ row }">
            <el-button
              v-if="row.continuable && !current"
              link
              type="primary"
              :data-testid="`rs-run-continue-${row.id}`"
              @click.stop="continueRun(row.id)"
              >继续</el-button
            >
            <el-button link :data-testid="`rs-run-open-${row.id}`" @click.stop="drawerRun = row.id"
              >详情</el-button
            >
          </template>
        </el-table-column>
      </el-table>
      <div v-else class="rs-cards">
        <PtRowCard v-for="r in runs" :key="r.id" interactive @click="drawerRun = r.id">
          <template #title>{{ r.summary || r.error || "—" }}</template>
          <template #meta>
            <span
              >{{ formatShortDateTime(r.started_at) }} · {{ reseedRunTrigger(r.trigger)
              }}<template v-if="reseedRunDuration(r)"> · {{ reseedRunDuration(r) }}</template></span
            >
          </template>
          <template #status>
            <PtStatusPill :tone="reseedRunStatus(r.status).tone" size="sm">{{
              reseedRunStatus(r.status).label
            }}</PtStatusPill>
          </template>
          <template #actions>
            <el-button
              v-if="r.continuable && !current"
              size="small"
              type="primary"
              @click.stop="continueRun(r.id)"
              >继续</el-button
            >
            <el-button size="small" @click.stop="drawerRun = r.id">详情</el-button>
          </template>
        </PtRowCard>
      </div>
      <el-pagination
        v-if="runsTotal > RUNS_PAGE"
        class="rs-pager"
        layout="prev, pager, next"
        :total="runsTotal"
        :page-size="RUNS_PAGE"
        :current-page="runsPage"
        @current-change="(p: number) => loadRuns(p)" />
    </PtPanel>

    <!-- 记录 -->
    <PtPanel
      v-else
      title="辅种记录"
      icon="list-checks"
      padding="none"
      :count="recordsLoaded ? recordsTotal : undefined">
      <PtToolbar>
        <el-select
          v-model="recordFilter.result"
          placeholder="全部结果"
          clearable
          class="rs-filter"
          data-testid="rs-record-result"
          @change="applyRecordFilter">
          <el-option
            v-for="o in RECORD_RESULT_OPTIONS"
            :key="o.value"
            :label="o.label"
            :value="o.value" />
        </el-select>
        <el-select
          v-model="recordFilter.site"
          placeholder="全部站点"
          clearable
          filterable
          class="rs-filter"
          data-testid="rs-record-site"
          @change="applyRecordFilter">
          <el-option v-for="name in recordSites" :key="name" :label="name" :value="name" />
        </el-select>
        <el-input
          v-model="recordFilter.q"
          placeholder="种子名或 info hash"
          clearable
          class="rs-filter-q"
          data-testid="rs-record-q"
          @keyup.enter="recordSearch.now"
          @clear="recordSearch.now">
          <template #prefix>
            <PtIcon name="search" :size="14" />
          </template>
        </el-input>
      </PtToolbar>
      <div v-if="(records.length && recordsDS.error.value) || !records.length" class="rs-inset">
        <div
          v-if="records.length && recordsDS.error.value"
          class="pt-note pt-note--warn"
          data-testid="rs-records-stale">
          <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
          <span>刷新失败：{{ recordsDS.errorText.value }}。下面是上次读到的。</span>
        </div>
        <PtDataState
          v-if="!records.length"
          :state="recordsDS.state.value"
          :title="
            recordsDS.error.value ? '' : recordFiltered ? '没有符合条件的记录' : '还没有辅种记录'
          "
          :sub="
            recordsDS.errorText.value ||
            (recordFiltered
              ? '换一个结果、站点或关键词试试'
              : '开启辅种并运行一轮后，这里列出每次尝试')
          " />
      </div>
      <el-table
        v-if="records.length && !isMobile"
        :data="records"
        class="pt-grid"
        row-key="id"
        data-testid="rs-records">
        <el-table-column width="46" align="center">
          <template #header>
            <el-checkbox
              :model-value="allPicked"
              :indeterminate="picked.length > 0 && !allPicked"
              :disabled="!pickable.length"
              aria-label="选择这一页能操作的记录"
              data-testid="rs-record-pick-all"
              @update:model-value="(v: string | number | boolean) => pickAll(Boolean(v))" />
          </template>
          <template #default="{ row }">
            <el-checkbox
              v-if="!reseedRecordActive(row)"
              :model-value="picked.includes(row.id)"
              :aria-label="`选择 ${row.name || row.info_hash}`"
              :data-testid="`rs-record-pick-${row.id}`"
              @update:model-value="
                (v: string | number | boolean) => togglePick(row.id, Boolean(v))
              " />
          </template>
        </el-table-column>
        <el-table-column label="种子" min-width="220" class-name="pt-cell-strong">
          <template #default="{ row }">
            <div class="rs-name">{{ row.name || row.info_hash }}</div>
            <div class="rs-sub">{{ row.site_name }} · {{ row.torrent_id }}</div>
          </template>
        </el-table-column>
        <el-table-column label="结果" width="110">
          <template #default="{ row }">
            <PtStatusPill :tone="reseedRecordResult(row).tone" size="sm">{{
              reseedRecordResult(row).label
            }}</PtStatusPill>
          </template>
        </el-table-column>
        <el-table-column
          label="说明"
          min-width="240"
          class-name="pt-cell-1line"
          show-overflow-tooltip>
          <template #default="{ row }">{{ reseedRecordMessage(row) }}</template>
        </el-table-column>
        <el-table-column label="时间" width="120">
          <template #default="{ row }">{{ formatShortDateTime(row.updated_at) }}</template>
        </el-table-column>
        <el-table-column label="" width="140" align="right">
          <template #default="{ row }">
            <el-button
              v-if="reseedRecordRetryable(row)"
              link
              type="primary"
              :disabled="recordBusy || Boolean(settings?.running)"
              :data-testid="`rs-record-retry-${row.id}`"
              @click="retryRecords([row])"
              >重试</el-button
            >
            <el-button
              v-if="reseedRecordCancelable(row)"
              link
              :disabled="recordBusy"
              :data-testid="`rs-record-cancel-${row.id}`"
              @click="cancelRecord(row)"
              >取消</el-button
            >
            <el-button
              v-if="!reseedRecordActive(row)"
              link
              type="danger"
              :disabled="recordBusy"
              :data-testid="`rs-record-delete-${row.id}`"
              @click="deleteRecords([row])"
              >删除</el-button
            >
          </template>
        </el-table-column>
      </el-table>
      <div v-else-if="records.length" class="rs-cards rs-inset">
        <PtRowCard v-for="r in records" :key="r.id">
          <template v-if="!reseedRecordActive(r)" #lead>
            <el-checkbox
              :model-value="picked.includes(r.id)"
              :aria-label="`选择 ${r.name || r.info_hash}`"
              @update:model-value="
                (v: string | number | boolean) => togglePick(r.id, Boolean(v))
              " />
          </template>
          <template #title>{{ r.name || r.info_hash }}</template>
          <template #meta>
            <span
              >{{ r.site_name }} · {{ r.torrent_id }} ·
              {{ formatShortDateTime(r.updated_at) }}</span
            >
            <span v-if="reseedRecordMessage(r)" class="rs-full">{{ reseedRecordMessage(r) }}</span>
          </template>
          <template #status>
            <PtStatusPill :tone="reseedRecordResult(r).tone" size="sm">{{
              reseedRecordResult(r).label
            }}</PtStatusPill>
          </template>
          <template
            v-if="reseedRecordRetryable(r) || reseedRecordCancelable(r) || !reseedRecordActive(r)"
            #actions>
            <el-button
              v-if="reseedRecordRetryable(r)"
              size="small"
              type="primary"
              :disabled="recordBusy || Boolean(settings?.running)"
              @click="retryRecords([r])"
              >重试</el-button
            >
            <el-button
              v-if="reseedRecordCancelable(r)"
              size="small"
              :disabled="recordBusy"
              @click="cancelRecord(r)"
              >取消</el-button
            >
            <el-button
              v-if="!reseedRecordActive(r)"
              size="small"
              type="danger"
              plain
              :disabled="recordBusy"
              @click="deleteRecords([r])"
              >删除</el-button
            >
          </template>
        </PtRowCard>
      </div>
      <!-- 画板 selband：勾选以后列表下方那条横幅，批量操作放在这里 -->
      <div v-if="picked.length" class="pt-band--sel" data-testid="rs-record-selband">
        <PtIcon name="square-check" :size="15" />
        <span>已选 {{ picked.length }} 条</span>
        <span class="pt-band__spacer" />
        <el-button @click="picked = []">
          <PtIcon name="x" :size="14" /><span>取消选择</span>
        </el-button>
        <el-button
          type="primary"
          :disabled="!pickedRetryable.length || recordBusy || Boolean(settings?.running)"
          data-testid="rs-record-retry-picked"
          @click="retryRecords(pickedRecords)">
          <PtIcon name="rotate-ccw" :size="14" /><span>重试 {{ pickedRetryable.length }} 条</span>
        </el-button>
        <el-button
          type="danger"
          plain
          :disabled="recordBusy"
          data-testid="rs-record-delete-picked"
          @click="deleteRecords(pickedRecords)">
          <PtIcon name="trash-2" :size="14" /><span>删除</span>
        </el-button>
      </div>
      <template v-if="recordsTotal > 0" #footer>
        <span class="pt-foot-note" data-testid="rs-records-note">{{ recordsNote }}</span>
        <el-pagination
          v-if="recordsTotal > RECORDS_PAGE"
          class="pt-pager"
          layout="prev, pager, next"
          :total="recordsTotal"
          :page-size="RECORDS_PAGE"
          :current-page="recordsPage"
          :pager-count="5"
          data-testid="rs-records-pager"
          @current-change="changeRecordsPage" />
      </template>
    </PtPanel>

    <el-dialog v-model="stopDialog" title="停止这一轮" width="440px" append-to-body>
      <p class="rs-dialog-text">
        正在下载的那一个会被打断（不记失败，以后照常再试）。已经建好的辅种任务照常加入和校验；没查完的部分可以在「轮次」里继续。
      </p>
      <el-checkbox v-model="stopCancelPending" data-testid="rs-stop-cancel-pending"
        >同时取消这一轮建的、还没加入下载器的任务</el-checkbox
      >
      <template #footer>
        <el-button @click="stopDialog = false">取消</el-button>
        <el-button type="danger" data-testid="rs-stop-confirm" @click="confirmStop">停止</el-button>
      </template>
    </el-dialog>

    <ReseedRunDrawer
      ref="runDrawer"
      :run-id="drawerRun"
      @close="drawerRun = null"
      @continue="continueRun" />
  </div>
</template>

<style scoped>
.rs-token {
  display: flex;
  gap: 8px;
  width: 100%;
}

.rs-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 0 16px;
}

.rs-pager {
  justify-content: flex-end;
  margin-top: var(--pt-space-3);
}

/* 记录面板贴边（表格、多选条、页脚贴着卡边），提示、空状态与手机行卡自己留白 */
.rs-inset {
  padding: var(--pt-pad);
}

.rs-filter {
  width: 150px;
}

.rs-filter-q {
  width: 220px;
}

@media (max-width: 768px) {
  .rs-filter,
  .rs-filter-q {
    flex: 1 1 140px;
    width: auto;
  }
}

.rs-dialog-text {
  margin: 0 0 12px;
  font-size: 13px;
  line-height: 1.6;
  color: var(--pt-t2);
}

.rs-sites-tip {
  margin: -6px 0 16px;
}

.rs-form-title {
  margin: 4px 0 12px;
  font-size: 13px;
  font-weight: 600;
  color: var(--pt-t1);
}

.rs-warn {
  color: var(--pt-warn);
}

.rs-tip {
  margin: 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--pt-t3);
}

.rs-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.rs-sub {
  font-size: 12px;
  color: var(--pt-t3);
}

.rs-cards {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.rs-full {
  flex-basis: 100%;
}

.rs-status {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 0;
  padding: 0;
  list-style: none;
  font-size: 13px;
  color: var(--pt-t1);
}

.rs-status li {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.rs-k {
  flex: 0 0 72px;
  color: var(--pt-t3);
}

.rs-sub {
  color: var(--pt-t3);
}
</style>
