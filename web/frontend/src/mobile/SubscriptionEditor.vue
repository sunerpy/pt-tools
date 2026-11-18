<script setup lang="ts">
/*
 * 手机 App 的追剧编辑页（追剧方案 F3，3.3）：从上到下四块，条目、整理与下载、怎么找、什么时候找，「保存」固定在底部。
 * 新建时读草稿显示片名、年份与季，按媒体库带出下载器、保存目录、分类、标签（规则与网页共用 utils/subscribeDefaults.ts）；
 * 可以试搜索，从结果上设条件（SubscriptionPreview.vue）。主机不支持 subscription_assist 时照以前的方式填。
 * 2026-11-07 用户：「现在手机上追剧时不够简洁……选择了媒体库 剧集 类型后，应该支持自动带出一些设置的比如报错目录、分类、标签等内容」。
 */
import { ElMessage } from "element-plus";
import { computed, onMounted, ref, watch } from "vue";
import { addKeyword } from "@/utils/keywordList";
import {
  type DefaultsDownloader,
  type DownloadField,
  type DownloadValues,
  type LibraryDefaults,
  type Sources,
  applyAll,
  bringOut,
  differs,
  effectiveLibrary,
  libraryDefaults,
  newSources,
  resetField,
  userSources,
} from "@/utils/subscribeDefaults";
import {
  type AppDraft,
  type AppLibrary,
  type MobileOptions,
  type PreviewRequest,
  type SubscriptionDetail,
  type SubscriptionForm,
  mobileApi,
} from "./api";
import { BridgeError, notify, request } from "./bridge";
import { useEnglish, useWords } from "./i18n";
import KeywordTags from "./KeywordTags.vue";
import SubscriptionPreview, {
  type PreviewFollow,
  type PreviewLock,
  type PreviewSummary,
} from "./SubscriptionPreview.vue";
import { downloadSummary, fieldNote, scheduleSummary, sourceSummary } from "./subscriptionText";

const props = defineProps<{ id: number; canWrite: boolean }>();
const t = useWords();
const en = useEnglish();
const query = new URLSearchParams(location.search);
const loading = ref(true);
const saving = ref(false);
const error = ref("");
const forbidden = ref(false);
const options = ref<MobileOptions>();
/** /media/libraries；读不到时为空，媒体库照旧用选项里的（只有编号和名字） */
const libraries = ref<AppLibrary[] | null>(null);
/** 主机支持草稿、试搜索与带出（features 里的 subscription_assist） */
const assist = ref(false);
/** 在改哪个订阅；0 是新建。「改成修改它」以后换成那个订阅 */
const editId = ref(props.id);
const editing = computed(() => editId.value > 0);
const original = ref("");
const title = ref("");
const year = ref(0);
/** 表单的初值：修改时订阅没带的字段（category 等空着时不带）保持这些值 */
function blankForm(): SubscriptionForm {
  return {
    media_type: query.get("media_type") || "tv",
    tmdb_id: Number(query.get("tmdb_id") || 0),
    season: Number(query.get("season") || 0),
    profile_id: 0,
    sites: [],
    downloader_id: 0,
    category: "",
    tags: "",
    save_path: "",
    schedule: { kind: "airtime", interval_minutes: 360, cron: "" },
    lock_version: true,
    start_episode: 1,
    follow_next_season: false,
    library_id: 0,
    upgrade: false,
    include_keywords: "",
    exclude_keywords: "",
  };
}
const form = ref<SubscriptionForm>(blankForm());
/** 打开时就知道是哪个条目（从探索进来）；从「+」进来要先填类型与 TMDB 编号 */
const knownItem = form.value.tmdb_id > 0;
/** 显示类型与 TMDB 编号的输入框：从「+」进来，或者读不到条目（可以改了再「查找」） */
const manualItem = ref(!knownItem);
const tv = computed(() => form.value.media_type === "tv");

// ---- 草稿：片名、年份、一共几季、这一季几集、「自动」会整理进哪个库、这一季是不是已经在追 ----
const draft = ref<AppDraft | null>(null);
const draftError = ref("");
const looking = ref(false);
let draftSeq = 0;
async function loadDraft() {
  const f = form.value;
  if (!assist.value || editing.value || !Number.isInteger(f.tmdb_id) || f.tmdb_id <= 0) return;
  const seq = ++draftSeq;
  looking.value = true;
  draftError.value = "";
  try {
    const d = await mobileApi.draft(f.media_type, f.tmdb_id, f.media_type === "tv" ? f.season : 0);
    if (seq !== draftSeq) return;
    draft.value = d;
    title.value = d.title;
    year.value = d.year;
    // 季填 0 时草稿换成最新一季：编辑页显示、保存的都是这一季
    if (d.media_type === "tv" && d.season > 0) f.season = d.season;
  } catch (e) {
    if (seq !== draftSeq) return;
    draft.value = null;
    manualItem.value = true;
    const msg = (e instanceof Error ? e.message : String(e))
      .trim()
      .replace(/^参数不对[:：]\s*/, "");
    draftError.value = msg || t("读不到这个条目的信息", "The item could not be read");
  } finally {
    if (seq === draftSeq) looking.value = false;
  }
}
/**
 * TMDB 编号用普通输入框：编号不是能加减的数量，也不该一打开就填上 1。
 * 只留数字，空着是 0（「查找」与保存时提示要填）。
 */
