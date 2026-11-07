<script setup lang="ts">
/*
 * 媒体库（路线图 M10）：整理设置、媒体库（目录、命名模板、整理方式、刮削）、下载器到 pt-tools 的路径映射、
 * 整理后要通知的媒体服务器。画板没有这一页，沿用媒体识别页的样式：页头 + 面板里的表单与表格，手机是行卡。
 */
import {
  type ClassifyRule,
  type ClassifyRules,
  type DownloaderSetting,
  type MediaCheckItem,
  type MediaKind,
  type MediaLibrary,
  type MediaLibraryInput,
  type MediaPathMap,
  type MediaPathMapInput,
  type MediaServer,
  type MediaServerInput,
  type MediaServerUser,
  type MediaWatchDir,
  type MediaWatchDirInput,
  type NotificationConfig,
  type OrganizeSettings,
  type OrganizeSettingsInput,
  chatopsApi,
  downloadersApi,
  mediaApi,
  organizeApi,
} from "@/api";
import DownloaderPathInput from "@/components/downloader/DownloaderPathInput.vue";
import PtIcon from "@/components/PtIcon";
import ImportLibraryDialog from "@/components/media/ImportLibraryDialog.vue";
import OrganizeDialog, { type OrganizeTarget } from "@/components/media/OrganizeDialog.vue";
import TmdbKeyNotice from "@/components/media/TmdbKeyNotice.vue";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import { useDataState } from "@/composables/useDataState";
import { useDownloaderPaths } from "@/composables/useDownloaderPaths";
import { useIsMobile } from "@/composables/useIsMobile";
import { formatShortDateTime } from "@/utils/format";
import { scopeMisses } from "@/utils/subscribeDefaults";
import {
  CLASSIFY_COUNTRIES,
  CLASSIFY_LANGUAGES,
  DEFAULT_SCRAPE_ITEMS,
  IMAGE_LANGS,
  MEDIA_MODES,
  MEDIA_SERVER_KINDS,
  OVERWRITE_MODES,
  SCRAPE_ITEMS,
  SCRAPE_SOURCE_FIELDS,
  SCRAPE_SOURCE_OPTIONS,
  TEMPLATE_VARS,
  TMDB_GENRES,
  classifyRuleSummary,
  mediaKindLabel,
  modeLabel,
  scrapeItemsFor,
  serverIndexView,
  serverKindLabel,
  syncResultText,
  templatePresets,
  tmdbSources,
} from "@/utils/media";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onMounted, ref, watch } from "vue";

const isMobile = useIsMobile();
const errText = (e: unknown, fallback: string) => (e as Error)?.message || fallback;

// ---- 整理设置 ----
const settings = ref<OrganizeSettings | null>(null);

/** 上一次、下一次扫描的一句话（时间在内存里，重启以后从头算） */
function runText(last?: string, next?: string): string {
  return [
    last ? `上次 ${formatShortDateTime(last)}` : "",
    next ? `下次大约 ${formatShortDateTime(next)}` : "",
  ]
    .filter(Boolean)
    .join("，");
}
const scanRunText = computed(() =>
  runText(settings.value?.runs?.scan_last, settings.value?.runs?.scan_next),
);
const watchRunText = computed(() =>
  runText(settings.value?.runs?.watch_last, settings.value?.runs?.watch_next),
);
const settingsFailed = ref(false);
const savingSettings = ref(false);
const blankSettings = (): OrganizeSettingsInput => ({
  auto_enabled: false,
  scan_enabled: false,
  scan_interval_min: 60,
  downloaders: [],
  categories: [],
  tags: [],
  save_paths: [],
  min_video_mb: 50,
  notify_channels: [],
  delete_links_on_remove: false,
  watch_interval_min: 10,
  watch_realtime: true,
});
const sform = ref<OrganizeSettingsInput>(blankSettings());
const savePaths = ref<string[]>([]);
// 整理范围的保存路径可以从下载器的预设目录里选：范围里选了下载器时列它们的，没选时列全部下载器的
const { pathOptionsFor, allPathOptions } = useDownloaderPaths();
const scopePathOptions = computed(() => {
  const seen = new Set<string>();
  const opts = sform.value.downloaders?.length
    ? pathOptionsFor(sform.value.downloaders)
    : allPathOptions();
  return opts.filter((o) => !seen.has(o.path) && seen.add(o.path));
});
const downloaders = ref<DownloaderSetting[]>([]);
/** 读到过下载器列表：没读到时不知道库里存的下载器还在不在 */
const downloadersLoaded = ref(false);
const channels = ref<NotificationConfig[]>([]);

function fillSettings(s: OrganizeSettings) {
  settings.value = s;
  // 只读的字段不进表单：保存时按严格解析，带上会被拒
  const { auto_since: _since, runs: _runs, ...rest } = s;
  sform.value = { ...rest };
  savePaths.value = [...(s.save_paths ?? [])];
}

/** 只刷新扫描的上次与下次（例如开关了监控目录以后）：不动表单里还没保存的改动 */
async function refreshRuns() {
  try {
    const s = await organizeApi.settings();
    if (s && settings.value) settings.value = { ...settings.value, runs: s.runs };
  } catch {
    // 只是少一行时间
  }
}

async function loadSettings() {
  try {
    const s = await organizeApi.settings();
    if (s) fillSettings(s);
    settingsFailed.value = false;
  } catch {
    settingsFailed.value = true;
  }
}

async function loadOptions() {
  const [dls, chs] = await Promise.allSettled([
    downloadersApi.list(),
    chatopsApi.notifications.list(),
  ]);
  if (dls.status === "fulfilled") {
    downloaders.value = dls.value ?? [];
    downloadersLoaded.value = true;
  }
  if (chs.status === "fulfilled") channels.value = chs.value ?? [];
}

async function saveSettings() {
  savingSettings.value = true;
  try {
    const body: OrganizeSettingsInput = {
      ...sform.value,
      save_paths: savePaths.value.map((p) => p.trim()).filter(Boolean),
    };
    fillSettings(await organizeApi.saveSettings(body));
    ElMessage.success("已保存整理设置");
  } catch (e) {
    ElMessage.error(errText(e, "保存失败"));
  } finally {
    savingSettings.value = false;
  }
}

const downloaderName = (id: number) =>
  downloaders.value.find((d) => d.id === id)?.name ?? `下载器 ${id}`;

// ---- 媒体库 ----
const libraries = ref<MediaLibrary[]>([]);
const librariesDS = useDataState();
const libDialog = ref(false);
const libSaving = ref(false);
const editingLibID = ref<number | null>(null);
const blankLibrary = (kind: MediaKind = "movie"): MediaLibraryInput => ({
  name: "",
  kind,
  anime: false,
  path: "",
  template: "",
  mode: "hardlink",
  scrape: true,
  scrape_overwrite: false,
  scrape_items: [...DEFAULT_SCRAPE_ITEMS],
  scrape_image_lang: "",
  scrape_sources: tmdbSources(),
  classify_dir: false,
  overwrite_mode: "",
  scrape_actor_zh: false,
  scrape_fill: false,
  site_subtitles: false,
  csf_url: "",
  csf_key: "",
  csf_path: "",
  download_downloader_id: 0,
  download_save_path: "",
  download_category: "",
  download_tags: "",
  enabled: true,
});
const libForm = ref<MediaLibraryInput>(blankLibrary());
/** 正在编辑的媒体库（看它保存过 ChineseSubFinder 的 Key 没有） */
const editingLib = computed(() => libraries.value.find((l) => l.id === editingLibID.value) ?? null);
const csfTesting = ref(false);
const preview = ref({ text: "", error: "" });
const checkItems = ref<MediaCheckItem[]>([]);
const checking = ref(false);

async function loadLibraries() {
  const pending = librariesDS.run(() => organizeApi.libraries());
  const data = await pending;
  if (librariesDS.isStale(pending) || !data) return;
  libraries.value = data;
}

/** 接口只收这几个字段（严格解析）：不能把整条记录发回去 */
function toLibraryInput(l: MediaLibrary): MediaLibraryInput {
  return {
    name: l.name,
    kind: l.kind,
    anime: l.anime,
    path: l.path,
    template: l.template,
    mode: l.mode || "hardlink",
    scrape: l.scrape,
    scrape_overwrite: l.scrape_overwrite,
    scrape_items: [...(l.scrape_items ?? DEFAULT_SCRAPE_ITEMS)],
    scrape_image_lang: l.scrape_image_lang ?? "",
    scrape_sources: { ...tmdbSources(), ...l.scrape_sources },
    classify_dir: l.classify_dir ?? false,
    overwrite_mode: l.overwrite_mode ?? "",
    scrape_actor_zh: l.scrape_actor_zh ?? false,
    scrape_fill: l.scrape_fill ?? false,
    site_subtitles: l.site_subtitles ?? false,
    csf_url: l.csf_url ?? "",
    csf_path: l.csf_path ?? "",
    download_downloader_id: l.download_downloader_id ?? 0,
    download_save_path: l.download_save_path ?? "",
    download_category: l.download_category ?? "",
    download_tags: l.download_tags ?? "",
    enabled: l.enabled,
  };
}

// ---- 新建订阅时带出的下载设置 ----
/** 库里存的下载器已经删了（打开时发现的）：表单改成不指定，保存时清掉 */
const libDownloaderGone = ref(false);
/** 下载器的选项：停用的标出来；库里存的下载器不在列表里（读不到列表）时也列出来，免得只显示一个编号 */
const libDownloaderOptions = computed(() => {
  const opts = downloaders.value
    .filter((d): d is DownloaderSetting & { id: number } => typeof d.id === "number")
    .map((d) => ({ id: d.id, label: d.enabled ? d.name : `${d.name}（已停用）` }));
  const id = libForm.value.download_downloader_id;
  if (id > 0 && !opts.some((o) => o.id === id)) opts.push({ id, label: `下载器 ${id}` });
  return opts;
});
const libDownloaderDisabled = computed(() => {
  const id = libForm.value.download_downloader_id;
  return id > 0 && downloaders.value.some((d) => d.id === id && !d.enabled);
});
/** 自动整理或定期扫描开着、这几项不在整理范围里时：照这样推送的种子下完不会自动整理 */
const libScopeMisses = computed(() => {
  const s = settings.value;
  if (!s || !(s.auto_enabled || s.scan_enabled)) return [];
  const f = libForm.value;
  return scopeMisses(s, {
    downloader_id: f.download_downloader_id,
    save_path: f.download_save_path,
    category: f.download_category,
    tags: f.download_tags,
  });
});

// ---- 导入已有文件 ----
const importVisible = ref(false);
const importLibrary = ref<MediaLibrary | null>(null);
function openImport(l: MediaLibrary) {
  importLibrary.value = l;
  importVisible.value = true;
}

function openLibrary(l?: MediaLibrary) {
  editingLibID.value = l?.id ?? null;
  libForm.value = l
    ? toLibraryInput(l)
    : blankLibrary(libraries.value.some((x) => x.kind === "movie") ? "tv" : "movie");
  // 下载器删了以后库里还存着它的编号：显示「不指定」并写明，保存时清掉（读不到下载器列表时不动）
  const dlID = libForm.value.download_downloader_id;
  libDownloaderGone.value =
    downloadersLoaded.value && dlID > 0 && !downloaders.value.some((d) => d.id === dlID);
  if (libDownloaderGone.value) libForm.value.download_downloader_id = 0;
  checkItems.value = [];
  libDialog.value = true;
  void refreshPreview();
  void loadSourceKeys();
}

/** 「媒体识别」里填了 Fanart.tv 与 OMDb 的 Key 没有（读不到时为 null，不提示） */
const sourceKeys = ref<{ fanart: boolean; omdb: boolean } | null>(null);
async function loadSourceKeys() {
  try {
    const s = await mediaApi.settings();
    sourceKeys.value = s ? { fanart: s.has_fanart_key, omdb: s.has_omdb_key } : null;
  } catch {
    sourceKeys.value = null;
  }
}

