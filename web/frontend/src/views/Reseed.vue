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
  type ReseedSettings,
  type ReseedSiteMapItem,
  reseedApi,
} from "@/api";
import PtIcon from "@/components/PtIcon";
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
  reseedRecordActive,
  reseedRecordMessage,
  reseedRecordResult,
  reseedSiteStatus,
} from "@/utils/reseed";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";

type Tab = "settings" | "sites" | "records";
const TABS: { label: string; value: Tab }[] = [
  { label: "设置", value: "settings" },
  { label: "站点", value: "sites" },
  { label: "记录", value: "records" },
];
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
  site_names: [] as string[],
  max_per_site_per_day: 20,
});
const loadFailed = ref(false);
const saving = ref(false);
const starting = ref(false);

// ---- 站点 ----
const sitesDS = useDataState();
const sites = ref<ReseedSiteMapItem[]>([]);
const sitesLoaded = ref(false);
const configuredSites = computed(() => sites.value.filter((s) => s.configured));

// ---- 记录 ----
const recordsDS = useDataState();
const records = ref<ReseedRecord[]>([]);
const recordsLoaded = ref(false);

let timer: ReturnType<typeof setInterval> | undefined;
const busy = computed(
  () => Boolean(settings.value?.running) || records.value.some((r) => reseedRecordActive(r)),
);

const headSub = computed(() => {
  if (loadFailed.value) return "设置没读到，点刷新重试";
  if (!settings.value?.has_token) return "填写 IYUU token 后才能辅种";
  return "用 IYUU 找出其他站点上数据相同的种子，暂停加入同一台下载器，校验到 100% 才开始做种";
});

function fill(s: ReseedSettings) {
  settings.value = s;
  form.value = {
    enabled: s.enabled,
    token: "",
    interval_hours: s.interval_hours,
    downloader_ids: [...(s.downloader_ids ?? [])],
    site_names: [...(s.site_names ?? [])],
    max_per_site_per_day: s.max_per_site_per_day,
  };
}

async function loadSettings() {
  try {
    const s = await reseedApi.settings();
    if (s) fill(s);
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

/** 正在运行或有任务没结束时每 5 秒刷新；都停了就不刷 */
function scheduleRefresh() {
  if (busy.value) {
    if (!timer)
      timer = setInterval(() => {
        void loadSettings();
        void loadRecords();
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
      site_names: form.value.site_names,
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
    ElMessage.success("已开始，这一轮的结果会写在「上次运行」里");
    await loadSettings();
    scheduleRefresh();
  } catch (e) {
    ElMessage.error((e as Error).message || "运行失败");
  } finally {
    starting.value = false;
  }
}

function refresh() {
  void loadSettings();
  void loadDownloaders();
  if (tab.value === "sites") void loadSites();
  if (tab.value === "records") void loadRecords();
}

watch(tab, (t) => {
  if (t === "sites" && !sitesLoaded.value) void loadSites();
  if (t === "records") void loadRecords();
});
watch(busy, () => scheduleRefresh());

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
  <div class="rs-page">
    <PtHeadSub>{{ headSub }}</PtHeadSub>
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button data-testid="rs-refresh" @click="refresh">
        <PtIcon name="refresh-cw" :size="15" /><span>刷新</span>
      </el-button>
      <el-button
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
      <template v-if="settings?.running && !isMobile" #note>正在运行</template>
    </PtToolbar>

    <!-- 设置 -->
    <PtPanel v-if="tab === 'settings'" title="辅种设置" icon="share-2">
      <div v-if="loadFailed" class="pt-note pt-note--warn">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>设置没读到，点刷新重试。</span>
      </div>
      <el-form class="pt-form rs-form" label-position="top" @submit.prevent>
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
                settings?.has_token ? '已设置；要更换时填写新的' : '在 IYUU 的微信公众号里获取'
              "
              data-testid="rs-token" />
            <el-button v-if="settings?.has_token" data-testid="rs-clear-token" @click="clearToken"
              >清除</el-button
            >
          </div>
        </el-form-item>
        <div class="rs-row">
          <el-form-item label="运行间隔（小时）">
            <el-input-number
              v-model="form.interval_hours"
              :min="1"
              :max="168"
              controls-position="right"
              data-testid="rs-interval" />
          </el-form-item>
          <el-form-item label="每站每天最多（个）">
            <el-input-number
              v-model="form.max_per_site_per_day"
              :min="1"
              :max="500"
              controls-position="right"
              data-testid="rs-max" />
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
        <el-form-item label="参与的站点">
          <el-select
            v-model="form.site_names"
            multiple
            clearable
            :placeholder="
              sitesLoaded ? '全部已配置、IYUU 也支持的站点' : '填写 token 后在「站点」里查看'
            "
            data-testid="rs-sites">
            <el-option
              v-for="s in configuredSites"
              :key="s.site_name"
              :label="`${s.nickname || s.iyuu_site}（${s.site_name}）`"
              :value="s.site_name!" />
          </el-select>
        </el-form-item>
        <el-button type="primary" :loading="saving" data-testid="rs-save" @click="save()">
          <PtIcon v-if="!saving" name="save" :size="14" /><span>保存</span>
        </el-button>
      </el-form>
      <p class="rs-tip">
        只辅种文件完全一样（路径和大小逐个相同）的种子；同一站点同一种子只尝试一次。
        <template v-if="settings?.last_run_at">
          上次运行 {{ formatShortDateTime(settings.last_run_at) }}：{{ settings.last_result }}
        </template>
      </p>
    </PtPanel>

    <!-- 站点 -->
    <PtPanel
      v-else-if="tab === 'sites'"
      title="IYUU 站点"
      icon="globe"
      :count="sitesLoaded ? sites.length : undefined">
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
        <el-table-column label="状态" width="130">
          <template #default="{ row }">
            <PtStatusPill :tone="reseedSiteStatus(row).tone" size="sm">{{
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
            <PtStatusPill :tone="reseedSiteStatus(s).tone" size="sm">{{
              reseedSiteStatus(s).label
            }}</PtStatusPill>
          </template>
        </PtRowCard>
      </div>
    </PtPanel>

    <!-- 记录 -->
    <PtPanel
      v-else
      title="辅种记录"
      icon="list-checks"
      :count="recordsLoaded ? records.length : undefined">
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
          <template #default="{ row }">{{ formatShortDateTime(row.created_at) }}</template>
        </el-table-column>
      </el-table>
      <div v-else class="rs-cards">
        <PtRowCard v-for="r in records" :key="r.id">
          <template #title>{{ r.name || r.info_hash }}</template>
          <template #meta>
            <span
              >{{ r.site_name }} · {{ r.torrent_id }} ·
              {{ formatShortDateTime(r.created_at) }}</span
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
  </div>
</template>

<style scoped>
.rs-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.rs-form {
  max-width: 640px;
}

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

.rs-tip {
  margin: 12px 0 0;
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
</style>