const tmdbText = computed({
  get: () => (form.value.tmdb_id > 0 ? String(form.value.tmdb_id) : ""),
  set: (v: string) => {
    const digits = v.replace(/\D/g, "").slice(0, 10);
    form.value.tmdb_id = digits ? Number(digits) : 0;
  },
});
/** 从「+」进来：填了类型与 TMDB 编号（季可以不填，0 是最新一季）以后点「查找」 */
async function lookup() {
  if (!Number.isInteger(form.value.tmdb_id) || form.value.tmdb_id <= 0) {
    draftError.value = t("请填写有效的 TMDB 编号", "Enter a valid TMDB ID");
    return;
  }
  await loadDraft();
}
async function chooseSeason(n: number) {
  form.value.season = n;
  await loadDraft();
}
// 新建时改了类型或 TMDB 编号：查到的条目不算了（季回到 0，再点「查找」）；选着的库类型不对时回到「自动」
watch(
  () => [form.value.media_type, form.value.tmdb_id] as const,
  ([type, id]) => {
    if (loading.value || editing.value) return;
    const d = draft.value;
    if (looking.value || (d && (d.media_type !== type || d.tmdb_id !== id))) {
      draftSeq++;
      looking.value = false;
      if (d) form.value.season = 0;
      draft.value = null;
      title.value = "";
      year.value = 0;
    }
    const lib = libraries.value?.find((l) => l.id === form.value.library_id);
    if (lib && lib.kind !== type) form.value.library_id = 0;
  },
);
const seasonChoices = computed(() => {
  const d = draft.value;
  if (!tv.value || !d || d.media_type !== "tv" || d.seasons <= 0) return [];
  return Array.from({ length: d.seasons }, (_, i) => i + 1);
});
/** 这一季（这部电影）已经在追：保存会被拒，给一个「改成修改它」 */
const alreadyId = computed(() =>
  !editing.value && draft.value ? draft.value.subscription_id || 0 : 0,
);
const heading = computed(() => {
  if (title.value) return title.value;
  return editing.value
    ? t("修改追剧订阅", "Edit subscription")
    : t("新增追剧订阅", "New subscription");
});
const itemLine = computed(() => {
  const parts: string[] = [];
  if (year.value) parts.push(String(year.value));
  parts.push(tv.value ? t("剧集", "TV") : t("电影", "Movie"));
  const d = draft.value;
  if (editing.value) {
    if (tv.value && form.value.season > 0)
      parts.push(t(`第 ${form.value.season} 季`, `Season ${form.value.season}`));
  } else if (d && tv.value) {
    if (d.seasons > 0) parts.push(t(`一共 ${d.seasons} 季`, `${d.seasons} seasons`));
    if (d.season > 0)
      parts.push(
        d.total_episodes > 0
          ? t(
              `第 ${d.season} 季 ${d.total_episodes} 集`,
              `Season ${d.season}: ${d.total_episodes} episodes`,
            )
          : t(`第 ${d.season} 季`, `Season ${d.season}`),
      );
  }
  return title.value || d ? parts.join(" · ") : "";
});

// ---- 整理与下载：只列同类型、启用着的库；新建时按生效的库带出四项，自己改过的不动 ----
const kindLibraries = computed(() =>
  (libraries.value ?? []).filter((l) => l.kind === form.value.media_type && l.enabled),
);
const libraryOptions = computed<{ id: number; label: string }[]>(() => {
  if (!libraries.value)
    return (options.value?.libraries ?? []).map((l) => ({ id: l.id, label: l.name }));
  const list = kindLibraries.value.map((l) => ({ id: l.id, label: l.name }));
  const id = form.value.library_id;
  // 修改时现在选着的库停用了、删了也列出来，看得出选的是什么
  if (id && !list.some((l) => l.id === id)) {
    const l = libraries.value.find((x) => x.id === id);
    list.push({
      id,
      label: l
        ? t(`${l.name}（已停用）`, `${l.name} (disabled)`)
        : t(`媒体库 ${id}（已删除）`, `Library ${id} (deleted)`),
    });
  }
  return list;
});
const autoLabel = computed(() => {
  const d = draft.value;
  if (!assist.value || editing.value || !d || !libraries.value)
    return t("自动（按类型选）", "Automatic (by type)");
  if (d.auto_library_id > 0) {
    const lib = libraries.value.find((l) => l.id === d.auto_library_id);
    if (lib) return t(`自动（会整理进：${lib.name}）`, `Automatic (organized into: ${lib.name})`);
  }
  return t(
    "自动（没有能用的库：整理会失败）",
    "Automatic (no usable library: organizing will fail)",
  );
});
/** 一个同类型、启用着的库都没有：保存了也整理不了 */
const noLibrary = computed(() => Boolean(libraries.value) && kindLibraries.value.length === 0);

