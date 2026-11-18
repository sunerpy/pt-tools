<script setup lang="ts">
/*
 * 导入已有媒体库（媒体中心 M5）：扫描媒体库目录里已经有的电影与剧集（不是 pt-tools 放进去的），认出条目以后
 * 给每个视频记一条整理记录。导入以后它们算作库里已有：订阅不再下载，条目详情与订阅进度看得到；删除记录时不跟着删文件、
 * 洗版不删、同名替换不换。扫描与导入都在后台进行，这里每秒看一次进度。没有画板：沿用整理弹窗的样式。
 */
import {
  type MediaFSList,
  type MediaImportItem,
  type MediaImportScan,
  type MediaLibrary,
  organizeApi,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import { useIsMobile } from "@/composables/useIsMobile";
import { formatBytes } from "@/utils/format";
import { matchSourceLabel } from "@/utils/media";
import { itemLink } from "@/utils/mediaItem";
import {
  type ImportItemState,
  importEpisodes,
  importItemState,
  importRunText,
} from "@/utils/mediaImport";
import { ElMessage } from "element-plus";
import { computed, onBeforeUnmount, ref, watch } from "vue";

const props = defineProps<{ modelValue: boolean; library: MediaLibrary | null }>();
const emit = defineEmits<{ "update:modelValue": [value: boolean]; done: [] }>();

const isMobile = useIsMobile();
const errText = (e: unknown, fallback: string) => (e as Error)?.message || fallback;
const visible = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emit("update:modelValue", v),
});

/** 轮询进度的间隔 */
const POLL_MS = 1000;

const path = ref("");
const scan = ref<MediaImportScan | null>(null);
const items = ref<MediaImportItem[]>([]);
const selected = ref<string[]>([]);
const filter = ref<"all" | ImportItemState>("all");
const scrape = ref(false);
const starting = ref(false);
const importing = ref(false);
const error = ref("");
/** 这次扫描里已经导入了的项（导入做完以后算作已有记录，不再选） */
const imported = ref(new Set<string>());
let submitted: string[] = [];
let timer: ReturnType<typeof setTimeout> | undefined;

const running = computed(() => scan.value?.status === "running");
const importRun = computed(() => scan.value?.import);
const importBusy = computed(
  () => importRun.value?.status === "queued" || importRun.value?.status === "running",
);

function stopPolling() {
  clearTimeout(timer);
  timer = undefined;
}

function reset() {
  stopPolling();
  path.value = props.library?.path ?? "";
  scan.value = null;
  items.value = [];
  selected.value = [];
  filter.value = "all";
  scrape.value = Boolean(props.library?.scrape);
  error.value = "";
  importing.value = false;
  imported.value = new Set();
  submitted = [];
  expanded.value = "";
  editing.value = "";
}

async function start() {
  if (!props.library) return;
  starting.value = true;
  error.value = "";
  try {
    scan.value = await organizeApi.importScan(props.library.id, path.value.trim());
    items.value = [];
    selected.value = [];
    imported.value = new Set();
    poll();
  } catch (e) {
    error.value = errText(e, "扫描没开始");
  } finally {
    starting.value = false;
  }
}

/** 过一会儿看一次扫描（与导入）的进度：扫描完了取每一项，导入做完了说结果 */
function poll(delay = POLL_MS) {
  stopPolling();
  timer = setTimeout(() => void check(), delay);
}

async function check() {
  const id = scan.value?.id;
  if (!id || !visible.value) return;
  try {
    const got = await organizeApi.importScanStatus(id, true);
    if (scan.value?.id !== id) return;
    scan.value = got;
    if (got.status === "running") {
      poll();
      return;
    }
    if (got.status === "done" && !items.value.length) await loadItems(id);
    if (got.import?.status === "queued" || got.import?.status === "running") {
      poll();
      return;
    }
    if (importing.value && got.import) finishImport();
  } catch (e) {
    error.value = errText(e, "读取扫描的进度失败");
    importing.value = false;
  }
}

async function loadItems(id: string) {
  const full = await organizeApi.importScanStatus(id);
  if (scan.value?.id !== id) return;
  items.value = full.items ?? [];
  selected.value = items.value.filter((it) => importItemState(it) === "ready").map((it) => it.path);
}

