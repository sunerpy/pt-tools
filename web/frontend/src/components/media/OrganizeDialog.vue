<script setup lang="ts">
/*
 * 整理入库（路线图 M10）：任务列表与下载器 Web UI 整理一个种子，整理历史与媒体库页手动整理一个文件或目录（指定路径）。
 * 打开时先预览（识别出的条目、选中的媒体库、每个文件放到哪里），确认后再整理；识别不对时可以填 TMDB 编号、
 * 换媒体库后重新预览；这一次的整理方式可以另选（不改媒体库的设置），选了马上重新预览。
 * 指定路径时先填路径再预览，改了路径要重新预览才能整理。
 */
import {
  type MediaKind,
  type MediaLibrary,
  type MediaMode,
  type OrganizePlan,
  type OrganizeRequest,
  type OrganizeResult,
  organizeApi,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useIsMobile } from "@/composables/useIsMobile";
import { loadTmdbImageBase } from "@/composables/useTmdbImageBase";
import {
  MEDIA_MODES,
  itemStatus,
  matchSourceLabel,
  mediaKindLabel,
  modeLabel,
  posterURL,
  seasonEpisode,
  tmdbPageURL,
} from "@/utils/media";
import { computed, ref, watch } from "vue";

/**
 * 整理什么：下载器里的一个种子，或 pt-tools 里看到的一个文件或文件夹（source_path 为空时等用户填）；
 * mode 是整理历史里重新整理时带上的、上次选的整理方式；media_type 与 tmdb_id 是条目详情里「手动整理到这个条目」预填的条目
 */
export type OrganizeTarget =
  | { downloader_id: number; hash: string; name?: string }
  | {
      source_path: string;
      name?: string;
      mode?: MediaMode;
      media_type?: MediaKind;
      tmdb_id?: number;
    };

const props = defineProps<{ target: OrganizeTarget | null }>();
const visible = defineModel<boolean>({ default: false });
const emit = defineEmits<{ done: [result: OrganizeResult] }>();

const isMobile = useIsMobile();
const errText = (e: unknown, fallback: string) => (e as Error)?.message || fallback;

const plan = ref<OrganizePlan | null>(null);
const loading = ref(false);
const loadError = ref("");
const running = ref(false);
const result = ref<OrganizeResult | null>(null);
const libraries = ref<MediaLibrary[]>([]);
/** 海报从 TMDB 的图片服务器取：浏览器连不上时不留一块破图 */
const posterFailed = ref(false);
/** 改的条目、媒体库与这一次的整理方式（mode 为空时跟媒体库） */
const override = ref<{
  type: MediaKind;
  id: number | undefined;
  library: number | undefined;
  mode: MediaMode | undefined;
}>({
  type: "movie",
  id: undefined,
  library: undefined,
  mode: undefined,
});

/** 指定路径：输入框里的路径，与上次预览用的路径（改了路径要重新预览才能整理） */
const pathMode = computed(() => Boolean(props.target && "source_path" in props.target));
const sourcePath = ref("");
const previewedPath = ref<string | null>(null);
/** 预览的序号：先发的预览晚回来时不覆盖后发的（指定路径时可能连着改几次路径预览） */
let previewSeq = 0;

function request(): OrganizeRequest | null {
  const t = props.target;
  if (!t) return null;
  let req: OrganizeRequest;
  if ("source_path" in t) {
    const p = sourcePath.value.trim();
    if (!p) return null;
    req = { source_path: p };
  } else {
    req = { downloader_id: t.downloader_id, hash: t.hash };
  }
  if (override.value.id && override.value.id > 0) {
    req.media_type = override.value.type;
    req.tmdb_id = override.value.id;
  }
  if (override.value.library) req.library_id = override.value.library;
  if (override.value.mode) req.mode = override.value.mode;
  return req;
}

async function preview() {
  const req = request();
  if (!req) return;
  const seq = ++previewSeq;
  loading.value = true;
  loadError.value = "";
  plan.value = null;
  posterFailed.value = false;
  previewedPath.value = "source_path" in req ? req.source_path : null;
  try {
    const got = await organizeApi.preview(req);
    if (seq === previewSeq) plan.value = got;
  } catch (e) {
    if (seq === previewSeq) loadError.value = errText(e, "预览失败");
  } finally {
    if (seq === previewSeq) loading.value = false;
  }
}