const downloaders = computed<DefaultsDownloader[]>(() =>
  (options.value?.downloaders ?? []).map((d) => ({
    id: d.id,
    name: d.name,
    enabled: d.enabled !== false,
  })),
);
const downloadersKnown = computed(
  () => Boolean(options.value) && !options.value?.errors?.downloaders,
);
const sources = ref<Sources>(newSources());
const libDefaults = computed<LibraryDefaults | null>(() => {
  if (!assist.value || !libraries.value) return null;
  const lib = effectiveLibrary(
    libraries.value,
    form.value.library_id,
    editing.value ? 0 : (draft.value?.auto_library_id ?? 0),
  );
  return lib ? libraryDefaults(lib, downloadersKnown.value ? downloaders.value : null) : null;
});
function downloadValues(): DownloadValues {
  const f = form.value;
  return {
    downloader_id: f.downloader_id || 0,
    save_path: f.save_path ?? "",
    category: f.category ?? "",
    tags: f.tags ?? "",
  };
}
function applyDefaults(next: { values: DownloadValues; sources: Sources }) {
  const f = form.value;
  const v = next.values;
  if (f.downloader_id !== v.downloader_id) f.downloader_id = v.downloader_id;
  if (f.save_path !== v.save_path) f.save_path = v.save_path;
  if (f.category !== v.category) f.category = v.category;
  if (f.tags !== v.tags) f.tags = v.tags;
  sources.value = next.sources;
}
/** 新建时按生效的库带出（没动过的项）；修改时不带出 */
function refreshDefaults() {
  if (editing.value || !assist.value || !options.value) return;
  applyDefaults(bringOut({ values: downloadValues(), sources: sources.value }, libDefaults.value));
}
// 换了库、换了季（草稿里「自动」会整理进的库）都会换生效的库；打开时由 load() 带出
watch(libDefaults, () => {
  if (!loading.value) refreshDefaults();
});
function touch(field: DownloadField) {
  if (sources.value[field].kind !== "user")
    sources.value = { ...sources.value, [field]: { kind: "user" } };
}
const diffFields = computed(() =>
  libDefaults.value ? differs(downloadValues(), libDefaults.value) : [],
);
const noteOf = (field: DownloadField) =>
  fieldNote(sources.value[field], libDefaults.value, field, en);
const canReset = (field: DownloadField) =>
  !editing.value && sources.value[field].kind === "user" && diffFields.value.includes(field);
/** 「换成媒体库「X」的」（同网页）：恢复成生效的库里的值 */
const resetLabel = computed(() => {
  const lib = libDefaults.value?.library ?? "";
  return t(`换成媒体库「${lib}」的`, `Use library "${lib}"`);
});
function resetDownloadField(field: DownloadField) {
  applyDefaults(
    resetField({ values: downloadValues(), sources: sources.value }, field, libDefaults.value),
  );
}
/** 修改时换了库、库的下载设置和订阅的不一样：出一条「套用」，点了才换 */
const originalLibrary = ref(0);
const applyLibrary = computed(() => {
  if (!editing.value || form.value.library_id === originalLibrary.value) return null;
  return libDefaults.value && diffFields.value.length ? libDefaults.value : null;
});
function applyLibraryDefaults() {
  applyDefaults(applyAll({ values: downloadValues(), sources: sources.value }, libDefaults.value));
}
const showDownload = ref(false);
const summary = computed(() =>
  downloadSummary(downloadValues(), options.value?.downloaders ?? [], en),
);
const summarySource = computed(() => sourceSummary(sources.value, libDefaults.value, en));
/** 库设的下载器用不了（停用、删了、读不到）：收起时也要看得到推到哪里 */
const downloaderNote = computed(() => {
  const d = libDefaults.value;
  if (
    !d ||
    ["ok", "unset"].includes(d.downloader) ||
    sources.value.downloader_id.kind !== "library"
  )
    return "";
  return noteOf("downloader_id");
});
const downloaderLabel = (d: { name: string; enabled?: boolean }) =>
  d.enabled === false ? t(`${d.name}（已停用）`, `${d.name} (disabled)`) : d.name;
/** 保存目录的候选：所选下载器的预设目录；下载器是「默认」时按下载器排在一起列出全部（写上是哪个下载器的） */
const pathChoices = computed(() => {
  const all = options.value?.directories ?? [];
  const id = form.value.downloader_id;
  const order = (options.value?.downloaders ?? []).map((d) => d.id);
  const names = new Map((options.value?.downloaders ?? []).map((d) => [d.id, d.name]));
  const list = id
    ? all.filter((d) => d.downloader_id === id)
    : [...all].sort((a, b) => order.indexOf(a.downloader_id) - order.indexOf(b.downloader_id));
  return list.map((d) => ({
    value: d.path,
    alias: d.alias ?? "",
    isDefault: Boolean(d.is_default),
    downloader: id ? "" : (names.get(d.downloader_id) ?? ""),
  }));
});
function suggestPaths(q: string, cb: (items: { value: string }[]) => void) {
  const s = (q ?? "").trim().toLowerCase();
  const all = pathChoices.value;
  if (!s || all.some((o) => o.value.toLowerCase() === s)) return cb(all);
  cb(all.filter((o) => o.value.toLowerCase().includes(s) || o.alias.toLowerCase().includes(s)));
}

