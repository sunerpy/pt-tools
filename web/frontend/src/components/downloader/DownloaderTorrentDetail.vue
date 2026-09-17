<script setup lang="ts">
import type { TorrentDetailResponse } from "@/api";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtProgress from "@/components/ui/PtProgress.vue";
import { formatBytes } from "@/utils/format";
import { computed } from "vue";

/**
 * 任务详情正文 —— 下方内联面板和右侧抽屉渲染的是同一份内容，
 * 原先在 DownloaderHub 里抄了两遍，改一处就得记得改另一处，所以抽成组件。
 *
 * 文件 / Tracker 的切换用 el-segmented（`.pt-seg`）而不是 el-tabs：
 * 抽屉和面板自己已经有一层标题，el-tabs 会再叠一条 48 高的页头。
 */
const props = defineProps<{
  detail: TorrentDetailResponse | null;
  loading?: boolean;
  /** 两处共用同一个当前分页，所以由调用方持有 */
  tab: string;
}>();

const emit = defineEmits<{ "update:tab": [value: string] }>();

const tabOptions = computed(() => [
  { label: `文件 (${props.detail?.files.length ?? 0})`, value: "files" },
  { label: `Tracker (${props.detail?.trackers.length ?? 0})`, value: "trackers" },
]);

const activeTab = computed({
  get: () => props.tab,
  set: (value: string) => emit("update:tab", value),
});
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

      <el-table
        v-if="activeTab === 'files'"
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
        <el-table-column
          prop="priority"
          label="优先级"
          width="90"
          align="right"
          class-name="pt-cell-num" />
        <template #empty>
          <PtDataState state="empty" title="没有文件信息" sub="下载器未返回文件列表" dense />
        </template>
      </el-table>

      <el-table v-else class="pt-grid" :data="props.detail.trackers" size="small" max-height="300">
        <el-table-column prop="url" label="URL" min-width="320" show-overflow-tooltip />
        <el-table-column prop="status" label="状态" width="80" align="center" />
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
</style>