/** 换了要整理的东西时重新开始（种子按下载器与编号，指定路径按路径） */
const targetKey = computed(() => {
  const t = props.target;
  if (!t) return "";
  return "source_path" in t ? `path|${t.source_path}` : `${t.downloader_id}|${t.hash}`;
});

watch(
  () => [visible.value, targetKey.value] as const,
  ([open]) => {
    if (!open) return;
    // 「媒体识别」里换了 TMDB 图片地址时，海报从换的地址加载（打开时才读，页面上没打开就不发请求）
    void loadTmdbImageBase();
    result.value = null;
    const t = props.target;
    const path = t && "source_path" in t ? t : null;
    override.value = {
      type: path?.media_type ?? "movie",
      id: path?.tmdb_id && path.tmdb_id > 0 ? path.tmdb_id : undefined,
      library: undefined,
      mode: path?.mode,
    };
    if (t && "source_path" in t) {
      // 从页头打开时路径是空的，等用户填了再预览；从整理历史打开时带着当时填的路径，马上预览
      previewSeq++;
      plan.value = null;
      loadError.value = "";
      loading.value = false;
      previewedPath.value = null;
      sourcePath.value = t.source_path;
      if (sourcePath.value.trim()) void preview();
    } else {
      void preview();
    }
    organizeApi
      .libraries()
      .then((l) => (libraries.value = l ?? []))
      .catch(() => (libraries.value = []));
  },
  { immediate: true },
);

const pending = computed(() => plan.value?.items.filter((i) => i.status === "pending").length ?? 0);
/** 指定路径时输入框里的路径改过了、还没重新预览 */
const pathChanged = computed(
  () => pathMode.value && plan.value !== null && sourcePath.value.trim() !== previewedPath.value,
);
const canRun = computed(() =>
  Boolean(plan.value && !plan.value.problem && pending.value > 0 && !pathChanged.value),
);
const libraryOptions = computed(() => {
  const kind = override.value.id ? override.value.type : plan.value?.match?.media_type;
  // 指定路径还没预览、也没填条目时不知道是电影还是剧集：列出所有启用的库（类型不对时预览会说明）
  if (!kind && pathMode.value) return libraries.value.filter((l) => l.enabled);
  return libraries.value.filter((l) => l.enabled && l.kind === kind);
});
const libraryLabel = (l: MediaLibrary) => `${l.name}（${modeLabel(l.mode)} · ${l.path}）`;
/** 指定路径用移动整理：源文件会搬进媒体库（这次选的移动，或者媒体库用移动） */
const moveWarning = computed(() => {
  if (!pathMode.value || plan.value?.downloader_id !== 0 || plan.value?.mode !== "move") return "";
  const who = override.value.mode === "move" ? "这次用移动整理" : "这个媒体库用移动整理";
  return `${who}：源文件会搬进媒体库、不再留在原处。别的下载器还在做种的文件不要用移动。`;
});
/** 没选整理方式时跟媒体库：预览过的写明库用的是哪种 */
const modeFollowLabel = computed(() =>
  plan.value?.library ? `跟媒体库（${modeLabel(plan.value.library.mode)}）` : "跟媒体库",
);
/** 这一次用的方式的说明（没选、也还不知道是哪个库时不写；指定路径移动时上面的提醒已经写明，不再写讲种子的那句） */
const modeHint = computed(() => {
  const lib = plan.value?.library;
  if ((!override.value.mode && !lib) || moveWarning.value) return "";
  const mode = override.value.mode || lib?.mode || "hardlink";
  return MEDIA_MODES.find((m) => m.value === mode)?.hint ?? "";
});
/** 换了整理方式马上重新预览：目标与提醒（例如移动）跟着变，免得按旧的预览点了整理 */
function onModeChange() {
  void preview();
}