async function cancel() {
  const id = scan.value?.id;
  if (!id) return;
  try {
    await organizeApi.cancelImportScan(id);
    poll(0);
  } catch (e) {
    error.value = errText(e, "取消失败");
  }
}

// ---- 列表 ----
/** 一项现在的情况：这次导入了的算作已有记录 */
function stateOf(it: MediaImportItem): ImportItemState {
  return imported.value.has(it.path) ? "recorded" : importItemState(it);
}
const counts = computed(() => {
  const c = { ready: 0, recorded: 0, problem: 0, videos: 0 };
  for (const it of items.value) {
    const st = stateOf(it);
    c[st]++;
    if (st === "ready") c.videos += it.ready;
  }
  return c;
});
const shown = computed(() =>
  filter.value === "all" ? items.value : items.value.filter((it) => stateOf(it) === filter.value),
);
const selectedSet = computed(() => new Set(selected.value));
const selectedVideos = computed(() =>
  items.value.filter((it) => selectedSet.value.has(it.path)).reduce((n, it) => n + it.ready, 0),
);

function toggle(it: MediaImportItem, on: boolean) {
  if (on && !selectedSet.value.has(it.path)) selected.value = [...selected.value, it.path];
  if (!on) selected.value = selected.value.filter((p) => p !== it.path);
}

/** 当前筛选下能导入的项都选上（或都不选） */
const readyShown = computed(() => shown.value.filter((it) => stateOf(it) === "ready"));
const allShownPicked = computed(
  () =>
    readyShown.value.length > 0 && readyShown.value.every((it) => selectedSet.value.has(it.path)),
);
function toggleAll(on: boolean) {
  const paths = new Set(readyShown.value.map((it) => it.path));
  selected.value = on
    ? [...new Set([...selected.value, ...paths])]
    : selected.value.filter((p) => !paths.has(p));
}

const STATE_PILL: Record<ImportItemState, { tone: "ok" | "neutral" | "warn"; label: string }> = {
  ready: { tone: "ok", label: "能导入" },
  recorded: { tone: "neutral", label: "已有记录" },
  problem: { tone: "warn", label: "导入不了" },
};

function pillOf(it: MediaImportItem) {
  return imported.value.has(it.path)
    ? { tone: "ok" as const, label: "已导入" }
    : STATE_PILL[importItemState(it)];
}

function summary(it: MediaImportItem): string {
  // 原盘、没有视频这类导入不了的项没有视频：只写原因
  if (!it.file_count) return "";
  const parts = [`${it.file_count} 个视频`, formatBytes(it.size)];
  const eps = props.library?.kind === "tv" ? importEpisodes(it.files, it.file_count) : "";
  if (eps) parts.push(eps);
  if (it.recorded) parts.push(`${it.recorded} 个已经有整理记录`);
  return parts.join(" · ");
}

const expanded = ref("");
const fileProblems = (it: MediaImportItem) => it.files.filter((f) => f.problem).length;
function episodeText(season?: number, episode?: number, end?: number): string {
  if (!episode) return "";
  const s = String(season ?? 0).padStart(2, "0");
  const e = String(episode).padStart(2, "0");
  return end && end > episode ? `S${s}E${e}-E${String(end).padStart(2, "0")}` : `S${s}E${e}`;
}

// ---- 改编号 ----
const editing = ref("");
const editID = ref<number | undefined>();
const savingEdit = ref(false);
function startEdit(it: MediaImportItem) {
  editing.value = it.path;
  editID.value = it.match?.tmdb_id;
}
async function saveEdit(it: MediaImportItem) {
  const id = scan.value?.id;
  if (!id || !editID.value) return;
  savingEdit.value = true;
  try {
    const got = await organizeApi.setImportItem(id, it.path, editID.value);
    items.value = items.value.map((x) => (x.path === got.path ? got : x));
    toggle(got, importItemState(got) === "ready");
    editing.value = "";
  } catch (e) {
    ElMessage.error(errText(e, "改条目失败"));
  } finally {
    savingEdit.value = false;
  }
}

// ---- 导入 ----
async function runImport() {
  const id = scan.value?.id;
  if (!id || !selected.value.length) return;
  importing.value = true;
  error.value = "";
  submitted = [...selected.value];
  try {
    const run = await organizeApi.importItems(id, submitted, scrape.value);
    if (scan.value?.id !== id) return;
    scan.value = { ...scan.value, import: run };
    if (run.status === "queued" || run.status === "running") poll();
    else finishImport();
  } catch (e) {
    importing.value = false;
    ElMessage.error(errText(e, "导入失败"));
  }
}

