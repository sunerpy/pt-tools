<script setup lang="ts">
/*
 * 整理历史（路线图 M10）：每个视频文件一条记录，写明整理到哪里、用什么方式、成功还是失败，以及刮削的结果。
 * 失败的可以重试（整个种子重新整理）；整理好的可以重新刮削（整部电影或剧集）；删除记录时可以连库里整理出的文件一起删。
 * 页头的「手动整理」指定一个文件或目录整理；指定路径整理失败或跳过的记录可以带着当时的路径再手动整理（换条目或库）。
 * M4b：失败与跳过的记录按分类筛选；勾选几条批量重试、重新刮削、删除；「按条目」把记录按电影、剧集的一季（没识别出的按种子名）
 * 分组，点一组看组里的记录；没识别出来的记录可以「让 AI 认」（打开整理弹窗认一次）。
 * 画板没有这一页，沿用列表页的样式：页头 + 筛选 + 表格，手机是行卡；多选条用 pt-band--sel。
 */
import {
  type MediaFailKind,
  type MediaHistoryBatchAction,
  type MediaHistoryGroup,
  type MediaHistoryItem,
  type MediaHistoryQuery,
  type MediaTransferStatus,
  organizeApi,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import OrganizeDialog, { type OrganizeTarget } from "@/components/media/OrganizeDialog.vue";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { formatBytes, formatShortDateTime } from "@/utils/format";
import {
  FAIL_KINDS,
  HISTORY_FILTERS,
  batchOutcome,
  failKindLabel,
  failKindOf,
  historyEntry,
  historyStatus,
  matchSourceLabel,
  modeLabel,
  organizeOutcome,
  failKindFrom,
  historyStatusFrom,
  rescrapeOutcome,
  scrapeStatus,
  seasonEpisode,
  triggerLabel,
} from "@/utils/media";
import { itemLink } from "@/utils/mediaItem";
import { ElMessage, ElMessageBox, type TableInstance } from "element-plus";
import { computed, onMounted, ref } from "vue";
import { useRoute } from "vue-router";

const isMobile = useIsMobile();
/* 测试里有的不挂路由：没有时当作没带筛选 */
const route = useRoute() as ReturnType<typeof useRoute> | undefined;
const errText = (e: unknown, fallback: string) => (e as Error)?.message || fallback;

const PAGE_SIZE = 50;
const items = ref<MediaHistoryItem[]>([]);
const total = ref(0);
const page = ref(1);
/** 首页的待处理带着筛选进来（?status=failed、?kind=recognize） */
const status = ref<"" | MediaTransferStatus>(historyStatusFrom(route?.query.status));
/** 失败与跳过的分类（只在看全部、失败或跳过时有） */
const kind = ref<"" | MediaFailKind>(
  status.value === "done" || status.value === "removed" ? "" : failKindFrom(route?.query.kind),
);
const keyword = ref("");
/** 「按条目」：列分组；group 是点开的那一组（看组里的记录） */
const byItem = ref(false);
const group = ref<MediaHistoryGroup | null>(null);
const groups = ref<MediaHistoryGroup[]>([]);
const showGroups = computed(() => byItem.value && !group.value);
const ds = useDataState({
  filtered: () => Boolean(status.value || kind.value || keyword.value.trim() || group.value),
});
const kindVisible = computed(() => status.value !== "done" && status.value !== "removed");

/** 筛选条件；只带填了的，组的条件在点开一组时带 */
function query(): MediaHistoryQuery {
  const q: MediaHistoryQuery = {
    status: status.value,
    q: keyword.value.trim(),
    limit: PAGE_SIZE,
    offset: (page.value - 1) * PAGE_SIZE,
  };
  if (kind.value) q.kind = kind.value;
  const g = group.value;
  if (g?.tmdb_id)
    Object.assign(q, { media_type: g.media_type, tmdb_id: g.tmdb_id, season: g.season ?? 0 });
  else if (g?.torrent) q.torrent = g.torrent;
  return q;
}

async function load() {
  if (showGroups.value) {
    const pending = ds.run(() => organizeApi.historyGroups(query()));
    const data = await pending;
    if (ds.isStale(pending) || !data) return;
    groups.value = data.groups;
    total.value = data.total;
    return;
  }
  const pending = ds.run(() => organizeApi.history(query()));
  const data = await pending;
  if (ds.isStale(pending) || !data) return;
  items.value = data.items;
  total.value = data.total;
  pruneSelection();
}

function search() {
  page.value = 1;
  clearSelection();
  void load();
}

/** 换了状态：看已整理、已删除时没有分类 */
function onStatus() {
  if (!kindVisible.value) kind.value = "";
  search();
}

/** 切换「按条目」：回到分组列表或记录列表的第一页 */
function toggleByItem() {
  group.value = null;
  items.value = [];
  groups.value = [];
  search();
}

function openGroup(g: MediaHistoryGroup) {
  group.value = g;
  items.value = [];
  search();
}

function closeGroup() {
  group.value = null;
  groups.value = [];
  search();
}

/** 一组的名字：标题（年份）、剧集加季；没识别出的是种子名 */
function groupTitle(g: MediaHistoryGroup) {
  if (!g.tmdb_id) return g.torrent || "没识别出来";
  const name = g.year ? `${g.title} (${g.year})` : g.title || `TMDB ${g.tmdb_id}`;
  return g.media_type === "tv" ? `${name} ${seasonEpisode(g.season)}` : name;
}

function groupCounts(g: MediaHistoryGroup) {
  const parts = [
    g.done ? `已整理 ${g.done}` : "",
    g.failed ? `失败 ${g.failed}` : "",
    g.skipped ? `跳过 ${g.skipped}` : "",
    g.removed ? `已删除 ${g.removed}` : "",
  ].filter(Boolean);
  return `${g.total} 条${parts.length ? `：${parts.join(" · ")}` : ""}`;
}

function groupLink(g: MediaHistoryGroup) {
  if (!g.tmdb_id || (g.media_type !== "movie" && g.media_type !== "tv")) return null;
  return itemLink(g.media_type, g.tmdb_id, "history", g.season);
}

// ---- 多选与批量 ----
const tableRef = ref<TableInstance>();
const selectedIds = ref<number[]>([]);
const selectedRows = computed(() => items.value.filter((r) => selectedIds.value.includes(r.id)));
const batching = ref<MediaHistoryBatchAction | null>(null);

function handleSelectionChange(rows: MediaHistoryItem[]) {
  selectedIds.value = rows.map((r) => r.id);
}

/** 手机行卡上的勾选：表格的多选是它自己管的，卡片这边维护同一份编号 */
function toggleSelect(r: MediaHistoryItem) {
  const i = selectedIds.value.indexOf(r.id);
  if (i === -1) selectedIds.value.push(r.id);
  else selectedIds.value.splice(i, 1);
}

function clearSelection() {
  tableRef.value?.clearSelection();
  selectedIds.value = [];
}

/** 重新读了以后去掉已经不在列表里的（换页、筛选时表格自己会清，行卡不会） */
function pruneSelection() {
  if (!selectedIds.value.length) return;
  const ids = new Set(items.value.map((r) => r.id));
  selectedIds.value = selectedIds.value.filter((id) => ids.has(id));
}

const selRetry = computed(() => selectedRows.value.filter(canRetry));
const selRescrape = computed(() => selectedRows.value.filter(canRescrape));
/** 批量按钮的字：选中的不是每条都能做时写明能做几条 */
const batchLabel = (what: string, n: number) =>
  n && n !== selectedIds.value.length ? `${what} ${n} 条` : what;

const BATCH_DONE: Record<MediaHistoryBatchAction, string> = {
  retry: "已重试",
  rescrape: "已重新刮削",
  delete: "已删除记录",
  delete_files: "已删除记录与文件",
};

async function batch(action: MediaHistoryBatchAction) {
  const rows =
    action === "retry"
      ? selRetry.value
      : action === "rescrape"
        ? selRescrape.value
        : selectedRows.value;
  if (!rows.length) return;
  let allowExisting = false;
  if (action === "delete" || action === "delete_files") {
    const files =
      action === "delete_files"
        ? "，连库里整理出的文件（视频、字幕、同名的 NFO 与图片）一起删；移动整理的文件是唯一的一份，那几条只能删记录，会写明"
        : "；库里与下载目录里的文件都不动";
    try {
      await ElMessageBox.confirm(`删除选中的 ${rows.length} 条整理记录${files}。`, "批量删除", {
        confirmButtonText: "删除",
        cancelButtonText: "取消",
        confirmButtonClass: "el-button--danger",
        type: "warning",
      });
    } catch {
      return;
    }
    // 导入的已有文件不是 pt-tools 放进去的：连文件删要再确认一次
    const existing = rows.filter((r) => r.mode === "existing" && r.has_files).length;
    if (action === "delete_files" && existing) {
      try {
        await ElMessageBox.confirm(
          `其中 ${existing} 条是导入时就在库里的文件（不是 pt-tools 放进去的），也删掉这些文件？`,
          "删除导入的已有文件",
          {
            confirmButtonText: "也删掉",
            cancelButtonText: "取消",
            confirmButtonClass: "el-button--danger",
            type: "warning",
          },
        );
      } catch {
        return;
      }
      allowExisting = true;
    }
  }
  batching.value = action;
  try {
    const ids = rows.map((r) => r.id);
    const res = allowExisting
      ? await organizeApi.historyBatch(ids, action, true)
      : await organizeApi.historyBatch(ids, action);
    const out = batchOutcome(res, BATCH_DONE[action]);
    const queued =
      (action === "retry" || action === "rescrape") && res.ok
        ? "（排进了整理队列，稍后刷新看结果）"
        : "";
    ElMessage[out.level](out.text + queued);
    clearSelection();
    await load();
  } catch (e) {
    ElMessage.error(errText(e, "批量操作失败"));
  } finally {
    batching.value = null;
  }
}

const fileName = (p: string) => p.split(/[/\\]/).pop() || p;
/** 库里的位置前面写什么：导入的已有文件就在库里（源文件就是它），不写箭头 */
const targetPrefix = (r: MediaHistoryItem) => (r.mode === "existing" ? "位置：" : "→ ");

const retrying = ref<number | null>(null);
async function retry(r: MediaHistoryItem) {
  retrying.value = r.id;
  try {
    const out = organizeOutcome(await organizeApi.retry(r.id));
    ElMessage[out.level](out.text);
    await load();
  } catch (e) {
    ElMessage.error(errText(e, "重试失败"));
  } finally {
    retrying.value = null;
  }
}

/** 识别出了条目的记录：条目名点进条目详情（剧集打开这一季） */
function itemOf(r: MediaHistoryItem) {
  if (r.tmdb_id <= 0 || (r.media_type !== "movie" && r.media_type !== "tv")) return null;
  return itemLink(r.media_type, r.tmdb_id, "history", r.season);
}

// ---- 重新刮削 ----
const rescraping = ref<number | null>(null);
const canRescrape = (r: MediaHistoryItem) => r.status === "done" && r.tmdb_id > 0;
async function rescrape(r: MediaHistoryItem) {
  const what =
    r.media_type === "tv"
      ? `「${r.title}」（这部剧在这个媒体库里整理好的每一集）`
      : `「${r.title}」`;
  try {
    await ElMessageBox.confirm(
      `按媒体库现在的刮削设置重写${what}的 NFO 与图片：pt-tools 写的、之后没被改过的文件换成新的，自己放的文件只在媒体库开着覆盖时换，媒体服务器里锁定了的 NFO 不动。`,
      "重新刮削",
      { confirmButtonText: "重新刮削", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  rescraping.value = r.id;
  try {
    const out = rescrapeOutcome(await organizeApi.rescrape(r.id));
    ElMessage[out.level](out.text);
    await load();
  } catch (e) {
    ElMessage.error(errText(e, "重新刮削失败"));
  } finally {
    rescraping.value = null;
  }
}

// ---- 删除 ----
const delDialog = ref(false);
const deleting = ref(false);
const delTarget = ref<MediaHistoryItem | null>(null);
const delFiles = ref(false);
const canDeleteFiles = computed(
  () => delTarget.value?.has_files === true && delTarget.value.mode !== "move",
);

function askDelete(r: MediaHistoryItem) {
  delTarget.value = r;
  delFiles.value = false;
  delDialog.value = true;
}

async function confirmDelete() {
  if (!delTarget.value) return;
  const files = delFiles.value && canDeleteFiles.value;
  // 导入的已有文件不是 pt-tools 放进去的：连文件删要再确认一次
  const existing = files && delTarget.value.mode === "existing";
  if (existing) {
    try {
      await ElMessageBox.confirm(
        "这是导入时就在库里的文件（不是 pt-tools 放进去的），确定连文件一起删掉？",
        "删除导入的已有文件",
        {
          confirmButtonText: "删掉",
          cancelButtonText: "取消",
          confirmButtonClass: "el-button--danger",
          type: "warning",
        },
      );
    } catch {
      return;
    }
  }
  deleting.value = true;
  try {
    const res = existing
      ? await organizeApi.deleteHistory(delTarget.value.id, files, true)
      : await organizeApi.deleteHistory(delTarget.value.id, files);
    if (res.kept?.length) ElMessage.warning(`已删除记录；这些文件留着：${res.kept.join("；")}`);
    else ElMessage.success(delFiles.value ? "已删除记录与库里的文件" : "已删除记录");
    delDialog.value = false;
    await load();
  } catch (e) {
    ElMessage.error(errText(e, "删除失败"));
  } finally {
    deleting.value = false;
  }
}

const canRetry = (r: MediaHistoryItem) => r.status === "failed" || r.status === "skipped";

// ---- 手动整理（指定一个文件或目录）与「让 AI 认」 ----
const organizeVisible = ref(false);
const organizeTarget = ref<OrganizeTarget | null>(null);
/** 没识别出来的记录：打开整理弹窗，预览以后马上让 AI 认一次 */
const canAskAI = (r: MediaHistoryItem) => failKindOf(r) === "recognize";
function askAI(r: MediaHistoryItem) {
  if (r.manual_path)
    organizeTarget.value = {
      source_path: r.manual_path,
      ai: true,
      ...(r.chosen_mode ? { mode: r.chosen_mode } : {}),
    };
  else if (r.watch_dir_id)
    organizeTarget.value = {
      watch_dir_id: r.watch_dir_id,
      entry: r.torrent_name,
      name: r.torrent_name,
      ai: true,
    };
  else
    organizeTarget.value = {
      downloader_id: r.downloader_id,
      hash: r.task_id || r.info_hash,
      name: r.torrent_name,
      ai: true,
    };
  organizeVisible.value = true;
}
/** 打开「手动整理」：从页头打开时路径是空的；从一条记录打开时带上当时填的路径 */
/** 打开手动整理：从记录打开时带上当时填的路径与选的整理方式 */
function openManual(path = "", mode: MediaHistoryItem["chosen_mode"] = "") {
  organizeTarget.value = mode ? { source_path: path, mode } : { source_path: path };
  organizeVisible.value = true;
}
/** 指定路径整理失败或跳过的记录：可以带着原路径重新打开，换条目或库 */
const canManual = (r: MediaHistoryItem) => canRetry(r) && Boolean(r.manual_path);
const rowTrigger = (r: MediaHistoryItem) =>
  r.manual_path ? "手动（指定路径）" : triggerLabel(r.trigger);
/** 条目下面那一行：条目从哪来（升级前的记录没有）、原盘的文件数与种子名。原盘写在这里，不跟在库里的路径后面：
 * 路径长，文件那一行放不下时看不到 */
const entrySub = (r: MediaHistoryItem) =>
  [
    matchSourceLabel(r.match_source),
    r.disc_files ? `原盘 ${r.disc_files} 个文件` : "",
    r.torrent_name,
  ]
    .filter(Boolean)
    .join(" · ");

onMounted(load);
</script>

<template>
  <div class="pt-cards pt-cards--wide">
    <PtHeadSub>每个视频文件一条记录：整理到哪里、用什么方式、成功还是失败</PtHeadSub>
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button data-testid="mh-manual" @click="openManual()">
        <PtIcon name="folder-open" :size="15" /><span>手动整理</span>
      </el-button>
      <el-button data-testid="mh-refresh" @click="load">
        <PtIcon name="refresh-cw" :size="15" /><span>刷新</span>
      </el-button>
    </Teleport>

    <PtPanel title="整理历史" icon="history" :count="total">
      <div class="mh-bar">
        <el-radio-group v-model="status" size="small" data-testid="mh-status" @change="onStatus">
          <el-radio-button v-for="f in HISTORY_FILTERS" :key="f.value || 'all'" :value="f.value">{{
            f.label
          }}</el-radio-button>
        </el-radio-group>
        <el-select
          v-if="kindVisible"
          v-model="kind"
          size="small"
          clearable
          class="mh-kind-select"
          placeholder="失败与跳过的分类"
          aria-label="失败与跳过的分类"
          data-testid="mh-kind"
          @change="search">
          <el-option v-for="k in FAIL_KINDS" :key="k.value" :label="k.label" :value="k.value" />
        </el-select>
        <el-radio-group v-model="byItem" size="small" data-testid="mh-view" @change="toggleByItem">
          <el-radio-button :value="false">按记录</el-radio-button>
          <el-radio-button :value="true">按条目</el-radio-button>
        </el-radio-group>
        <el-input
          v-model="keyword"
          class="mh-search"
          clearable
          placeholder="种子名、标题或路径"
          data-testid="mh-keyword"
          @keyup.enter="search"
          @clear="search">
          <template #prefix><PtIcon name="search" :size="14" /></template>
        </el-input>
      </div>

      <div v-if="group" class="mh-group-bar" data-testid="mh-group-bar">
        <el-button size="small" data-testid="mh-group-back" @click="closeGroup">
          <PtIcon name="arrow-left" :size="14" /><span>返回分组</span>
        </el-button>
        <span class="mh-strong">{{ groupTitle(group) }}</span>
        <span class="mh-sub">{{ groupCounts(group) }}</span>
      </div>

      <div
        v-if="(showGroups ? groups.length : items.length) && ds.error.value"
        class="pt-note pt-note--warn mh-stale"
        data-testid="mh-stale">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>刷新失败：{{ ds.errorText.value }}。下面是上次读到的。</span>
      </div>
      <template v-if="showGroups">
        <PtDataState
          v-if="!groups.length"
          :state="ds.state.value"
          :title="ds.state.value === 'empty' ? '还没有整理记录' : ''"
          :sub="ds.errorText.value"
          data-testid="mh-groups-state" />
        <el-table
          v-else-if="!isMobile"
          :data="groups"
          class="pt-grid"
          data-testid="mh-groups"
          @row-click="openGroup">
          <el-table-column
            label="条目"
            min-width="260"
            class-name="pt-cell-1line"
            show-overflow-tooltip>
            <template #default="{ row }">
              <div class="mh-strong">
                <router-link
                  v-if="groupLink(row)"
                  class="mh-link"
                  :to="groupLink(row)!"
                  @click.stop
                  >{{ groupTitle(row) }}</router-link
                >
                <template v-else>{{ groupTitle(row) }}</template>
              </div>
              <div class="mh-sub">{{ row.tmdb_id ? "" : "没识别出来的种子" }}</div>
            </template>
          </el-table-column>
          <el-table-column label="记录" min-width="220">
            <template #default="{ row }">
              <span class="mh-group-counts">{{ groupCounts(row) }}</span>
            </template>
          </el-table-column>
          <el-table-column label="最近" width="124">
            <template #default="{ row }">{{ formatShortDateTime(row.latest_at) }}</template>
          </el-table-column>
          <el-table-column label="" width="100" align="right">
            <template #default="{ row, $index }">
              <el-button link :data-testid="`mh-group-${$index}`" @click.stop="openGroup(row)"
                >看记录</el-button
              >
            </template>
          </el-table-column>
        </el-table>
        <div v-else class="mh-cards" data-testid="mh-groups">
          <PtRowCard
            v-for="(g, i) in groups"
            :key="`${g.media_type}|${g.tmdb_id}|${g.season}|${g.torrent}`">
            <template #title>{{ groupTitle(g) }}</template>
            <template #meta>
              <span class="mh-full">{{ groupCounts(g) }}</span>
              <span>{{ formatShortDateTime(g.latest_at) }}</span>
            </template>
            <template #actions>
              <el-button size="small" :data-testid="`mh-group-${i}`" @click="openGroup(g)"
                >看记录</el-button
              >
            </template>
          </PtRowCard>
        </div>
      </template>
      <PtDataState
        v-else-if="!items.length"
        :state="ds.state.value"
        :title="ds.state.value === 'empty' ? '还没有整理记录' : ''"
        :sub="
          ds.errorText.value ||
          (ds.state.value === 'empty'
            ? '在「媒体库」里打开自动整理，或点「手动整理」指定一个文件或目录，也可以在任务列表、下载器 Web UI 里整理种子'
            : '')
        "
        data-testid="mh-state" />
      <el-table
        v-else-if="!isMobile"
        ref="tableRef"
        :data="items"
        row-key="id"
        class="pt-grid"
        data-testid="mh-table"
        @selection-change="handleSelectionChange">
        <el-table-column type="selection" width="46" />
        <el-table-column label="时间" width="124">
          <template #default="{ row }">{{ formatShortDateTime(row.updated_at) }}</template>
        </el-table-column>
        <el-table-column
          label="条目"
          min-width="200"
          class-name="pt-cell-1line"
          show-overflow-tooltip>
          <template #default="{ row }">
            <div class="mh-strong">
              <router-link
                v-if="itemOf(row)"
                class="mh-link"
                :to="itemOf(row)!"
                :data-testid="`mh-item-${row.id}`"
                >{{ historyEntry(row) }}</router-link
              >
              <template v-else>{{ historyEntry(row) || "没识别出来" }}</template>
            </div>
            <div class="mh-sub">{{ entrySub(row) }}</div>
          </template>
        </el-table-column>
        <el-table-column
          label="文件"
          min-width="260"
          class-name="pt-cell-1line"
          show-overflow-tooltip>
          <template #default="{ row }">
            <div>{{ fileName(row.source_path) }}</div>
            <div v-if="row.target_path" class="mh-sub">
              {{ targetPrefix(row) }}{{ row.target_path
              }}<template v-if="row.subtitles"> · {{ row.subtitles }} 个字幕</template>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="方式" width="110">
          <template #default="{ row }">
            <div>{{ modeLabel(row.mode) }}</div>
            <div class="mh-sub">{{ row.library_name || "" }}</div>
          </template>
        </el-table-column>
        <el-table-column
          label="状态"
          min-width="200"
          class-name="pt-cell-1line"
          show-overflow-tooltip>
          <template #default="{ row }">
            <PtStatusPill :tone="historyStatus(row.status).tone" size="sm">{{
              historyStatus(row.status).label
            }}</PtStatusPill>
            <span v-if="failKindOf(row)" class="mh-kind" :data-testid="`mh-kind-${row.id}`">{{
              failKindLabel(failKindOf(row))
            }}</span>
            <span class="mh-sub mh-trigger">{{ rowTrigger(row) }}</span>
            <PtStatusPill
              v-if="scrapeStatus(row.scrape_status)"
              class="mh-scrape"
              :tone="scrapeStatus(row.scrape_status)!.tone"
              size="sm"
              :data-testid="`mh-scrape-${row.id}`"
              >{{ scrapeStatus(row.scrape_status)!.label }}</PtStatusPill
            >
            <div v-if="row.message" class="mh-sub">{{ row.message }}</div>
            <div v-if="row.scrape_note" class="mh-sub">刮削：{{ row.scrape_note }}</div>
            <div v-if="row.next_retry_at" class="mh-sub">
              {{ formatShortDateTime(row.next_retry_at) }} 自动重试（第 {{ row.attempts }} 次失败）
            </div>
          </template>
        </el-table-column>
        <el-table-column label="" width="190" align="right">
          <template #default="{ row }">
            <el-button
              v-if="canAskAI(row)"
              link
              :data-testid="`mh-ai-${row.id}`"
              @click="askAI(row)"
              >让 AI 认</el-button
            >
            <el-button
              v-if="canRetry(row)"
              link
              :loading="retrying === row.id"
              :data-testid="`mh-retry-${row.id}`"
              @click="retry(row)"
              >重试</el-button
            >
            <el-button
              v-if="canManual(row)"
              link
              :data-testid="`mh-manual-${row.id}`"
              @click="openManual(row.manual_path, row.chosen_mode)"
              >手动整理</el-button
            >
            <el-button
              v-if="canRescrape(row)"
              link
              :loading="rescraping === row.id"
              :data-testid="`mh-rescrape-${row.id}`"
              @click="rescrape(row)"
              >重新刮削</el-button
            >
            <el-button link type="danger" :data-testid="`mh-del-${row.id}`" @click="askDelete(row)"
              >删除</el-button
            >
          </template>
        </el-table-column>
      </el-table>
      <div v-else class="mh-cards">
        <PtRowCard v-for="r in items" :key="r.id">
          <template #lead>
            <el-checkbox
              :model-value="selectedIds.includes(r.id)"
              :aria-label="`选择 ${fileName(r.source_path)}`"
              :data-testid="`mh-check-${r.id}`"
              @update:model-value="toggleSelect(r)" />
          </template>
          <template #title>
            <router-link v-if="itemOf(r)" class="mh-link" :to="itemOf(r)!">{{
              historyEntry(r)
            }}</router-link>
            <template v-else>{{ historyEntry(r) || "没识别出来" }}</template>
          </template>
          <template #meta>
            <span v-if="r.match_source">{{ matchSourceLabel(r.match_source) }}</span>
            <span class="mh-full">{{ fileName(r.source_path) }}</span>
            <span v-if="r.target_path" class="mh-full"
              >{{ targetPrefix(r) }}{{ r.target_path }}</span
            >
            <span v-if="r.disc_files">原盘 {{ r.disc_files }} 个文件</span>
            <span v-if="r.subtitles">{{ r.subtitles }} 个字幕</span>
            <span
              >{{ modeLabel(r.mode) }} · {{ rowTrigger(r) }} ·
              {{ formatShortDateTime(r.updated_at) }}</span
            >
            <span v-if="r.size">{{ formatBytes(r.size) }}</span>
            <span v-if="failKindOf(r)">{{ failKindLabel(failKindOf(r)) }}</span>
            <span v-if="r.message" class="mh-full">{{ r.message }}</span>
            <span v-if="scrapeStatus(r.scrape_status)" class="mh-full"
              >{{ scrapeStatus(r.scrape_status)!.label
              }}<template v-if="r.scrape_note">：{{ r.scrape_note }}</template></span
            >
          </template>
          <template #status>
            <PtStatusPill dot :tone="historyStatus(r.status).tone" size="sm">{{
              historyStatus(r.status).label
            }}</PtStatusPill>
          </template>
          <template #actions>
            <el-button v-if="canAskAI(r)" size="small" @click="askAI(r)">让 AI 认</el-button>
            <el-button
              v-if="canRetry(r)"
              size="small"
              :loading="retrying === r.id"
              @click="retry(r)"
              >重试</el-button
            >
            <el-button
              v-if="canManual(r)"
              size="small"
              @click="openManual(r.manual_path, r.chosen_mode)"
              >手动整理</el-button
            >
            <el-button
              v-if="canRescrape(r)"
              size="small"
              :loading="rescraping === r.id"
              @click="rescrape(r)"
              >重新刮削</el-button
            >
            <el-button size="small" type="danger" plain @click="askDelete(r)">删除</el-button>
          </template>
        </PtRowCard>
      </div>

      <!-- 多选时列表下方的横幅（同暂停种子页），批量操作放在这里 -->
      <div
        v-if="!showGroups && selectedIds.length"
        class="pt-band--sel mh-band"
        data-testid="mh-band">
        <PtIcon name="square-check" :size="15" />
        <span class="mh-band__count">已选 {{ selectedIds.length }} 条</span>
        <span class="pt-band__spacer" />
        <el-button @click="clearSelection">
          <PtIcon name="x" :size="14" /><span>取消选择</span>
        </el-button>
        <el-button
          :disabled="!selRetry.length"
          :loading="batching === 'retry'"
          data-testid="mh-batch-retry"
          @click="batch('retry')"
          >{{ batchLabel("重试", selRetry.length) }}</el-button
        >
        <el-button
          :disabled="!selRescrape.length"
          :loading="batching === 'rescrape'"
          data-testid="mh-batch-rescrape"
          @click="batch('rescrape')"
          >{{ batchLabel("重新刮削", selRescrape.length) }}</el-button
        >
        <el-button
          type="danger"
          plain
          :loading="batching === 'delete'"
          data-testid="mh-batch-delete"
          @click="batch('delete')"
          >删除记录</el-button
        >
        <el-button
          type="danger"
          :loading="batching === 'delete_files'"
          data-testid="mh-batch-delete-files"
          @click="batch('delete_files')"
          >连文件删除</el-button
        >
      </div>

      <div v-if="total > PAGE_SIZE" class="mh-pager">
        <el-pagination
          v-model:current-page="page"
          class="pt-pager"
          :page-size="PAGE_SIZE"
          :total="total"
          :pager-count="5"
          layout="prev, pager, next"
          @current-change="load" />
      </div>
    </PtPanel>

    <el-dialog
      v-model="delDialog"
      class="pt-dialog"
      title="删除整理记录"
      :width="isMobile ? '94%' : '460px'"
      align-center>
      <p class="mh-del-text" data-testid="mh-del-text">
        删除「{{ delTarget ? fileName(delTarget.source_path) : "" }}」的整理记录？{{
          delTarget?.mode === "existing" ? "" : "下载目录里的文件不动。"
        }}
      </p>
      <el-checkbox v-model="delFiles" :disabled="!canDeleteFiles" data-testid="mh-del-files"
        >同时删除库里整理出的文件（视频、字幕、同名的 NFO 与图片）</el-checkbox
      >
      <p v-if="delTarget?.mode === 'move'" class="mh-sub mh-del-text">
        移动整理的文件是唯一的一份，这里不删。
      </p>
      <p
        v-else-if="delTarget?.mode === 'existing'"
        class="mh-sub mh-del-text"
        data-testid="mh-del-existing">
        这是导入时就在库里的文件（不是 pt-tools 放进去的）：勾上以后还要再确认一次。
      </p>
      <p v-else-if="!canDeleteFiles" class="mh-sub mh-del-text">这条记录没有整理出文件。</p>
      <template #footer>
        <el-button @click="delDialog = false">取消</el-button>
        <el-button
          type="danger"
          :loading="deleting"
          data-testid="mh-del-confirm"
          @click="confirmDelete"
          >删除</el-button
        >
      </template>
    </el-dialog>

    <OrganizeDialog v-model="organizeVisible" :target="organizeTarget" @done="load" />
  </div>
</template>

<style scoped>
.mh-link {
  color: inherit;
  text-decoration: none;
}

.mh-link:hover {
  color: var(--pt-p);
  text-decoration: underline;
}

.mh-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
  margin-bottom: 12px;
}

.mh-search {
  flex: 1 1 220px;
  max-width: 360px;
}

.mh-stale {
  margin-bottom: 12px;
}

.mh-strong {
  font-weight: 600;
  color: var(--pt-t1);
}

.mh-sub {
  font-size: 12px;
  color: var(--pt-t3);
}

.mh-trigger {
  margin-left: 6px;
}

.mh-kind {
  margin-left: 6px;
  font-size: 12px;
  color: var(--pt-t2);
}

.mh-kind-select {
  width: 160px;
}

.mh-group-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 10px;
  align-items: center;
  margin-bottom: 12px;
}

.mh-group-counts {
  font-size: 13px;
  color: var(--pt-t2);
}

/* 五个按钮在窄的宽度下一行放不下：换行排（一行时上下 6 + 按钮 30 + 顶边 1，最小高度 44 照旧） */
.mh-band {
  flex-wrap: wrap;
  row-gap: var(--pt-space-2);
  padding-block: 6px;
  margin-top: 12px;
}

.mh-band__count {
  white-space: nowrap;
}

/* 手机上「已选 N 条」单独一行，按钮在下面 */
@media (max-width: 768px) {
  .mh-band .pt-band__spacer {
    flex-basis: 100%;
  }
}

.mh-scrape {
  margin-left: 6px;
}

.mh-cards {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.mh-full {
  flex-basis: 100%;
  word-break: break-all;
}

.mh-pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 12px;
}

.mh-del-text {
  margin: 0 0 10px;
  font-size: 13px;
  line-height: 1.6;
  word-break: break-all;
}
</style>