/** 这种媒体库的数据来源项：电影没有「每一集」 */
const sourceFields = computed(() =>
  SCRAPE_SOURCE_FIELDS.filter((f) => libForm.value.kind === "tv" || !f.tvOnly),
);

/** 选了要 Key 的来源、却还没填 Key 时的提示 */
const sourceWarnings = computed(() => {
  const f = libForm.value;
  const keys = sourceKeys.value;
  if (!f.scrape || !keys) return [];
  const out: string[] = [];
  const fanartItems = SCRAPE_ITEMS.filter(
    (i) => i.fanartOnly && f.scrape_items.includes(i.value) && (f.kind === "movie" || !i.movieOnly),
  ).map((i) => i.label);
  if (!keys.fanart && (f.scrape_sources.artwork === "fanart" || fanartItems.length)) {
    // 最后一项是英文名（Clearart）时和后面的中文空一格
    const items = fanartItems.join("、");
    const lead = /[A-Za-z0-9]$/.test(items) ? `${items} ` : items;
    out.push(
      `${fanartItems.length ? `${lead}只有 Fanart.tv 有，` : "图片选了 Fanart.tv，"}还没在「媒体识别」里填 Fanart.tv 的 Key：${fanartItems.length ? "这几项不写" : "用 TMDB 的"}`,
    );
  }
  if (!keys.omdb && f.scrape_sources.rating === "omdb") {
    out.push("评分选了 IMDb，还没在「媒体识别」里填 OMDb 的 Key：用 TMDB 的");
  }
  return out;
});

let previewSeq = 0;
/** 上一次预览的类型与模板：打开弹窗时已经预览过，watch 再触发一次时不重复请求 */
let previewKey = "";
async function refreshPreview() {
  const seq = ++previewSeq;
  const kind = libForm.value.kind;
  const template = libForm.value.template.trim();
  previewKey = `${kind}\n${template}`;
  try {
    const r = await organizeApi.templatePreview({ kind, template });
    if (seq !== previewSeq) return;
    preview.value = { text: r.preview, error: r.error ?? "" };
  } catch (e) {
    if (seq !== previewSeq) return;
    preview.value = { text: "", error: errText(e, "预览失败") };
  }
}

let previewTimer: ReturnType<typeof setTimeout> | undefined;
watch(
  () => [libForm.value.template, libForm.value.kind] as const,
  () => {
    if (
      !libDialog.value ||
      `${libForm.value.kind}\n${libForm.value.template.trim()}` === previewKey
    )
      return;
    clearTimeout(previewTimer);
    previewTimer = setTimeout(() => void refreshPreview(), 300);
  },
);

const presets = computed(() => templatePresets(libForm.value.kind));
/** 模板变量的写法（{{.Title}}）：插值里写不了成对的花括号，用 v-text 显示 */
const braces = (expr: string) => `{{${expr}}}`;
const modeHint = computed(
  () => MEDIA_MODES.find((m) => m.value === libForm.value.mode)?.hint ?? "",
);

async function checkLibrary() {
  if (!libForm.value.path.trim()) {
    ElMessage.warning("先填写库目录");
    return;
  }
  checking.value = true;
  try {
    checkItems.value = await organizeApi.checkLibrary({
      path: libForm.value.path.trim(),
      mode: libForm.value.mode,
    });
  } catch (e) {
    checkItems.value = [{ name: "检查", ok: false, message: errText(e, "检查失败") }];
  } finally {
    checking.value = false;
  }
}

async function saveLibrary() {
  const f = libForm.value;
  if (!f.name.trim() || !f.path.trim()) {
    ElMessage.warning("填写名称与库目录");
    return;
  }
  const items = scrapeItemsFor(f.kind).map((i) => i.value);
  if (f.scrape && !f.scrape_items.some((i) => items.includes(i))) {
    ElMessage.warning("刮削内容至少选一项；不想刮削就关掉刮削");
    return;
  }
  const csfURL = f.csf_url.trim();
  const csfKey = f.csf_key?.trim() ?? "";
  if (csfURL && !csfKey && !editingLib.value?.has_csf_key) {
    ElMessage.warning("填了 ChineseSubFinder 的地址就要填它的 API Key");
    return;
  }
  libSaving.value = true;
  try {
    const body: MediaLibraryInput = {
      ...f,
      name: f.name.trim(),
      path: f.path.trim(),
      template: f.template.trim(),
      csf_url: csfURL,
      csf_path: f.csf_path.trim(),
      download_save_path: f.download_save_path.trim(),
      download_category: f.download_category.trim(),
      download_tags: f.download_tags.trim(),
    };
    // Key 只在填了时带上：不带时保留保存的
    delete body.csf_key;
    if (csfKey) body.csf_key = csfKey;
    if (editingLibID.value) await organizeApi.updateLibrary(editingLibID.value, body);
    else await organizeApi.createLibrary(body);
    ElMessage.success("已保存媒体库");
    libDialog.value = false;
    await loadLibraries();
  } catch (e) {
    ElMessage.error(errText(e, "保存失败"));
  } finally {
    libSaving.value = false;
  }
}

/** 访问一次 ChineseSubFinder（不建任务）：没填 Key 时用这个库保存的 */
async function testCSF() {
  const url = libForm.value.csf_url.trim();
  if (!url) {
    ElMessage.warning("先填 ChineseSubFinder 的地址");
    return;
  }
  csfTesting.value = true;
  try {
    await organizeApi.testCSF({
      id: editingLibID.value ?? undefined,
      url,
      key: libForm.value.csf_key?.trim() || undefined,
    });
    ElMessage.success("ChineseSubFinder 连接正常");
  } catch (e) {
    ElMessage.error(errText(e, "连接失败"));
  } finally {
    csfTesting.value = false;
  }
}

async function toggleLibrary(l: MediaLibrary, enabled: boolean) {
  try {
    await organizeApi.updateLibrary(l.id, { ...toLibraryInput(l), enabled });
  } catch (e) {
    ElMessage.error(errText(e, "保存失败"));
  }
  await loadLibraries();
}

async function removeLibrary(l: MediaLibrary) {
  try {
    await ElMessageBox.confirm(
      `删除媒体库「${l.name}」？库里的文件与整理记录都不动。`,
      "删除媒体库",
      {
        type: "warning",
        confirmButtonText: "删除",
        cancelButtonText: "取消",
      },
    );
  } catch {
    return;
  }
  try {
    await organizeApi.deleteLibrary(l.id);
    ElMessage.success("已删除");
    await loadLibraries();
  } catch (e) {
    ElMessage.error(errText(e, "删除失败"));
  }
}

const libKindText = (l: Pick<MediaLibrary, "kind" | "anime">) =>
  `${mediaKindLabel(l.kind)}${l.anime ? " · 只收动画" : ""}`;

// ---- 二级分类 ----
const CLASSIFY_KINDS: readonly { kind: MediaKind; label: string }[] = [
  { kind: "movie", label: "电影" },
  { kind: "tv", label: "剧集" },
];
const classify = ref<ClassifyRules>({ movie: [], tv: [] });
const classifyPresets = ref<ClassifyRules>({ movie: [], tv: [] });
const classifyDS = useDataState();
const classifySaving = ref(false);
const ruleDialog = ref(false);
const ruleKind = ref<MediaKind>("movie");
const ruleIndex = ref<number | null>(null);
const ruleForm = ref<Required<ClassifyRule>>({
  name: "",
  genres: [],
  languages: [],
  countries: [],
  max_runtime: 0,
  keywords: [],
});
const ruleTitle = computed(
  () => `${ruleIndex.value === null ? "添加" : "编辑"}${mediaKindLabel(ruleKind.value)}分类`,
);

async function loadClassify() {
  const pending = classifyDS.run(() => organizeApi.classify());
  const data = await pending;
  if (classifyDS.isStale(pending) || !data) return;
  classify.value = data.rules;
  classifyPresets.value = data.presets;
}

/** 整份保存二级分类（接口按整份替换）；成功时换成服务端规整后的 */
async function saveClassify(next: ClassifyRules, done: string): Promise<boolean> {
  classifySaving.value = true;
  try {
    const v = await organizeApi.saveClassify(next);
    classify.value = v.rules;
    ElMessage.success(done);
    return true;
  } catch (e) {
    ElMessage.error(errText(e, "保存失败"));
    return false;
  } finally {
    classifySaving.value = false;
  }
}

function openRule(kind: MediaKind, index?: number) {
  const r = index === undefined ? undefined : classify.value[kind][index];
  ruleKind.value = kind;
  ruleIndex.value = index ?? null;
  ruleForm.value = {
    name: r?.name ?? "",
    genres: [...(r?.genres ?? [])],
    languages: [...(r?.languages ?? [])],
    countries: [...(r?.countries ?? [])],
    max_runtime: r?.max_runtime ?? 0,
    keywords: [...(r?.keywords ?? [])],
  };
  ruleDialog.value = true;
}

async function saveRule() {
  const f = ruleForm.value;
  if (!f.name.trim()) {
    ElMessage.warning("填写分类名称");
    return;
  }
  const rule: ClassifyRule = {
    name: f.name.trim(),
    genres: [...f.genres],
    languages: [...f.languages],
    countries: [...f.countries],
  };
  if (f.max_runtime) rule.max_runtime = f.max_runtime;
  const keywords = f.keywords.map((k) => k.trim()).filter(Boolean);
  if (keywords.length) rule.keywords = keywords;
  const list = [...classify.value[ruleKind.value]];
  if (ruleIndex.value === null) list.push(rule);
  else list[ruleIndex.value] = rule;
  if (await saveClassify({ ...classify.value, [ruleKind.value]: list }, "已保存分类")) {
    ruleDialog.value = false;
  }
}

async function moveRule(kind: MediaKind, index: number, delta: number) {
  const list = [...classify.value[kind]];
  const j = index + delta;
  if (j < 0 || j >= list.length) return;
  [list[index], list[j]] = [list[j]!, list[index]!];
  await saveClassify({ ...classify.value, [kind]: list }, "已调整顺序");
}

