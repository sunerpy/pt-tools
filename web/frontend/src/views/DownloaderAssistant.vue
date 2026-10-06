<script setup lang="ts">
/*
 * 下载器助手（路线图 M4）：失效种子、补站点标签、替换 tracker、失效种子定时扫描。
 * 画板没有这一页，沿用下载组列表页的样式：页头 + 带形工具栏 + 面板里的表格，手机是行卡。
 * 每项先扫描或预览，勾选后才执行；执行前后端还会逐个重新核对。
 */
import {
  type AssistantDeadTorrent,
  type AssistantSiteTag,
  type AssistantTrackerMatch,
  chatopsApi,
  type DeadTorrentScanSettings,
  type DownloaderSetting,
  downloaderAssistantApi,
  downloadersApi,
  type NotificationConfig,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtTag from "@/components/ui/PtTag.vue";
import PtToolbar from "@/components/ui/PtToolbar.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { applySummary, applyTone, deadReasonLabel } from "@/utils/downloaderAssistant";
import { formatBytes } from "@/utils/format";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onMounted, ref, watch } from "vue";

type Tab = "dead" | "tags" | "trackers" | "scan";

const TABS: { label: string; value: Tab }[] = [
  { label: "失效种子", value: "dead" },
  { label: "补站点标签", value: "tags" },
  { label: "替换 Tracker", value: "trackers" },
  { label: "定时扫描", value: "scan" },
];

const isMobile = useIsMobile();
const tab = ref<Tab>("dead");

const downloaders = ref<DownloaderSetting[]>([]);
const downloaderId = ref<number | undefined>(undefined);
const downloadersFailed = ref(false);
const currentDownloader = computed(() =>
  downloaders.value.find((d) => d.id === downloaderId.value),
);

// ---- 失效种子 ----
const deadDS = useDataState();
const dead = ref<AssistantDeadTorrent[]>([]);
const deadScanned = ref(false);
const deadSel = ref<string[]>([]);
const removeData = ref(false);
const deleting = ref(false);
const deadSize = computed(() =>
  dead.value.filter((d) => deadSel.value.includes(d.hash)).reduce((s, d) => s + d.size, 0),
);

// ---- 补站点标签 ----
const tagDS = useDataState();
const tags = ref<AssistantSiteTag[]>([]);
const tagsScanned = ref(false);
const tagSel = ref<string[]>([]);
const tagging = ref(false);

// ---- 替换 tracker ----
const trDS = useDataState();
const trFrom = ref("");
const trTo = ref("");
const previewed = ref<{ from: string; to: string } | null>(null);
const matches = ref<AssistantTrackerMatch[]>([]);
const trSupported = ref(true);
const trSel = ref<string[]>([]);
const replacing = ref(false);
const trChanged = computed(
  () =>
    previewed.value !== null &&
    (previewed.value.from !== trFrom.value || previewed.value.to !== trTo.value),
);
const matchHashes = computed(() => [...new Set(matches.value.map((m) => m.hash))]);

// ---- 定时扫描 ----
const scanForm = ref<DeadTorrentScanSettings>({
  enabled: false,
  interval_hours: 24,
  channel_ids: [],
});
const channels = ref<NotificationConfig[]>([]);
const scanLoadFailed = ref(false);
const scanSaving = ref(false);

const headSub = computed(() => {
  if (downloadersFailed.value) return "下载器列表没读到，点刷新重试";
  if (!downloaders.value.length) return "还没有启用的下载器，先在「下载器设置」里添加";
  return "找出站点已删除的失效种子、按 tracker 补站点标签、批量替换 tracker 地址；都是先预览、勾选确认后才执行";
});

async function loadDownloaders() {
  try {
    const list = await downloadersApi.list();
    downloaders.value = (list ?? []).filter((d) => d.enabled);
    downloadersFailed.value = false;
    if (!downloaders.value.some((d) => d.id === downloaderId.value)) {
      downloaderId.value = downloaders.value[0]?.id;
    }
  } catch {
    downloadersFailed.value = true;
  }
}

