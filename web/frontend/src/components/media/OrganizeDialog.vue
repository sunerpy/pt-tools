<script setup lang="ts">
/*
 * 整理入库（路线图 M10）：任务列表与下载器 Web UI 整理一个种子，整理历史与媒体库页手动整理一个文件或目录（指定路径）。
 * 打开时先预览（识别出的条目、选中的媒体库、每个文件放到哪里），确认后再整理；识别不对时可以填 TMDB 编号、
 * 换媒体库后重新预览；这一次的整理方式可以另选（不改媒体库的设置），选了马上重新预览。
 * 指定路径时先填路径（或点「浏览」选）再预览，改了路径要重新预览才能整理。
 * 没识别出来时可以「让 AI 认」（M4b）：AI 从 TMDB 的候选里挑一个，填进「识别不对」那一行并给出预览，整理还是用户点；
 * 勾着「记成纠正」时整理前先存一条纠正。
 */
import {
  type MediaAIRecognizeResult,
  type MediaFSList,
  type MediaFSRoot,
  type MediaKind,
  type MediaLibrary,
  type MediaMode,
  type OrganizePlan,
  type OrganizeRequest,
  type OrganizeResult,
  mediaApi,
  organizeApi,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useIsMobile } from "@/composables/useIsMobile";
import { loadTmdbImageBase } from "@/composables/useTmdbImageBase";
import { formatBytes } from "@/utils/format";
import {
  MEDIA_MODES,
  classifyNoteText,
  discLabel,
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
 * 整理什么：下载器里的一个种子、监控目录里的一项，或 pt-tools 里看到的一个文件或文件夹（source_path 为空时等用户填）；
 * mode 是整理历史里重新整理时带上的、上次选的整理方式；media_type 与 tmdb_id 是条目详情里「手动整理到这个条目」预填的条目；
 * ai 为真时预览是没识别出来的话马上让 AI 认一次（整理历史里的「让 AI 认」）
 */
export type OrganizeTarget =
  | { downloader_id: number; hash: string; name?: string; ai?: boolean }
  | { watch_dir_id: number; entry: string; name?: string; ai?: boolean }
  | {
      source_path: string;
      name?: string;
      mode?: MediaMode;
      media_type?: MediaKind;
      tmdb_id?: number;
      ai?: boolean;
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

// ---- 让 AI 认 ----
const ai = ref<MediaAIRecognizeResult | null>(null);
const aiLoading = ref(false);
const aiError = ref("");
/** 「让 AI 认」用不了的原因（AI 助手没启用或没配好）：不为空时按钮不能点 */
const aiUnavailable = ref("");
/** 这次打开问没问过 AI 能不能用（问一次） */
let aiStatusAsked: Promise<boolean> | null = null;
/** 整理前把 AI 认出的条目记成纠正（以后同名的种子直接用这个条目） */
const aiSaveOverride = ref(true);
/** 打开时要认一次（整理历史里点的「让 AI 认」），预览回来以后看 */
let aiOnOpen = false;

function resetAI() {
  ai.value = null;
  aiLoading.value = false;
  aiError.value = "";
  aiUnavailable.value = "";
  aiStatusAsked = null;
  aiSaveOverride.value = true;
}

/** 问一次 AI 能不能用：不能用时记下原因；问不到时当能用（点了以后接口会写明原因） */
function checkAI(): Promise<boolean> {
  aiStatusAsked ??= organizeApi.aiStatus().then(
    (st) => {
      if (st && !st.ready) aiUnavailable.value = st.reason || "AI 助手不能用";
      return st?.ready !== false;
    },
    () => true,
  );
  return aiStatusAsked;
}

/** 整理请求；withEntry 为假时不带「识别不对」那一行填的条目（让 AI 认的是种子本身） */
function request(withEntry = true): OrganizeRequest | null {
  const t = props.target;
  if (!t) return null;
  let req: OrganizeRequest;
  if ("source_path" in t) {
    const p = sourcePath.value.trim();
    if (!p) return null;
    req = { source_path: p };
  } else if ("watch_dir_id" in t) {
    req = { watch_dir_id: t.watch_dir_id, entry: t.entry };
  } else {
    req = { downloader_id: t.downloader_id, hash: t.hash };
  }
  if (withEntry && override.value.id && override.value.id > 0) {
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
    if (seq === previewSeq) {
      plan.value = got;
      // 整理历史里点的「让 AI 认」：打开以后认一次（只认一次，之后用户自己点）
      if (aiOnOpen && got.fail_kind === "recognize") {
        aiOnOpen = false;
        void checkAI().then((ok) => {
          if (ok) void askAI();
        });
      }
    }
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
  if ("source_path" in t) return `path|${t.source_path}`;
  if ("watch_dir_id" in t) return `watch|${t.watch_dir_id}|${t.entry}`;
  return `${t.downloader_id}|${t.hash}`;
});

watch(
  () => [visible.value, targetKey.value] as const,
  ([open]) => {
    if (!open) return;
    // 「媒体识别」里换了 TMDB 图片地址时，海报从换的地址加载（打开时才读，页面上没打开就不发请求）
    void loadTmdbImageBase();
    result.value = null;
    resetAI();
    const t = props.target;
    aiOnOpen = Boolean(t?.ai);
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
/** 按钮上写的文件数：一张原盘是它放进库的文件数 */
const pendingFiles = computed(
  () =>
    plan.value?.items
      .filter((i) => i.status === "pending")
      .reduce((n, i) => n + (i.disc ? i.disc.files : 1), 0) ?? 0,
);
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
    if (aiOverride.value && ai.value) {
      try {
        await mediaApi.setOverride({
          title: ai.value.input.title,
          subtitle: ai.value.input.subtitle || undefined,
          media_type: override.value.type,
          tmdb_id: override.value.id!,
        });
      } catch (e) {
        loadError.value = `记成纠正失败，没有整理：${errText(e, "保存纠正失败")}`;
        return;
      }
    }
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

/** 预览卡在没识别出来时可以让 AI 认（先问一次 AI 能不能用） */
const aiShown = computed(() => plan.value?.fail_kind === "recognize");
watch(aiShown, (shown) => {
  if (shown) void checkAI();
});
/** AI 选中的候选（都不是时为空） */
const aiChosen = computed(() =>
  ai.value && ai.value.chosen >= 0 ? (ai.value.candidates[ai.value.chosen] ?? null) : null,
);
/** 「识别不对」那一行还是 AI 填的条目时才能记成纠正（用户改过就不记） */
const aiOverride = computed(
  () =>
    Boolean(aiChosen.value) &&
    aiSaveOverride.value &&
    override.value.id === aiChosen.value!.id &&
    override.value.type === aiChosen.value!.media_type,
);
const aiUsage = computed(() => {
  const r = ai.value;
  if (!r?.model) return "";
  const tokens =
    r.prompt_tokens || r.completion_tokens
      ? `，输入 ${r.prompt_tokens ?? 0} 个 token、输出 ${r.completion_tokens ?? 0} 个`
      : "";
  return `模型 ${r.model}${tokens}`;
});
/** 都不是时列出前几个候选，用户自己挑时有个参考 */
const aiCandidatesText = computed(() =>
  (ai.value?.candidates ?? [])
    .slice(0, 5)
    .map(
      (c) =>
        `${c.title}${c.year ? `（${c.year}）` : ""} · ${mediaKindLabel(c.media_type)} · TMDB ${c.id}`,
    )
    .join("；"),
);

async function askAI() {
  const req = request(false);
  if (!req) return;
  aiLoading.value = true;
  aiError.value = "";
  try {
    const got = await organizeApi.aiRecognize(req);
    ai.value = got;
    const chosen = got.chosen >= 0 ? got.candidates[got.chosen] : null;
    if (chosen && got.preview) {
      override.value.type = chosen.media_type;
      override.value.id = chosen.id;
      posterFailed.value = false;
      plan.value = got.preview;
    }
  } catch (e) {
    const msg = errText(e, "AI 没认成").trim();
    if ((e as { status?: number }).status === 409) aiUnavailable.value = msg;
    else aiError.value = msg;
  } finally {
    aiLoading.value = false;
  }
}

// ---- 浏览文件（指定路径时选一个文件或文件夹） ----
const fsVisible = ref(false);
const fsLoading = ref(false);
const fsError = ref("");
const fsRoots = ref<MediaFSRoot[]>([]);
/** 正在看的目录；为空时列根目录 */
const fsList = ref<MediaFSList | null>(null);
/** 选中的文件（点文件夹是进去） */
const fsPicked = ref("");
let fsSeq = 0;

const FS_ROOT_KINDS: Record<MediaFSRoot["kinds"][number], string> = {
  library: "媒体库",
  watch: "监控目录",
  map: "路径映射",
  downloader: "下载目录",
};
const rootLabel = (r: MediaFSRoot) =>
  [r.kinds.map((k) => FS_ROOT_KINDS[k]).join("、"), ...(r.labels ?? [])].join(" · ");

async function fsLoad(load: () => Promise<void>) {
  const seq = ++fsSeq;
  fsLoading.value = true;
  fsError.value = "";
  try {
    await load();
  } catch (e) {
    if (seq === fsSeq) fsError.value = errText(e, "读取目录失败").trim();
  } finally {
    if (seq === fsSeq) fsLoading.value = false;
  }
}

function openBrowser() {
  fsVisible.value = true;
  fsPicked.value = "";
  fsList.value = null;
  void fsLoad(async () => {
    const seq = fsSeq;
    const got = await organizeApi.fsRoots();
    if (seq === fsSeq) fsRoots.value = got.roots ?? [];
  });
}

function fsOpen(path: string) {
  fsPicked.value = "";
  void fsLoad(async () => {
    const seq = fsSeq;
    const got = await organizeApi.fsList(path);
    if (seq === fsSeq) fsList.value = got;
  });
}

/** 上一级；已经在根目录时回到根目录列表 */
function fsUp() {
  const parent = fsList.value?.parent;
  if (parent) fsOpen(parent);
  else {
    fsSeq++;
    fsLoading.value = false;
    fsError.value = "";
    fsPicked.value = "";
    fsList.value = null;
  }
}

function fsChoose(path: string) {
  if (!path) return;
  sourcePath.value = path;
  fsVisible.value = false;
  void preview();
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
          <el-button data-testid="organize-source-browse" @click="openBrowser">
            <PtIcon name="folder-open" :size="14" /><span>浏览</span>
          </el-button>
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
        <div v-if="aiShown || ai || aiError" class="od__ai" data-testid="organize-ai">
          <div v-if="aiShown" class="od__inline">
            <el-button
              :loading="aiLoading"
              :disabled="Boolean(aiUnavailable)"
              data-testid="organize-ai-ask"
              @click="askAI">
              <PtIcon v-if="!aiLoading" name="sparkles" :size="14" /><span>让 AI 认</span>
            </el-button>
            <span class="od__sub od__ai-tip"
              >把种子名和 TMDB 上搜到的候选交给 AI 助手的模型挑一个，只认、不整理</span
            >
          </div>
          <div
            v-if="aiShown && aiUnavailable"
            class="od__stale"
            data-testid="organize-ai-unavailable">
            {{ aiUnavailable }}
          </div>
          <div v-if="aiError" class="od__stale" data-testid="organize-ai-error">{{ aiError }}</div>
          <div v-if="ai" class="pt-note" data-testid="organize-ai-result">
            <PtIcon name="sparkles" :size="14" class="pt-note__icon" />
            <span>
              <template v-if="aiChosen"
                >AI 认成「{{ aiChosen.title
                }}<template v-if="aiChosen.year">（{{ aiChosen.year }}）</template>」（{{
                  mediaKindLabel(aiChosen.media_type)
                }}
                · TMDB {{ aiChosen.id }}），填进了下面的「识别不对」，预览按它算。</template
              >
              <template v-else-if="ai.candidates.length"
                >AI 认为候选里都不是。候选：{{ aiCandidatesText }}</template
              >
              <template v-else>{{ ai.reason }}</template>
              <template v-if="ai.reason && ai.candidates.length"
                ><br />理由：{{ ai.reason }}</template
              >
              <template v-if="aiUsage"><br />{{ aiUsage }}</template>
            </span>
          </div>
          <el-checkbox
            v-if="aiChosen && override.id === aiChosen.id && override.type === aiChosen.media_type"
            v-model="aiSaveOverride"
            data-testid="organize-ai-override"
            >整理时记成纠正：以后名字一样的种子直接认成这个条目</el-checkbox
          >
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
            <div v-if="plan.classify" class="od__sub" data-testid="organize-classify">
              二级分类：{{ classifyNoteText(plan.match.media_type, plan.classify) }}
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
              <span v-if="it.disc" class="od__ep" data-testid="organize-disc">{{
                discLabel(it.disc)
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
          >整理{{ pendingFiles ? ` ${pendingFiles} 个文件` : "" }}</span
        >
      </el-button>
    </template>
  </el-dialog>

  <el-dialog
    v-model="fsVisible"
    class="pt-dialog"
    title="选一个文件或文件夹"
    :width="isMobile ? '94%' : '620px'"
    append-to-body
    align-center
    data-testid="organize-fs">
    <div class="od-fs">
      <div class="od-fs__bar">
        <el-button
          v-if="fsList || fsError"
          size="small"
          :disabled="fsLoading"
          data-testid="organize-fs-up"
          @click="fsUp">
          <PtIcon name="chevron-up" :size="14" /><span>{{
            fsList?.parent ? "上一级" : "根目录"
          }}</span>
        </el-button>
        <span class="od-fs__path" data-testid="organize-fs-path">{{
          fsList ? fsList.path : "能浏览的目录：媒体库、监控目录、路径映射与下载器的下载目录"
        }}</span>
      </div>
      <div v-if="fsLoading" class="od__loading">
        <PtIcon name="loader-circle" :size="16" class="pt-spin" /><span>正在读取…</span>
      </div>
      <div v-else-if="fsError" class="pt-note pt-note--warn" data-testid="organize-fs-error">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>{{ fsError }}</span>
      </div>
      <ul v-else-if="!fsList" class="od-fs__list" data-testid="organize-fs-roots">
        <li v-if="!fsRoots.length" class="od__sub od-fs__empty">
          还没有能浏览的目录：先在「媒体库」里添加媒体库、监控目录或路径映射
        </li>
        <li v-for="r in fsRoots" :key="r.path">
          <button type="button" class="od-fs__row" @click="fsOpen(r.path)">
            <PtIcon name="folder" :size="15" class="od-fs__icon" />
            <span class="od-fs__name">{{ r.path }}</span>
            <span class="od__sub od-fs__aside">{{ rootLabel(r) }}</span>
          </button>
        </li>
      </ul>
      <ul v-else class="od-fs__list" data-testid="organize-fs-entries">
        <li v-if="!fsList.entries.length" class="od__sub od-fs__empty">这个文件夹是空的</li>
        <li v-for="e in fsList.entries" :key="e.path">
          <button
            type="button"
            class="od-fs__row"
            :class="{ 'is-picked': fsPicked === e.path }"
            :aria-pressed="e.dir ? undefined : fsPicked === e.path"
            @click="e.dir ? fsOpen(e.path) : (fsPicked = e.path)">
            <PtIcon :name="e.dir ? 'folder' : 'file-text'" :size="15" class="od-fs__icon" />
            <span class="od-fs__name">{{ e.name }}</span>
            <span v-if="!e.dir && e.size" class="od__sub od-fs__aside">{{
              formatBytes(e.size)
            }}</span>
          </button>
        </li>
        <li v-if="fsList.truncated" class="od__sub od-fs__empty">
          只列了前 2000 项：文件太多时直接在源路径里填
        </li>
      </ul>
    </div>
    <template #footer>
      <el-button @click="fsVisible = false">取消</el-button>
      <el-button v-if="fsList" data-testid="organize-fs-dir" @click="fsChoose(fsList.path)"
        >选这个文件夹</el-button
      >
      <el-button
        type="primary"
        :disabled="!fsPicked"
        data-testid="organize-fs-file"
        @click="fsChoose(fsPicked)"
        >选这个文件</el-button
      >
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

.od__ai {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.od__ai-tip {
  align-self: center;
}

.od-fs {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-width: 0;
}

.od-fs__bar {
  display: flex;
  gap: 8px;
  align-items: center;
  min-width: 0;
}

.od-fs__path {
  min-width: 0;
  font-size: 12px;
  color: var(--pt-t2);
  word-break: break-all;
}

.od-fs__list {
  display: flex;
  flex-direction: column;
  max-height: 420px;
  margin: 0;
  padding: 0;
  overflow-y: auto;
  list-style: none;
  border-top: 1px solid var(--pt-border);
}

.od-fs__row {
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

.od-fs__row:hover {
  background: var(--pt-bg-surface-muted);
}

.od-fs__row.is-picked {
  background: var(--pt-p-soft);
}

.od-fs__icon {
  flex: 0 0 auto;
  color: var(--pt-t3);
}

.od-fs__name {
  flex: 1 1 auto;
  min-width: 0;
  word-break: break-all;
}

.od-fs__aside {
  flex: 0 0 auto;
  max-width: 45%;
  text-align: right;
}

.od-fs__empty {
  padding: 12px 6px;
}

@media (max-width: 768px) {
  .od-fs__row {
    min-height: 44px;
  }
}
</style>