async function removeRule(kind: MediaKind, index: number) {
  const r = classify.value[kind][index];
  if (!r) return;
  try {
    await ElMessageBox.confirm(
      `删除分类「${r.name}」？已经放进这个分类目录的文件不动。`,
      "删除分类",
      { type: "warning", confirmButtonText: "删除", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  const list = classify.value[kind].filter((_, i) => i !== index);
  await saveClassify({ ...classify.value, [kind]: list }, "已删除");
}

async function fillClassifyPresets() {
  if (classify.value.movie.length || classify.value.tv.length) {
    try {
      await ElMessageBox.confirm("用常用分类替换现在的规则？", "填入常用分类", {
        type: "warning",
        confirmButtonText: "替换",
        cancelButtonText: "取消",
      });
    } catch {
      return;
    }
  }
  const presets = JSON.parse(JSON.stringify(classifyPresets.value)) as ClassifyRules;
  await saveClassify(presets, "已填入常用分类");
}

// ---- 目录监控 ----
const watchDirs = ref<MediaWatchDir[]>([]);
const watchDS = useDataState();
const watchDialog = ref(false);
const watchSaving = ref(false);
const watchForm = ref<MediaWatchDirInput>({ path: "", enabled: true });

async function loadWatchDirs() {
  const pending = watchDS.run(() => organizeApi.watchDirs());
  const data = await pending;
  if (watchDS.isStale(pending) || !data) return;
  watchDirs.value = data;
}

// 只能添加：路径不能改（排着的整理与整理记录只记着目录的编号），要换目录就删掉再添加
function openWatchDir() {
  watchForm.value = { path: "", enabled: true };
  watchDialog.value = true;
}

async function saveWatchDir() {
  const f = watchForm.value;
  if (!f.path.trim()) {
    ElMessage.warning("填写要监控的目录");
    return;
  }
  watchSaving.value = true;
  try {
    await organizeApi.createWatchDir({ ...f, path: f.path.trim() });
    ElMessage.success("已保存监控目录");
    watchDialog.value = false;
    await Promise.all([loadWatchDirs(), refreshRuns()]);
  } catch (e) {
    ElMessage.error(errText(e, "保存失败"));
  } finally {
    watchSaving.value = false;
  }
}

async function toggleWatchDir(d: MediaWatchDir, enabled: boolean) {
  try {
    await organizeApi.updateWatchDir(d.id, { path: d.path, enabled });
  } catch (e) {
    ElMessage.error(errText(e, "保存失败"));
  }
  await Promise.all([loadWatchDirs(), refreshRuns()]);
}

async function removeWatchDir(d: MediaWatchDir) {
  try {
    await ElMessageBox.confirm(
      `不再监控「${d.path}」？已经整理进库的文件与记录不动。`,
      "删除监控目录",
      {
        type: "warning",
        confirmButtonText: "删除",
        cancelButtonText: "取消",
      },
    );
  } catch {
    return;
  }
  try {
    await organizeApi.deleteWatchDir(d.id);
    ElMessage.success("已删除");
    await Promise.all([loadWatchDirs(), refreshRuns()]);
  } catch (e) {
    ElMessage.error(errText(e, "删除失败"));
  }
}

// ---- 路径映射 ----
const pathMaps = ref<MediaPathMap[]>([]);
const mapsDS = useDataState();
const mapDialog = ref(false);
const mapSaving = ref(false);
const editingMapID = ref<number | null>(null);
const mapForm = ref<MediaPathMapInput>({
  downloader_id: 0,
  downloader_prefix: "",
  local_prefix: "",
});

async function loadPathMaps() {
  const pending = mapsDS.run(() => organizeApi.pathMaps());
  const data = await pending;
  if (mapsDS.isStale(pending) || !data) return;
  pathMaps.value = data;
}

function openPathMap(m?: MediaPathMap) {
  editingMapID.value = m?.id ?? null;
  mapForm.value = m
    ? {
        downloader_id: m.downloader_id,
        downloader_prefix: m.downloader_prefix,
        local_prefix: m.local_prefix,
      }
    : { downloader_id: downloaders.value[0]?.id ?? 0, downloader_prefix: "", local_prefix: "" };
  mapDialog.value = true;
}

async function savePathMap() {
  const f = mapForm.value;
  if (!f.downloader_id || !f.downloader_prefix.trim() || !f.local_prefix.trim()) {
    ElMessage.warning("选择下载器并填写两边的路径");
    return;
  }
  mapSaving.value = true;
  try {
    const body = {
      ...f,
      downloader_prefix: f.downloader_prefix.trim(),
      local_prefix: f.local_prefix.trim(),
    };
    if (editingMapID.value) await organizeApi.updatePathMap(editingMapID.value, body);
    else await organizeApi.createPathMap(body);
    ElMessage.success("已保存路径映射");
    mapDialog.value = false;
    await loadPathMaps();
  } catch (e) {
    ElMessage.error(errText(e, "保存失败"));
  } finally {
    mapSaving.value = false;
  }
}

async function removePathMap(m: MediaPathMap) {
  try {
    await ElMessageBox.confirm(`删除「${m.downloader_prefix}」的路径映射？`, "删除路径映射", {
      type: "warning",
      confirmButtonText: "删除",
      cancelButtonText: "取消",
    });
  } catch {
    return;
  }
  try {
    await organizeApi.deletePathMap(m.id);
    ElMessage.success("已删除");
    await loadPathMaps();
  } catch (e) {
    ElMessage.error(errText(e, "删除失败"));
  }
}

// ---- 媒体服务器 ----
const servers = ref<MediaServer[]>([]);
const serversDS = useDataState();
const serverDialog = ref(false);
const serverSaving = ref(false);
const serverTesting = ref(false);
const editingServer = ref<MediaServer | null>(null);
const blankServer = (): Required<MediaServerInput> => ({
  name: "",
  kind: "emby",
  url: "",
  token: "",
  enabled: true,
  refresh_mode: "path",
  local_prefix: "",
  server_prefix: "",
  play_url: "",
  index_hours: 6,
  count_library: false,
  resume_user_id: "",
  resume_user_name: "",
  playback_channels: [],
});
const serverForm = ref<Required<MediaServerInput>>(blankServer());
/** 每台服务器继续观看的条数（选了用户的 Emby、Jellyfin）；没取到的不写 */
const resumeCounts = ref<Record<number, number>>({});

async function loadServers() {
  const pending = serversDS.run(() => organizeApi.servers());
  const data = await pending;
  if (serversDS.isStale(pending) || !data) return;
  servers.value = data;
  if (editingServer.value)
    editingServer.value = data.find((x) => x.id === editingServer.value?.id) ?? editingServer.value;
  void loadResumeCounts(data);
}

async function loadResumeCounts(list: MediaServer[]) {
  if (!list.some((x) => x.enabled && x.kind !== "plex" && x.resume_user_id)) {
    resumeCounts.value = {};
    return;
  }
  try {
    const { servers: groups } = await organizeApi.resume();
    resumeCounts.value = Object.fromEntries(
      groups.filter((g) => !g.error).map((g) => [g.server_id, g.items.length]),
    );
  } catch {
    resumeCounts.value = {};
  }
}

function openServer(s?: MediaServer) {
  editingServer.value = s ?? null;
  webhookURL.value = "";
  serverUsers.value = s?.resume_user_id
    ? [{ id: s.resume_user_id, name: s.resume_user_name || s.resume_user_id }]
    : [];
  serverForm.value = s
    ? {
        name: s.name,
        kind: s.kind,
        url: s.url,
        token: "",
        enabled: s.enabled,
        refresh_mode: s.refresh_mode || "path",
        local_prefix: s.local_prefix,
        server_prefix: s.server_prefix,
        play_url: s.play_url,
        index_hours: s.index_hours,
        count_library: s.count_library,
        resume_user_id: s.resume_user_id,
        resume_user_name: s.resume_user_name,
        playback_channels: [...s.playback_channels],
      }
    : blankServer();
  serverDialog.value = true;
}

function serverBody(): MediaServerInput {
  const { token, ...rest } = serverForm.value;
  const plex = rest.kind === "plex";
  const body: MediaServerInput = {
    ...rest,
    name: rest.name.trim(),
    url: rest.url.trim(),
    local_prefix: rest.local_prefix.trim(),
    server_prefix: rest.server_prefix.trim(),
    play_url: rest.play_url.trim(),
    // Plex 没有继续观看
    resume_user_id: plex ? "" : rest.resume_user_id,
    resume_user_name: plex ? "" : rest.resume_user_name,
  };
  if (token.trim()) body.token = token.trim();
  return body;
}

// 继续观看的用户：从服务器读（请求同测试连接）
const serverUsers = ref<MediaServerUser[]>([]);
const usersLoading = ref(false);
async function loadServerUsers() {
  const f = serverForm.value;
  if (!f.url.trim()) {
    ElMessage.warning("先填地址");
    return;
  }
  usersLoading.value = true;
  try {
    serverUsers.value = await organizeApi.serverUsers({
      id: editingServer.value?.id,
      kind: f.kind,
      url: f.url.trim(),
      token: f.token.trim() || undefined,
    });
    if (!serverUsers.value.length) ElMessage.info("服务器上没有用户");
  } catch (e) {
    ElMessage.error(errText(e, "读取用户失败"));
  } finally {
    usersLoading.value = false;
  }
}
function pickResumeUser(id: string) {
  serverForm.value.resume_user_id = id || "";
  serverForm.value.resume_user_name = serverUsers.value.find((u) => u.id === id)?.name ?? "";
}

// Webhook：令牌只在生成的这一次给，地址显示在弹窗里
const webhookURL = ref("");
const webhookBusy = ref(false);
async function createWebhook() {
  const s = editingServer.value;
  if (!s) return;
  if (s.webhook) {
    try {
      await ElMessageBox.confirm(
        "重新生成以后，原来的地址马上作废，要在媒体服务器里换成新的地址。",
        "重新生成 Webhook 地址",
        { type: "warning", confirmButtonText: "重新生成", cancelButtonText: "取消" },
      );
    } catch {
      return;
    }
  }
  webhookBusy.value = true;
  try {
    const got = await organizeApi.createWebhook(s.id);
    webhookURL.value = `${window.location.origin}${got.path}`;
    await loadServers();
  } catch (e) {
    ElMessage.error(errText(e, "生成失败"));
  } finally {
    webhookBusy.value = false;
  }
}
async function deleteWebhook() {
  const s = editingServer.value;
  if (!s) return;
  try {
    await ElMessageBox.confirm("关掉以后媒体服务器发来的 Webhook 一律不认。", "关掉 Webhook", {
      type: "warning",
      confirmButtonText: "关掉",
      cancelButtonText: "取消",
    });
  } catch {
    return;
  }
  webhookBusy.value = true;
  try {
    await organizeApi.deleteWebhook(s.id);
    webhookURL.value = "";
    ElMessage.success("已关掉 Webhook");
    await loadServers();
  } catch (e) {
    ElMessage.error(errText(e, "关掉失败"));
  } finally {
    webhookBusy.value = false;
  }
}
async function copyWebhook() {
  try {
    await navigator.clipboard.writeText(webhookURL.value);
    ElMessage.success("已复制");
  } catch {
    ElMessage.warning("没能复制，请手动选中复制");
  }
}

// 马上同步索引：等同步做完（大的库要几分钟）
const syncingServer = ref(0);
async function syncServer(s: MediaServer) {
  syncingServer.value = s.id;
  try {
    ElMessage.success(syncResultText(await organizeApi.syncServer(s.id)));
  } catch (e) {
    ElMessage.error(errText(e, "同步失败"));
  } finally {
    syncingServer.value = 0;
    await loadServers();
  }
}

async function saveServer() {
  const f = serverForm.value;
  if (!f.name.trim() || !f.url.trim()) {
    ElMessage.warning("填写名称与地址");
    return;
  }
  if (!editingServer.value && !f.token.trim()) {
    ElMessage.warning("填写 API Key 或 Token");
    return;
  }
  serverSaving.value = true;
  try {
    if (editingServer.value) await organizeApi.updateServer(editingServer.value.id, serverBody());
    else await organizeApi.createServer(serverBody());
    ElMessage.success("已保存媒体服务器");
    serverDialog.value = false;
    await loadServers();
  } catch (e) {
    ElMessage.error(errText(e, "保存失败"));
  } finally {
    serverSaving.value = false;
  }
}

async function testServer(s?: MediaServer) {
  const f = s
    ? { id: s.id, kind: s.kind, url: s.url }
    : {
        id: editingServer.value?.id,
        kind: serverForm.value.kind,
        url: serverForm.value.url.trim(),
        token: serverForm.value.token.trim() || undefined,
      };
  serverTesting.value = true;
  try {
    const info = await organizeApi.testServer(f);
    const libs = info.libraries !== undefined ? `，${info.libraries} 个媒体库` : "";
    ElMessage.success(`连接正常：${info.name || serverKindLabel(f.kind)} ${info.version}${libs}`);
  } catch (e) {
    ElMessage.error(errText(e, "连接失败"));
  } finally {
    serverTesting.value = false;
  }
}

async function toggleServer(s: MediaServer, enabled: boolean) {
  try {
    await organizeApi.updateServer(s.id, {
      name: s.name,
      kind: s.kind,
      url: s.url,
      enabled,
      refresh_mode: s.refresh_mode || "path",
      local_prefix: s.local_prefix,
      server_prefix: s.server_prefix,
    });
  } catch (e) {
    ElMessage.error(errText(e, "保存失败"));
  }
  await loadServers();
}

async function removeServer(s: MediaServer) {
  try {
    await ElMessageBox.confirm(`删除媒体服务器「${s.name}」？`, "删除媒体服务器", {
      type: "warning",
      confirmButtonText: "删除",
      cancelButtonText: "取消",
    });
  } catch {
    return;
  }
  try {
    await organizeApi.deleteServer(s.id);
    ElMessage.success("已删除");
    await loadServers();
  } catch (e) {
    ElMessage.error(errText(e, "删除失败"));
  }
}

const refreshText = (mode: string) => (mode === "library" ? "刷新整个媒体库" : "只扫整理到的目录");

function refresh() {
  void Promise.all([
    loadSettings(),
    loadOptions(),
    loadLibraries(),
    loadClassify(),
    loadWatchDirs(),
    loadPathMaps(),
    loadServers(),
  ]);
}

// ---- 手动整理：指定一个文件或目录（不用加监控目录），放进所选的媒体库 ----
const organizeVisible = ref(false);
const organizeTarget = ref<OrganizeTarget | null>(null);
function openManual() {
  organizeTarget.value = { source_path: "" };
  organizeVisible.value = true;
}

onMounted(refresh);
</script>

<template>
  <div class="pt-cards pt-cards--wide">
    <PtHeadSub
      >把下载完的电影与剧集按模板放进媒体库，写好 NFO 与海报，再通知媒体服务器扫描</PtHeadSub
    >
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button data-testid="ml-manual" @click="openManual">
        <PtIcon name="folder-open" :size="15" /><span>手动整理</span>
      </el-button>
      <el-button data-testid="ml-refresh" @click="refresh">
        <PtIcon name="refresh-cw" :size="15" /><span>刷新</span>
      </el-button>
    </Teleport>
    <TmdbKeyNotice class="pt-cards__full" usage="整理入库的识别与刮削" />

    <div class="pt-cols">
      <div class="pt-col">
        <!--
          媒体库（这一页的主角与「添加」入口）放在整理设置前面：1600 以下是单栏，整理设置那张长表单
          排在前面时，进页面要先滚过一屏设置才看得到有哪些库。
        -->
        <PtPanel
          title="媒体库"
          icon="folder-open"
          :count="libraries.length"
          action="添加"
          action-icon="plus"
          @action="openLibrary()">
          <p class="ml-tip ml-tip--top">
            每个库收一类条目：电影或剧集，可以只收动画（动画优先进只收动画的库）。目录要填 pt-tools
            里看到的路径， Docker 里要把媒体库挂进容器。
          </p>
          <div
            v-if="libraries.length && librariesDS.error.value"
            class="pt-note pt-note--warn ml-stale"
            data-testid="ml-libs-stale">
            <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
            <span>刷新失败：{{ librariesDS.errorText.value }}。下面是上次读到的。</span>
          </div>
          <PtDataState
            v-if="!libraries.length"
            :state="librariesDS.state.value"
            :title="librariesDS.state.value === 'empty' ? '还没有媒体库' : ''"
            :sub="
              librariesDS.errorText.value ||
              (librariesDS.state.value === 'empty' ? '点右上角「添加」，电影与剧集各建一个' : '')
            "
            data-testid="ml-libs-state" />
          <el-table
            v-else-if="!isMobile"
            :data="libraries"
            row-key="id"
            class="pt-grid"
            data-testid="ml-libs">
            <el-table-column label="名称" min-width="120" prop="name" class-name="pt-cell-strong" />
            <el-table-column label="类型" width="130">
              <template #default="{ row }">{{ libKindText(row) }}</template>
            </el-table-column>
            <el-table-column
              label="目录与命名"
              min-width="300"
              class-name="pt-cell-1line"
              show-overflow-tooltip>
              <template #default="{ row }">
                <div class="ml-path">{{ row.path }}</div>
                <div class="ml-sub">{{ row.preview }}</div>
              </template>
            </el-table-column>
            <el-table-column label="整理方式" width="96">
              <template #default="{ row }">{{ modeLabel(row.mode) }}</template>
            </el-table-column>
            <el-table-column label="刮削" width="80">
              <template #default="{ row }">
                <PtTag v-if="row.scrape">{{ row.scrape_overwrite ? "覆盖" : "开" }}</PtTag>
                <span v-else class="ml-sub">关</span>
              </template>
            </el-table-column>
            <el-table-column label="启用" width="76">
              <template #default="{ row }">
                <el-switch
                  :model-value="row.enabled"
                  :aria-label="`启用媒体库 ${row.name}`"
                  :data-testid="`ml-lib-enabled-${row.id}`"
                  @update:model-value="
                    (v: string | number | boolean) => toggleLibrary(row, Boolean(v))
                  " />
              </template>
            </el-table-column>
            <el-table-column label="" width="210" align="right">
              <template #default="{ row }">
                <el-button link :data-testid="`ml-lib-import-${row.id}`" @click="openImport(row)"
                  >导入已有文件</el-button
                >
                <el-button link :data-testid="`ml-lib-edit-${row.id}`" @click="openLibrary(row)"
                  >编辑</el-button
                >
                <el-button
                  link
                  type="danger"
                  :data-testid="`ml-lib-del-${row.id}`"
                  @click="removeLibrary(row)"
                  >删除</el-button
                >
              </template>
            </el-table-column>
          </el-table>
          <div v-else class="ml-cards">
            <PtRowCard v-for="l in libraries" :key="l.id">
              <template #title>{{ l.name }}</template>
              <template #meta>
                <span
                  >{{ libKindText(l) }} · {{ modeLabel(l.mode)
                  }}{{ l.scrape ? " · 刮削" : "" }}</span
                >
                <span class="ml-full">{{ l.path }}</span>
                <span class="ml-full">{{ l.preview }}</span>
              </template>
              <template #status>
                <el-switch
                  :model-value="l.enabled"
                  :aria-label="`启用媒体库 ${l.name}`"
                  @update:model-value="
                    (v: string | number | boolean) => toggleLibrary(l, Boolean(v))
                  " />
              </template>
              <template #actions>
                <el-button size="small" @click="openImport(l)">导入已有文件</el-button>
                <el-button size="small" @click="openLibrary(l)">编辑</el-button>
                <el-button size="small" type="danger" plain @click="removeLibrary(l)"
                  >删除</el-button
                >
              </template>
            </PtRowCard>
          </div>
        </PtPanel>

        <PtPanel title="整理设置" icon="sliders-horizontal">
          <div v-if="settingsFailed" class="pt-note pt-note--warn">
            <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
            <span>设置没读到，点刷新重试。</span>
          </div>
          <el-form class="pt-form ml-form" label-position="top" @submit.prevent>
            <el-form-item label="自动整理">
              <div class="ml-switch">
                <el-switch v-model="sform.auto_enabled" data-testid="ml-auto" />
                <span>pt-tools 推送的种子下载完成后自动整理</span>
              </div>
              <div v-if="settings?.auto_since && sform.auto_enabled" class="field-tip">
                只整理 {{ formatShortDateTime(settings.auto_since) }} 以后下载完成的种子
              </div>
            </el-form-item>
            <el-form-item label="定期扫描下载器">
              <div class="ml-switch">
                <el-switch v-model="sform.scan_enabled" data-testid="ml-scan" />
                <span>按下面的范围整理下载器里已完成的种子（不是 pt-tools 推送的也整理）</span>
              </div>
              <div v-if="sform.scan_enabled" class="ml-inline">
                <span>每</span>
                <el-input-number
                  v-model="sform.scan_interval_min"
                  :min="10"
                  :max="1440"
                  controls-position="right"
                  data-testid="ml-scan-interval"
                  aria-label="扫描间隔（分钟）" />
                <span>分钟一次</span>
              </div>
              <div
                v-if="sform.scan_enabled && scanRunText"
                class="field-tip"
                data-testid="ml-scan-runs">
                {{ scanRunText }}
              </div>
            </el-form-item>
            <el-form-item label="目录监控">
              <div class="ml-switch">
                <el-switch v-model="sform.watch_realtime" data-testid="ml-watch-realtime" />
                <span>实时监控：目录里有改动时不等扫描，改动停了 2 分钟就整理</span>
              </div>
              <div class="ml-inline ml-inline--grow">
                <span>每</span>
                <el-input-number
                  v-model="sform.watch_interval_min"
                  :min="1"
                  :max="1440"
                  controls-position="right"
                  data-testid="ml-watch-interval"
                  aria-label="目录监控的扫描间隔（分钟）" />
                <span>{{
                  sform.watch_realtime
                    ? "分钟再扫一遍开着的目录（实时监控漏掉的照样整理）"
                    : "分钟扫一次下面「目录监控」里开着的目录"
                }}</span>
              </div>
              <div
                v-if="watchDirs.some((d) => d.enabled) && watchRunText"
                class="field-tip"
                data-testid="ml-watch-runs">
                {{ watchRunText }}
              </div>
            </el-form-item>
            <div class="ml-row">
              <el-form-item label="范围：下载器（不选时不限）">
                <el-select
                  v-model="sform.downloaders"
                  multiple
                  clearable
                  placeholder="全部下载器"
                  data-testid="ml-scope-dl">
                  <el-option
                    v-for="d in downloaders"
                    :key="d.id"
                    :label="d.name"
                    :value="d.id as number" />
                </el-select>
              </el-form-item>
              <el-form-item label="范围：分类（不填时不限）">
                <el-select
                  v-model="sform.categories"
                  multiple
                  filterable
                  allow-create
                  default-first-option
                  :reserve-keyword="false"
                  placeholder="输入后回车，如 movies"
                  data-testid="ml-scope-cat" />
              </el-form-item>
              <el-form-item label="范围：标签（不填时不限）">
                <el-select
                  v-model="sform.tags"
                  multiple
                  filterable
                  allow-create
                  default-first-option
                  :reserve-keyword="false"
                  placeholder="输入后回车"
                  data-testid="ml-scope-tag" />
              </el-form-item>
              <el-form-item label="范围：保存路径（下载器里看到的，不填时不限）">
                <el-select
                  v-model="savePaths"
                  multiple
                  filterable
                  allow-create
                  default-first-option
                  :reserve-keyword="false"
                  placeholder="选预设目录，或输入后回车"
                  data-testid="ml-scope-path">
                  <el-option
                    v-for="o in scopePathOptions"
                    :key="o.path"
                    :label="o.alias && o.alias !== o.path ? `${o.alias}（${o.path}）` : o.path"
                    :value="o.path" />
                </el-select>
              </el-form-item>
            </div>
            <div class="ml-row">
              <el-form-item label="视频最小体积（MB）">
                <el-input-number
                  v-model="sform.min_video_mb"
                  :min="1"
                  :max="10240"
                  controls-position="right"
                  data-testid="ml-min-mb" />
                <div class="field-tip">更小的视频当作样片、花絮，不整理</div>
              </el-form-item>
              <el-form-item label="入库通知">
                <el-select
                  v-model="sform.notify_channels"
                  multiple
                  clearable
                  placeholder="不通知"
                  data-testid="ml-notify">
                  <el-option v-for="c in channels" :key="c.id" :label="c.name" :value="c.id" />
                </el-select>
                <div class="field-tip">新入库时发标题、季集、画质与海报链接</div>
              </el-form-item>
            </div>
            <el-form-item label="删种时一并删除入库链接">
              <div class="ml-switch">
                <el-switch v-model="sform.delete_links_on_remove" data-testid="ml-delete-links" />
                <span>种子连数据一起删掉以后，把从它整理出的硬链接与软链接也删掉</span>
              </div>
              <div class="field-tip">
                默认关闭：删种后库里的文件照样能看。打开后硬链接入库的种子才能被低空间紧急清理挑中（删掉以后空间才腾得出来）；复制与移动的文件不删。
              </div>
              <!-- 打开以后会删媒体库里的文件：后果写在开关旁边，用警示色，不和普通开关混在一起 -->
              <div
                v-if="sform.delete_links_on_remove"
                class="pt-note pt-note--warn ml-delete-warn"
                data-testid="ml-delete-links-warn">
                <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
                <span
                  >已打开：之后删种（连数据一起删）时，媒体库里从这个种子整理出来的硬链接、软链接也会被删掉，媒体服务器里对应的条目随之消失。</span
                >
              </div>
            </el-form-item>
            <el-button
              type="primary"
              :loading="savingSettings"
              data-testid="ml-save-settings"
              @click="saveSettings">
              <PtIcon v-if="!savingSettings" name="save" :size="14" /><span>保存</span>
            </el-button>
          </el-form>
        </PtPanel>
      </div>

      <div class="pt-col">
        <PtPanel
          title="二级分类"
          icon="tags"
          :count="classify.movie.length + classify.tv.length"
          action="填入常用分类"
          action-icon="sparkles"
          @action="fillClassifyPresets">
          <p class="ml-tip ml-tip--top">
            媒体库勾选「按二级分类放进子目录」以后，条目按这里的规则放进库目录下的分类目录，例如「电影/华语电影/…」。从上往下取第一条对得上的：一条里的几个条件都要满足，一个条件里选了几个时满足其中一个就行；都对不上时直接放进库目录。
          </p>
          <div
            v-if="classifyDS.error.value"
            class="pt-note pt-note--warn ml-stale"
            data-testid="ml-classify-stale">
            <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
            <span>读取失败：{{ classifyDS.errorText.value }}</span>
          </div>
          <div class="ml-classify" data-testid="ml-classify">
            <section
              v-for="k in CLASSIFY_KINDS"
              :key="k.kind"
              class="ml-classify__kind"
              :data-testid="`ml-classify-${k.kind}`">
              <div class="ml-classify__head">
                <span class="ml-classify__title">{{ k.label }}</span>
                <el-button
                  link
                  type="primary"
                  :data-testid="`ml-classify-add-${k.kind}`"
                  @click="openRule(k.kind)"
                  >添加</el-button
                >
              </div>
              <div v-if="!classify[k.kind].length" class="ml-sub">
                没有规则：{{ k.label }}不分类
              </div>
              <ol v-else class="ml-classify__list">
                <li
                  v-for="(r, i) in classify[k.kind]"
                  :key="r.name"
                  class="ml-classify__row"
                  :data-testid="`ml-rule-${k.kind}-${i}`">
                  <div class="ml-classify__text">
                    <span class="ml-classify__name">{{ r.name }}</span>
                    <span class="ml-sub">{{ classifyRuleSummary(k.kind, r) }}</span>
                  </div>
                  <div class="ml-classify__acts">
                    <el-button
                      link
                      :disabled="i === 0 || classifySaving"
                      :aria-label="`上移 ${r.name}`"
                      @click="moveRule(k.kind, i, -1)"
                      >上移</el-button
                    >
                    <el-button
                      link
                      :disabled="i === classify[k.kind].length - 1 || classifySaving"
                      :aria-label="`下移 ${r.name}`"
                      :data-testid="`ml-rule-down-${k.kind}-${i}`"
                      @click="moveRule(k.kind, i, 1)"
                      >下移</el-button
                    >
                    <el-button
                      link
                      :data-testid="`ml-rule-edit-${k.kind}-${i}`"
                      @click="openRule(k.kind, i)"
                      >编辑</el-button
                    >
                    <el-button
                      link
                      type="danger"
                      :data-testid="`ml-rule-del-${k.kind}-${i}`"
                      @click="removeRule(k.kind, i)"
                      >删除</el-button
                    >
                  </div>
                </li>
              </ol>
            </section>
          </div>
        </PtPanel>

        <PtPanel
          title="目录监控"
          icon="eye"
          :count="watchDirs.length"
          action="添加"
          action-icon="plus"
          @action="openWatchDir()">
          <p class="ml-tip ml-tip--top">
            放进这些目录的视频（不是下载器里的种子，例如手动拷进来的、别的工具下载的）按媒体库整理：目录下的每一项（一个文件，或一个文件夹）当作一个种子，按名字识别。最近
            2
            分钟里还在改动的先不整理；文件夹里后来多了几集时再整理新的那几集。不要监控下载器的下载目录（那里的种子由上面的自动整理与定期扫描处理），也不能是媒体库的目录。要马上整理某个文件或文件夹（不用加监控目录），点页面上方的「手动整理」。
          </p>
          <div
            v-if="watchDS.error.value"
            class="pt-note pt-note--warn ml-stale"
            data-testid="ml-watch-stale">
            <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
            <span>读取失败：{{ watchDS.errorText.value }}</span>
          </div>
          <!-- 和同页的媒体库、路径映射、媒体服务器一样用状态块，不再是一行灰字 -->
          <PtDataState
            v-if="!watchDirs.length"
            state="empty"
            dense
            title="还没有监控的目录"
            sub="手动拷进来的、别的工具下载的视频，所在的目录加进来以后会按媒体库整理"
            data-testid="ml-watch-empty">
            <template #action>
              <el-button size="small" @click="openWatchDir()">
                <PtIcon name="plus" :size="14" /><span>添加目录</span>
              </el-button>
            </template>
          </PtDataState>
          <ul v-else class="ml-watch" data-testid="ml-watch">
            <li
              v-for="d in watchDirs"
              :key="d.id"
              class="ml-classify__row"
              :data-testid="`ml-watch-${d.id}`">
              <div class="ml-classify__text">
                <span class="ml-path">{{ d.path }}</span>
                <span
                  v-if="d.enabled && d.realtime === 'on'"
                  class="ml-watch__rt"
                  :data-testid="`ml-watch-rt-${d.id}`"
                  >实时监控中</span
                >
                <span
                  v-else-if="d.enabled && d.realtime === 'unavailable'"
                  class="ml-watch__rt ml-watch__rt--warn"
                  :data-testid="`ml-watch-rt-${d.id}`"
                  >实时监控不可用：{{ d.realtime_note || "原因不明" }}，按间隔扫描</span
                >
              </div>
              <div class="ml-classify__acts">
                <el-switch
                  :model-value="d.enabled"
                  :aria-label="`监控 ${d.path}`"
                  :data-testid="`ml-watch-enabled-${d.id}`"
                  @update:model-value="
                    (v: string | number | boolean) => toggleWatchDir(d, Boolean(v))
                  " />
                <el-button
                  link
                  type="danger"
                  :data-testid="`ml-watch-del-${d.id}`"
                  @click="removeWatchDir(d)"
                  >删除</el-button
                >
              </div>
            </li>
          </ul>
        </PtPanel>

        <PtPanel
          title="路径映射"
          icon="route"
          :count="pathMaps.length"
          action="添加"
          action-icon="plus"
          @action="openPathMap()">
          <p class="ml-tip ml-tip--top">
            下载器与 pt-tools 看到的路径不同时（比如两者在不同的容器里）要加：下载器里的 /downloads
            在 pt-tools 里是哪个目录。 路径相同时不用加。
          </p>
          <div v-if="pathMaps.length && mapsDS.error.value" class="pt-note pt-note--warn ml-stale">
            <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
            <span>刷新失败：{{ mapsDS.errorText.value }}。下面是上次读到的。</span>
          </div>
          <PtDataState
            v-if="!pathMaps.length"
            :state="mapsDS.state.value"
            :title="mapsDS.state.value === 'empty' ? '没有路径映射' : ''"
            :sub="
              mapsDS.errorText.value ||
              (mapsDS.state.value === 'empty' ? '下载器与 pt-tools 看到的路径相同时不用加' : '')
            "
            data-testid="ml-maps-state" />
          <el-table
            v-else-if="!isMobile"
            :data="pathMaps"
            row-key="id"
            class="pt-grid"
            data-testid="ml-maps">
            <el-table-column label="下载器" width="160">
              <template #default="{ row }">{{ downloaderName(row.downloader_id) }}</template>
            </el-table-column>
            <el-table-column
              label="下载器里的路径"
              min-width="200"
              prop="downloader_prefix"
              class-name="pt-cell-1line"
              show-overflow-tooltip />
            <el-table-column
              label="pt-tools 里的路径"
              min-width="200"
              prop="local_prefix"
              class-name="pt-cell-1line"
              show-overflow-tooltip />
            <el-table-column label="" width="120" align="right">
              <template #default="{ row }">
                <el-button link :data-testid="`ml-map-edit-${row.id}`" @click="openPathMap(row)"
                  >编辑</el-button
                >
                <el-button
                  link
                  type="danger"
                  :data-testid="`ml-map-del-${row.id}`"
                  @click="removePathMap(row)"
                  >删除</el-button
                >
              </template>
            </el-table-column>
          </el-table>
          <div v-else class="ml-cards">
            <PtRowCard v-for="m in pathMaps" :key="m.id">
              <template #title>{{ downloaderName(m.downloader_id) }}</template>
              <template #meta>
                <span class="ml-full">{{ m.downloader_prefix }}</span>
                <span class="ml-full">→ {{ m.local_prefix }}</span>
              </template>
              <template #actions>
                <el-button size="small" @click="openPathMap(m)">编辑</el-button>
                <el-button size="small" type="danger" plain @click="removePathMap(m)"
                  >删除</el-button
                >
              </template>
            </PtRowCard>
          </div>
        </PtPanel>

        <PtPanel
          title="媒体服务器"
          icon="server"
          :count="servers.length"
          action="添加"
          action-icon="plus"
          @action="openServer()">
          <p class="ml-tip ml-tip--top">
            整理完通知 Emby、Jellyfin 或 Plex 扫描新目录。媒体服务器看到的路径与 pt-tools
            不同时，填上两边的路径前缀。索引定期记下服务器里有哪些电影与剧集：条目详情写服务器里有没有、能直接播放，pt-tools
            整理过的片子在服务器里没了时提示。
          </p>
          <div
            v-if="servers.length && serversDS.error.value"
            class="pt-note pt-note--warn ml-stale">
            <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
            <span>刷新失败：{{ serversDS.errorText.value }}。下面是上次读到的。</span>
          </div>
          <PtDataState
            v-if="!servers.length"
            :state="serversDS.state.value"
            :title="serversDS.state.value === 'empty' ? '还没有媒体服务器' : ''"
            :sub="
              serversDS.errorText.value ||
              (serversDS.state.value === 'empty'
                ? '不加也能整理，媒体服务器会按自己的计划扫描'
                : '')
            "
            data-testid="ml-servers-state" />
          <el-table
            v-else-if="!isMobile"
            :data="servers"
            row-key="id"
            class="pt-grid"
            data-testid="ml-servers">
            <el-table-column label="名称" min-width="120" prop="name" class-name="pt-cell-strong" />
            <el-table-column label="种类" width="96">
              <template #default="{ row }">{{ serverKindLabel(row.kind) }}</template>
            </el-table-column>
            <el-table-column
              label="地址"
              min-width="140"
              prop="url"
              class-name="pt-cell-1line"
              show-overflow-tooltip />
            <el-table-column label="刷新" width="136">
              <template #default="{ row }">{{ refreshText(row.refresh_mode) }}</template>
            </el-table-column>
            <el-table-column
              label="最近一次"
              min-width="150"
              class-name="pt-cell-1line"
              show-overflow-tooltip>
              <template #default="{ row }">
                <PtStatusPill v-if="row.last_error" tone="dang" size="sm">失败</PtStatusPill>
                <PtStatusPill v-else-if="row.last_refresh_at" tone="ok" size="sm"
                  >正常</PtStatusPill
                >
                <span class="ml-sub">
                  {{
                    row.last_error ||
                    (row.last_refresh_at ? formatShortDateTime(row.last_refresh_at) : "还没通知过")
                  }}</span
                >
              </template>
            </el-table-column>
            <el-table-column label="索引" min-width="160" class-name="pt-cell-1line">
              <template #default="{ row }">
                <el-tooltip
                  :content="serverIndexView(row, resumeCounts[row.id]).tip"
                  placement="top">
                  <span class="ml-index" :data-testid="`ml-server-index-${row.id}`">
                    <PtStatusPill :tone="serverIndexView(row).tone" size="sm">{{
                      serverIndexView(row).label
                    }}</PtStatusPill>
                    <span class="ml-sub">{{
                      serverIndexView(row, resumeCounts[row.id]).text
                    }}</span>
                  </span>
                </el-tooltip>
              </template>
            </el-table-column>
            <el-table-column label="启用" width="76">
              <template #default="{ row }">
                <el-switch
                  :model-value="row.enabled"
                  :aria-label="`启用媒体服务器 ${row.name}`"
                  :data-testid="`ml-server-enabled-${row.id}`"
                  @update:model-value="
                    (v: string | number | boolean) => toggleServer(row, Boolean(v))
                  " />
              </template>
            </el-table-column>
            <!-- 列多（加了索引）：窄的窗口里表格横着滚，操作列钉在右边，测试、同步、编辑、删除一直看得到 -->
            <el-table-column label="" width="200" align="right" fixed="right">
              <template #default="{ row }">
                <el-button
                  link
                  :loading="serverTesting"
                  :data-testid="`ml-server-test-${row.id}`"
                  @click="testServer(row)"
                  >测试</el-button
                >
                <el-button
                  link
                  :loading="syncingServer === row.id"
                  :disabled="!row.enabled || row.index_hours === 0"
                  :data-testid="`ml-server-sync-${row.id}`"
                  @click="syncServer(row)"
                  >同步</el-button
                >
                <el-button link :data-testid="`ml-server-edit-${row.id}`" @click="openServer(row)"
                  >编辑</el-button
                >
                <el-button
                  link
                  type="danger"
                  :data-testid="`ml-server-del-${row.id}`"
                  @click="removeServer(row)"
                  >删除</el-button
                >
              </template>
            </el-table-column>
          </el-table>
          <div v-else class="ml-cards">
            <PtRowCard v-for="s in servers" :key="s.id">
              <template #title>{{ s.name }}</template>
              <template #meta>
                <span>{{ serverKindLabel(s.kind) }} · {{ refreshText(s.refresh_mode) }}</span>
                <span class="ml-full">{{ s.url }}</span>
                <span v-if="s.last_error" class="ml-full ml-err">{{ s.last_error }}</span>
                <span
                  class="ml-full"
                  :class="{ 'ml-err': serverIndexView(s).tone === 'dang' }"
                  :data-testid="`ml-server-index-${s.id}`"
                  >索引{{ serverIndexView(s).label }}：{{
                    serverIndexView(s, resumeCounts[s.id]).text
                  }}</span
                >
              </template>
              <template #status>
                <el-switch
                  :model-value="s.enabled"
                  :aria-label="`启用媒体服务器 ${s.name}`"
                  @update:model-value="
                    (v: string | number | boolean) => toggleServer(s, Boolean(v))
                  " />
              </template>
              <template #actions>
                <el-button size="small" :loading="serverTesting" @click="testServer(s)"
                  >测试</el-button
                >
                <el-button
                  size="small"
                  :loading="syncingServer === s.id"
                  :disabled="!s.enabled || s.index_hours === 0"
                  @click="syncServer(s)"
                  >同步</el-button
                >
                <el-button size="small" @click="openServer(s)">编辑</el-button>
                <el-button size="small" type="danger" plain @click="removeServer(s)"
                  >删除</el-button
                >
              </template>
            </PtRowCard>
          </div>
        </PtPanel>
      </div>
    </div>

    <el-dialog
      v-model="libDialog"
      class="pt-dialog"
      :title="editingLibID ? '编辑媒体库' : '添加媒体库'"
      :width="isMobile ? '94%' : '640px'"
      align-center>
      <el-form class="pt-form" label-position="top" @submit.prevent>
        <div class="ml-row">
          <el-form-item label="名称" required>
            <el-input v-model="libForm.name" placeholder="电影" data-testid="ml-lib-name" />
          </el-form-item>
          <el-form-item label="类型">
            <el-radio-group v-model="libForm.kind" data-testid="ml-lib-kind">
              <el-radio-button value="movie">电影</el-radio-button>
              <el-radio-button value="tv">剧集</el-radio-button>
            </el-radio-group>
            <el-checkbox v-model="libForm.anime" data-testid="ml-lib-anime">只收动画</el-checkbox>
          </el-form-item>
        </div>
        <el-form-item label="库目录（pt-tools 里的路径）" required>
          <div class="ml-inline ml-inline--grow">
            <el-input
              v-model="libForm.path"
              placeholder="/media/movies"
              data-testid="ml-lib-path" />
            <el-button :loading="checking" data-testid="ml-lib-check" @click="checkLibrary"
              >检查</el-button
            >
          </div>
          <ul v-if="checkItems.length" class="ml-checks" data-testid="ml-lib-checks">
            <li v-for="c in checkItems" :key="c.name" :class="c.ok ? 'is-ok' : 'is-bad'">
              <PtIcon :name="c.ok ? 'circle-check' : 'circle-x'" :size="13" />
              <span
                >{{ c.name }}<template v-if="c.message">：{{ c.message }}</template></span
              >
            </li>
          </ul>
        </el-form-item>
        <el-form-item label="整理方式">
          <el-radio-group v-model="libForm.mode" data-testid="ml-lib-mode">
            <el-radio-button v-for="m in MEDIA_MODES" :key="m.value" :value="m.value">{{
              m.label
            }}</el-radio-button>
          </el-radio-group>
          <div class="field-tip">{{ modeHint }}</div>
        </el-form-item>
        <el-form-item label="命名模板（不写扩展名）">
          <div class="ml-presets">
            <el-button
              v-for="p in presets"
              :key="p.label"
              size="small"
              :type="libForm.template === p.template ? 'primary' : 'default'"
              plain
              @click="libForm.template = p.template"
              >{{ p.label }}</el-button
            >
          </div>
          <el-input
            v-model="libForm.template"
            type="textarea"
            :rows="3"
            placeholder="留空用默认模板"
            data-testid="ml-lib-template" />
          <div class="field-tip" data-testid="ml-lib-preview">
            <template v-if="preview.error"
              ><span class="ml-err">{{ preview.error }}</span></template
            >
            <template v-else>示例：{{ preview.text }}</template>
          </div>
          <details class="ml-vars">
            <summary>可用的变量</summary>
            <ul>
              <li v-for="v in TEMPLATE_VARS" :key="v.name">
                <code v-text="braces(v.name)" /> {{ v.desc }}
              </li>
              <li><code v-text="braces('pad .Season 2')" /> 补零到两位</li>
            </ul>
          </details>
        </el-form-item>
        <el-form-item label="同名文件">
          <el-select
            v-model="libForm.overwrite_mode"
            class="ml-lang"
            :empty-values="[null, undefined]"
            data-testid="ml-lib-overwrite-mode">
            <el-option
              v-for="m in OVERWRITE_MODES"
              :key="m.value"
              :label="m.label"
              :value="m.value" />
          </el-select>
          <div class="field-tip">
            目标位置已经有 pt-tools 之前放进库的旧版本（同一部或同一集）时怎么办。只替换 pt-tools
            放进库的文件，用户自己放的、移动整理的不替换；旧版本的字幕与 NFO
            一起换掉。命名模板里带画质时不同版本的文件名不同，不会撞上。
          </div>
        </el-form-item>
        <el-form-item label="二级分类">
          <el-checkbox v-model="libForm.classify_dir" data-testid="ml-lib-classify"
            >按二级分类放进子目录</el-checkbox
          >
          <div class="field-tip">
            放进「库目录/分类/…」，分类按「二级分类」里的规则；没有规则或都对不上时直接放进库目录。改了以后只影响之后整理的文件。
          </div>
        </el-form-item>
        <el-form-item label="刮削">
          <div class="ml-switch">
            <el-switch v-model="libForm.scrape" data-testid="ml-lib-scrape" />
            <span>写 NFO 与图片给媒体服务器用（交给媒体服务器自己刮时关掉）</span>
          </div>
          <template v-if="libForm.scrape">
            <el-checkbox-group
              v-model="libForm.scrape_items"
              class="ml-inline"
              data-testid="ml-lib-items">
              <el-checkbox
                v-for="i in scrapeItemsFor(libForm.kind)"
                :key="i.value"
                :value="i.value"
                :title="i.file"
                :data-testid="`ml-lib-item-${i.value}`"
                >{{ i.label }}</el-checkbox
              >
            </el-checkbox-group>
            <div class="ml-inline">
              <span>图片语言</span>
              <el-select
                v-model="libForm.scrape_image_lang"
                class="ml-lang"
                :empty-values="[null, undefined]"
                aria-label="图片语言"
                data-testid="ml-lib-image-lang">
                <el-option
                  v-for="l in IMAGE_LANGS"
                  :key="l.value"
                  :label="l.label"
                  :value="l.value" />
              </el-select>
            </div>
            <div class="field-tip">
              Logo 写成 clearlogo.png，Clearart、横幅、横版海报与碟片图写成
              clearart.png、banner.jpg、 landscape.jpg 与 discart.png，Emby、Jellyfin 与 Kodi
              都认。选了图片语言时，海报优先用这种语言的、背景优先用没有文字的，没有就用来源默认的那张；Logo
              依次找这种语言（无文字是没有文字的）、识别语言与英文的。
            </div>
            <div class="ml-sources" data-testid="ml-lib-sources">
              <div v-for="sf in sourceFields" :key="sf.key" class="ml-inline">
                <span class="ml-source-label">{{ sf.label }}</span>
                <el-select
                  v-model="libForm.scrape_sources[sf.key]"
                  class="ml-lang"
                  :empty-values="[null, undefined]"
                  :aria-label="`${sf.label}的数据来源`"
                  :data-testid="`ml-lib-source-${sf.key}`">
                  <el-option
                    v-for="o in SCRAPE_SOURCE_OPTIONS[sf.key]"
                    :key="o.value"
                    :label="o.label"
                    :value="o.value" />
                </el-select>
              </div>
            </div>
            <div class="field-tip">
              数据来源按项选，选的来源找不到这部、或者没有这一项时用 TMDB
              的；取到的其它来源的评分也写进
              NFO，主要的评分写在最后（媒体服务器显示它）。豆瓣按中文名与年份找同一部（剧集按季找），站点给了豆瓣编号的电影直接用编号；豆瓣没有开放的接口，每
              2 秒最多请求一次。Bangumi 只用于动画（TMDB 类型里有「动画」的），日文的简介不用。IMDb
              的评分经 OMDb 取、Fanart.tv 的图片也要 Key，都在「媒体识别」里填。
            </div>
            <div
              v-for="w in sourceWarnings"
              :key="w"
              class="pt-note pt-note--warn ml-source-warn"
              data-testid="ml-lib-source-warn">
              <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
              <span>{{ w }}</span>
            </div>
          </template>
          <el-checkbox
            v-model="libForm.scrape_overwrite"
            :disabled="!libForm.scrape"
            data-testid="ml-lib-overwrite"
            >覆盖已有的 NFO 与图片</el-checkbox
          >
          <el-checkbox
            v-model="libForm.scrape_actor_zh"
            :disabled="!libForm.scrape"
            data-testid="ml-lib-actor-zh"
            >演员名用中文</el-checkbox
          >
          <el-checkbox
            v-model="libForm.scrape_fill"
            :disabled="!libForm.scrape"
            data-testid="ml-lib-fill"
            >每周补一次缺的刮削</el-checkbox
          >
          <div v-if="libForm.scrape" class="field-tip">
            演员名用中文：NFO 里前 15 位演员用 TMDB
            上的中文译名，没有译名的用原名，角色名不变。每周补刮：刮削结果是部分刮削或刮削失败的条目每周重新刮削一次（一次最多
            50 部），自己放的、别的工具写的文件不换。
          </div>
        </el-form-item>
        <div class="ml-dlg-sec">字幕</div>
        <el-form-item label="站点字幕">
          <el-checkbox v-model="libForm.site_subtitles" data-testid="ml-lib-site-subs"
            >整理完到站点上下载字幕</el-checkbox
          >
          <div class="field-tip">
            种子里没有字幕、种子是 pt-tools 从 NexusPHP
            架构的站点推送的时，到种子详情页取站点上的字幕：有中文的只取中文的，几个视频时按文件名或季集对上；zip
            解开只取字幕文件，rar 与 7z
            不解。放成「视频名.语言.后缀」，记在整理记录里，删除时一起删；取不到的原因写在整理历史里。
          </div>
        </el-form-item>
        <el-form-item label="ChineseSubFinder 的地址">
          <div class="ml-inline ml-inline--grow">
            <el-input
              v-model="libForm.csf_url"
              placeholder="http://192.168.1.10:19035/（不用就空着）"
              data-testid="ml-lib-csf-url" />
            <el-button :loading="csfTesting" data-testid="ml-lib-csf-test" @click="testCSF"
              >测试</el-button
            >
          </div>
          <div class="field-tip">
            新整理出视频以后交给 ChineseSubFinder 下载字幕（原盘不交）。剧集要开着刮削：它读每一集的
            NFO。交不上的原因写在整理历史里。
          </div>
        </el-form-item>
        <div class="ml-row">
          <el-form-item label="API Key">
            <el-input
              v-model="libForm.csf_key"
              type="password"
              show-password
              autocomplete="new-password"
              :placeholder="editingLib?.has_csf_key ? '已设置；要更换时填写新的' : ''"
              data-testid="ml-lib-csf-key" />
            <div class="field-tip">在 ChineseSubFinder 的「实验室」设置里打开 API Key</div>
          </el-form-item>
          <el-form-item label="ChineseSubFinder 里的库目录">
            <el-input
              v-model="libForm.csf_path"
              :placeholder="libForm.path || '/media/movies'"
              data-testid="ml-lib-csf-path" />
            <div class="field-tip">它那边看到的这个库的路径；和库目录一样时空着</div>
          </el-form-item>
        </div>
        <div class="ml-dlg-sec">新建订阅时带出的下载设置</div>
        <div class="field-tip ml-dl-intro" data-testid="ml-lib-dl-intro">
          新建订阅时选了这个库（或者按类型自动选到这个库），这几项会填进订阅的表单，保存前可以改；已有的订阅不变。都可以空着。
        </div>
        <el-form-item label="下载器">
          <el-select
            v-model="libForm.download_downloader_id"
            class="ml-dl-select"
            data-testid="ml-lib-dl-downloader">
            <el-option :value="0" label="不指定" />
            <el-option
              v-for="o in libDownloaderOptions"
              :key="o.id"
              :value="o.id"
              :label="o.label" />
          </el-select>
          <div
            v-if="libDownloaderGone && libForm.download_downloader_id === 0"
            class="pt-note pt-note--warn ml-dl-note"
            data-testid="ml-lib-dl-gone">
            <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
            <span>原来设的下载器已删除，保存时改成不指定</span>
          </div>
          <div
            v-else-if="libDownloaderDisabled"
            class="pt-note pt-note--warn ml-dl-note"
            data-testid="ml-lib-dl-disabled">
            <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
            <span>这个下载器停用期间，新建订阅不会带出它</span>
          </div>
        </el-form-item>
        <el-form-item label="保存目录">
          <DownloaderPathInput
            v-model="libForm.download_save_path"
            :downloader-ids="libForm.download_downloader_id || null"
            placeholder="空着按下载器的默认目录"
            data-testid="ml-lib-dl-path" />
          <div class="field-tip">下载器里看到的目录，不是上面的库目录</div>
        </el-form-item>
        <div class="ml-row">
          <el-form-item label="分类">
            <el-input
              v-model="libForm.download_category"
              placeholder="不加分类就空着"
              data-testid="ml-lib-dl-category" />
          </el-form-item>
          <el-form-item label="标签">
            <el-input
              v-model="libForm.download_tags"
              placeholder="几个标签用逗号分开"
              data-testid="ml-lib-dl-tags" />
          </el-form-item>
        </div>
        <div class="field-tip ml-dl-qb" data-testid="ml-lib-dl-qb">
          qBittorrent
          的默认种子管理模式是「自动」时，种子按分类的目录放（没有分类时放默认目录），这里的保存目录不起作用；订阅设置里开了「下载时按条目建子目录」时，pt-tools
          让 qBittorrent 照保存目录放。
        </div>
        <div
          v-if="libScopeMisses.length"
          class="pt-note pt-note--warn ml-dl-note"
          data-testid="ml-lib-dl-scope">
          <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
          <span>这样推送的种子下完不会自动整理：{{ libScopeMisses.join("；") }}</span>
        </div>
        <el-form-item label="启用">
          <el-switch v-model="libForm.enabled" data-testid="ml-lib-enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="libDialog = false">取消</el-button>
        <el-button
          type="primary"
          :loading="libSaving"
          data-testid="ml-lib-save"
          @click="saveLibrary"
          >保存</el-button
        >
      </template>
    </el-dialog>

    <el-dialog
      v-model="ruleDialog"
      class="pt-dialog"
      :title="ruleTitle"
      :width="isMobile ? '94%' : '560px'"
      align-center>
      <el-form class="pt-form" label-position="top" @submit.prevent>
        <el-form-item label="分类名称（也是目录名）" required>
          <el-input
            v-model="ruleForm.name"
            maxlength="32"
            placeholder="华语电影"
            data-testid="ml-rule-name" />
        </el-form-item>
        <el-form-item label="类型">
          <el-select
            v-model="ruleForm.genres"
            multiple
            filterable
            placeholder="不限"
            data-testid="ml-rule-genres">
            <el-option
              v-for="g in TMDB_GENRES[ruleKind]"
              :key="g.id"
              :label="g.name"
              :value="g.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="原始语言">
          <el-select
            v-model="ruleForm.languages"
            multiple
            filterable
            allow-create
            default-first-option
            placeholder="不限；不在列表里的可以直接输入代码，例如 vi"
            data-testid="ml-rule-languages">
            <el-option
              v-for="l in CLASSIFY_LANGUAGES"
              :key="l.value"
              :label="`${l.label}（${l.value}）`"
              :value="l.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="出品国家或地区">
          <el-select
            v-model="ruleForm.countries"
            multiple
            filterable
            allow-create
            default-first-option
            placeholder="不限；不在列表里的可以直接输入代码，例如 BR"
            data-testid="ml-rule-countries">
            <el-option
              v-for="c in CLASSIFY_COUNTRIES"
              :key="c.value"
              :label="`${c.label}（${c.value}）`"
              :value="c.value" />
          </el-select>
        </el-form-item>
        <div class="ml-rule-pair">
          <el-form-item label="片长不超过（分钟）">
            <el-input-number
              v-model="ruleForm.max_runtime"
              :min="0"
              :max="600"
              controls-position="right"
              data-testid="ml-rule-runtime" />
          </el-form-item>
          <el-form-item label="或标题里有">
            <el-select
              v-model="ruleForm.keywords"
              multiple
              filterable
              allow-create
              default-first-option
              :reserve-keyword="false"
              placeholder="输入关键字后回车，例如 短剧"
              data-testid="ml-rule-keywords" />
          </el-form-item>
        </div>
        <div class="field-tip">
          几个条件都要满足；一个条件里选了几个时满足其中一个就行。片长与标题关键字算一个条件：片长（剧集按一集）不超过这么多分钟，或者种子名、站点标题里有其中一个关键字；片长为
          0 是不看片长，TMDB 上没写片长时只看关键字。都不填时，前面的分类都对不上的放进这里。
        </div>
      </el-form>
      <template #footer>
        <el-button @click="ruleDialog = false">取消</el-button>
        <el-button
          type="primary"
          :loading="classifySaving"
          data-testid="ml-rule-save"
          @click="saveRule"
          >保存</el-button
        >
      </template>
    </el-dialog>

    <el-dialog
      v-model="watchDialog"
      class="pt-dialog"
      title="添加监控目录"
      :width="isMobile ? '94%' : '520px'"
      align-center>
      <el-form class="pt-form" label-position="top" @submit.prevent>
        <el-form-item label="目录（pt-tools 里看到的路径）" required>
          <el-input
            v-model="watchForm.path"
            placeholder="/media/incoming"
            data-testid="ml-watch-path" />
          <div class="field-tip">
            按各个媒体库的整理方式放进库里；用硬链接时，这个目录要和媒体库在同一个分区上。添加以后路径不能改，要换目录就删掉再添加
          </div>
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="watchForm.enabled" data-testid="ml-watch-form-enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="watchDialog = false">取消</el-button>
        <el-button
          type="primary"
          :loading="watchSaving"
          data-testid="ml-watch-save"
          @click="saveWatchDir"
          >保存</el-button
        >
      </template>
    </el-dialog>

    <el-dialog
      v-model="mapDialog"
      class="pt-dialog"
      :title="editingMapID ? '编辑路径映射' : '添加路径映射'"
      :width="isMobile ? '94%' : '520px'"
      align-center>
      <el-form class="pt-form" label-position="top" @submit.prevent>
        <el-form-item label="下载器" required>
          <el-select v-model="mapForm.downloader_id" data-testid="ml-map-dl">
            <el-option
              v-for="d in downloaders"
              :key="d.id"
              :label="d.name"
              :value="d.id as number" />
          </el-select>
        </el-form-item>
        <el-form-item label="下载器里的路径" required>
          <DownloaderPathInput
            v-model="mapForm.downloader_prefix"
            :downloader-ids="mapForm.downloader_id"
            placeholder="/downloads"
            data-testid="ml-map-from" />
        </el-form-item>
        <el-form-item label="pt-tools 里的路径" required>
          <el-input
            v-model="mapForm.local_prefix"
            placeholder="/media/downloads"
            data-testid="ml-map-to" />
          <div class="field-tip">按路径分段取最长的匹配：/data 不匹配 /database</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="mapDialog = false">取消</el-button>
        <el-button
          type="primary"
          :loading="mapSaving"
          data-testid="ml-map-save"
          @click="savePathMap"
          >保存</el-button
        >
      </template>
    </el-dialog>

    <el-dialog
      v-model="serverDialog"
      class="pt-dialog"
      :title="editingServer ? '编辑媒体服务器' : '添加媒体服务器'"
      :width="isMobile ? '94%' : '560px'"
      align-center>
      <el-form class="pt-form" label-position="top" @submit.prevent>
        <div class="ml-row">
          <el-form-item label="名称" required>
            <el-input
              v-model="serverForm.name"
              placeholder="家里的 Emby"
              data-testid="ml-server-name" />
          </el-form-item>
          <el-form-item label="种类">
            <el-radio-group v-model="serverForm.kind" data-testid="ml-server-kind">
              <el-radio-button v-for="k in MEDIA_SERVER_KINDS" :key="k.value" :value="k.value">{{
                k.label
              }}</el-radio-button>
            </el-radio-group>
          </el-form-item>
        </div>
        <el-form-item label="地址" required>
          <el-input
            v-model="serverForm.url"
            :placeholder="
              serverForm.kind === 'plex' ? 'http://192.168.1.10:32400' : 'http://192.168.1.10:8096'
            "
            data-testid="ml-server-url" />
        </el-form-item>
        <el-form-item
          :label="serverForm.kind === 'plex' ? 'Token' : 'API Key'"
          :required="!editingServer">
          <el-input
            v-model="serverForm.token"
            type="password"
            show-password
            autocomplete="new-password"
            :placeholder="editingServer?.has_token ? '已设置；要更换时填写新的' : ''"
            data-testid="ml-server-token" />
          <div class="field-tip">
            <template v-if="serverForm.kind === 'plex'"
              >X-Plex-Token，在 Plex 网页里查看任一媒体的「获取信息 → 查看 XML」，地址里的
              X-Plex-Token</template
            >
            <template v-else>控制台 → 高级 → API 密钥，新建一个</template>
          </div>
        </el-form-item>
        <el-form-item label="刷新方式">
          <el-radio-group v-model="serverForm.refresh_mode" data-testid="ml-server-refresh">
            <el-radio-button value="path">只扫整理到的目录</el-radio-button>
            <el-radio-button value="library">刷新整个媒体库</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <div class="ml-row">
          <el-form-item label="pt-tools 里的路径前缀（可选）">
            <el-input
              v-model="serverForm.local_prefix"
              placeholder="/media"
              data-testid="ml-server-local" />
          </el-form-item>
          <el-form-item label="媒体服务器里的路径前缀（可选）">
            <el-input
              v-model="serverForm.server_prefix"
              placeholder="/data/media"
              data-testid="ml-server-remote" />
          </el-form-item>
        </div>
        <el-form-item label="打开链接用的地址（可选）">
          <el-input
            v-model="serverForm.play_url"
            placeholder="https://jellyfin.example.com"
            data-testid="ml-server-play-url" />
          <div class="field-tip">
            详情页「在
            {{ serverKindLabel(serverForm.kind) }}
            播放」打开的地址；上面的地址浏览器打不开（比如容器里的名字）时填，不填用上面的
          </div>
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="serverForm.enabled" data-testid="ml-server-enabled" />
        </el-form-item>

        <div class="ml-dlg-sec">索引与继续观看</div>
        <div class="ml-row">
          <el-form-item label="索引间隔（小时）">
            <el-input-number
              v-model="serverForm.index_hours"
              :min="0"
              :max="168"
              controls-position="right"
              data-testid="ml-server-index-hours" />
            <div class="field-tip">
              0 是关闭。记下服务器里的电影、剧集与每一集，新入库时有 Webhook 会提前同步
            </div>
          </el-form-item>
          <el-form-item v-if="serverForm.kind !== 'plex'" label="继续观看的用户">
            <div class="ml-inline ml-inline--grow">
              <el-select
                :model-value="serverForm.resume_user_id"
                clearable
                placeholder="不取继续观看"
                data-testid="ml-server-resume-user"
                @update:model-value="(v: string) => pickResumeUser(v)">
                <el-option v-for="u in serverUsers" :key="u.id" :label="u.name" :value="u.id" />
              </el-select>
              <el-button
                :loading="usersLoading"
                data-testid="ml-server-users"
                @click="loadServerUsers"
                >读取用户</el-button
              >
            </div>
            <div class="field-tip">这个用户没看完的片子，首页「继续观看」显示</div>
          </el-form-item>
        </div>
        <el-form-item label="把服务器里有的集算作已入库">
          <div class="ml-switch">
            <el-switch v-model="serverForm.count_library" data-testid="ml-server-count" />
            <span>追剧时这台服务器里有的集不再下载（不是 pt-tools 整理进去的也算）</span>
          </div>
        </el-form-item>

        <div class="ml-dlg-sec">Webhook 与播放通知</div>
        <el-form-item label="Webhook">
          <template v-if="!editingServer">
            <div class="field-tip">保存以后，编辑这台服务器时生成地址</div>
          </template>
          <template v-else>
            <div class="ml-inline">
              <PtStatusPill :tone="editingServer.webhook ? 'ok' : 'neutral'" size="sm">{{
                editingServer.webhook ? "已开启" : "没开"
              }}</PtStatusPill>
              <span
                v-if="editingServer.webhook_at"
                class="ml-sub"
                data-testid="ml-server-webhook-at"
                >最近一次收到 {{ formatShortDateTime(editingServer.webhook_at) }}
                {{ editingServer.webhook_event }}</span
              >
              <el-button
                size="small"
                :loading="webhookBusy"
                data-testid="ml-server-webhook-create"
                @click="createWebhook"
                >{{ editingServer.webhook ? "重新生成地址" : "生成地址" }}</el-button
              >
              <el-button
                v-if="editingServer.webhook"
                size="small"
                :disabled="webhookBusy"
                data-testid="ml-server-webhook-delete"
                @click="deleteWebhook"
                >关掉</el-button
              >
            </div>
            <div v-if="webhookURL" class="ml-hook" data-testid="ml-server-webhook-url">
              <el-input :model-value="webhookURL" readonly />
              <el-button @click="copyWebhook"
                ><PtIcon name="copy" :size="14" /><span>复制</span></el-button
              >
            </div>
            <div class="field-tip">
              <template v-if="webhookURL"
                >地址只显示这一次。媒体服务器访问 pt-tools
                的地址和浏览器不同时，把前面的地址换掉。</template
              >
              <template v-if="serverForm.kind === 'jellyfin'"
                >Jellyfin 装 Webhook 插件，加一个 Generic 目标，勾上 Item Added、Item
                Deleted、Playback Start、Playback Stop</template
              >
              <template v-else-if="serverForm.kind === 'emby'"
                >Emby 在设置 → 通知（Webhooks）里加这个地址，选新媒体、删除与播放的事件</template
              >
              <template v-else>Plex 在设置 → Webhooks 里加这个地址（要 Plex Pass）</template>
              。新入库与删除 2 分钟后同步索引，开始、停止播放发通知。
            </div>
          </template>
        </el-form-item>
        <el-form-item label="播放通知">
          <el-select
            v-model="serverForm.playback_channels"
            multiple
            clearable
            placeholder="不通知"
            data-testid="ml-server-playback">
            <el-option v-for="c in channels" :key="c.id" :label="c.name" :value="c.id" />
          </el-select>
          <div class="field-tip">Webhook 发来开始播放、停止播放时通知谁在看什么</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button :loading="serverTesting" data-testid="ml-server-test" @click="testServer()"
          >测试连接</el-button
        >
        <el-button @click="serverDialog = false">取消</el-button>
        <el-button
          type="primary"
          :loading="serverSaving"
          data-testid="ml-server-save"
          @click="saveServer"
          >保存</el-button
        >
      </template>
    </el-dialog>

    <OrganizeDialog v-model="organizeVisible" :target="organizeTarget" />
    <ImportLibraryDialog v-model="importVisible" :library="importLibrary" />
  </div>
</template>

<style scoped>
.ml-form {
  max-width: 900px;
}

.ml-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 0 16px;
}

.ml-switch {
  display: flex;
  gap: 10px;
  align-items: center;
  font-size: 13px;
  line-height: 1.6;
  color: var(--pt-t2);
}

.ml-inline {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  margin-top: 8px;
  font-size: 13px;
  color: var(--pt-t2);
}

.ml-inline--grow {
  width: 100%;
  margin-top: 0;
}

.ml-lang {
  width: 240px;
  max-width: 100%;
}

.ml-sources {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, max-content));
  column-gap: 20px;
}