/** 目标路径去掉媒体库目录，只留库里的相对路径 */
function inLibrary(target?: string): string {
  const root = plan.value?.library?.path;
  if (!target) return "";
  if (root && target.startsWith(root)) return target.slice(root.length).replace(/^[/\\]/, "");
  return target;
}

async function run() {
  const req = request();
  if (!req) return;
  running.value = true;
  try {
    const res = await organizeApi.organize(req);
    result.value = res;
    if (res.plan) plan.value = res.plan;
    emit("done", res);
  } catch (e) {
    result.value = null;
    loadError.value = errText(e, "整理失败");
  } finally {
    running.value = false;
  }
}

const resultText = computed(() => {
  const r = result.value;
  if (!r) return "";
  if (r.queued) return "整理还在后台进行（文件较大时会这样），稍后在「整理历史」里看结果。";
  const parts = [`新入库 ${r.created} 个`];
  if (r.done) parts.push(`已在库里 ${r.done} 个`);
  if (r.skipped) parts.push(`跳过 ${r.skipped} 个`);
  if (r.failed) parts.push(`失败 ${r.failed} 个`);
  return parts.join("，");
});
/** 指定路径整理有失败时说明去哪里看、怎么重来 */
const resultHint = computed(() =>
  pathMode.value && result.value?.failed && !result.value.queued
    ? "失败的文件已记进「整理历史」：暂时的问题会自动重试，也可以在那里点「重试」。"
    : "",
);
</script>

