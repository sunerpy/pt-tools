<script setup lang="ts">
import type { TorrentDetailResponse, TorrentFileInfo, TorrentTrackerInfo } from "@/api";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtProgress from "@/components/ui/PtProgress.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useIsMobile } from "@/composables/useIsMobile";
import { formatBytes } from "@/utils/format";
import { computed } from "vue";

/**
 * 任务详情正文 —— 下方内联面板和右侧抽屉渲染的是同一份内容，
 * 原先在 DownloaderHub 里抄了两遍，改一处就得记得改另一处，所以抽成组件。
 *
 * 文件 / Tracker 的切换用 el-segmented（`.pt-seg`）而不是 el-tabs：
 * 抽屉和面板自己已经有一层标题，el-tabs 会再叠一条 48 高的页头。
 */
type Tone = "ok" | "warn" | "dang" | "info" | "primary" | "neutral";

const props = defineProps<{
  detail: TorrentDetailResponse | null;
  loading?: boolean;
  /** 两处共用同一个当前分页，所以由调用方持有 */
  tab: string;
}>();

const emit = defineEmits<{ "update:tab": [value: string] }>();

/**
 * 文件表和 Tracker 表在手机上都要降级成行卡（设计文档 §9）：
 * 两张表最窄的一张也要 700+ 宽，横着滚看不到列头，还和页面纵向滚动打架。
 *
 * 六态不在这里接：详情数据由 DownloaderHub 请求后传进来，这个组件不发请求，
 * loading / 失败都是父页面的状态。这里只处理「传进来的数组是空的」。
 */
const isMobile = useIsMobile();

const tabOptions = computed(() => [
  { label: `文件 (${props.detail?.files.length ?? 0})`, value: "files" },
  { label: `Tracker (${props.detail?.trackers.length ?? 0})`, value: "trackers" },
]);

const activeTab = computed({
  get: () => props.tab,
  set: (value: string) => emit("update:tab", value),
});

function filePercent(file: TorrentFileInfo): number {
  return Math.round((file.progress || 0) * 100);
}

/**
 * 下完的文件给 ok，其余走主色 —— 单个文件下不满不算异常，不用 warn/dang。
 *
 * 返回类型写成 PtProgress 的 tone 而不是本文件的 Tone：Tone 多一个 neutral，
 * 而 PtProgress 不收 neutral，用宽类型会编译不过。
 */
function fileTone(file: TorrentFileInfo): "ok" | "primary" {
  return filePercent(file) >= 100 ? "ok" : "primary";
}

/**
 * 优先级在桌面表里是原始数字，手机上一行只放得下几个词，数字换成词更值。
 * 取值由 downloader 层统一成 qBittorrent 的刻度（见 downloader.TorrentFile.Priority：
 * 0=不下载 1=普通 6=高 7=最高），Transmission 也归一化到同一套。
 */
const FILE_PRIORITY: Record<number, string> = {
  0: "不下载",
  1: "普通",
  6: "高",
  7: "最高",
};

function filePriorityText(file: TorrentFileInfo): string {
  return FILE_PRIORITY[file.priority] ?? `优先级 ${file.priority}`;
}

/**
 * Tracker 状态同理：接口给的是 downloader.TorrentTracker.Status 的数字
 * （0=禁用 1=未联系 2=工作中 3=更新中 4=出错），行卡上直接摆个「4」没人看得懂。
 */
const TRACKER_STATUS: Record<number, { text: string; tone: Tone }> = {
  0: { text: "已禁用", tone: "neutral" },
  1: { text: "未联系", tone: "info" },
  2: { text: "工作中", tone: "ok" },
  3: { text: "更新中", tone: "info" },
  4: { text: "出错", tone: "dang" },
};

function trackerStatus(tracker: TorrentTrackerInfo): { text: string; tone: Tone } {
  return TRACKER_STATUS[tracker.status] ?? { text: `状态 ${tracker.status}`, tone: "neutral" };
}

/** 失败原因（qBittorrent 的 msg / Transmission 的 lastAnnounceResult），可能没有 */
function trackerMessage(tracker: TorrentTrackerInfo): string {
  return tracker.message?.trim() ?? "";
}
</script>