// ---- 怎么找：试搜索 ----
const previewOpen = ref(false);
const lastPreview = ref<PreviewSummary | null>(null);
const previewRequest = computed<PreviewRequest | null>(() => {
  const f = form.value;
  if (!Number.isInteger(f.tmdb_id) || f.tmdb_id <= 0) return null;
  return {
    media_type: f.media_type,
    tmdb_id: f.tmdb_id,
    season: f.media_type === "tv" ? f.season : 0,
    profile_id: f.profile_id,
    sites: [...f.sites],
    include_keywords: f.include_keywords,
    exclude_keywords: f.exclude_keywords,
    include_unmatched: true,
  };
});
/** 只读设备没有试搜索：原生桥拒绝一切非 GET 请求 */
const canPreview = computed(() => assist.value && props.canWrite && Boolean(previewRequest.value));
const previewSites = computed(() => form.value.sites.length || options.value?.sites.length || 0);
const lastLine = computed(() => {
  const s = lastPreview.value;
  if (!s) return "";
  const sec = s.seconds.toFixed(1);
  return tv.value
    ? t(
        `上次：符合 ${s.ok} · 不符合 ${s.reject} · 对不上这一季 ${s.unmatched}（${sec} 秒）`,
        `Last: ${s.ok} matching · ${s.reject} rejected · ${s.unmatched} not this season (${sec} s)`,
      )
    : t(
        `上次：符合 ${s.ok} · 不符合 ${s.reject} · 对不上 ${s.unmatched}（${sec} 秒）`,
        `Last: ${s.ok} matching · ${s.reject} rejected · ${s.unmatched} not this movie (${sec} s)`,
      );
});
function addWord(to: "include_keywords" | "exclude_keywords", word: string) {
  const r = addKeyword(form.value[to], word);
  if (!r.ok) {
    ElMessage.warning(
      r.reason === "duplicate"
        ? t(`已经有「${word.trim()}」了`, `"${word.trim()}" is already there`)
        : r.reason === "too_long"
          ? t(
              "加了以后超过 1024 字节：先删掉几个词",
              "More than 1024 bytes: remove some words first",
            )
          : r.reason === "separator"
            ? t(
                "这一段里有逗号，会被当成几个词：选一段不带逗号的",
                "This part has a comma: pick a part without one",
              )
            : t("没有可以加的词", "Nothing to add"),
    );
    return;
  }
  form.value[to] = r.text;
  ElMessage.success(
    to === "include_keywords"
      ? t(`已加进标题包含：${word.trim()}`, `Added to title includes: ${word.trim()}`)
      : t(`已加进标题排除：${word.trim()}`, `Added to title excludes: ${word.trim()}`),
  );
}

// ---- 什么时候找：定时摘要、起始集、版本锁 ----
const showSchedule = ref(false);
const scheduleLine = computed(() => scheduleSummary(form.value.schedule, en));
/** 锁住的版本：修改时是订阅的，新建时是从试搜索里选的；lockEdited 是用户改过（只在改了时发） */
const lock = ref<PreviewLock>({ group: "", resolution: "" });
const lockEdited = ref(false);
const lockSet = computed(() => Boolean(lock.value.group || lock.value.resolution));
const lockLabel = computed(() =>
  [lock.value.group, lock.value.resolution].filter(Boolean).join(" "),
);
/** 交给试搜索的锁：版本锁关着时为空 */
const previewLock = computed<PreviewLock | null>(() =>
  tv.value && form.value.lock_version ? { ...lock.value } : null,
);
function lockTo(v: PreviewLock) {
  lock.value = { ...v };
  lockEdited.value = true;
  form.value.lock_version = true;
  ElMessage.success(
    t(
      `版本锁改成 ${lockLabel.value}：保存后生效`,
      `Version lock set to ${lockLabel.value}: takes effect when saved`,
    ),
  );
}
function clearLock() {
  lock.value = { group: "", resolution: "" };
  lockEdited.value = true;
}
/** 保存时从这个种子开始追（新建剧集）：起始集设成它的第一集；「不从它开始」改回原来的起始集 */
const fromTorrent = ref<{ site: string; torrent_id: string; title: string } | null>(null);
let startBefore = 1;
function followFrom(f: PreviewFollow) {
  if (!fromTorrent.value) startBefore = form.value.start_episode;
  fromTorrent.value = { site: f.site, torrent_id: f.torrent_id, title: f.title };
  if (f.start > 0) form.value.start_episode = f.start;
  ElMessage.success(
    f.start > 0
      ? t(
          `保存以后从这个种子开始追（从第 ${f.start} 集起）`,
          `Saving starts from this torrent (episode ${f.start} on)`,
        )
      : t(
          "保存以后从这个种子开始追（整季包，起始集不变）",
          "Saving starts from this torrent (whole season, start unchanged)",
        ),
  );
}
function unfollow() {
  fromTorrent.value = null;
  form.value.start_episode = startBefore;
}