<template>
  <el-dialog
    v-model="visible"
    class="pt-dialog"
    :title="pathMode ? '手动整理' : '整理入库'"
    :width="isMobile ? '94%' : '760px'"
    append-to-body
    align-center
    data-testid="organize-dialog">
    <div class="od">
      <div v-if="pathMode" class="od__source">
        <div class="od__label">源路径</div>
        <div class="od__inline">
          <el-input
            v-model="sourcePath"
            class="od__source-input"
            clearable
            placeholder="例如 /data/downloads/Oppenheimer.2023.2160p.BluRay.x265-FRDS"
            data-testid="organize-source"
            aria-label="源路径"
            @keyup.enter="preview" />
          <el-button
            :loading="loading"
            :disabled="!sourcePath.trim()"
            data-testid="organize-source-preview"
            @click="preview"
            >预览</el-button
          >
        </div>
        <div class="od__sub">
          pt-tools 里看到的文件或文件夹：一部电影或一季剧集的文件夹，或单个视频文件。Docker
          里填容器里的路径。
        </div>
        <div class="od__sub">
          文件放进所选媒体库的目录，文件名按库的命名模板；要放到别的目录，先在「媒体库」里加一个库。
        </div>
        <div v-if="pathChanged" class="od__stale" data-testid="organize-stale">
          路径改过了：点「预览」看新的计划，再整理
        </div>
      </div>

      <div v-if="!pathMode || plan" class="od__torrent">
        <div class="od__name" data-testid="organize-name">
          {{ plan?.name || target?.name || "-" }}
        </div>
        <div v-if="plan" class="od__sub">
          {{ plan.downloader_name }} · {{ plan.save_path }}
          <template v-if="plan.mapped"> → pt-tools 里是 {{ plan.local_path }}</template>
        </div>
      </div>

      <div v-if="loading" class="od__loading" data-testid="organize-loading">
        <PtIcon name="loader-circle" :size="16" class="pt-spin" /><span>正在识别…</span>
      </div>
      <div v-else-if="loadError" class="pt-note pt-note--warn" data-testid="organize-error">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>{{ loadError }}</span>
      </div>

      <template v-if="plan">
        <div v-if="plan.problem" class="pt-note pt-note--warn" data-testid="organize-problem">
          <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
          <span>{{ plan.problem }}</span>
        </div>
        <div v-if="plan.notes?.length" class="od__notes" data-testid="organize-notes">
          <div v-for="n in plan.notes" :key="n" class="pt-note">
            <PtIcon name="info" :size="14" class="pt-note__icon" />
            <span>{{ n }}</span>
          </div>
        </div>
        <div v-if="moveWarning" class="pt-note pt-note--warn" data-testid="organize-move-warn">
          <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
          <span>{{ moveWarning }}</span>
        </div>

        <div v-if="plan.match" class="od__match" data-testid="organize-match">
          <img
            v-if="posterURL(plan.match.poster_path, 'w92') && !posterFailed"
            class="od__poster"
            :src="posterURL(plan.match.poster_path, 'w92')"
            :alt="`${plan.match.title} 的海报`"
            loading="lazy"
            @error="posterFailed = true" />
          <div class="od__match-body">
            <a
              class="od__title"
              :href="tmdbPageURL(plan.match.media_type, plan.match.id)"
              target="_blank"
              rel="noopener noreferrer"
              >{{ plan.match.title
              }}<template v-if="plan.match.year"> ({{ plan.match.year }})</template></a
            >
            <div class="od__meta">
              <span>{{ mediaKindLabel(plan.match.media_type) }}</span>
              <span>TMDB {{ plan.match.id }}</span>
              <span v-if="plan.source">{{ matchSourceLabel(plan.source) }}</span>
            </div>
            <div v-if="plan.library" class="od__meta" data-testid="organize-library">
              <span>媒体库：{{ plan.library.name }}</span>
              <span v-if="plan.category" data-testid="organize-category"
                >分类：{{ plan.category }}</span
              >
              <span>{{ modeLabel(plan.mode) }}</span>
              <span>{{ plan.library.scrape ? "写 NFO 与图片" : "不刮削（媒体库关着刮削）" }}</span>
              <span class="od__path">{{ plan.library.path }}</span>
            </div>
          </div>
        </div>

        <div v-if="plan.items.length" class="od__items" data-testid="organize-items">
          <div v-for="it in plan.items" :key="it.source" class="od__item">
            <div class="od__item-head">
              <PtStatusPill :tone="itemStatus(it.status).tone" size="sm">{{
                itemStatus(it.status).label
              }}</PtStatusPill>
              <span v-if="it.episode" class="od__ep">{{
                seasonEpisode(it.season, it.episode, it.episode_end)
              }}</span>
              <span class="od__rel" :title="it.rel">{{ it.rel }}</span>
            </div>
            <div v-if="it.target" class="od__target">
              <PtIcon name="corner-down-right" :size="13" />
              <span :title="it.target">{{ inLibrary(it.target) }}</span>
              <span v-if="it.subtitles?.length" class="od__subs"
                >+ {{ it.subtitles.length }} 个字幕</span
              >
            </div>
            <div v-if="it.message" class="od__msg">{{ it.message }}</div>
          </div>
        </div>

        <details v-if="plan.skipped?.length" class="od__skipped" data-testid="organize-skipped">
          <summary>没选上的文件（{{ plan.skipped.length }} 个）</summary>
          <ul>
            <li v-for="s in plan.skipped" :key="s.file.rel">{{ s.file.rel }}：{{ s.reason }}</li>
          </ul>
        </details>
      </template>

      <!-- 指定路径时没有预览结果也显示：先填条目再预览 -->
      <div v-if="plan || pathMode" class="od__override">
        <div class="od__label">
          {{
            pathMode ? "条目与媒体库（TMDB 编号留空时按名字自动识别）" : "识别不对、想换媒体库时"
          }}
        </div>
        <div class="od__inline">
          <el-select
            v-model="override.type"
            class="od__kind"
            data-testid="organize-type"
            aria-label="类型">
            <el-option label="电影" value="movie" />
            <el-option label="剧集" value="tv" />
          </el-select>
          <el-input-number
            v-model="override.id"
            :min="1"
            :controls="false"
            placeholder="TMDB 编号"
            data-testid="organize-tmdb-id"
            aria-label="TMDB 编号" />
          <el-select
            v-model="override.library"
            clearable
            placeholder="按类型自动选库"
            class="od__lib"
            data-testid="organize-library-select"
            aria-label="媒体库">
            <el-option
              v-for="l in libraryOptions"
              :key="l.id"
              :label="libraryLabel(l)"
              :value="l.id" />
          </el-select>
          <el-button
            :loading="loading"
            :disabled="pathMode && !sourcePath.trim()"
            data-testid="organize-repreview"
            @click="preview"
            >重新预览</el-button
          >
        </div>
      </div>

      <div v-if="plan || pathMode" class="od__mode">
        <div class="od__label">整理方式（只管这一次，不改媒体库的设置）</div>
        <el-select
          v-model="override.mode"
          clearable
          :placeholder="modeFollowLabel"
          class="od__mode-select"
          data-testid="organize-mode"
          aria-label="整理方式"
          @change="onModeChange">
          <el-option v-for="m in MEDIA_MODES" :key="m.value" :label="m.label" :value="m.value" />
        </el-select>
        <div v-if="modeHint" class="od__sub" data-testid="organize-mode-hint">{{ modeHint }}</div>
      </div>

      <div
        v-if="result"
        class="pt-note"
        :class="result.failed ? 'pt-note--warn' : ''"
        data-testid="organize-result">
        <PtIcon
          :name="result.failed ? 'triangle-alert' : 'circle-check'"
          :size="14"
          class="pt-note__icon" />
        <span
          >{{ resultText
          }}<template v-for="m in result.messages ?? []" :key="m"><br />{{ m }}</template
          ><template v-if="resultHint"><br />{{ resultHint }}</template></span
        >
      </div>
    </div>

    <template #footer>
      <el-button @click="visible = false">{{ result ? "关闭" : "取消" }}</el-button>
      <el-button
        type="primary"
        :loading="running"
        :disabled="!canRun || loading"
        data-testid="organize-run"
        @click="run">
        <PtIcon v-if="!running" name="folder-open" :size="14" /><span
          >整理{{ pending ? ` ${pending} 个文件` : "" }}</span
        >
      </el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.od {
  display: flex;
  flex-direction: column;
  gap: 14px;
  min-width: 0;
}