<template>
  <div v-loading="props.loading" element-loading-text="加载详情…" class="dtd">
    <template v-if="props.detail">
      <dl class="dtd__meta">
        <div class="dtd__row">
          <dt>标题</dt>
          <dd>{{ props.detail.torrent.title }}</dd>
        </div>
        <div class="dtd__row">
          <dt>下载器</dt>
          <dd>
            {{ props.detail.torrent.downloader_name }} ({{ props.detail.torrent.downloader_type }})
          </dd>
        </div>
        <div class="dtd__row">
          <dt>Hash</dt>
          <dd class="dtd__mono">{{ props.detail.torrent.info_hash || "-" }}</dd>
        </div>
        <div class="dtd__row">
          <dt>保存路径</dt>
          <dd class="dtd__mono">{{ props.detail.torrent.save_path || "-" }}</dd>
        </div>
      </dl>

      <el-segmented v-model="activeTab" class="pt-seg dtd__seg" :options="tabOptions" />

      <template v-if="activeTab === 'files'">
        <el-table
          v-if="!isMobile"
          class="pt-grid"
          :data="props.detail.files"
          size="small"
          max-height="300">
          <el-table-column prop="index" label="#" width="64" class-name="pt-cell-num" />
          <el-table-column
            prop="name"
            label="文件"
            min-width="260"
            show-overflow-tooltip
            class-name="pt-cell-strong" />
          <el-table-column label="大小" width="120" align="right" class-name="pt-cell-num">
            <template #default="{ row }">{{ formatBytes(row.size) }}</template>
          </el-table-column>
          <el-table-column label="进度" width="160">
            <template #default="{ row }">
              <div class="dtd__pg">
                <PtProgress :percent="Math.round((row.progress || 0) * 100)" />
                <span>{{ Math.round((row.progress || 0) * 100) }}%</span>
              </div>
            </template>
          </el-table-column>
          <!-- 和行卡走同一张查表：这一列原来直接摆 priority 的数字，没人看得懂 -->
          <el-table-column prop="priority" label="优先级" width="90" align="right">
            <template #default="{ row }">{{ filePriorityText(row) }}</template>
          </el-table-column>
          <template #empty>
            <PtDataState state="empty" title="没有文件信息" sub="下载器未返回文件列表" dense />
          </template>
        </el-table>

        <!-- 文件行卡：标题给文件名（长路径靠 PtRowCard 的两行截断），
             第二行是序号 / 大小 / 优先级，底下一条该文件自己的进度 -->
        <div v-else class="dtd__cards">
          <PtDataState
            v-if="!props.detail.files.length"
            state="empty"
            title="没有文件信息"
            sub="下载器未返回文件列表"
            dense />

          <!-- key 里拼上数组位置：qBittorrent 4.4 以前 /torrents/files 不返回 index，
               整张表都是 0，只用 file.index 会撞 key -->
          <PtRowCard v-for="(file, i) in props.detail.files" :key="`${i}-${file.name}`">
            <template #title>{{ file.name || "-" }}</template>

            <template #meta>
              <span class="dtd__num">#{{ file.index }}</span>
              <span class="dtd__num">{{ formatBytes(file.size) }}</span>
              <span>{{ filePriorityText(file) }}</span>
            </template>

            <template #progress>
              <PtProgress :percent="filePercent(file)" :tone="fileTone(file)" />
              <span class="dtd__cardpct">{{ filePercent(file) }}%</span>
            </template>
          </PtRowCard>
        </div>
      </template>

      <template v-else>
        <el-table
          v-if="!isMobile"
          class="pt-grid"
          :data="props.detail.trackers"
          size="small"
          max-height="300">
          <el-table-column prop="url" label="URL" min-width="320" show-overflow-tooltip />
          <!-- 同上：status 原来显示的是 0..4 这个枚举数字 -->
          <el-table-column prop="status" label="状态" width="90" align="center">
            <template #default="{ row }">
              <PtStatusPill :tone="trackerStatus(row).tone" size="sm">
                {{ trackerStatus(row).text }}
              </PtStatusPill>
            </template>
          </el-table-column>
          <el-table-column
            prop="seeds"
            label="Seeds"
            width="90"
            align="right"
            class-name="pt-cell-num" />
          <el-table-column
            prop="peers"
            label="Peers"
            width="90"
            align="right"
            class-name="pt-cell-num" />
          <el-table-column
            prop="leeches"
            label="Leeches"
            width="90"
            align="right"
            class-name="pt-cell-num" />
          <template #empty>
            <PtDataState state="empty" title="没有 Tracker" sub="下载器未返回 Tracker 列表" dense />
          </template>
        </el-table>

        <!-- Tracker 行卡：标题给 URL，第二行是三个计数（标签沿用桌面列头的
             Seeds / Peers / Leeches），失败原因也挂在第二行，状态走右上角胶囊 -->
        <div v-else class="dtd__cards">
          <PtDataState
            v-if="!props.detail.trackers.length"
            state="empty"
            title="没有 Tracker"
            sub="下载器未返回 Tracker 列表"
            dense />

          <PtRowCard v-for="(tracker, i) in props.detail.trackers" :key="`${i}-${tracker.url}`">
            <template #title>{{ tracker.url || "-" }}</template>

            <template #meta>
              <span class="dtd__num">Seeds {{ tracker.seeds }}</span>
              <span class="dtd__num">Peers {{ tracker.peers }}</span>
              <span class="dtd__num">Leeches {{ tracker.leeches }}</span>
              <span
                v-if="trackerMessage(tracker)"
                :class="{ 'dtd__msg--err': trackerStatus(tracker).tone === 'dang' }">
                {{ trackerMessage(tracker) }}
              </span>
            </template>

            <template #status>
              <PtStatusPill :tone="trackerStatus(tracker).tone" size="sm">
                {{ trackerStatus(tracker).text }}
              </PtStatusPill>
            </template>
          </PtRowCard>
        </div>
      </template>
    </template>

    <PtDataState
      v-else-if="!props.loading"
      state="empty"
      title="没有详情"
      sub="任务信息获取失败或已被移除"
      dense />
  </div>
