<script setup lang="ts">
/*
 * 消息模板（路线图 M21、M22）：按事件改通知的标题与正文，所有通知通道都按它发（Go 模板，写法是 {{.site}}）；
 * ChatOps 命令回复与 AI 助手回复也能改，它们只有正文。
 * 左边是按组列出的事件，右边是编辑：变量表点一下插到光标处，改了以后用示例数据实时预览。
 * 保存前后端用示例数据渲染一遍，写错的变量名、不合规的循环会被拒并写明原因；空着的一项用内置文字。
 * 手机上没有左栏，事件在编辑卡顶上的下拉框里选（按组分开）。
 */
import {
  chatopsApi,
  type NotifyTemplateCatalog,
  type NotifyTemplateVar,
  type NotifyTemplateView,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { apiErrorDetail } from "@/utils/notifyChannels";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";

const ds = useDataState();
const isMobile = useIsMobile();
const catalog = ref<NotifyTemplateCatalog | null>(null);
const currentType = ref("");
const form = ref({ title: "", text: "" });
const saving = ref(false);
const resetting = ref(false);
const preview = ref<{ ok: true; title: string; text: string } | { ok: false; msg: string } | null>(
  null,
);
/** 最近聚焦的输入框：点变量时插到这里（默认正文） */
const focused = ref<"title" | "text">("text");

interface InputHandle {
  input?: HTMLInputElement;
  textarea?: HTMLTextAreaElement;
}
const titleRef = ref<InputHandle | null>(null);
const textRef = ref<InputHandle | null>(null);

const current = computed<NotifyTemplateView | null>(
  () => catalog.value?.events.find((e) => e.type === currentType.value) ?? null,
);
const dirty = computed(
  () =>
    !!current.value &&
    (form.value.title !== current.value.title || form.value.text !== current.value.text),
);
/** 命令回复与助手回复只有正文：不显示标题框、标题变量与预览里的标题 */
const noTitle = computed(() => !!current.value?.no_title);

/** 事件目录的搜索词：三十个事件排成两千多高的一列，找后面的命令回复、助手回复要一路滚到底 */
const eventQuery = ref("");

/** 按组排好的事件，组的顺序以目录给的为准；没有事件的组不显示（搜索时按名称与类型筛） */
const grouped = computed(() => {
  const cat = catalog.value;
  if (!cat) return [];
  const q = eventQuery.value.trim().toLowerCase();
  const hit = (e: NotifyTemplateView) =>
    !q || e.name.toLowerCase().includes(q) || e.type.toLowerCase().includes(q);
  const groups = cat.groups?.length ? cat.groups : [{ key: "notify", name: "通知", desc: "" }];
  return groups
    .map((g) => ({
      ...g,
      events: cat.events.filter((e) => (e.group || "notify") === g.key && hit(e)),
    }))
    .filter((g) => g.events.length > 0);
});

const visibleVars = computed<NotifyTemplateVar[]>(() => {
  if (!catalog.value || !current.value) return [];
  const common = noTitle.value
    ? catalog.value.common_vars.filter((v) => v.key !== "title")
    : catalog.value.common_vars;
  return [...common, ...current.value.vars];
});

const TYPE_LABELS: Record<NotifyTemplateVar["type"], string> = {
  string: "文字",
  int: "整数",
  float: "小数",
  bool: "真假",
  list: "列表",
};

/** 函数速查：写法里有花括号，放在脚本里，模板里按文字显示 */
const FUNCS: { code: string; desc: string }[] = [
  { code: "{{if .free}}免费{{else}}不免费{{end}}", desc: "条件" },
  { code: "{{range .items}}{{.site}} {{end}}", desc: "遍历列表（只能遍历列表变量）" },
  {
    code: "{{if gt .seeders 10}}热门{{end}}",
    desc: "比较：eq ne lt le gt ge，整数与小数可以直接比",
  },
  { code: '{{.rule | default "无"}}', desc: "为空时用默认值" },
  { code: "{{.text | truncate 100}}", desc: "按字符截断，截掉了补「…」" },
  {
    code: '{{if contains "2160p" .torrent_title}}4K{{end}}',
    desc: "包含；还有 hasPrefix、hasSuffix",
  },
  { code: '{{.torrent_title | replace "." " "}}', desc: "替换；还有 upper、lower、trim" },
  { code: "{{bytes .size_bytes}}", desc: "字节数写成 15.00 GB" },
  {
    code: '{{date "01-02 15:04" .timestamp}}',
    desc: "按版式写时间（Go 的写法：2006-01-02 15:04:05）",
  },
  { code: '{{printf "%.1f" .bonus}}', desc: "格式化（宽度、精度最多 256）" },
  { code: "{{json .title}}", desc: "写成 JSON 字面量（Webhook 请求体里用）" },
];

function varCode(v: NotifyTemplateVar): string {
  return `{{.${v.key}}}`;
}

/** 点变量插入的内容：列表插一段 range，把每一项的字段都列出来 */
function snippetOf(v: NotifyTemplateVar): string {
  if (v.type === "list") {
    const fields = (v.item ?? []).map((f) => `{{.${f.key}}}`).join(" ");
    return `{{range .${v.key}}}\n${fields}\n{{end}}`;
  }
  return varCode(v);
}

function sampleOf(v: NotifyTemplateVar): string {
  if (v.sample === undefined || v.sample === null || v.type === "list") return "";
  const s = String(v.sample);
  return s.length > 40 ? `${s.slice(0, 40)}…` : s;
}

function insert(snippet: string) {
  const field = focused.value;
  const handle = field === "title" ? titleRef.value : textRef.value;
  const el = (field === "title" ? handle?.input : handle?.textarea) ?? null;
  const cur = form.value[field];
  const start = el?.selectionStart ?? cur.length;
  const end = el?.selectionEnd ?? cur.length;
  form.value[field] = cur.slice(0, start) + snippet + cur.slice(end);
  void nextTick(() => {
    if (!el) return;
    el.focus();
    const pos = start + snippet.length;
    el.setSelectionRange(pos, pos);
  });
}

async function load() {
  const pending = ds.run(() => chatopsApi.templates.list());
  const got = await pending;
  if (ds.isStale(pending) || !got) return;
  catalog.value = got;
  const first = got.events[0]?.type ?? "";
  open(
    currentType.value && got.events.some((e) => e.type === currentType.value)
      ? currentType.value
      : first,
  );
}

function open(type: string) {
  currentType.value = type;
  const v = catalog.value?.events.find((e) => e.type === type);
  form.value = { title: v?.title ?? "", text: v?.text ?? "" };
  focused.value = "text";
  void runPreview();
}

async function select(type: string) {
  if (type === currentType.value) return;
  if (dirty.value) {
    try {
      await ElMessageBox.confirm("这个事件的修改还没保存，切换以后会丢掉。", "放弃修改？", {
        confirmButtonText: "放弃并切换",
        cancelButtonText: "留下",
        type: "warning",
      });
    } catch {
      return;
    }
  }
  open(type);
}

function replaceView(v: NotifyTemplateView) {
  if (!catalog.value) return;
  catalog.value = {
    ...catalog.value,
    events: catalog.value.events.map((e) => (e.type === v.type ? v : e)),
  };
}

async function save() {
  const type = currentType.value;
  saving.value = true;
  try {
    const v = await chatopsApi.templates.save(type, { ...form.value });
    replaceView(v);
    if (currentType.value === type) form.value = { title: v.title, text: v.text };
    ElMessage.success(
      v.customized ? "已保存" : v.no_title ? "正文空着：已恢复默认" : "两项都空着：已恢复默认",
    );
  } catch (e) {
    ElMessage.error(apiErrorDetail(e, "保存失败"));
  } finally {
    saving.value = false;
  }
}

async function reset() {
  const v = current.value;
  if (!v) return;
  try {
    await ElMessageBox.confirm(
      `「${v.name}」恢复成内置的${v.no_title ? "正文" : "标题与正文"}？`,
      "恢复默认",
      {
        confirmButtonText: "恢复默认",
        cancelButtonText: "取消",
        type: "warning",
      },
    );
  } catch {
    return;
  }
  resetting.value = true;
  try {
    await chatopsApi.templates.reset(v.type);
    replaceView({ ...v, title: "", text: "", customized: false, updated_at: undefined });
    if (currentType.value === v.type) form.value = { title: "", text: "" };
    ElMessage.success("已恢复默认");
  } catch (e) {
    ElMessage.error(apiErrorDetail(e, "恢复默认失败"));
  } finally {
    resetting.value = false;
  }
}

// 预览：改了以后 400 毫秒再请求；晚到的旧结果丢掉
let previewSeq = 0;
let previewTimer: ReturnType<typeof setTimeout> | undefined;

async function runPreview() {
  const type = currentType.value;
  if (!type) return;
  const seq = ++previewSeq;
  try {
    const got = await chatopsApi.templates.preview(type, { ...form.value });
    if (seq === previewSeq) preview.value = { ok: true, title: got.title, text: got.text };
  } catch (e) {
    if (seq === previewSeq) preview.value = { ok: false, msg: apiErrorDetail(e, "预览失败") };
  }
}

watch(
  () => [form.value.title, form.value.text],
  () => {
    clearTimeout(previewTimer);
    previewTimer = setTimeout(() => void runPreview(), 400);
  },
);

onMounted(load);
onBeforeUnmount(() => clearTimeout(previewTimer));
</script>

<template>
  <div class="pt-cards pt-cards--side">
    <PtHeadSub
      >按事件改通知、ChatOps 命令回复与 AI
      助手回复的文字，所有通知通道都按它发；空着的一项用内置文字</PtHeadSub
    >

    <PtDataState
      v-if="!catalog"
      class="pt-cards__full"
      :state="ds.state.value"
      :sub="ds.errorText.value"
      data-testid="nt-state" />

    <template v-else>
      <PtPanel
        v-if="!isMobile"
        title="事件"
        icon="bell"
        :count="catalog.events.length"
        data-testid="nt-events">
        <el-input
          v-model="eventQuery"
          class="nt-search"
          size="small"
          clearable
          placeholder="搜索事件名称或类型"
          aria-label="搜索事件"
          data-testid="nt-event-search">
          <template #prefix><PtIcon name="search" :size="13" /></template>
        </el-input>
        <PtDataState
          v-if="!grouped.length"
          dense
          state="zero"
          title="没有匹配的事件"
          sub="换个关键词，名称和类型（例如 rss_pushed）都能搜">
          <template #action>
            <el-button size="small" @click="eventQuery = ''">
              <PtIcon name="x" :size="14" /><span>清空</span>
            </el-button>
          </template>
        </PtDataState>
        <section
          v-for="g in grouped"
          :key="g.key"
          class="nt-group"
          :aria-label="g.name"
          :data-testid="`nt-group-${g.key}`">
          <h3 class="nt-group__name">{{ g.name }}</h3>
          <p v-if="g.desc" class="nt-group__desc">{{ g.desc }}</p>
          <ul class="nt-list">
            <li v-for="e in g.events" :key="e.type">
              <button
                type="button"
                class="nt-item"
                :class="{ 'is-active': e.type === currentType }"
                :aria-current="e.type === currentType ? 'true' : undefined"
                :data-testid="`nt-event-${e.type}`"
                @click="select(e.type)">
                <span class="nt-item__name">{{ e.name }}</span>
                <span class="nt-item__meta">
                  <code>{{ e.type }}</code>
                  <PtStatusPill v-if="e.customized" tone="primary" size="sm">已自定义</PtStatusPill>
                </span>
              </button>
            </li>
          </ul>
        </section>
      </PtPanel>

      <!--
        编辑卡按自己的内容高，不跟着左边的长目录拉成两千多高：以前「恢复默认 / 保存」被顶到整页最底下。
        卡比一屏长时页脚贴住可视区底部。
      -->
      <PtPanel
        v-if="current"
        class="nt-editor"
        :title="current.name"
        icon="file-text"
        sticky-foot
        data-testid="nt-editor">
        <template #actions>
          <PtStatusPill
            :tone="current.customized ? 'primary' : 'neutral'"
            size="sm"
            data-testid="nt-state-pill"
            >{{ current.customized ? "已自定义" : "内置文字" }}</PtStatusPill
          >
        </template>

        <el-select
          v-if="isMobile"
          :model-value="currentType"
          class="nt-select"
          aria-label="选择事件"
          data-testid="nt-event-select"
          @change="select">
          <el-option-group v-for="g in grouped" :key="g.key" :label="g.name">
            <el-option
              v-for="e in g.events"
              :key="e.type"
              :value="e.type"
              :label="e.customized ? `${e.name}（已自定义）` : e.name" />
          </el-option-group>
        </el-select>
        <p class="nt-desc">{{ current.desc }}</p>

        <el-form class="pt-form" label-position="top" @submit.prevent>
          <el-form-item v-if="!noTitle" label="标题模板">
            <el-input
              ref="titleRef"
              v-model="form.title"
              placeholder="留空用内置标题"
              data-testid="nt-title"
              @focus="focused = 'title'" />
          </el-form-item>
          <el-form-item label="正文模板">
            <el-input
              ref="textRef"
              v-model="form.text"
              type="textarea"
              :autosize="{ minRows: 6, maxRows: 18 }"
              placeholder="留空用内置正文"
              data-testid="nt-text"
              @focus="focused = 'text'" />
          </el-form-item>
        </el-form>

        <h3 class="nt-sub">预览（示例数据）</h3>
        <div v-if="preview" class="nt-preview" data-testid="nt-preview">
          <template v-if="preview.ok">
            <div v-if="!noTitle" class="nt-preview__title">{{ preview.title }}</div>
            <div class="nt-preview__text">{{ preview.text }}</div>
          </template>
          <div v-else class="nt-preview__error" role="alert" data-testid="nt-preview-error">
            <PtIcon name="circle-alert" :size="14" aria-hidden="true" />
            <span>{{ preview.msg }}</span>
          </div>
        </div>

        <h3 class="nt-sub">
          变量<span class="nt-sub__hint"
            >点一下插到{{ focused === "title" ? "标题" : "正文" }}的光标处</span
          >
        </h3>
        <ul class="nt-vars" data-testid="nt-vars">
          <li v-for="v in visibleVars" :key="v.key" class="nt-var">
            <button
              type="button"
              class="nt-var__code"
              :data-testid="`nt-var-${v.key}`"
              :aria-label="`插入变量 ${v.key}`"
              @click="insert(snippetOf(v))">
              {{ varCode(v) }}
            </button>
            <span class="nt-var__type">{{ TYPE_LABELS[v.type] }}</span>
            <span class="nt-var__desc"
              >{{ v.desc
              }}<span v-if="sampleOf(v)" class="nt-var__sample">，例：{{ sampleOf(v) }}</span></span
            >
            <ul v-if="v.item?.length" class="nt-var__items">
              <li v-for="f in v.item" :key="f.key">
                <button
                  type="button"
                  class="nt-var__code"
                  :data-testid="`nt-var-${v.key}-${f.key}`"
                  :aria-label="`插入列表项变量 ${f.key}`"
                  @click="insert(varCode(f))">
                  {{ varCode(f) }}
                </button>
                <span class="nt-var__desc">{{ f.desc }}（在 range 里用）</span>
              </li>
            </ul>
          </li>
        </ul>

        <details class="nt-funcs">
          <summary>函数与写法</summary>
          <ul>
            <li v-for="f in FUNCS" :key="f.code">
              <code>{{ f.code }}</code
              ><span>{{ f.desc }}</span>
            </li>
          </ul>
        </details>

        <h3 class="nt-sub">内置的样子</h3>
        <div class="nt-preview nt-preview--builtin" data-testid="nt-builtin">
          <div v-if="!noTitle" class="nt-preview__title">{{ current.sample_title }}</div>
          <div class="nt-preview__text">{{ current.sample_text }}</div>
        </div>

        <template #footer>
          <el-button
            :disabled="!current.customized"
            :loading="resetting"
            data-testid="nt-reset"
            @click="reset">
            <PtIcon name="rotate-ccw" :size="14" /><span>恢复默认</span>
          </el-button>
          <el-button
            type="primary"
            :disabled="!dirty"
            :loading="saving"
            data-testid="nt-save"
            @click="save">
            <PtIcon name="save" :size="14" /><span>保存</span>
          </el-button>
        </template>
      </PtPanel>
    </template>
  </div>
</template>

<style scoped>
.nt-search {
  margin-bottom: var(--pt-space-3);
}

.nt-editor {
  align-self: start;
}

.nt-group + .nt-group {
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--pt-border);
}