.ml-source-label {
  min-width: 48px;
}

.ml-source-warn {
  margin-top: 8px;
}

/* 新建订阅时带出的下载设置 */
.ml-dl-intro {
  margin: 0 0 12px;
}

.ml-dl-select {
  width: 100%;
}

.ml-dl-note {
  margin-top: 8px;
}

.ml-dl-qb {
  margin: -6px 0 12px;
}

@media (max-width: 767px) {
  .ml-sources {
    grid-template-columns: minmax(0, 1fr);
  }
}

.ml-classify {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 16px;
}

.ml-classify__kind {
  min-width: 0;
}

.ml-classify__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 6px;
}

.ml-classify__title,
.ml-classify__name {
  font-weight: 600;
  color: var(--pt-t1);
}

.ml-classify__list {
  margin: 0;
  padding: 0;
  list-style: none;
}

.ml-classify__row {
  display: flex;
  gap: 8px;
  align-items: center;
  justify-content: space-between;
  padding: 6px 0;
  border-top: 1px solid var(--pt-border);
}

.ml-classify__text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.ml-watch {
  margin: 0;
  padding: 0;
  list-style: none;
}

.ml-watch__rt {
  font-size: 12px;
  color: var(--pt-t3);
}

.ml-watch__rt--warn {
  color: var(--pt-warn);
}