// ---- 读取、未保存提示、保存 ----
function snapshot() {
  return JSON.stringify({ form: form.value, lock: lock.value, from: fromTorrent.value });
}
watch(
  [form, lock, fromTorrent],
  () => {
    if (!loading.value) notify("dirty", { value: snapshot() !== original.value });
  },
  { deep: true },
);
/** 换成修改这个订阅：先回到初值，订阅没带的字段不能留着新建时带出或填过的值 */
function applySubscription(current: SubscriptionDetail) {
  title.value = current.title;
  year.value = current.year ?? 0;
  const next = blankForm();
  for (const key of Object.keys(next) as (keyof SubscriptionForm)[]) {
    if (current[key] !== undefined && current[key] !== null)
      Object.assign(next, { [key]: current[key] });
  }
  form.value = next;
  lock.value = { group: current.lock_group ?? "", resolution: current.lock_resolution ?? "" };
  lockEdited.value = false;
  originalLibrary.value = form.value.library_id;
  sources.value = userSources();
}
async function load() {
  loading.value = true;
  error.value = "";
  forbidden.value = false;
  try {
    const [meta, opts, libs, current] = await Promise.all([
      // 读不到 meta 时当作旧主机：不读草稿、不带出、不试搜索
      mobileApi.meta().catch(() => null),
      mobileApi.options(),
      mobileApi.libraries().catch(() => null),
      editId.value ? mobileApi.subscription(editId.value) : Promise.resolve(null),
    ]);
    assist.value = Boolean(meta?.features?.includes("subscription_assist"));
    options.value = opts;
    libraries.value = Array.isArray(libs) ? libs : null;
    if (current) applySubscription(current);
    else if (knownItem) await loadDraft();
    // 带出完成以后再取未保存的基准：带出本身不算改动
    refreshDefaults();
    original.value = snapshot();
    notify("dirty", { value: false });
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
    forbidden.value = e instanceof BridgeError && [401, 403].includes(e.status);
  } finally {
    loading.value = false;
  }
}
/** 这一季已经在追：直接改成修改那个订阅 */
async function switchToEdit() {
  const id = alreadyId.value;
  if (!id) return;
  loading.value = true;
  error.value = "";
  try {
    const current = await mobileApi.subscription(id);
    editId.value = id;
    draft.value = null;
    draftError.value = "";
    fromTorrent.value = null;
    previewOpen.value = false;
    lastPreview.value = null;
    applySubscription(current);
    original.value = snapshot();
    notify("dirty", { value: false });
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    loading.value = false;
  }
}
async function save() {
  if (saving.value || !props.canWrite) return;
  error.value = "";
  if (!editing.value && (!Number.isInteger(form.value.tmdb_id) || form.value.tmdb_id <= 0)) {
    error.value = t("请填写有效的 TMDB 编号", "Enter a valid TMDB ID");
    return;
  }
  saving.value = true;
  try {
    const { media_type, tmdb_id, season, ...editable } = form.value;
    const payload: Record<string, unknown> = editing.value
      ? { ...editable }
      : { media_type, tmdb_id, season, ...editable };
    // 修改时没换媒体库就不发：库后来停用了的订阅，改别的设置也能保存（和网页 #855 一样）
    if (editing.value && form.value.library_id === originalLibrary.value) delete payload.library_id;
    // 锁住的版本只在用户改了时发；清掉是两项都发空串（下一个下载成功的版本成为锁）
    if (tv.value && lockEdited.value && form.value.lock_version) {
      payload.lock_group = lock.value.group;
      payload.lock_resolution = lock.value.resolution;
    }
    if (!editing.value && tv.value && fromTorrent.value)
      payload.from_torrent = { ...fromTorrent.value };
    await request(
      `/api/app/v1/subscriptions${editing.value ? `/${editId.value}` : ""}`,
      editing.value ? "PUT" : "POST",
      payload,
    );
    notify("dirty", { value: false });
    notify("close", { changed: true });
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    saving.value = false;
  }
}
onMounted(() => void load());
</script>
<template>
  <div v-if="loading" role="status" class="mobile-state">{{ t("正在读取…", "Loading…") }}</div>
  <template v-else>
    <div v-if="error" role="alert" class="mobile-error">{{ error }}</div>
    <el-button v-if="!options && !forbidden" @click="load">{{ t("重试", "Retry") }}</el-button>
    <el-form
      v-if="options"
      class="se-form"
      label-position="top"
      :disabled="!canWrite || saving"
      @submit.prevent="save">
      <p v-if="!canWrite" role="status">
        {{ t("这台设备只有只读权限", "This device is read-only") }}
      </p>
      <p v-if="options.errors && Object.keys(options.errors).length" class="mobile-note">
        {{
          t(
            "部分选项未能加载，保存前请重试",
            "Some options could not be loaded. Retry before saving.",
          )
        }}
      </p>

      <!-- 1 条目 -->
      <section class="mobile-section" data-testid="se-item">
        <h2 data-testid="se-title">{{ heading }}</h2>
        <p v-if="itemLine" class="mobile-note se-line" data-testid="se-item-line">{{ itemLine }}</p>
        <p v-if="looking" role="status" class="mobile-note">
          {{ t("正在读取条目…", "Reading the item…") }}
        </p>
        <template v-if="!editing">
          <template v-if="!assist || manualItem">
            <el-form-item :label="t('类型', 'Type')"
              ><el-select v-model="form.media_type" data-testid="se-type"
                ><el-option value="tv" :label="t('剧集', 'TV')" /><el-option
                  value="movie"
                  :label="t('电影', 'Movie')" /></el-select
            ></el-form-item>
            <el-form-item label="TMDB ID"
              ><el-input
                v-model="tmdbText"
                inputmode="numeric"
                :placeholder="t('例如 1396', 'e.g. 1396')"
                data-testid="se-tmdb"
            /></el-form-item>
            <el-button
              v-if="assist"
              class="se-lookup"
              :loading="looking"
              data-testid="se-lookup"
              @click="lookup"
              >{{ t("查找", "Look up") }}</el-button
            >
          </template>
          <el-form-item
            v-if="tv && !seasonChoices.length"
            :label="t('季（0 表示最新一季）', 'Season (0 = latest)')"
            ><el-input-number v-model="form.season" :min="0" :precision="0" data-testid="se-season"
          /></el-form-item>
          <el-form-item v-else-if="seasonChoices.length" :label="t('季', 'Season')">
            <el-select
              v-if="seasonChoices.length > 8"
              :model-value="form.season"
              data-testid="se-season-select"
              @update:model-value="(v: number) => void chooseSeason(Number(v))">
              <el-option
                v-for="n in seasonChoices"
                :key="n"
                :value="n"
                :label="t(`第 ${n} 季`, `Season ${n}`)" />
            </el-select>
            <div v-else class="se-chips" role="radiogroup">
              <button
                v-for="n in seasonChoices"
                :key="n"
                type="button"
                role="radio"
                class="se-chip"
                :class="{ 'is-on': form.season === n }"
                :aria-checked="form.season === n"
                :disabled="!canWrite || saving"
                :data-testid="`se-season-${n}`"
                @click="chooseSeason(n)">
                {{ t(`第 ${n} 季`, `Season ${n}`) }}
              </button>
            </div>
          </el-form-item>
          <div v-if="draftError" class="mobile-error" data-testid="se-draft-error">
            {{ draftError }}
          </div>
          <div v-if="alreadyId" class="se-already" role="status" data-testid="se-already">
            <span>{{
              tv
                ? t("这一季已经在追，再建会被拒", "You already follow this season")
                : t("这部电影已经订阅过，再建会被拒", "You already subscribed to this movie")
            }}</span>
            <el-button data-testid="se-switch-edit" @click="switchToEdit">{{
              t("改成修改它", "Edit it instead")
            }}</el-button>
          </div>
        </template>
      </section>

      <!-- 2 整理与下载 -->
      <section class="mobile-section" data-testid="se-download">
        <h2>{{ t("整理与下载", "Organizing and download") }}</h2>
        <el-form-item :label="t('媒体库', 'Library')"
          ><el-select v-model="form.library_id" data-testid="se-library"
            ><el-option :value="0" :label="autoLabel" /><el-option
              v-for="l in libraryOptions"
              :key="l.id"
              :value="l.id"
              :label="l.label" /></el-select
        ></el-form-item>
        <p v-if="noLibrary" class="mobile-note se-warn" data-testid="se-no-library">
          {{
            tv
              ? t(
                  "整理会失败：先在网页的媒体库里添加剧集媒体库",
                  "Organizing will fail: add a TV library on the web first",
                )
              : t(
                  "整理会失败：先在网页的媒体库里添加电影媒体库",
                  "Organizing will fail: add a movie library on the web first",
                )
          }}
        </p>
        <div v-if="applyLibrary" class="se-apply" data-testid="se-apply">
          <span>{{
            t(
              `套用「${applyLibrary.library}」的下载设置？`,
              `Use library "${applyLibrary.library}"'s download settings?`,
            )
          }}</span>
          <el-button data-testid="se-apply-button" @click="applyLibraryDefaults">{{
            t("套用", "Use")
          }}</el-button>
        </div>
        <div class="se-summary">
          <div class="se-summary__text" data-testid="se-download-summary">{{ summary }}</div>
          <div
            v-if="summarySource"
            class="mobile-note se-summary__source"
            data-testid="se-download-source">
            {{ summarySource }}
          </div>
          <div v-if="downloaderNote" class="mobile-note se-warn" data-testid="se-downloader-note">
            {{ downloaderNote }}
          </div>
          <el-button text data-testid="se-download-toggle" @click="showDownload = !showDownload">{{
            showDownload ? t("收起", "Hide") : t("修改", "Change")
          }}</el-button>
        </div>
        <div v-if="showDownload" class="se-fields" data-testid="se-download-fields">
          <el-form-item :label="t('下载器', 'Downloader')">
            <el-select
              v-model="form.downloader_id"
              data-testid="se-downloader"
              @change="touch('downloader_id')">
              <el-option :value="0" :label="t('默认', 'Default')" />
              <el-option
                v-for="d in options.downloaders"
                :key="d.id"
                :value="d.id"
                :label="downloaderLabel(d)" />
            </el-select>
            <p v-if="noteOf('downloader_id')" class="mobile-note se-fieldnote">
              {{ noteOf("downloader_id") }}
            </p>
            <el-button
              v-if="canReset('downloader_id')"
              text
              @click="resetDownloadField('downloader_id')"
              >{{ resetLabel }}</el-button
            >
          </el-form-item>
          <el-form-item :label="t('保存目录', 'Save path')">
            <el-autocomplete
              v-model="form.save_path"
              :fetch-suggestions="suggestPaths"
              clearable
              :debounce="0"
              :placeholder="t('空着按下载器的默认目录', 'Empty: downloader default')"
              data-testid="se-save-path"
              @input="touch('save_path')"
              @select="touch('save_path')"
              @clear="touch('save_path')">
              <template #default="{ item }">
                <div class="se-path">
                  <span>{{ item.alias || item.value }}</span>
                  <span v-if="item.isDefault" class="se-path__tag">{{ t("默认", "Default") }}</span>
                  <span v-if="item.downloader" class="se-path__tag">{{ item.downloader }}</span>
                </div>
                <div v-if="item.alias && item.alias !== item.value" class="se-path__full">
                  {{ item.value }}
                </div>
              </template>
            </el-autocomplete>
            <p v-if="noteOf('save_path')" class="mobile-note se-fieldnote">
              {{ noteOf("save_path") }}
            </p>
            <el-button v-if="canReset('save_path')" text @click="resetDownloadField('save_path')">{{
              resetLabel
            }}</el-button>
          </el-form-item>
          <el-form-item :label="t('分类', 'Category')">
            <el-input
              v-model="form.category"
              data-testid="se-category"
              @input="touch('category')" />
            <p v-if="noteOf('category')" class="mobile-note se-fieldnote">
              {{ noteOf("category") }}
            </p>
            <el-button
              v-if="canReset('category')"
              text
              data-testid="se-category-reset"
              @click="resetDownloadField('category')"
              >{{ resetLabel }}</el-button
            >
          </el-form-item>
          <el-form-item :label="t('标签（逗号分隔）', 'Tags (comma separated)')">
            <el-input v-model="form.tags" data-testid="se-tags" @input="touch('tags')" />
            <p v-if="noteOf('tags')" class="mobile-note se-fieldnote">{{ noteOf("tags") }}</p>
            <el-button v-if="canReset('tags')" text @click="resetDownloadField('tags')">{{
              resetLabel
            }}</el-button>
          </el-form-item>
        </div>
      </section>

      <!-- 3 怎么找 -->
      <section class="mobile-section" data-testid="se-search">
        <h2>{{ t("怎么找", "How to search") }}</h2>
        <KeywordTags
          v-model="form.include_keywords"
          :label="t('标题包含（都要有其中一个）', 'Title includes (any of them)')"
          :placeholder="t('加一个词，例如 CHDWEB', 'Add a word, e.g. CHDWEB')"
          testid="se-include"
          :disabled="!canWrite || saving" />
        <KeywordTags
          v-model="form.exclude_keywords"
          :label="t('标题排除（有一个就不要）', 'Title excludes (none of them)')"
          :placeholder="t('加一个词，例如 HDTV', 'Add a word, e.g. HDTV')"
          testid="se-exclude"
          :disabled="!canWrite || saving" />
        <el-form-item :label="t('质量档案', 'Quality profile')"
          ><el-select v-model="form.profile_id"
            ><el-option :value="0" :label="t('默认', 'Default')" /><el-option
              v-for="p in options.profiles"
              :key="p.id"
              :value="p.id"
              :label="p.name" /></el-select
        ></el-form-item>
        <el-form-item :label="t('站点（不选则搜索全部）', 'Sites (empty = all)')"
          ><el-select v-model="form.sites" multiple
            ><el-option
              v-for="s in options.sites"
              :key="s.name"
              :value="s.name"
              :label="s.display_name || s.name" /></el-select
        ></el-form-item>
        <div v-if="canPreview" class="se-preview">
          <el-button data-testid="se-preview" @click="previewOpen = true">{{
            t("试搜索", "Search preview")
          }}</el-button>
          <span v-if="lastLine" class="mobile-note" data-testid="se-preview-last">{{
            lastLine
          }}</span>
        </div>
      </section>

      <!-- 4 什么时候找（电影只有允许升级） -->
      <section class="mobile-section" data-testid="se-when">
        <h2>{{ tv ? t("什么时候找", "When to search") : t("升级", "Upgrades") }}</h2>
        <template v-if="tv">
          <button
            type="button"
            class="se-row"
            :aria-expanded="showSchedule"
            data-testid="se-schedule-toggle"
            @click="showSchedule = !showSchedule">
            <span class="se-row__label">{{ t("搜索定时", "Search schedule") }}</span>
            <span class="se-row__value" data-testid="se-schedule-line">{{ scheduleLine }}</span>
          </button>
          <div v-if="showSchedule" class="se-fields" data-testid="se-schedule-fields">
            <el-form-item :label="t('搜索定时', 'Search schedule')"
              ><el-select v-model="form.schedule.kind"
                ><el-option value="" :label="t('跟随全局间隔', 'Global interval')" /><el-option
                  value="daily"
                  :label="t('每天', 'Daily')" /><el-option
                  value="every_days"
                  :label="t('每隔几天', 'Every N days')" /><el-option
                  value="weekly"
                  :label="t('每周', 'Weekly')" /><el-option
                  value="monthly"
                  :label="t('每月', 'Monthly')" /><el-option
                  value="airtime"
                  :label="t('按播出时间', 'Airtime')" /><el-option
                  value="interval"
                  :label="t('固定间隔', 'Interval')" /><el-option
                  value="cron"
                  label="Cron" /></el-select
            ></el-form-item>
            <el-form-item
              v-if="
                ['airtime', 'daily', 'every_days', 'weekly', 'monthly'].includes(form.schedule.kind)
              "
              :label="t('搜索时间（HH:mm）', 'Search time (HH:mm)')"
              ><el-input v-model="form.schedule.at" placeholder="20:00"
            /></el-form-item>
            <el-form-item
              v-if="form.schedule.kind === 'every_days'"
              :label="t('每隔几天', 'Every N days')"
              ><el-input-number v-model="form.schedule.every" :min="1" :precision="0"
            /></el-form-item>
            <el-form-item
              v-if="['weekly', 'monthly'].includes(form.schedule.kind)"
              :label="
                t(
                  form.schedule.kind === 'weekly' ? '星期（0 是周日）' : '每月日期',
                  form.schedule.kind === 'weekly' ? 'Weekdays (0 = Sunday)' : 'Days of month',
                )
              "
              ><el-select v-model="form.schedule.days" multiple
                ><el-option
                  v-for="n in form.schedule.kind === 'weekly' ? 7 : 31"
                  :key="n"
                  :value="form.schedule.kind === 'weekly' ? n - 1 : n"
                  :label="String(form.schedule.kind === 'weekly' ? n - 1 : n)" /></el-select
            ></el-form-item>
            <el-form-item
              v-if="form.schedule.kind === 'interval'"
              :label="t('间隔（分钟，30–10080）', 'Interval (minutes, 30–10080)')"
              ><el-input-number
                v-model="form.schedule.interval_minutes"
                :min="30"
                :max="10080"
                :precision="0"
            /></el-form-item>
            <el-form-item v-if="form.schedule.kind === 'cron'" label="Cron"
              ><el-input v-model="form.schedule.cron" placeholder="0 20 * * *"
            /></el-form-item>
          </div>
          <el-form-item :label="t('起始集', 'Start episode')"
            ><el-input-number
              v-model="form.start_episode"
              :min="1"
              :precision="0"
              data-testid="se-start"
          /></el-form-item>
          <div v-if="fromTorrent" class="se-from" data-testid="se-from">
            <span>{{
              t(
                `保存时从这个种子开始追：${fromTorrent.title}`,
                `Saving starts from: ${fromTorrent.title}`,
              )
            }}</span>
            <el-button text data-testid="se-unfollow" @click="unfollow">{{
              t("不从它开始", "Don't start from it")
            }}</el-button>
          </div>
          <div class="se-switch">
            <span>{{ t("锁定版本", "Lock release version") }}</span>
            <el-switch v-model="form.lock_version" data-testid="se-lock" />
          </div>
          <div v-if="form.lock_version" class="se-lockline" data-testid="se-lock-line">
            <template v-if="lockSet">
              <span>{{ t(`锁住：${lockLabel}`, `Locked: ${lockLabel}`) }}</span>
              <el-button text data-testid="se-lock-clear" @click="clearLock">{{
                t("清掉", "Clear")
              }}</el-button>
            </template>
            <span v-else class="mobile-note">{{
              t(
                "还没定版本：第一个下载成功的版本会成为锁",
                "Not set yet: the first version downloaded becomes the lock",
              )
            }}</span>
          </div>
          <div class="se-switch">
            <span>{{ t("自动追下一季", "Follow next season") }}</span>
            <el-switch v-model="form.follow_next_season" data-testid="se-next" />
          </div>
        </template>
        <div class="se-switch">
          <span>{{ t("允许升级版本", "Allow quality upgrades") }}</span>
          <el-switch v-model="form.upgrade" data-testid="se-upgrade" />
        </div>
      </section>

      <div v-if="canWrite" class="mobile-actions mobile-actions--fixed">
        <el-button type="primary" native-type="submit" :loading="saving" data-testid="se-save">{{
          t("保存", "Save")
        }}</el-button>
      </div>
    </el-form>
    <SubscriptionPreview
      v-if="canPreview"
      v-model="previewOpen"
      :request="previewRequest"
      :title="title || String(form.tmdb_id)"
      :tv="tv"
      :can-follow="!editing && tv"
      :lock="previewLock"
      :follow="fromTorrent"
      :site-count="previewSites"
      @include="(w) => addWord('include_keywords', w)"
      @exclude="(w) => addWord('exclude_keywords', w)"
      @lock="lockTo"
      @follow="followFrom"
      @summary="(s) => (lastPreview = s)" />
  </template>
