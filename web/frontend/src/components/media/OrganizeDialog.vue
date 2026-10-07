<script setup lang="ts">
/*
 * 整理入库（路线图 M10）：任务列表与下载器 Web UI 共用。打开时先预览（识别出的条目、选中的媒体库、
 * 每个文件放到哪里），确认后再整理；识别不对时可以填 TMDB 编号、换媒体库后重新预览。
 */
import {
  type MediaKind,
  type MediaLibrary,
  type OrganizePlan,
  type OrganizeRequest,
  type OrganizeResult,
  organizeApi,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import {
  itemStatus,
  mediaKindLabel,
  mediaSourceLabel,
  modeLabel,
  posterURL,
  seasonEpisode,
  tmdbPageURL,
} from "@/utils/media";
import { computed, ref, watch } from "vue";

export interface OrganizeTarget {
  downloader_id: number;
  hash: string;
  name?: string;
}

const props = defineProps<{ target: OrganizeTarget | null }>();
const visible = defineModel<boolean>({ default: false });
const emit = defineEmits<{ done: [result: OrganizeResult] }>();

const errText = (e: unknown, fallback: string) => (e as Error)?.message || fallback;

const plan = ref<OrganizePlan | null>(null);
const loading = ref(false);
const loadError = ref("");
const running = ref(false);
const result = ref<OrganizeResult | null>(null);
const libraries = ref<MediaLibrary[]>([]);
const override = ref<{ type: MediaKind; id: number | undefined; library: number | undefined }>({
  type: "movie",
  id: undefined,
  library: undefined,
});

function request(): OrganizeRequest | null {
  if (!props.target) return null;
  const req: OrganizeRequest = {
    downloader_id: props.target.downloader_id,
    hash: props.target.hash,
  };
  if (override.value.id && override.value.id > 0) {
    req.media_type = override.value.type;
    req.tmdb_id = override.value.id;
  }
  if (override.value.library) req.library_id = override.value.library;
  return req;
}

async function preview() {
  const req = request();
  if (!req) return;
  loading.value = true;
  loadError.value = "";
  plan.value = null;
  try {
    plan.value = await organizeApi.preview(req);
  } catch (e) {
    loadError.value = errText(e, "预览失败");
  } finally {
    loading.value = false;
  }
}

watch(
  () => [visible.value, props.target?.downloader_id, props.target?.hash] as const,
  ([open]) => {
    if (!open) return;
    result.value = null;
    override.value = { type: "movie", id: undefined, library: undefined };
    void preview();
    organizeApi
      .libraries()
      .then((l) => (libraries.value = l ?? []))
      .catch(() => (libraries.value = []));
  },
  { immediate: true },
);

const pending = computed(() => plan.value?.items.filter((i) => i.status === "pending").length ?? 0);
const canRun = computed(() => Boolean(plan.value && !plan.value.problem && pending.value > 0));
const libraryOptions = computed(() =>
  libraries.value.filter(
    (l) =>
      l.enabled &&
      l.kind === (override.value.id ? override.value.type : plan.value?.match?.media_type),
  ),
);

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
</script>

<template>
  <el-dialog
    v-model="visible"
    class="pt-dialog"
    title="整理入库"
    width="760px"
    align-center
    data-testid="organize-dialog">
    <div class="od">
      <div class="od__torrent">
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

        <div v-if="plan.match" class="od__match" data-testid="organize-match">
          <img
            v-if="plan.match.poster_path"
            class="od__poster"
            :src="posterURL(plan.match.poster_path, 'w92')"
            :alt="`${plan.match.title} 的海报`"
            loading="lazy" />
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
              <span v-if="plan.source">{{
                plan.source === "manual" ? "手动指定" : mediaSourceLabel(plan.source)
              }}</span>
            </div>
            <div v-if="plan.library" class="od__meta" data-testid="organize-library">
              <span>媒体库：{{ plan.library.name }}</span>
              <span>{{ modeLabel(plan.mode) }}</span>
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

        <div class="od__override">
          <div class="od__label">识别不对、想换媒体库时</div>
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
              <el-option v-for="l in libraryOptions" :key="l.id" :label="l.name" :value="l.id" />
            </el-select>
            <el-button :loading="loading" data-testid="organize-repreview" @click="preview"
              >重新预览</el-button
            >
          </div>
        </div>
      </template>

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
          }}<template v-for="m in result.messages ?? []" :key="m"><br />{{ m }}</template></span
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
  overflow: hidden;
  font-size: 13px;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.od__target {
  display: flex;
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
</style>
