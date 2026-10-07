<script setup lang="ts">
/*
 * 媒体库（路线图 M10）：整理设置、媒体库（目录、命名模板、整理方式、刮削）、下载器到 pt-tools 的路径映射、
 * 整理后要通知的媒体服务器。画板没有这一页，沿用媒体识别页的样式：页头 + 面板里的表单与表格，手机是行卡。
 */
import {
  type DownloaderSetting,
  type MediaCheckItem,
  type MediaKind,
  type MediaLibrary,
  type MediaLibraryInput,
  type MediaPathMap,
  type MediaPathMapInput,
  type MediaServer,
  type MediaServerInput,
  type NotificationConfig,
  type OrganizeSettings,
  type OrganizeSettingsInput,
  chatopsApi,
  downloadersApi,
  organizeApi,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { formatShortDateTime } from "@/utils/format";
import {
  MEDIA_MODES,
  MEDIA_SERVER_KINDS,
  TEMPLATE_VARS,
  mediaKindLabel,
  modeLabel,
  serverKindLabel,
  templatePresets,
} from "@/utils/media";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onMounted, ref, watch } from "vue";

const isMobile = useIsMobile();
const errText = (e: unknown, fallback: string) => (e as Error)?.message || fallback;

// ---- 整理设置 ----
const settings = ref<OrganizeSettings | null>(null);
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
});
const sform = ref<OrganizeSettingsInput>(blankSettings());
const savePathsText = ref("");
const downloaders = ref<DownloaderSetting[]>([]);
const channels = ref<NotificationConfig[]>([]);

function fillSettings(s: OrganizeSettings) {
  settings.value = s;
  const { auto_since: _since, ...rest } = s;
  sform.value = { ...rest };
  savePathsText.value = s.save_paths.join("\n");
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
  if (dls.status === "fulfilled") downloaders.value = dls.value ?? [];
  if (chs.status === "fulfilled") channels.value = chs.value ?? [];
}

async function saveSettings() {
  savingSettings.value = true;
  try {
    const body: OrganizeSettingsInput = {
      ...sform.value,
      save_paths: savePathsText.value
        .split("\n")
        .map((p) => p.trim())
        .filter(Boolean),
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
  enabled: true,
});
const libForm = ref<MediaLibraryInput>(blankLibrary());
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
    enabled: l.enabled,
  };
}

function openLibrary(l?: MediaLibrary) {
  editingLibID.value = l?.id ?? null;
  libForm.value = l
    ? toLibraryInput(l)
    : blankLibrary(libraries.value.some((x) => x.kind === "movie") ? "tv" : "movie");
  checkItems.value = [];
  libDialog.value = true;
  void refreshPreview();
}

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
  libSaving.value = true;
  try {
    const body = { ...f, name: f.name.trim(), path: f.path.trim(), template: f.template.trim() };
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
const serverForm = ref<MediaServerInput & { token: string }>({
  name: "",
  kind: "emby",
  url: "",
  token: "",
  enabled: true,
  refresh_mode: "path",
  local_prefix: "",
  server_prefix: "",
});

async function loadServers() {
  const pending = serversDS.run(() => organizeApi.servers());
  const data = await pending;
  if (serversDS.isStale(pending) || !data) return;
  servers.value = data;
}

function openServer(s?: MediaServer) {
  editingServer.value = s ?? null;
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
      }
    : {
        name: "",
        kind: "emby",
        url: "",
        token: "",
        enabled: true,
        refresh_mode: "path",
        local_prefix: "",
        server_prefix: "",
      };
  serverDialog.value = true;
}

function serverBody(): MediaServerInput {
  const { token, ...rest } = serverForm.value;
  const body: MediaServerInput = {
    ...rest,
    name: rest.name.trim(),
    url: rest.url.trim(),
    local_prefix: rest.local_prefix.trim(),
    server_prefix: rest.server_prefix.trim(),
  };
  if (token.trim()) body.token = token.trim();
  return body;
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
  void Promise.all([loadSettings(), loadOptions(), loadLibraries(), loadPathMaps(), loadServers()]);
}

onMounted(refresh);
</script>