</template>
<style scoped>
/* 表单自己的字（摘要、定时与开关那几行）和表单项的内容一样大（14）；不写的话按浏览器的 16，比旁边的选择框大一圈 */
.se-form {
  font-size: var(--pt-fz-body-lg);
}
/* 「查找」和下面的表单项隔开一个表单项的间距 */
.se-lookup {
  margin-bottom: 20px;
}
.se-line {
  margin-top: -8px;
}
.se-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.se-chip {
  min-height: 44px;
  padding: 0 14px;
  border: 1px solid var(--pt-border-strong);
  border-radius: var(--pt-r-sm);
  background: var(--pt-surface);
  color: var(--pt-t1);
  font: inherit;
  cursor: pointer;
}
.se-chip.is-on {
  border-color: var(--pt-p);
  background: var(--pt-p);
  color: var(--pt-on-p);
}
.se-chip:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}
.se-already,
.se-apply,
.se-from {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
  margin-top: 12px;
  padding: 10px 12px;
  border-radius: var(--pt-r-sm);
  background: var(--pt-hover);
}
.se-already span,
.se-apply span,
.se-from span {
  flex: 1 1 200px;
  min-width: 0;
  overflow-wrap: anywhere;
}
.se-already {
  color: var(--pt-warn);
}
.se-warn {
  color: var(--pt-warn);
}
.se-summary {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
}
.se-summary__text {
  color: var(--pt-t1);
  overflow-wrap: anywhere;
}
.se-summary__source {
  margin: 0;
}
.se-fields {
  margin-top: 12px;
}
.se-fieldnote {
  margin: 4px 0 0;
  width: 100%;
}
.se-path {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}
.se-path__tag {
  color: var(--pt-t3);
  font-size: var(--pt-fz-sm);
}
.se-path__full {
  color: var(--pt-t3);
  font-size: var(--pt-fz-sm);
  overflow-wrap: anywhere;
}
.se-preview {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
}
.se-preview .mobile-note {
  margin: 0;
  flex: 1 1 180px;
}
.se-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  width: 100%;
  min-height: 44px;
  margin-bottom: 12px;
  padding: 0 12px;
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-sm);
  background: var(--pt-surface);
  color: var(--pt-t1);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.se-row__label {
  color: var(--pt-t2);
}
.se-row__value {
  overflow-wrap: anywhere;
  text-align: right;
}
.se-switch {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 44px;
}
.se-lockline {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: -4px 0 4px;
  color: var(--pt-t2);
  font-size: var(--pt-fz-sm);
}
.se-lockline .mobile-note {
  margin: 0;
}
</style>