.od__torrent {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.od__source {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.od__source-input {
  flex: 1 1 320px;
  min-width: 0;
}

.od__stale {
  font-size: 12px;
  color: var(--pt-warn);
}

.od__notes {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.od__name {
  font-size: 14px;
  font-weight: 600;
  color: var(--pt-t1);
  word-break: break-all;
}

.od__sub,
.od__msg,
.od__label {
  font-size: 12px;
  color: var(--pt-t3);
  word-break: break-all;
}

.od__loading {
  display: flex;
  gap: 8px;
  align-items: center;
  font-size: 13px;
  color: var(--pt-t2);
}

.od__match {
  display: flex;
  gap: 12px;
  align-items: flex-start;
}

.od__poster {
  flex: 0 0 auto;
  width: 60px;
  border-radius: 6px;
  background: var(--pt-bg-surface-muted);
}

.od__match-body {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.od__title {
  font-size: 15px;
  font-weight: 600;
}

.od__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 10px;
  font-size: 12px;
  color: var(--pt-t2);
}

.od__path {
  color: var(--pt-t3);
  word-break: break-all;
}

.od__items {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 320px;
  overflow-y: auto;
  padding-top: 12px;
  border-top: 1px solid var(--pt-border);
}

.od__item {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.od__item-head {
  display: flex;
  gap: 8px;
  align-items: center;
  min-width: 0;
}

.od__ep {
  flex: 0 0 auto;
  font-size: 12px;
  font-weight: 600;
  color: var(--pt-t1);
}

.od__rel {
  min-width: 0;
  overflow: hidden;
  font-size: 13px;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.od__target {
  display: flex;
  min-width: 0;
  overflow-wrap: anywhere;
  gap: 4px;
  align-items: center;
  padding-left: 4px;
  font-size: 12px;
  color: var(--pt-t2);
  word-break: break-all;
}

.od__subs {
  flex: 0 0 auto;
  color: var(--pt-t3);
}

.od__skipped {
  font-size: 12px;
  color: var(--pt-t3);
}

.od__skipped ul {
  margin: 6px 0 0;
  padding-left: 18px;
  word-break: break-all;
}

.od__override {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.od__inline {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.od__kind {
  width: 96px;
}

.od__lib {
  width: 180px;
}

.od__mode {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.od__mode-select {
  width: 200px;
}
</style>