<template>
  <div class="pt-cards pt-cards--wide">
    <PtHeadSub
      >把下载完的电影与剧集按模板放进媒体库，写好 NFO 与海报，再通知媒体服务器扫描</PtHeadSub
    >
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button data-testid="ml-refresh" @click="refresh">
        <PtIcon name="refresh-cw" :size="15" /><span>刷新</span>
      </el-button>
    </Teleport>

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
          <el-form-item label="范围：保存路径（下载器里看到的，一行一个）">
            <el-input
              v-model="savePathsText"
              type="textarea"
              :rows="2"
              placeholder="/downloads/movies"
              data-testid="ml-scope-path" />
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
        <el-table-column label="" width="120" align="right">
          <template #default="{ row }">
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
              >{{ libKindText(l) }} · {{ modeLabel(l.mode) }}{{ l.scrape ? " · 刮削" : "" }}</span
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
            <el-button size="small" @click="openLibrary(l)">编辑</el-button>
            <el-button size="small" type="danger" plain @click="removeLibrary(l)">删除</el-button>
          </template>
        </PtRowCard>
      </div>
    </PtPanel>

    <PtPanel
      title="路径映射"
      icon="route"
      :count="pathMaps.length"
      action="添加"
      action-icon="plus"
      @action="openPathMap()">
      <p class="ml-tip ml-tip--top">
        下载器与 pt-tools 看到的路径不同时（比如两者在不同的容器里）要加：下载器里的 /downloads 在
        pt-tools 里是哪个目录。 路径相同时不用加。
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
            <el-button size="small" type="danger" plain @click="removePathMap(m)">删除</el-button>
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
        不同时，填上两边的路径前缀。
      </p>
      <div v-if="servers.length && serversDS.error.value" class="pt-note pt-note--warn ml-stale">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>刷新失败：{{ serversDS.errorText.value }}。下面是上次读到的。</span>
      </div>
      <PtDataState
        v-if="!servers.length"
        :state="serversDS.state.value"
        :title="serversDS.state.value === 'empty' ? '还没有媒体服务器' : ''"
        :sub="
          serversDS.errorText.value ||
          (serversDS.state.value === 'empty' ? '不加也能整理，媒体服务器会按自己的计划扫描' : '')
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
          min-width="200"
          prop="url"
          class-name="pt-cell-1line"
          show-overflow-tooltip />
        <el-table-column label="刷新" width="130">
          <template #default="{ row }">{{ refreshText(row.refresh_mode) }}</template>
        </el-table-column>
        <el-table-column
          label="最近一次"
          min-width="160"
          class-name="pt-cell-1line"
          show-overflow-tooltip>
          <template #default="{ row }">
            <PtStatusPill v-if="row.last_error" tone="dang" size="sm">失败</PtStatusPill>
            <PtStatusPill v-else-if="row.last_refresh_at" tone="ok" size="sm">正常</PtStatusPill>
            <span class="ml-sub">
              {{
                row.last_error ||
                (row.last_refresh_at ? formatShortDateTime(row.last_refresh_at) : "还没通知过")
              }}</span
            >
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
        <el-table-column label="" width="160" align="right">
          <template #default="{ row }">
            <el-button
              link
              :loading="serverTesting"
              :data-testid="`ml-server-test-${row.id}`"
              @click="testServer(row)"
              >测试</el-button
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
          </template>
          <template #status>
            <el-switch
              :model-value="s.enabled"
              :aria-label="`启用媒体服务器 ${s.name}`"
              @update:model-value="(v: string | number | boolean) => toggleServer(s, Boolean(v))" />
          </template>
          <template #actions>
            <el-button size="small" :loading="serverTesting" @click="testServer(s)">测试</el-button>
            <el-button size="small" @click="openServer(s)">编辑</el-button>
            <el-button size="small" type="danger" plain @click="removeServer(s)">删除</el-button>
          </template>
        </PtRowCard>
      </div>
    </PtPanel>

    <el-dialog
      v-model="libDialog"
      class="pt-dialog"
      :title="editingLibID ? '编辑媒体库' : '添加媒体库'"
      width="640px"
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
        <el-form-item label="刮削">
          <div class="ml-switch">
            <el-switch v-model="libForm.scrape" data-testid="ml-lib-scrape" />
            <span>写 NFO 与海报、背景、季海报、集截图（交给媒体服务器自己刮时关掉）</span>
          </div>
          <el-checkbox
            v-model="libForm.scrape_overwrite"
            :disabled="!libForm.scrape"
            data-testid="ml-lib-overwrite"
            >覆盖已有的 NFO 与图片</el-checkbox
          >
        </el-form-item>
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
      v-model="mapDialog"
      class="pt-dialog"
      :title="editingMapID ? '编辑路径映射' : '添加路径映射'"
      width="520px"
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
          <el-input
            v-model="mapForm.downloader_prefix"
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
      width="560px"
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
        <el-form-item label="启用">
          <el-switch v-model="serverForm.enabled" data-testid="ml-server-enabled" />
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

.ml-inline--grow > .el-input {
  flex: 1 1 240px;
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
}

.ml-checks li {
  display: flex;
  gap: 6px;
  align-items: flex-start;
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