function finishImport() {
  importing.value = false;
  const run = importRun.value;
  if (!run) return;
  // 交上去的项算作导入了（没导入成的原因写在结果里，重新扫描以后再看）：不再选它们
  imported.value = new Set([...imported.value, ...submitted]);
  selected.value = selected.value.filter((p) => !imported.value.has(p));
  submitted = [];
  ElMessage[run.failed || run.status === "failed" ? "warning" : "success"](importRunText(run));
  emit("done");
}

// ---- 浏览（只在库目录里） ----
const fsVisible = ref(false);
const fsLoading = ref(false);
const fsError = ref("");
const fsList = ref<MediaFSList | null>(null);
let fsSeq = 0;
const libRoot = computed(() => props.library?.path ?? "");
const fsDirs = computed(() => fsList.value?.entries.filter((e) => e.dir) ?? []);
const atRoot = computed(() => !fsList.value || fsList.value.path === libRoot.value);

async function fsOpen(p: string) {
  const seq = ++fsSeq;
  fsLoading.value = true;
  fsError.value = "";
  try {
    const got = await organizeApi.fsList(p);
    if (seq === fsSeq) fsList.value = got;
  } catch (e) {
    if (seq === fsSeq) fsError.value = errText(e, "读取目录失败").trim();
  } finally {
    if (seq === fsSeq) fsLoading.value = false;
  }
}

function openBrowser() {
  fsVisible.value = true;
  fsList.value = null;
  void fsOpen(path.value.trim() || libRoot.value);
}

function fsUp() {
  const parent = fsList.value?.parent;
  if (parent && !atRoot.value) void fsOpen(parent);
}

function fsChoose() {
  if (!fsList.value) return;
  path.value = fsList.value.path;
  fsVisible.value = false;
}

const progressText = computed(() => {
  const s = scan.value;
  if (!s) return "";
  return s.total ? `已认 ${s.scanned} / ${s.total} 项` : "正在找有哪些电影与剧集…";
});

// 打开时从头来（放在最后：reset 用到上面所有的状态）
watch(
  visible,
  (v) => {
    if (v) reset();
    else stopPolling();
  },
  { immediate: true },
);
onBeforeUnmount(stopPolling);
</script>