.nt-group__name {
  margin: 0 0 2px;
  padding: 0 10px;
  font-size: 12px;
  font-weight: 600;
  color: var(--pt-t2);
}

.nt-group__desc {
  margin: 0 0 6px;
  padding: 0 10px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--pt-t3);
}

.nt-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.nt-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
  width: 100%;
  min-height: 44px;
  padding: 8px 10px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--pt-t1);
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.nt-item:hover {
  background: color-mix(in srgb, var(--pt-p) 6%, transparent);
}

.nt-item.is-active {
  background: color-mix(in srgb, var(--pt-p) 10%, transparent);
}

.nt-item:focus-visible,
.nt-var__code:focus-visible {
  outline: 2px solid var(--pt-p);
  outline-offset: -2px;
}

.nt-item__name {
  font-size: 13px;
}

.nt-item__meta {
  display: flex;
  gap: 6px;
  align-items: center;
  font-size: 12px;
  color: var(--pt-t3);
}

.nt-select {
  width: 100%;
  margin-bottom: 8px;
}

.nt-desc {
  margin: 0 0 12px;
  font-size: 12px;
  line-height: 1.6;
  color: var(--pt-t3);
}

.nt-sub {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: baseline;
  margin: 16px 0 8px;
  padding-top: 12px;
  border-top: 1px solid var(--pt-border);
  font-size: 13px;
  font-weight: 600;
  color: var(--pt-t1);
}