.ml-classify__acts {
  display: flex;
  flex-shrink: 0;
  gap: 4px;
}

.ml-inline--grow > .el-input {
  flex: 1 1 240px;
}

.ml-inline--grow > .el-select {
  flex: 1 1 180px;
}

/* 媒体服务器弹窗里的分组标题 */
.ml-dlg-sec {
  margin: 4px 0 12px;
  padding-top: 12px;
  font-size: 12px;
  font-weight: 700;
  color: var(--pt-t2);
  border-top: 1px solid var(--pt-border);
}

/* 索引那一列：状态与条目数一行 */
.ml-index {
  display: inline-flex;
  gap: 6px;
  align-items: center;
  max-width: 100%;
  overflow: hidden;
}

.ml-index .ml-sub {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 生成的 Webhook 地址：只读输入框加复制 */
.ml-hook {
  display: flex;
  gap: 8px;
  width: 100%;
  margin-top: 8px;
}

.ml-hook .el-input {
  flex: 1 1 auto;
}

.ml-tip {
  margin: 12px 0 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--pt-t3);
}

.ml-tip--top {
  margin: 0 0 12px;
}

.ml-stale {
  margin-bottom: 12px;
}

.ml-path {
  overflow: hidden;
  text-overflow: ellipsis;
}

.ml-delete-warn {
  margin-top: var(--pt-space-2);
}