async function loadScanSettings() {
  const [cfg, list] = await Promise.all([
    downloaderAssistantApi.getDeadScan().catch(() => null),
    chatopsApi.notifications.list().catch(() => [] as NotificationConfig[]),
  ]);
  channels.value = (list ?? []).filter((c) => c.enabled);
  if (cfg) {
    scanForm.value = { ...cfg, channel_ids: [...(cfg.channel_ids ?? [])] };
    scanLoadFailed.value = false;
  } else {
    scanLoadFailed.value = true;
  }
}

function resetResults() {
  dead.value = [];
  deadScanned.value = false;
  deadSel.value = [];
  tags.value = [];
  tagsScanned.value = false;
  tagSel.value = [];
  matches.value = [];
  previewed.value = null;
  trSel.value = [];
}

watch(downloaderId, resetResults);

async function scanDead() {
  const id = downloaderId.value;
  if (!id) return;
  const pending = deadDS.run(() => downloaderAssistantApi.dead(id));
  const data = await pending;
  if (deadDS.isStale(pending) || id !== downloaderId.value) return;
  dead.value = data?.items ?? [];
  deadScanned.value = data !== null;
  deadSel.value = [];
}

async function deleteDead() {
  const id = downloaderId.value;
  if (!id || !deadSel.value.length) return;
  const n = deadSel.value.length;
  try {
    await ElMessageBox.confirm(
      removeData.value
        ? `删除选中的 ${n} 个种子，并删除它们的数据文件（共 ${formatBytes(deadSize.value)}）？删除后无法恢复。`
        : `从下载器里删除选中的 ${n} 个种子？数据文件保留在磁盘上。`,
      "删除失效种子",
      { type: "warning", confirmButtonText: "删除", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  deleting.value = true;
  try {
    const res = await downloaderAssistantApi.deleteDead(id, deadSel.value, removeData.value);
    ElMessage({ type: applyTone(res), message: applySummary("删除", res), duration: 5000 });
    await scanDead();
  } catch (e) {
    ElMessage.error((e as Error).message || "删除失败");
  } finally {
    deleting.value = false;
  }
}

async function scanTags() {
  const id = downloaderId.value;
  if (!id) return;
  const pending = tagDS.run(() => downloaderAssistantApi.siteTags(id));
  const data = await pending;
  if (tagDS.isStale(pending) || id !== downloaderId.value) return;
  tags.value = data?.items ?? [];
  tagsScanned.value = data !== null;
  tagSel.value = [];
}

async function applyTags() {
  const id = downloaderId.value;
  if (!id || !tagSel.value.length) return;
  const items = tags.value
    .filter((t) => tagSel.value.includes(t.hash))
    .map((t) => ({ hash: t.hash, site: t.site }));
  tagging.value = true;
  try {
    const res = await downloaderAssistantApi.applySiteTags(id, items);
    ElMessage({ type: applyTone(res), message: applySummary("补标签", res), duration: 5000 });
    await scanTags();
  } catch (e) {
    ElMessage.error((e as Error).message || "补标签失败");
  } finally {
    tagging.value = false;
  }
}

async function previewTrackers() {
  const id = downloaderId.value;
  if (!id) return;
  const from = trFrom.value;
  const to = trTo.value;
  const pending = trDS.run(() => downloaderAssistantApi.previewTrackers(id, from, to));
  const data = await pending;
  if (trDS.isStale(pending) || id !== downloaderId.value) return;
  matches.value = data?.items ?? [];
  trSupported.value = data?.supported ?? true;
  previewed.value = data ? { from, to } : null;
  trSel.value = [];
}

async function applyTrackers() {
  const id = downloaderId.value;
  const p = previewed.value;
  if (!id || !p || trChanged.value || !trSel.value.length) return;
  try {
    await ElMessageBox.confirm(
      `把选中的 ${trSel.value.length} 个种子的 tracker 地址里的「${p.from}」换成「${p.to}」？`,
      "替换 tracker",
      { type: "warning", confirmButtonText: "替换", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  replacing.value = true;
  try {
    const res = await downloaderAssistantApi.applyTrackers(id, p.from, p.to, trSel.value);
    ElMessage({ type: applyTone(res), message: applySummary("替换", res), duration: 5000 });
    await previewTrackers();
  } catch (e) {
    ElMessage.error((e as Error).message || "替换失败");
  } finally {
    replacing.value = false;
  }
}

async function saveScan() {
  if (scanForm.value.enabled && scanForm.value.channel_ids.length === 0) {
    ElMessage.warning("开启定时扫描至少要选一个通知通道");
    return;
  }
  scanSaving.value = true;
  try {
    const saved = await downloaderAssistantApi.saveDeadScan(scanForm.value);
    scanForm.value = { ...saved, channel_ids: [...(saved.channel_ids ?? [])] };
    ElMessage.success("已保存");
  } catch (e) {
    ElMessage.error((e as Error).message || "保存失败");
  } finally {
    scanSaving.value = false;
  }
}

function refresh() {
  if (tab.value === "dead" && deadScanned.value) scanDead();
  else if (tab.value === "tags" && tagsScanned.value) scanTags();
  else if (tab.value === "trackers" && previewed.value && !trChanged.value) previewTrackers();
  else if (tab.value === "scan") loadScanSettings();
  else loadDownloaders();
}

function toggle(list: string[], hash: string, on: boolean) {
  const i = list.indexOf(hash);
  if (on && i < 0) list.push(hash);
  if (!on && i >= 0) list.splice(i, 1);
}

onMounted(() => {
  loadDownloaders();
  loadScanSettings();
});
</script>

<template>
  <div class="da-page">
    <PtHeadSub>{{ headSub }}</PtHeadSub>
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-select
        v-model="downloaderId"
        class="da-dl"
        placeholder="选择下载器"
        aria-label="下载器"
        data-testid="da-downloader">
        <el-option v-for="d in downloaders" :key="d.id" :label="d.name" :value="d.id" />
      </el-select>
      <el-button @click="refresh">
        <PtIcon name="refresh-cw" :size="15" /><span>刷新</span>
      </el-button>
    </Teleport>

    <PtToolbar band>
      <el-segmented
        v-model="tab"
        class="pt-seg"
        :options="TABS"
        :props="{ label: 'label', value: 'value' }"
        data-testid="da-tabs" />
      <template v-if="currentDownloader" #note>{{ currentDownloader.name }}</template>
    </PtToolbar>

    <!-- 失效种子 -->
    <PtPanel
      v-if="tab === 'dead'"
      title="失效种子"
      icon="circle-x"
      :count="deadScanned ? dead.length : undefined">
      <div class="da-bar">
        <el-button
          type="primary"
          :loading="deadDS.loading.value"
          :disabled="!downloaderId"
          data-testid="da-dead-scan"
          @click="scanDead">
          <PtIcon v-if="!deadDS.loading.value" name="scan" :size="14" /><span>扫描</span>
        </el-button>
        <el-checkbox v-model="removeData" data-testid="da-dead-remove-data"
          >同时删除数据文件</el-checkbox
        >
        <el-button
          type="danger"
          plain
          :loading="deleting"
          :disabled="!deadSel.length"
          data-testid="da-dead-delete"
          @click="deleteDead">
          <PtIcon v-if="!deleting" name="trash-2" :size="14" /><span
            >删除选中{{ deadSel.length ? ` ${deadSel.length} 个` : "" }}</span
          >
        </el-button>
      </div>
      <p class="da-tip">
        tracker 报告「未注册」或「种子不存在」、并且没有任何 tracker 在正常工作的种子。提到 passkey
        或账号的报错不算失效。删除前会逐个重新确认，已经恢复正常的不删。
      </p>

      <PtDataState
        v-if="!dead.length"
        :state="deadDS.state.value"
        :title="deadScanned || deadDS.error.value ? '' : '还没扫描'"
        :sub="
          deadDS.errorText.value ||
          (deadScanned ? '没有失效种子' : '选好下载器后点「扫描」；种子多时要几十秒')
        " />
      <el-table
        v-else-if="!isMobile"
        :data="dead"
        row-key="hash"
        class="pt-grid"
        @selection-change="(rows: AssistantDeadTorrent[]) => (deadSel = rows.map((r) => r.hash))">
        <el-table-column type="selection" width="44" />
        <el-table-column label="种子" min-width="260" class-name="pt-cell-strong">
          <template #default="{ row }">
            <div class="da-name">{{ row.name }}</div>
            <div class="da-sub">{{ row.site || "未识别的站点" }} · {{ row.tracker_host }}</div>
          </template>
        </el-table-column>
        <el-table-column
          label="体积"
          width="110"
          class-name="pt-cell-num"
          label-class-name="pt-cell-num">
          <template #default="{ row }">{{ formatBytes(row.size) }}</template>
        </el-table-column>
        <el-table-column label="原因" width="96">
          <template #default="{ row }">
            <PtTag>{{ deadReasonLabel(row.reason) }}</PtTag>
          </template>
        </el-table-column>
        <el-table-column label="tracker 消息" min-width="200" class-name="pt-cell-1line">
          <template #default="{ row }">{{ row.message }}</template>
        </el-table-column>
      </el-table>
      <div v-else class="da-cards">
        <PtRowCard v-for="d in dead" :key="d.hash">
          <template #lead>
            <el-checkbox
              :model-value="deadSel.includes(d.hash)"
              :aria-label="`选择 ${d.name}`"
              @update:model-value="
                (v: string | number | boolean) => toggle(deadSel, d.hash, Boolean(v))
              " />
          </template>
          <template #title>{{ d.name }}</template>
          <template #meta>
            <PtTag>{{ deadReasonLabel(d.reason) }}</PtTag>
            <span>{{ d.site || d.tracker_host }}</span>
            <span>{{ formatBytes(d.size) }}</span>
            <span class="da-row-full">{{ d.message }}</span>
          </template>
        </PtRowCard>
      </div>
    </PtPanel>

    <!-- 补站点标签 -->
    <PtPanel
      v-else-if="tab === 'tags'"
      title="补站点标签"
      icon="tags"
      :count="tagsScanned ? tags.length : undefined">
      <div class="da-bar">
        <el-button
          type="primary"
          :loading="tagDS.loading.value"
          :disabled="!downloaderId"
          data-testid="da-tags-scan"
          @click="scanTags">
          <PtIcon v-if="!tagDS.loading.value" name="scan" :size="14" /><span>扫描</span>
        </el-button>
        <el-button
          :loading="tagging"
          :disabled="!tagSel.length"
          data-testid="da-tags-apply"
          @click="applyTags">
          <PtIcon v-if="!tagging" name="tag" :size="14" /><span
            >给选中的{{ tagSel.length ? ` ${tagSel.length} 个` : "" }}打标签</span
          >
        </el-button>
      </div>
      <p class="da-tip">
        按 tracker
        认出站点、但分类和标签里都没有站点名的种子。打上站点标签后，站点做种容量、清理范围和下载器
        Web UI 的站点筛选都能认出它们；原有标签保留。
      </p>
      <PtDataState
        v-if="!tags.length"
        :state="tagDS.state.value"
        :title="tagsScanned || tagDS.error.value ? '' : '还没扫描'"
        :sub="
          tagDS.errorText.value ||
          (tagsScanned ? '所有能认出站点的种子都已经有站点标签' : '选好下载器后点「扫描」')
        " />
      <el-table
        v-else-if="!isMobile"
        :data="tags"
        row-key="hash"
        class="pt-grid"
        @selection-change="(rows: AssistantSiteTag[]) => (tagSel = rows.map((r) => r.hash))">
        <el-table-column type="selection" width="44" />
        <el-table-column label="种子" min-width="260" class-name="pt-cell-strong">
          <template #default="{ row }">
            <div class="da-name">{{ row.name }}</div>
            <div class="da-sub">{{ row.tracker_host }}</div>
          </template>
        </el-table-column>
        <el-table-column label="站点" width="140">
          <template #default="{ row }">
            <PtTag>{{ row.site_name }}</PtTag>
          </template>
        </el-table-column>
        <el-table-column label="现有标签 / 分类" min-width="180" class-name="pt-cell-1line">
          <template #default="{ row }">{{
            [row.tags, row.category].filter(Boolean).join(" · ") || "无"
          }}</template>
        </el-table-column>
      </el-table>
      <div v-else class="da-cards">
        <PtRowCard v-for="t in tags" :key="t.hash">
          <template #lead>
            <el-checkbox
              :model-value="tagSel.includes(t.hash)"
              :aria-label="`选择 ${t.name}`"
              @update:model-value="
                (v: string | number | boolean) => toggle(tagSel, t.hash, Boolean(v))
              " />
          </template>
          <template #title>{{ t.name }}</template>
          <template #meta>
            <PtTag>{{ t.site_name }}</PtTag>
            <span>{{ t.tracker_host }}</span>
            <span>{{ [t.tags, t.category].filter(Boolean).join(" · ") || "无标签" }}</span>
          </template>
        </PtRowCard>
      </div>
    </PtPanel>

    <!-- 替换 tracker -->
    <PtPanel
      v-else-if="tab === 'trackers'"
      title="替换 Tracker"
      icon="link"
      :count="previewed ? matches.length : undefined">
      <div class="da-bar da-bar--form">
        <el-input
          v-model="trFrom"
          class="da-input"
          placeholder="原内容，例如 old.tracker.com"
          aria-label="要替换的内容"
          data-testid="da-tr-from" />
        <PtIcon name="arrow-right" :size="14" class="da-arrow" />
        <el-input
          v-model="trTo"
          class="da-input"
          placeholder="替换成，例如 new.tracker.com"
          aria-label="替换成"
          data-testid="da-tr-to" />
        <el-button
          type="primary"
          :loading="trDS.loading.value"
          :disabled="!downloaderId || trFrom.trim().length < 3 || !trTo.trim()"
          data-testid="da-tr-preview"
          @click="previewTrackers">
          <PtIcon v-if="!trDS.loading.value" name="scan" :size="14" /><span>预览</span>
        </el-button>
        <el-button
          :loading="replacing"
          :disabled="!trSel.length || trChanged || !trSupported"
          data-testid="da-tr-apply"
          @click="applyTrackers">
          <span>替换选中{{ trSel.length ? ` ${trSel.length} 个` : "" }}</span>
        </el-button>
      </div>
      <p class="da-tip">
        把 tracker 地址里第一处「原内容」换成「替换成」，常用于站点换域名或重置 passkey。原内容至少
        3 个字；预览里的 passkey 已遮住，替换时用的是下载器里的完整地址。
      </p>
      <div v-if="previewed && !trSupported" class="pt-note pt-note--warn">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>这台下载器不支持修改 tracker，只能预览。</span>
      </div>
      <div v-if="trChanged" class="pt-note">
        <PtIcon name="info" :size="14" class="pt-note__icon" />
        <span>改了替换内容，请重新预览后再执行。</span>
      </div>
      <PtDataState
        v-if="!matches.length"
        :state="trDS.state.value"
        :title="previewed || trDS.error.value ? '' : '还没预览'"
        :sub="
          trDS.errorText.value ||
          (previewed ? 'tracker 地址里没有这段内容' : '填好原内容和替换成，点「预览」')
        " />
      <el-table
        v-else-if="!isMobile"
        :data="matches"
        class="pt-grid"
        @selection-change="
          (rows: AssistantTrackerMatch[]) => (trSel = [...new Set(rows.map((r) => r.hash))])
        ">
        <el-table-column type="selection" width="44" />
        <el-table-column label="种子" min-width="200" class-name="pt-cell-strong">
          <template #default="{ row }">{{ row.name }}</template>
        </el-table-column>
        <el-table-column label="原地址 → 新地址" min-width="360">
          <template #default="{ row }">
            <div class="da-url">{{ row.old }}</div>
            <div class="da-url da-url--new">{{ row.new }}</div>
          </template>
        </el-table-column>
      </el-table>
      <div v-else class="da-cards">
        <PtRowCard v-for="(m, i) in matches" :key="`${m.hash}-${i}`">
          <template #lead>
            <el-checkbox
              :model-value="trSel.includes(m.hash)"
              :aria-label="`选择 ${m.name}`"
              @update:model-value="
                (v: string | number | boolean) => toggle(trSel, m.hash, Boolean(v))
              " />
          </template>
          <template #title>{{ m.name }}</template>
          <template #meta>
            <span class="da-url da-row-full">{{ m.old }}</span>
            <span class="da-url da-url--new da-row-full">{{ m.new }}</span>
          </template>
        </PtRowCard>
      </div>
      <p v-if="matches.length" class="da-foot">
        {{ matchHashes.length }} 个种子、{{ matches.length }} 个地址匹配。
      </p>
    </PtPanel>

    <!-- 定时扫描 -->
    <PtPanel v-else title="失效种子定时扫描" icon="calendar-clock">
      <div v-if="scanLoadFailed" class="pt-note pt-note--warn">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>设置没读到，点刷新重试。</span>
      </div>
      <el-form label-position="top" class="da-form" @submit.prevent>
        <el-form-item label="开启定时扫描">
          <el-switch
            v-model="scanForm.enabled"
            :disabled="scanLoadFailed"
            data-testid="da-scan-enabled" />
          <div class="da-field-tip">
            按间隔扫描所有启用的下载器，发现新的失效种子时发一条通知。只通知，不会自动删种；同一批种子只通知一次。
          </div>
        </el-form-item>
        <div class="da-field-row">
          <el-form-item label="扫描间隔（小时）">
            <el-input-number
              v-model="scanForm.interval_hours"
              :min="6"
              :max="168"
              :disabled="scanLoadFailed"
              data-testid="da-scan-interval" />
            <div class="da-field-tip">6–168 小时；种子多时一轮要读每个种子的 tracker 状态</div>
          </el-form-item>
          <el-form-item label="接收通道">
            <el-select
              v-model="scanForm.channel_ids"
              multiple
              collapse-tags
              placeholder="选择通知通道"
              :disabled="scanLoadFailed"
              data-testid="da-scan-channels">
              <el-option v-for="c in channels" :key="c.id" :label="c.name" :value="c.id" />
            </el-select>
            <div class="da-field-tip">开启时至少选一个；没有通道时先去「消息通知」里添加</div>
          </el-form-item>
        </div>
        <el-button
          type="primary"
          :loading="scanSaving"
          :disabled="scanLoadFailed"
          data-testid="da-scan-save"
          @click="saveScan">
          <PtIcon v-if="!scanSaving" name="save" :size="14" /><span>保存</span>
        </el-button>
      </el-form>
    </PtPanel>
  </div>
</template>

<style scoped>
.da-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.da-dl {
  width: 180px;
}

.da-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 12px;
  align-items: center;
}

.da-bar--form .da-input {
  flex: 1 1 200px;
  max-width: 320px;
}

.da-arrow {
  color: var(--pt-t3);
}

.da-tip,
.da-foot,
.da-field-tip {
  margin: 8px 0 12px;
  font-size: 12px;
  line-height: 1.6;
  color: var(--pt-t3);
}

.da-field-tip {
  margin: 4px 0 0;
}

.da-field-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 0 16px;
}

.da-form {
  max-width: 640px;
}

.da-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.da-sub {
  font-size: 12px;
  color: var(--pt-t3);
}

.da-url {
  font-family: var(--pt-font-mono);
  font-size: 12px;
  color: var(--pt-t2);
  word-break: break-all;
}

.da-url--new {
  color: var(--pt-t1);
}

.da-cards {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.da-row-full {
  flex-basis: 100%;
}

@media (max-width: 768px) {
  .da-dl {
    width: 100%;
  }

  .da-bar--form .da-input {
    max-width: none;
  }
}
</style>