<template>
  <el-dialog
    v-model="visible"
    class="pt-dialog"
    :title="library ? `导入已有文件：${library.name}` : '导入已有文件'"
    :width="isMobile ? '94%' : '760px'"
    align-center
    data-testid="import-dialog">
    <div class="id">
      <p class="id__tip">
        找出媒体库里已经有的{{ library?.kind === "tv" ? "剧集" : "电影" }}（不是 pt-tools
        放进去的），认出条目以后记下来：之后订阅不再下载它们，条目详情与订阅进度里看得到。文件不动，删除记录时也不跟着删。
      </p>

      <div class="id__path">
        <el-input
          v-model="path"
          class="id__path-input"
          :disabled="running || importBusy"
          placeholder="库目录或里面的文件夹"
          aria-label="扫描的路径"
          data-testid="import-path" />
        <el-button
          :disabled="running || importBusy"
          data-testid="import-browse"
          @click="openBrowser"
          ><PtIcon name="folder-open" :size="14" /><span>浏览</span></el-button
        >
        <el-button
          v-if="!running"
          type="primary"
          :loading="starting"
          :disabled="importBusy"
          data-testid="import-scan"
          @click="start"
          ><PtIcon v-if="!starting" name="search" :size="14" /><span>{{
            scan ? "重新扫描" : "开始扫描"
          }}</span></el-button
        >
        <el-button v-else data-testid="import-cancel" @click="cancel">取消扫描</el-button>
      </div>

      <div v-if="error" class="pt-note pt-note--warn" data-testid="import-error">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>{{ error }}</span>
      </div>

      <div v-if="running" class="id__progress" data-testid="import-progress">
        <PtIcon name="loader-circle" :size="16" class="pt-spin" />
        <span>{{ progressText }}</span>
        <el-progress
          v-if="scan?.total"
          class="id__bar"
          :percentage="Math.round((scan.scanned / scan.total) * 100)"
          :show-text="false" />
      </div>
      <div v-else-if="scan?.status === 'failed'" class="pt-note pt-note--warn">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>扫描失败：{{ scan.error }}</span>
      </div>
      <div v-else-if="scan?.status === 'canceled'" class="id__sub" data-testid="import-canceled">
        扫描取消了。
      </div>

      <template v-if="scan?.status === 'done'">
        <div v-if="!items.length" class="id__sub" data-testid="import-empty">
          这个路径里没有找到电影或剧集。
        </div>
        <template v-else>
          <div class="id__bar-row">
            <span class="id__counts" data-testid="import-counts"
              >能导入 {{ counts.ready }} 项（{{ counts.videos }} 个视频） · 已有记录
              {{ counts.recorded }} 项 · 导入不了 {{ counts.problem }} 项</span
            >
            <el-radio-group v-model="filter" size="small" data-testid="import-filter">
              <el-radio-button value="all">全部</el-radio-button>
              <el-radio-button value="ready">能导入</el-radio-button>
              <el-radio-button value="problem">导入不了</el-radio-button>
              <el-radio-button value="recorded">已有记录</el-radio-button>
            </el-radio-group>
          </div>
          <div v-if="scan.truncated" class="pt-note pt-note--warn" data-testid="import-truncated">
            <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
            <span>多于 1000 项，只列了前 1000 项：剩下的分开扫描里面的文件夹。</span>
          </div>
          <el-checkbox
            v-if="readyShown.length"
            :model-value="allShownPicked"
            data-testid="import-all"
            @update:model-value="(v: string | number | boolean) => toggleAll(Boolean(v))"
            >选上这里能导入的 {{ readyShown.length }} 项</el-checkbox
          >
          <ul class="id__list" data-testid="import-items">
            <li v-for="it in shown" :key="it.path" class="id__item" :data-testid="`import-item`">
              <div class="id__item-head">
                <el-checkbox
                  :model-value="selectedSet.has(it.path)"
                  :disabled="stateOf(it) !== 'ready'"
                  :aria-label="`导入 ${it.name}`"
                  @update:model-value="(v: string | number | boolean) => toggle(it, Boolean(v))" />
                <span class="id__name">{{ it.name }}</span>
                <PtStatusPill :tone="pillOf(it).tone" size="sm">{{
                  pillOf(it).label
                }}</PtStatusPill>
              </div>
              <div class="id__item-body">
                <div v-if="it.match" class="id__match">
                  <router-link
                    class="id__title"
                    :to="itemLink(it.match.media_type, it.match.tmdb_id, 'explore')"
                    target="_blank"
                    >{{ it.match.title || it.match.original_title
                    }}{{ it.match.year ? ` (${it.match.year})` : "" }}</router-link
                  >
                  <span class="id__sub">TMDB {{ it.match.tmdb_id }}</span>
                  <PtTag v-if="it.source">{{ matchSourceLabel(it.source) }}</PtTag>
                </div>
                <div v-if="summary(it)" class="id__sub">{{ summary(it) }}</div>
                <div v-if="it.problem" class="id__problem" data-testid="import-problem">
                  {{ it.problem }}
                </div>
                <div v-for="n in it.notes ?? []" :key="n" class="id__sub">{{ n }}</div>
                <div class="id__actions">
                  <template v-if="editing === it.path">
                    <el-input-number
                      v-model="editID"
                      :min="1"
                      :controls="false"
                      class="id__edit"
                      :placeholder="`${library?.kind === 'tv' ? '剧集' : '电影'}的 TMDB 编号`"
                      aria-label="TMDB 编号"
                      data-testid="import-edit-id" />
                    <el-button
                      size="small"
                      type="primary"
                      :loading="savingEdit"
                      :disabled="!editID"
                      data-testid="import-edit-save"
                      @click="saveEdit(it)"
                      >确定</el-button
                    >
                    <el-button size="small" @click="editing = ''">取消</el-button>
                  </template>
                  <template v-else>
                    <el-button
                      v-if="stateOf(it) !== 'recorded' && it.ready > 0 && !importBusy"
                      link
                      size="small"
                      data-testid="import-edit"
                      @click="startEdit(it)"
                      >{{ it.match ? "改编号" : "填 TMDB 编号" }}</el-button
                    >
                    <el-button
                      v-if="it.files.length"
                      link
                      size="small"
                      :aria-expanded="expanded === it.path"
                      @click="expanded = expanded === it.path ? '' : it.path"
                      >{{ expanded === it.path ? "收起视频" : "看视频"
                      }}{{
                        fileProblems(it) ? `（${fileProblems(it)} 个导入不了）` : ""
                      }}</el-button
                    >
                  </template>
                </div>
                <ul v-if="expanded === it.path" class="id__files" data-testid="import-files">
                  <li v-for="f in it.files" :key="f.rel" class="id__file">
                    <span v-if="episodeText(f.season, f.episode, f.episode_end)" class="id__ep">{{
                      episodeText(f.season, f.episode, f.episode_end)
                    }}</span>
                    <span class="id__rel">{{ f.rel }}</span>
                    <span class="id__sub">{{ formatBytes(f.size) }}</span>
                    <span v-if="f.recorded" class="id__sub">已经有整理记录</span>
                    <span v-if="f.problem" class="id__problem">{{ f.problem }}</span>
                  </li>
                  <li v-if="it.file_count > it.files.length" class="id__sub">
                    只列了前 {{ it.files.length }} 个视频
                  </li>
                </ul>
              </div>
            </li>
          </ul>
        </template>
      </template>

      <div
        v-if="importRun"
        class="pt-note"
        :class="importRun.failed || importRun.status === 'failed' ? 'pt-note--warn' : 'pt-note--ok'"
        data-testid="import-result">
        <PtIcon
          :name="
            importBusy ? 'loader-circle' : importRun.failed ? 'triangle-alert' : 'circle-check'
          "
          :size="14"
          class="pt-note__icon"
          :class="{ 'pt-spin': importBusy }" />
        <span
          >{{ importRunText(importRun)
          }}<template v-if="!importBusy && importRun.status === 'done'"
            >。做完的在「整理历史」里，方式写「已有文件」。</template
          ><template v-for="m in importRun.messages ?? []" :key="m"><br />{{ m }}</template></span
        >
      </div>
    </div>

    <template #footer>
      <div class="id__foot">
        <el-checkbox
          v-model="scrape"
          :disabled="!library?.scrape"
          :title="library?.scrape ? '' : '这个媒体库关着刮削'"
          data-testid="import-scrape"
          >补上缺的 NFO 与图片</el-checkbox
        >
        <span class="id__foot-gap" />
        <el-button @click="visible = false">{{ importRun ? "关闭" : "取消" }}</el-button>
        <el-button
          type="primary"
          :loading="importing"
          :disabled="!selected.length || importBusy || running"
          data-testid="import-run"
          @click="runImport"
          ><PtIcon v-if="!importing" name="download" :size="14" /><span
            >导入{{
              selected.length ? ` ${selected.length} 项（${selectedVideos} 个视频）` : ""
            }}</span
          ></el-button
        >
      </div>
    </template>
  </el-dialog>

  <el-dialog
    v-model="fsVisible"
    class="pt-dialog"
    title="选一个文件夹"
    :width="isMobile ? '94%' : '560px'"
    append-to-body
    align-center
    data-testid="import-fs">
    <div class="id-fs">
      <div class="id-fs__bar">
        <el-button
          size="small"
          :disabled="fsLoading || atRoot"
          data-testid="import-fs-up"
          @click="fsUp"
          ><PtIcon name="chevron-up" :size="14" /><span>上一级</span></el-button
        >
        <span class="id-fs__path">{{ fsList?.path ?? libRoot }}</span>
      </div>
      <div v-if="fsLoading" class="id__progress">
        <PtIcon name="loader-circle" :size="16" class="pt-spin" /><span>正在读取…</span>
      </div>
      <div v-else-if="fsError" class="pt-note pt-note--warn">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>{{ fsError }}</span>
      </div>
      <ul v-else class="id-fs__list" data-testid="import-fs-dirs">
        <li v-if="!fsDirs.length" class="id__sub id-fs__empty">这里面没有文件夹</li>
        <li v-for="d in fsDirs" :key="d.path">
          <button type="button" class="id-fs__row" @click="fsOpen(d.path)">
            <PtIcon name="folder" :size="15" class="id-fs__icon" />
            <span class="id-fs__name">{{ d.name }}</span>
          </button>
        </li>
      </ul>
    </div>
    <template #footer>
      <el-button @click="fsVisible = false">取消</el-button>
      <el-button type="primary" :disabled="!fsList" data-testid="import-fs-pick" @click="fsChoose"
        >选这个文件夹</el-button
      >
    </template>
  </el-dialog>