/* 片长与关键字是一个条件：并排放，手机上上下排 */
.ml-rule-pair {
  display: grid;
  grid-template-columns: 160px minmax(0, 1fr);
  gap: 12px;
}

@media (max-width: 768px) {
  .ml-rule-pair {
    grid-template-columns: minmax(0, 1fr);
    gap: 0;
  }
}

.ml-sub {
  font-size: 12px;
  color: var(--pt-t3);
}

.ml-err {
  color: var(--pt-dang);
}

.ml-cards {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.ml-full {
  flex-basis: 100%;
  word-break: break-all;
}

.ml-checks {
  display: flex;
  flex-direction: column;
  gap: 4px;
  width: 100%;
  margin: 8px 0 0;
  padding: 0;
  list-style: none;
  font-size: 12px;
  line-height: 1.6;
}

.ml-checks li {
  display: flex;
  gap: 6px;
  align-items: center;
  word-break: break-all;
}

.ml-checks .is-ok {
  color: var(--pt-ok);
}

.ml-checks .is-bad {
  color: var(--pt-dang);
}

.ml-presets {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 8px;
}

.ml-presets .el-button + .el-button {
  margin-left: 0;
}

.ml-vars {
  width: 100%;
  margin-top: 6px;
  font-size: 12px;
  color: var(--pt-t3);
}

.ml-vars ul {
  margin: 6px 0 0;
  padding-left: 18px;
  line-height: 1.7;
}
</style>