.nt-sub__hint {
  font-size: 12px;
  font-weight: 400;
  color: var(--pt-t3);
}

.nt-preview {
  padding: 10px 12px;
  border: 1px solid var(--pt-border);
  border-radius: 8px;
  font-size: 13px;
  line-height: 1.6;
  color: var(--pt-t1);
}

/* 标题与正文按原样换行（通知是纯文本） */
.nt-preview__title,
.nt-preview__text {
  white-space: pre-wrap;
  word-break: break-word;
}

.nt-preview__title {
  margin-bottom: 4px;
  font-weight: 600;
}

.nt-preview__error {
  display: flex;
  gap: 6px;
  align-items: flex-start;
  color: var(--pt-dang);
  white-space: normal;
}

.nt-preview--builtin {
  color: var(--pt-t2);
}

.nt-vars,
.nt-var__items {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.nt-var {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: baseline;
  font-size: 12px;
}

.nt-var__items {
  flex-basis: 100%;
  padding-left: 16px;
}

.nt-var__items li {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: baseline;
}

.nt-var__code {
  padding: 2px 6px;
  border: 1px solid var(--pt-border);
  border-radius: 6px;
  background: var(--pt-bg-surface-muted);
  color: var(--pt-p);
  font-family: var(--pt-font-mono);
  font-size: 12px;
  cursor: pointer;
}

.nt-var__code:hover {
  border-color: var(--pt-p);
}

.nt-var__type {
  font-size: 11px;
  color: var(--pt-t3);
}

.nt-var__desc {
  color: var(--pt-t2);
}

.nt-var__sample {
  color: var(--pt-t3);
}

.nt-funcs {
  margin-top: 12px;
  font-size: 12px;
  color: var(--pt-t2);
}

.nt-funcs summary {
  font-size: 13px;
  font-weight: 600;
  color: var(--pt-t1);
  cursor: pointer;
}

.nt-funcs ul {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 8px 0 0;
  padding: 0;
  list-style: none;
}

.nt-funcs li {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: baseline;
}

.nt-funcs code {
  font-family: var(--pt-font-mono);
  color: var(--pt-t1);
}
</style>