</template>

<style scoped>
.id {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-width: 0;
}

.id__tip,
.id__sub {
  margin: 0;
  font-size: 12px;
  color: var(--pt-t3);
  word-break: break-all;
}

.id__path {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}

.id__path-input {
  flex: 1 1 280px;
  min-width: 0;
}

.id__progress {
  display: flex;
  gap: 8px;
  align-items: center;
  font-size: 13px;
  color: var(--pt-t2);
}

.id__bar {
  flex: 1 1 auto;
  min-width: 120px;
}

.id__bar-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 12px;
  align-items: center;
  justify-content: space-between;
}

.id__counts {
  font-size: 13px;
  color: var(--pt-t2);
}

.id__list {
  display: flex;
  flex-direction: column;
  max-height: 420px;
  margin: 0;
  padding: 0;
  overflow-y: auto;
  list-style: none;
  border-top: 1px solid var(--pt-border);
}

.id__item {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
  padding: 10px 4px;
  border-bottom: 1px solid var(--pt-border);
}

.id__item-head {
  display: flex;
  gap: 8px;
  align-items: center;
  min-width: 0;
}

.id__name {
  flex: 1 1 auto;
  min-width: 0;
  font-size: 13px;
  font-weight: 600;
  color: var(--pt-t1);
  word-break: break-all;
}