</template>

<style scoped>
.dtd {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-3);
  min-height: 160px;
}

/* 四行元信息：标签固定 68 宽，值自己换行，长 hash 和长路径不撑破容器 */
.dtd__meta {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  margin: 0;
}

.dtd__row {
  display: flex;
  gap: var(--pt-space-3);
  font-size: var(--pt-fz-body);
  line-height: var(--pt-lh-body);
}

.dtd__row dt {
  flex: 0 0 68px;
  color: var(--pt-t3);
}

.dtd__row dd {
  min-width: 0;
  margin: 0;
  color: var(--pt-t1);
  word-break: break-all;
}

.dtd__mono {
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-sm);
}

.dtd__seg {
  align-self: flex-start;
}

.dtd__pg {
  display: flex;
  gap: var(--pt-space-2);
  align-items: center;
}

.dtd__pg span {
  min-width: 34px;
  font-size: var(--pt-fz-label);
  font-weight: 600;
  color: var(--pt-t2);
  text-align: right;
}

/*
 * 移动端行卡列表。限高与桌面两张表的 max-height="300" 同理：几百个文件的种子
 * 会把内联面板顶出屏幕几十屏，元信息和分段切换全被推走。这里放宽到 60vh，
 * 手机上仍能一眼看到列表边界，超出部分自己滚（不阻断向外冒泡，滚到底继续带动页面）。
 */
.dtd__cards {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  max-height: 60vh;
  overflow-y: auto;
}

/* 数字对齐，免得同一列的大小和计数长短不一时跳动 */
.dtd__num {
  font-variant-numeric: tabular-nums;
}

/* 进度百分比贴右，和上面那条进度条右端对齐 */
.dtd__cardpct {
  align-self: flex-end;
  font-size: var(--pt-fz-foot);
  font-variant-numeric: tabular-nums;
  color: var(--pt-t3);
}

.dtd__msg--err {
  color: var(--pt-dang);
}
</style>
