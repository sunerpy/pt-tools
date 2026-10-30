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
  type ReseedRun,
  type ReseedSettings,
  type ReseedSiteMapItem,
  type ReseedSiteSelection,
  reseedApi,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import ReseedRunDrawer from "@/components/reseed/ReseedRunDrawer.vue";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtToolbar from "@/components/ui/PtToolbar.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { formatShortDateTime } from "@/utils/format";
import {
  applySiteSelection,
  reseedRecordActive,
  reseedRecordMessage,
  reseedRecordResult,
  reseedRunDuration,
  reseedRunProgress,
  reseedRunStatus,
  reseedRunTrigger,
  reseedSiteStatus,
} from "@/utils/reseed";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";

type Tab = "settings" | "sites" | "runs" | "records";
const TABS: { label: string; value: Tab }[] = [
  { label: "设置", value: "settings" },
  { label: "站点", value: "sites" },
  { label: "轮次", value: "runs" },
  { label: "记录", value: "records" },
];
/** 轮次列表每页几条 */
const RUNS_PAGE = 20;
/** 正在运行或有任务没结束时多久刷新一次 */
const REFRESH_MS = 5000;

const isMobile = useIsMobile();
const tab = ref<Tab>("settings");
const downloaders = ref<DownloaderSetting[]>([]);

// ---- 设置 ----
const settings = ref<ReseedSettings | null>(null);
const form = ref({
  enabled: false,
  token: "",
  interval_hours: 12,
  downloader_ids: [] as number[],
  max_per_site_per_day: 20,
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
const recordsLoaded = ref(false);

// ---- 轮次 ----
const runsDS = useDataState();
const runs = ref<ReseedRun[]>([]);
const runsTotal = ref(0);
const runsPage = ref(1);
const runsLoaded = ref(false);
/** 抽屉里看的那一轮 */
const drawerRun = ref<number | null>(null);
const runDrawer = ref<InstanceType<typeof ReseedRunDrawer> | null>(null);
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
  };
}

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

async function loadRecords() {
  const pending = recordsDS.run(() => reseedApi.records());
  const data = await pending;
  if (recordsDS.isStale(pending)) return;
  if (data) {
    records.value = data.items ?? [];
    recordsLoaded.value = true;
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
/* 一轮结束（或开始）时刷新轮次列表与抽屉 */
watch(
  () => current.value?.id,
  () => {
    if (tab.value === "runs") void loadRuns();
    void runDrawer.value?.reload();
  },
);

onMounted(async () => {
  await Promise.all([loadSettings(), loadDownloaders()]);
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
        data-testid="rs-tabs" />
      <template v-if="current" #note>
        <span data-testid="rs-progress"
          >{{ current.status === "paused" ? "已暂停" : "正在运行" }}：{{
            reseedRunProgress(current)
          }}</span
        >
      </template>
      <template v-else-if="settings?.running && !isMobile" #note>正在运行</template>
    </PtToolbar>

    <!-- 设置 -->
    <PtPanel v-if="tab === 'settings'" title="辅种设置" icon="share-2">
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
            <p class="rs-tip">
              只辅种文件完全一样（路径和大小逐个相同）的种子；核对没通过的不再尝试，下载不到种子的过
              7 天再试。
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
      :count="recordsLoaded ? records.length : undefined">
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
        :title="recordsDS.error.value ? '' : '还没有辅种记录'"
        :sub="recordsDS.errorText.value || '开启辅种并运行一轮后，这里列出每次尝试'" />
      <el-table
        v-else-if="!isMobile"
        :data="records"
        class="pt-grid"
        row-key="id"
        data-testid="rs-records">
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
      </el-table>
      <div v-else class="rs-cards">
        <PtRowCard v-for="r in records" :key="r.id">
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
        </PtRowCard>
      </div>
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

.rs-dialog-text {
  margin: 0 0 12px;
  font-size: 13px;
  line-height: 1.6;
  color: var(--pt-t2);
}

.rs-sites-tip {
  margin: -6px 0 16px;
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
