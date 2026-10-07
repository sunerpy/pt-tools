<script setup lang="ts">
/*
 * 转移做种（路线图 M5）：把下完的种子从一台下载器搬到另一台继续做种，数据不动。
 * 任务在「下载器 Web UI」或「任务列表」里勾选种子后点「转移到…」建立；这一页看进度、设路径映射和定时规则。
 * 画板没有这一页，沿用下载组列表页的样式：页头 + 带形工具栏 + 面板里的表格，手机是行卡。
 */
import {
  type DownloaderSetting,
  downloadersApi,
  type TransferJob,
  type TransferPathMap,
  type TransferRule,
  type TransferRuleConfig,
  transferApi,
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
import { formatBytes, formatShortDateTime } from "@/utils/format";
import {
  defaultTransferRule,
  transferCancelable,
  transferProgressText,
  transferRuleSummary,
  transferStateLabel,
  transferStateTone,
} from "@/utils/transfer";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";

type Tab = "jobs" | "maps" | "rules";
const TABS: { label: string; value: Tab }[] = [
  { label: "任务", value: "jobs" },
  { label: "路径映射", value: "maps" },
  { label: "定时规则", value: "rules" },
];
type JobFilter = "" | "active" | "finished";
const JOB_FILTERS: { label: string; value: JobFilter }[] = [
  { label: "全部", value: "" },
  { label: "进行中", value: "active" },
  { label: "已结束", value: "finished" },
];
/** 有进行中的任务时多久刷新一次 */
const ACTIVE_REFRESH_MS = 5000;

const isMobile = useIsMobile();
const tab = ref<Tab>("jobs");
const downloaders = ref<DownloaderSetting[]>([]);
const downloadersFailed = ref(false);

// ---- 任务 ----
const jobsDS = useDataState();
const jobs = ref<TransferJob[]>([]);
const jobFilter = ref<JobFilter>("");
const jobsLoaded = ref(false);
const hasActive = computed(() => jobs.value.some((j) => !j.final));
const finishedCount = computed(() => jobs.value.filter((j) => j.final).length);
let timer: ReturnType<typeof setInterval> | undefined;

// ---- 路径映射 ----
const mapSource = ref<number | undefined>(undefined);
const mapTarget = ref<number | undefined>(undefined);
const maps = ref<TransferPathMap[]>([]);
const mapsLoaded = ref(false);
const mapsLoading = ref(false);
const mapsSaving = ref(false);
const mapsError = ref("");

// ---- 定时规则 ----
const rulesDS = useDataState();
const rules = ref<TransferRule[]>([]);
const rulesLoaded = ref(false);
const ruleDialog = ref(false);
const ruleForm = ref<TransferRuleConfig>(defaultTransferRule());
const ruleEditing = ref<number | null>(null);
const ruleSaving = ref(false);
const running = ref<number | null>(null);

const headSub = computed(() => {
  if (downloadersFailed.value) return "下载器列表没读到，点刷新重试";
  if (downloaders.value.length < 2) return "至少要有两台启用的下载器才能转移做种";
  return "把下完的种子搬到另一台下载器继续做种，数据不动：校验到 100% 才从源下载器移除，不到 100% 就撤回";
});

function downloaderName(id: number): string {
  return downloaders.value.find((d) => d.id === id)?.name ?? `#${id}`;
}

async function loadDownloaders() {
  try {
    downloaders.value = ((await downloadersApi.list()) ?? []).filter((d) => d.enabled);
    downloadersFailed.value = false;
  } catch {
    downloadersFailed.value = true;
  }
  const ids = downloaders.value.map((d) => d.id);
  if (!mapSource.value || !ids.includes(mapSource.value)) mapSource.value = ids[0];
  if (!mapTarget.value || !ids.includes(mapTarget.value) || mapTarget.value === mapSource.value) {
    mapTarget.value = ids.find((id) => id !== mapSource.value);
  }
}

async function loadJobs() {
  const pending = jobsDS.run(() => transferApi.jobs(jobFilter.value));
  const data = await pending;
  if (jobsDS.isStale(pending)) return;
  if (data) {
    jobs.value = data.items ?? [];
    jobsLoaded.value = true;
  }
  scheduleRefresh();
}

/** 有进行中的任务时每 5 秒刷新；都结束了就停 */
function scheduleRefresh() {
  if (hasActive.value && tab.value === "jobs") {
    if (!timer) timer = setInterval(() => void loadJobs(), ACTIVE_REFRESH_MS);
  } else if (timer) {
    clearInterval(timer);
    timer = undefined;
  }
}

async function cancelJob(job: TransferJob) {
  try {
    await transferApi.cancel(job.id);
    ElMessage.success("已取消");
  } catch (e) {
    ElMessage.error((e as Error).message || "取消失败");
  }
  await loadJobs();
}

async function clearFinished() {
  try {
    await ElMessageBox.confirm(
      `清除 ${finishedCount.value} 个已结束的任务记录？只删记录，不动下载器。`,
      "清除已结束",
      {
        type: "warning",
        confirmButtonText: "清除",
        cancelButtonText: "取消",
      },
    );
  } catch {
    return;
  }
  try {
    const res = await transferApi.clearFinished();
    ElMessage.success(`已清除 ${res?.deleted ?? 0} 条`);
  } catch (e) {
    ElMessage.error((e as Error).message || "清除失败");
  }
  await loadJobs();
}

async function loadMaps() {
  const src = mapSource.value;
  const dst = mapTarget.value;
  mapsError.value = "";
  if (!src || !dst || src === dst) {
    maps.value = [];
    mapsLoaded.value = false;
    return;
  }
  mapsLoading.value = true;
  try {
    const res = await transferApi.pathMaps(src, dst);
    if (src !== mapSource.value || dst !== mapTarget.value) return;
    maps.value = (res?.items ?? []).map((m) => ({
      source_prefix: m.source_prefix,
      target_prefix: m.target_prefix,
    }));
    mapsLoaded.value = true;
  } catch (e) {
    mapsError.value = (e as Error).message || "路径映射没读到";
  } finally {
    mapsLoading.value = false;
  }
}

function addMap() {
  maps.value.push({ source_prefix: "", target_prefix: "" });
}

function removeMap(i: number) {
  maps.value.splice(i, 1);
}

async function saveMaps() {
  const src = mapSource.value;
  const dst = mapTarget.value;
  if (!src || !dst) return;
  mapsSaving.value = true;
  try {
    const rows = maps.value.filter((m) => m.source_prefix.trim() || m.target_prefix.trim());
    const res = await transferApi.savePathMaps(src, dst, rows);
    maps.value = (res?.items ?? []).map((m) => ({
      source_prefix: m.source_prefix,
      target_prefix: m.target_prefix,
    }));
    ElMessage.success("已保存");
  } catch (e) {
    ElMessage.error((e as Error).message || "保存失败");
  } finally {
    mapsSaving.value = false;
  }
}

async function loadRules() {
  const pending = rulesDS.run(() => transferApi.rules());
  const data = await pending;
  if (rulesDS.isStale(pending)) return;
  if (data) {
    rules.value = data.items ?? [];
    rulesLoaded.value = true;
  }
}

function openRule(rule?: TransferRule) {
  if (rule) {
    ruleEditing.value = rule.id;
    ruleForm.value = {
      name: rule.name,
      enabled: rule.enabled,
      source_downloader_id: rule.source_downloader_id,
      target_downloader_id: rule.target_downloader_id,
      category: rule.category,
      tag: rule.tag,
      site_name: rule.site_name,
      min_seeding_hours: rule.min_seeding_hours,
      max_per_run: rule.max_per_run,
      interval_min: rule.interval_min,
    };
  } else {
    ruleEditing.value = null;
    const f = defaultTransferRule();
    f.source_downloader_id = downloaders.value[0]?.id ?? 0;
    f.target_downloader_id = downloaders.value[1]?.id ?? 0;
    ruleForm.value = f;
  }
  ruleDialog.value = true;
}

async function saveRule() {
  ruleSaving.value = true;
  try {
    if (ruleEditing.value) await transferApi.updateRule(ruleEditing.value, ruleForm.value);
    else await transferApi.createRule(ruleForm.value);
    ruleDialog.value = false;
    ElMessage.success("已保存");
    await loadRules();
  } catch (e) {
    ElMessage.error((e as Error).message || "保存失败");
  } finally {
    ruleSaving.value = false;
  }
}

async function toggleRule(rule: TransferRule, enabled: boolean) {
  try {
    await transferApi.updateRule(rule.id, { ...rule, enabled });
  } catch (e) {
    ElMessage.error((e as Error).message || "保存失败");
  }
  await loadRules();
}

async function runRule(rule: TransferRule) {
  running.value = rule.id;
  try {
    const res = await transferApi.runRule(rule.id);
    ElMessage({
      type: res?.error ? "warning" : "success",
      message: res?.summary ?? "已运行",
      duration: 5000,
    });
    if (res?.result.created) await loadJobs();
  } catch (e) {
    ElMessage.error((e as Error).message || "运行失败");
  } finally {
    running.value = null;
  }
  await loadRules();
}

async function deleteRule(rule: TransferRule) {
  try {
    await ElMessageBox.confirm(`删除规则「${rule.name}」？它建过的任务记录保留。`, "删除规则", {
      type: "warning",
      confirmButtonText: "删除",
      cancelButtonText: "取消",
      confirmButtonClass: "el-button--danger",
    });
  } catch {
    return;
  }
  try {
    await transferApi.deleteRule(rule.id);
    ElMessage.success("已删除");
  } catch (e) {
    ElMessage.error((e as Error).message || "删除失败");
  }
  await loadRules();
}

function refresh() {
  void loadDownloaders();
  if (tab.value === "jobs") void loadJobs();
  else if (tab.value === "maps") void loadMaps();
  else void loadRules();
}

watch(tab, (t) => {
  if (t === "jobs") void loadJobs();
  else if (t === "maps") void loadMaps();
  else if (!rulesLoaded.value) void loadRules();
  scheduleRefresh();
});
watch(jobFilter, () => void loadJobs());
watch([mapSource, mapTarget], () => void loadMaps());

onMounted(async () => {
  await loadDownloaders();
  void loadJobs();
});
onBeforeUnmount(() => {
  if (timer) clearInterval(timer);
});
</script>

<template>
  <div class="tf-page">
    <PtHeadSub>{{ headSub }}</PtHeadSub>
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button data-testid="tf-refresh" @click="refresh">
        <PtIcon name="refresh-cw" :size="15" /><span>刷新</span>
      </el-button>
    </Teleport>

    <PtToolbar band>
      <el-segmented
        v-model="tab"
        class="pt-seg"
        :options="TABS"
        :props="{ label: 'label', value: 'value' }"
        data-testid="tf-tabs" />
    </PtToolbar>

    <!-- 任务 -->
    <PtPanel
      v-if="tab === 'jobs'"
      title="转移任务"
      icon="arrow-right-left"
      :count="jobsLoaded ? jobs.length : undefined">
      <div class="tf-bar">
        <el-segmented
          v-model="jobFilter"
          class="pt-seg"
          :options="JOB_FILTERS"
          :props="{ label: 'label', value: 'value' }"
          data-testid="tf-job-filter" />
        <el-button :disabled="!finishedCount" data-testid="tf-clear" @click="clearFinished">
          <PtIcon name="trash-2" :size="14" /><span>清除已结束</span>
        </el-button>
      </div>
      <p class="tf-tip">
        在「下载器 Web
        UI」或「任务列表」里勾选种子，点「转移到…」建任务。每台目标下载器同一时间只校验一个种子。
      </p>
      <PtDataState
        v-if="!jobs.length"
        :state="jobsDS.state.value"
        :title="jobsDS.error.value ? '' : '还没有转移任务'"
        :sub="jobsDS.errorText.value || '在「下载器 Web UI」里勾选种子，点「转移到…」'" />
      <el-table
        v-else-if="!isMobile"
        :data="jobs"
        class="pt-grid"
        row-key="id"
        data-testid="tf-jobs">
        <el-table-column label="种子" min-width="220" class-name="pt-cell-strong">
          <template #default="{ row }">
            <div class="tf-name">{{ row.name || row.info_hash }}</div>
            <div class="tf-sub">
              {{ formatBytes(row.total_size)
              }}<template v-if="row.site_name"> · {{ row.site_name }}</template>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="源 → 目标" min-width="200">
          <template #default="{ row }">
            <div>{{ row.source_name }} → {{ row.target_name }}</div>
            <div class="tf-path">{{ row.target_save_path || row.source_save_path }}</div>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="130">
          <template #default="{ row }">
            <PtStatusPill :tone="transferStateTone(row.state)" size="sm">{{
              transferStateLabel(row.state)
            }}</PtStatusPill>
            <span v-if="transferProgressText(row)" class="tf-sub tf-pct">{{
              transferProgressText(row)
            }}</span>
          </template>
        </el-table-column>
        <el-table-column
          label="说明"
          min-width="220"
          class-name="pt-cell-1line"
          show-overflow-tooltip>
          <template #default="{ row }">{{ row.message }}</template>
        </el-table-column>
        <el-table-column label="更新时间" width="120">
          <template #default="{ row }">{{ formatShortDateTime(row.updated_at) }}</template>
        </el-table-column>
        <el-table-column label="" width="80" align="right">
          <template #default="{ row }">
            <el-button v-if="transferCancelable(row)" link size="small" @click="cancelJob(row)"
              >取消</el-button
            >
          </template>
        </el-table-column>
      </el-table>
      <div v-else class="tf-cards">
        <PtRowCard v-for="j in jobs" :key="j.id">
          <template #title>{{ j.name || j.info_hash }}</template>
          <template #meta>
            <span>{{ j.source_name }} → {{ j.target_name }} · {{ formatBytes(j.total_size) }}</span>
            <span v-if="j.message" class="tf-full">{{ j.message }}</span>
          </template>
          <template #status>
            <PtStatusPill :tone="transferStateTone(j.state)" size="sm"
              >{{ transferStateLabel(j.state)
              }}{{ transferProgressText(j) ? ` ${transferProgressText(j)}` : "" }}</PtStatusPill
            >
          </template>
          <template v-if="transferCancelable(j)" #actions>
            <el-button link size="small" @click="cancelJob(j)">取消</el-button>
          </template>
        </PtRowCard>
      </div>
    </PtPanel>

    <!-- 路径映射 -->
    <PtPanel v-else-if="tab === 'maps'" title="路径映射" icon="folder-open">
      <p class="tf-tip">
        两台下载器看到的同一份数据路径不一样时（比如不同容器的挂载点），在这里写对应关系：源下载器的路径前缀
        → 目标下载器的路径前缀。转移时取最长的匹配前缀换算保存路径；没有匹配时用同一个路径。
      </p>
      <div class="tf-bar">
        <el-select
          v-model="mapSource"
          class="tf-dl"
          placeholder="源下载器"
          aria-label="源下载器"
          data-testid="tf-map-source">
          <el-option v-for="d in downloaders" :key="d.id" :label="d.name" :value="d.id" />
        </el-select>
        <PtIcon name="arrow-right" :size="14" class="tf-arrow" />
        <el-select
          v-model="mapTarget"
          class="tf-dl"
          placeholder="目标下载器"
          aria-label="目标下载器"
          data-testid="tf-map-target">
          <el-option
            v-for="d in downloaders"
            :key="d.id"
            :label="d.name"
            :value="d.id"
            :disabled="d.id === mapSource" />
        </el-select>
      </div>
      <div v-if="mapsError" class="pt-note pt-note--warn">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>{{ mapsError }}</span>
      </div>
      <div
        v-if="mapSource && mapTarget && mapSource !== mapTarget"
        v-loading="mapsLoading"
        class="tf-maps">
        <div v-for="(m, i) in maps" :key="i" class="tf-map">
          <el-input
            v-model="m.source_prefix"
            :placeholder="`${downloaderName(mapSource)} 里的路径，如 /downloads`"
            :aria-label="`第 ${i + 1} 条的源路径`" />
          <PtIcon name="arrow-right" :size="14" class="tf-arrow" />
          <el-input
            v-model="m.target_prefix"
            :placeholder="`${downloaderName(mapTarget)} 里的路径，如 /data`"
            :aria-label="`第 ${i + 1} 条的目标路径`" />
          <el-button link :aria-label="`删除第 ${i + 1} 条`" @click="removeMap(i)">
            <PtIcon name="x" :size="15" />
          </el-button>
        </div>
        <p v-if="mapsLoaded && !maps.length" class="tf-tip">
          还没有映射：两台下载器按同一个路径找数据。
        </p>
        <div class="tf-bar">
          <el-button data-testid="tf-map-add" @click="addMap">
            <PtIcon name="plus" :size="14" /><span>添加一条</span>
          </el-button>
          <el-button
            type="primary"
            :loading="mapsSaving"
            data-testid="tf-map-save"
            @click="saveMaps">
            <PtIcon v-if="!mapsSaving" name="save" :size="14" /><span>保存</span>
          </el-button>
        </div>
      </div>
    </PtPanel>

    <!-- 定时规则 -->
    <PtPanel
      v-else
      title="定时规则"
      icon="calendar-clock"
      :count="rulesLoaded ? rules.length : undefined">
      <div class="tf-bar">
        <el-button
          type="primary"
          :disabled="downloaders.length < 2"
          data-testid="tf-rule-new"
          @click="openRule()">
          <PtIcon name="plus" :size="14" /><span>新建规则</span>
        </el-button>
      </div>
      <p class="tf-tip">
        规则按间隔把源下载器里已下完、符合条件的种子建成转移任务（按添加时间从早到晚，每轮有上限）；刷流种子不参与。默认关闭。
      </p>
      <PtDataState
        v-if="!rules.length"
        :state="rulesDS.state.value"
        :title="rulesDS.error.value ? '' : '还没有定时规则'"
        :sub="
          rulesDS.errorText.value || '点「新建规则」，比如把做种满 30 天的种子转到 NAS 上的下载器'
        " />
      <el-table
        v-else-if="!isMobile"
        :data="rules"
        class="pt-grid"
        row-key="id"
        data-testid="tf-rules">
        <el-table-column label="开启" width="70">
          <template #default="{ row }">
            <el-switch
              :model-value="row.enabled"
              size="small"
              :aria-label="`开启 ${row.name}`"
              @change="(v: string | number | boolean) => toggleRule(row, Boolean(v))" />
          </template>
        </el-table-column>
        <el-table-column label="规则" min-width="200" class-name="pt-cell-strong">
          <template #default="{ row }">
            <div>{{ row.name }}</div>
            <div class="tf-sub">{{ row.source_name }} → {{ row.target_name }}</div>
          </template>
        </el-table-column>
        <el-table-column label="条件" min-width="200">
          <template #default="{ row }">
            <div>{{ transferRuleSummary(row) }}</div>
            <div class="tf-sub">
              每 {{ row.interval_min }} 分钟，每轮最多 {{ row.max_per_run }} 个
            </div>
          </template>
        </el-table-column>
        <el-table-column
          label="上次运行"
          min-width="220"
          class-name="pt-cell-1line"
          show-overflow-tooltip>
          <template #default="{ row }">
            <template v-if="row.last_run_at"
              >{{ formatShortDateTime(row.last_run_at) }} · {{ row.last_result }}</template
            >
            <span v-else class="tf-sub">还没运行</span>
          </template>
        </el-table-column>
        <el-table-column label="" width="190" align="right">
          <template #default="{ row }">
            <el-button link size="small" :loading="running === row.id" @click="runRule(row)"
              >立即运行</el-button
            >
            <el-button link size="small" @click="openRule(row)">编辑</el-button>
            <el-button link size="small" type="danger" @click="deleteRule(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div v-else class="tf-cards">
        <PtRowCard v-for="r in rules" :key="r.id">
          <template #title>{{ r.name }}</template>
          <template #meta>
            <span>{{ r.source_name }} → {{ r.target_name }}</span>
            <span class="tf-full">{{ transferRuleSummary(r) }}</span>
            <span v-if="r.last_run_at" class="tf-full"
              >{{ formatShortDateTime(r.last_run_at) }} · {{ r.last_result }}</span
            >
          </template>
          <template #status>
            <el-switch
              :model-value="r.enabled"
              size="small"
              :aria-label="`开启 ${r.name}`"
              @change="(v: string | number | boolean) => toggleRule(r, Boolean(v))" />
          </template>
          <template #actions>
            <el-button link size="small" :loading="running === r.id" @click="runRule(r)"
              >立即运行</el-button
            >
            <el-button link size="small" @click="openRule(r)">编辑</el-button>
            <el-button link size="small" type="danger" @click="deleteRule(r)">删除</el-button>
          </template>
        </PtRowCard>
      </div>
    </PtPanel>

    <el-dialog
      v-model="ruleDialog"
      class="pt-dialog"
      :title="ruleEditing ? '编辑规则' : '新建规则'"
      :width="isMobile ? '94%' : '560px'"
      append-to-body>
      <el-form class="pt-form" label-position="top" @submit.prevent>
        <el-form-item label="名称">
          <el-input v-model="ruleForm.name" maxlength="64" data-testid="tf-rule-name" />
        </el-form-item>
        <el-form-item label="开启">
          <el-switch v-model="ruleForm.enabled" data-testid="tf-rule-enabled" />
        </el-form-item>
        <div class="tf-row">
          <el-form-item label="源下载器">
            <el-select v-model="ruleForm.source_downloader_id" data-testid="tf-rule-source">
              <el-option v-for="d in downloaders" :key="d.id" :label="d.name" :value="d.id" />
            </el-select>
          </el-form-item>
          <el-form-item label="目标下载器">
            <el-select v-model="ruleForm.target_downloader_id" data-testid="tf-rule-target">
              <el-option
                v-for="d in downloaders"
                :key="d.id"
                :label="d.name"
                :value="d.id"
                :disabled="d.id === ruleForm.source_downloader_id" />
            </el-select>
          </el-form-item>
        </div>
        <div class="tf-row">
          <el-form-item label="分类等于">
            <el-input v-model="ruleForm.category" placeholder="不限" />
          </el-form-item>
          <el-form-item label="标签含有">
            <el-input v-model="ruleForm.tag" placeholder="不限" />
          </el-form-item>
        </div>
        <div class="tf-row">
          <el-form-item label="站点">
            <el-input v-model="ruleForm.site_name" placeholder="不限，填站点 ID 如 hdsky" />
          </el-form-item>
          <el-form-item label="做种至少（小时）">
            <el-input-number
              v-model="ruleForm.min_seeding_hours"
              :min="0"
              :max="87600"
              controls-position="right" />
          </el-form-item>
        </div>
        <div class="tf-row">
          <el-form-item label="每轮最多（个）">
            <el-input-number
              v-model="ruleForm.max_per_run"
              :min="1"
              :max="100"
              controls-position="right" />
          </el-form-item>
          <el-form-item label="间隔（分钟）">
            <el-input-number
              v-model="ruleForm.interval_min"
              :min="10"
              :max="10080"
              controls-position="right" />
          </el-form-item>
        </div>
      </el-form>
      <template #footer>
        <el-button @click="ruleDialog = false">取消</el-button>
        <el-button type="primary" :loading="ruleSaving" data-testid="tf-rule-save" @click="saveRule"
          >保存</el-button
        >
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.tf-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.tf-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}

.tf-tip {
  margin: 8px 0 12px;
  font-size: 12px;
  line-height: 1.6;
  color: var(--pt-t3);
}

.tf-dl {
  width: 220px;
}

.tf-arrow {
  color: var(--pt-t3);
}

.tf-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tf-sub {
  font-size: 12px;
  color: var(--pt-t3);
}

.tf-pct {
  margin-left: 6px;
}

.tf-path {
  font-family: var(--pt-font-mono);
  font-size: 12px;
  color: var(--pt-t3);
  word-break: break-all;
}

.tf-maps {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 12px;
}

.tf-map {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr) auto;
  gap: 8px;
  align-items: center;
}

.tf-cards {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.tf-full {
  flex-basis: 100%;
}

.tf-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 0 16px;
}
</style>