.id__item-body {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
  padding-left: 24px;
}

.id__match {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 8px;
  align-items: center;
}

.id__title {
  font-size: 13px;
  color: var(--pt-t1);
  text-decoration: none;
}

.id__title:hover {
  color: var(--pt-p);
  text-decoration: underline;
}

.id__problem {
  font-size: 12px;
  color: var(--pt-warn);
  word-break: break-all;
}

.id__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 8px;
  align-items: center;
}

.id__actions .el-button + .el-button {
  margin-left: 0;
}

.id__edit {
  width: 180px;
}

.id__files {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 4px 0 0;
  padding: 6px 0 0;
  list-style: none;
  border-top: 1px dashed var(--pt-border);
}

.id__file {
  display: flex;
  flex-wrap: wrap;
  gap: 2px 8px;
  align-items: baseline;
  min-width: 0;
  font-size: 12px;
}

.id__ep {
  flex: 0 0 auto;
  font-weight: 600;
  color: var(--pt-t1);
}

.id__rel {
  min-width: 0;
  color: var(--pt-t2);
  word-break: break-all;
}

.id__foot {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}

.id__foot-gap {
  flex: 1 1 auto;
}

.id__foot .el-button + .el-button {
  margin-left: 0;
}

.id-fs {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-width: 0;
}

.id-fs__bar {
  display: flex;
  gap: 8px;
  align-items: center;
  min-width: 0;
}

.id-fs__path {
  min-width: 0;
  font-size: 12px;
  color: var(--pt-t2);
  word-break: break-all;
}

.id-fs__list {
  display: flex;
  flex-direction: column;
  max-height: 420px;
  margin: 0;
  padding: 0;
  overflow-y: auto;
  list-style: none;
  border-top: 1px solid var(--pt-border);
}

.id-fs__row {
  display: flex;
  gap: 8px;
  align-items: center;
  width: 100%;
  min-height: 40px;
  padding: 8px 6px;
  font: inherit;
  font-size: 13px;
  color: var(--pt-t1);
  text-align: left;
  cursor: pointer;
  background: none;
  border: 0;
  border-bottom: 1px solid var(--pt-border);
}

.id-fs__row:hover {
  background: var(--pt-bg-surface-muted);
}

.id-fs__icon {
  flex: 0 0 auto;
  color: var(--pt-t3);
}

.id-fs__name {
  flex: 1 1 auto;
  min-width: 0;
  word-break: break-all;
}

.id-fs__empty {
  padding: 12px 6px;
}

@media (max-width: 768px) {
  /* 手机上弹窗本身滚动：列表不再单独限高，免得两层滚动；文件夹一行至少 44 高，好点 */
  .id__list {
    max-height: none;
    overflow-y: visible;
  }

  .id-fs__row {
    min-height: 44px;
  }
}
</style>
