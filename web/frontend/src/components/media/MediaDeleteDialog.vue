<script setup lang="ts">
/*
 * 删除一部片（媒体中心 M7，画板 03 的片段「删除这一季」）：条目详情大图里的「删除」、种子行的「删除这个种子」、
 * 库里的文件不见了与媒体服务器里删了的「处理」都打开它。打开时读删除预览（库里的文件、关联的种子与能不能删、订阅），
 * 按勾选删，每一步单独写结果。服务端执行时不信预览：现读下载器、重新算保护，预览以后才变成受保护的照样不删。
 * 桌面是 600 宽的弹窗，手机是全屏页。
 */
import { type DeletePreview, type DeleteResult, type DeleteTorrent, subscribeApi } from "@/api";
import PtIcon from "@/components/PtIcon";
import { useIsMobile } from "@/composables/useIsMobile";
import {
  type DeleteChoice,
  type DeleteTarget,
  type TorrentPick,
  canDelete,
  defaultChoice,
  deleteRequest,
  deleteSummary,
  deleteTitle,
  deleteTorrentSub,
  deleteTorrentTitle,
  existingLine,
  fileGroups,
  fileMeta,
  filesView,
  issueLine,
  protectedText,
  resultText,
  sharedBy,
  stepKindLabel,
  subscriptionOptions,
  torrentKey,
} from "@/utils/mediaDelete";
import { ElMessage } from "element-plus";
import { computed, ref, watch } from "vue";

const props = defineProps<{ modelValue: boolean; target: DeleteTarget | null }>();
const emit = defineEmits<{ "update:modelValue": [value: boolean]; done: [] }>();

const isMobile = useIsMobile();
const errText = (e: unknown, fallback: string) => (e as Error)?.message || fallback;
const visible = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emit("update:modelValue", v),
});

const preview = ref<DeletePreview | null>(null);
const choice = ref<DeleteChoice | null>(null);
const loading = ref(false);
const error = ref("");
const showFiles = ref(false);
const running = ref(false);
const result = ref<DeleteResult | null>(null);
let seq = 0;

async function load() {
  const t = props.target;
  if (!t) return;
  const my = ++seq;
  loading.value = true;
  error.value = "";
  preview.value = null;
  choice.value = null;
  result.value = null;
  showFiles.value = false;
  try {
    const got = await subscribeApi.deletePreview(t.selector);
    if (my !== seq) return;
    preview.value = got;
    choice.value = defaultChoice(got, t.preset, t.selector.media_type);
  } catch (e) {
    if (my !== seq) return;
    error.value = errText(e, "读取删除预览失败");
  } finally {
    if (my === seq) loading.value = false;
  }
}

// 每次打开（父组件每次给一个新的 target）都重新读：预览以后下载器里的种子、保护可能变了
watch(
  () => (props.modelValue ? props.target : null),
  (t) => {
    if (t) void load();
  },
  { immediate: true },
);

const title = computed(() => (props.target ? deleteTitle(props.target) : "删除"));
const req = computed(() =>
  props.target && preview.value && choice.value
    ? deleteRequest(props.target.selector, preview.value, choice.value)
    : null,
);
const ready = computed(() =>
  Boolean(req.value && preview.value && canDelete(req.value, preview.value)),
);
const summary = computed(() =>
  req.value && preview.value ? deleteSummary(req.value, preview.value) : "",
);
const files = computed(() =>
  props.target && preview.value ? filesView(preview.value.files, props.target.selector) : null,
);
const existing = computed(() => fileGroups(preview.value?.files ?? []).existing);
const subOptions = computed(() =>
  props.target && preview.value
    ? subscriptionOptions(preview.value, props.target.selector.media_type)
    : [],
);
/** 每个种子还在用这份数据、这次又不删的种子（有的话只删种子） */
const shared = computed<Record<string, string[]>>(() => {
  const c = choice.value;
  if (!preview.value || !c) return {};
  return Object.fromEntries(preview.value.torrents.map((t) => [torrentKey(t), sharedBy(t, c)]));
});
/** 「处理」打开的：最上面写要处理的问题 */
const issuesText = computed(() => {
  const p = preview.value;
  if (props.target?.preset.kind !== "issues" || !p?.issues.length) return "";
  return `要处理的：${p.issues.map(issueLine).join("；")}。删掉库里的文件与整理记录以后，这些问题改成已处理。`;
});

function pick(t: DeleteTorrent): TorrentPick {
  return choice.value!.torrents[torrentKey(t)]!;
}

/** 受保护的种子：勾「仍然删除」时一并勾上删，取消时一并取消 */
function setForce(t: DeleteTorrent, v: boolean) {
  const p = pick(t);
  p.force = v;
  p.on = v;
}

async function run() {
  const r = req.value;
  if (!r || !ready.value || running.value) return;
  running.value = true;
  try {
    result.value = await subscribeApi.deleteItem(r);
    // 删完就让页面重新读，结果留在弹窗里看
    emit("done");
  } catch (e) {
    ElMessage.error(errText(e, "删除失败"));
  } finally {
    running.value = false;
  }
}
</script>

<template>
  <el-dialog
    v-model="visible"
    class="pt-dialog md-dialog"
    :title="title"
    :width="isMobile ? '100%' : '600px'"
    :fullscreen="isMobile"
    align-center
    data-testid="delete-dialog">
    <div v-if="loading" class="md-state" data-testid="delete-loading">
      <PtIcon name="loader-circle" :size="16" class="pt-spin" />
      <span>正在读取库里的文件、关联的种子与订阅…</span>
    </div>
    <div v-else-if="error" class="pt-note pt-note--warn md-note" data-testid="delete-error">
      <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
      <span>{{ error }}</span>
      <button type="button" class="md-link" @click="load">重试</button>
    </div>

    <template v-else-if="result">
      <div
        class="pt-note md-note"
        :class="result.failed ? 'pt-note--warn' : 'pt-note--ok'"
        data-testid="delete-result">
        <PtIcon
          :name="result.failed ? 'triangle-alert' : 'circle-check'"
          :size="14"
          class="pt-note__icon" />
        <span>{{ resultText(result) }}</span>
      </div>
      <ul class="md-steps" data-testid="delete-steps">
        <li
          v-for="(st, i) in result.steps"
          :key="i"
          class="md-step"
          :class="{ 'is-fail': !st.ok }"
          data-testid="delete-step">
          <PtIcon :name="st.ok ? 'circle-check' : 'circle-x'" :size="14" class="md-step__icon" />
          <span class="md-step__kind">{{ stepKindLabel(st.kind) }}</span>
          <span class="md-step__body">
            <span class="md-step__target">{{ st.target }}</span>
            <span class="md-step__msg">{{ st.message }}</span>
          </span>
        </li>
      </ul>
    </template>

    <template v-else-if="preview && choice && files">
      <div
        v-if="preview.errors.length"
        class="pt-note pt-note--warn md-note"
        data-testid="delete-warn">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>有几部分没读到：{{ preview.errors.join("；") }}</span>
      </div>
      <div v-if="issuesText" class="pt-note md-note" data-testid="delete-issues">
        <PtIcon name="info" :size="14" class="pt-note__icon" />
        <span>{{ issuesText }}</span>
      </div>

      <!-- 库里的文件：勾选框、说明、「看文件」展开清单；导入前就有的文件另外勾 -->
      <section class="md-sec md-sec--files" data-testid="delete-files">
        <div class="md-row">
          <el-checkbox
            v-model="choice.files"
            class="md-check"
            :disabled="!files.enabled"
            aria-label="删除库里的文件"
            data-testid="delete-files-check" />
          <div class="md-row__text">
            <div class="md-row__title">删除库里的文件</div>
            <div class="md-row__sub">{{ files.line }}</div>
            <div v-for="n in files.notes" :key="n" class="md-row__sub">{{ n }}</div>
          </div>
          <button
            v-if="preview.files.length"
            type="button"
            class="md-link md-row__more"
            :aria-expanded="showFiles"
            data-testid="delete-files-more"
            @click="showFiles = !showFiles">
            {{ showFiles ? "收起" : "看文件" }}
          </button>
        </div>
        <div v-if="existing.length" class="md-row md-row--sub" data-testid="delete-existing">
          <el-checkbox
            v-model="choice.existing"
            class="md-check"
            aria-label="也删导入前就有的文件"
            data-testid="delete-existing-check" />
          <div class="md-row__text">
            <div class="md-row__title">也删导入前就有的文件</div>
            <div class="md-row__sub">{{ existingLine(preview.files) }}</div>
          </div>
        </div>
        <ul v-if="showFiles" class="md-files" data-testid="delete-file-list">
          <li v-for="f in preview.files" :key="f.id" class="md-file">
            <span class="md-file__path">{{ f.path }}</span>
            <span class="md-file__meta">{{ fileMeta(f) }}</span>
            <span v-if="f.note" class="md-file__meta" :class="{ 'is-warn': !f.deletable }">{{
              f.note
            }}</span>
          </li>
        </ul>
      </section>

      <!-- 种子：每台下载器上的每个种子一行，各自判断能不能删 -->
      <section class="md-sec md-sec--torrents" data-testid="delete-torrents">
        <div class="md-sec__title">种子（每台下载器分开判断）</div>
        <div v-if="!preview.torrents.length" class="md-row__sub md-empty">
          下载器里没有关联的种子
        </div>
        <div
          v-for="t in preview.torrents"
          :key="torrentKey(t)"
          class="md-tor"
          data-testid="delete-torrent">
          <el-checkbox
            :model-value="pick(t).on"
            class="md-check"
            :disabled="!t.deletable && !pick(t).force"
            :aria-label="`删除种子 ${t.title}`"
            data-testid="delete-torrent-check"
            @update:model-value="(v: string | number | boolean) => (pick(t).on = Boolean(v))" />
          <div class="md-tor__main">
            <div class="md-row__text">
              <div class="md-row__title" :title="t.title">{{ deleteTorrentTitle(t) }}</div>
              <div v-if="t.deletable" class="md-row__sub">{{ deleteTorrentSub(t) }}</div>
              <div v-else class="md-row__sub md-guard" :title="t.protection">
                <PtIcon name="shield" :size="12" class="md-guard__icon" />
                <span>{{ protectedText(t) }}</span>
              </div>
              <div
                v-if="t.covers_more && t.more"
                class="md-row__sub"
                data-testid="delete-torrent-more">
                {{ t.more }}，默认不勾
              </div>
              <div
                v-if="shared[torrentKey(t)]?.length"
                class="md-row__sub"
                data-testid="delete-torrent-shared">
                数据还有别的种子在用（{{ shared[torrentKey(t)]!.join("、") }}），只删种子
              </div>
            </div>
            <div class="md-tor__acts">
              <el-select
                v-if="t.deletable || pick(t).force"
                class="md-how"
                :model-value="shared[torrentKey(t)]?.length ? false : pick(t).data"
                :disabled="Boolean(shared[torrentKey(t)]?.length)"
                aria-label="连数据一起删还是只删种子"
                data-testid="delete-torrent-how"
                @update:model-value="(v: boolean) => (pick(t).data = v)">
                <el-option :value="true" label="连数据一起删" />
                <el-option :value="false" label="只删种子" />
              </el-select>
              <el-checkbox
                v-if="!t.deletable"
                class="md-force"
                :model-value="pick(t).force"
                data-testid="delete-torrent-force"
                @update:model-value="(v: string | number | boolean) => setForce(t, Boolean(v))"
                >仍然删除</el-checkbox
              >
            </div>
          </div>
        </div>
      </section>

      <!-- 订阅：暂停（默认）、删除、保留；没有订阅时不显示 -->
      <section
        v-if="preview.subscriptions.length"
        class="md-sec md-sec--subs"
        data-testid="delete-subscription">
        <div class="md-sec__title">订阅</div>
        <el-radio-group v-model="choice.subscription" class="md-subs">
          <div v-for="o in subOptions" :key="o.value" class="md-row md-sub">
            <el-radio
              :value="o.value"
              class="md-check"
              :aria-label="o.label"
              :data-testid="`delete-sub-${o.value}`" />
            <div class="md-row__text md-sub__text" @click="choice.subscription = o.value">
              <div class="md-row__title" :class="{ 'is-on': choice.subscription === o.value }">
                {{ o.label }}
              </div>
              <div class="md-row__sub">{{ o.desc }}</div>
            </div>
          </div>
        </el-radio-group>
      </section>
    </template>

    <template #footer>
      <span v-if="!result && summary" class="md-summary" data-testid="delete-summary">{{
        summary
      }}</span>
      <span v-else class="md-summary" />
      <el-button v-if="result" type="primary" data-testid="delete-close" @click="visible = false"
        >关闭</el-button
      >
      <template v-else>
        <el-button @click="visible = false">取消</el-button>
        <el-button
          type="danger"
          :loading="running"
          :disabled="!ready"
          data-testid="delete-run"
          @click="run"
          >删除</el-button
        >
      </template>
    </template>
  </el-dialog>
</template>

<style scoped>
.md-state {
  display: flex;
  gap: 8px;
  align-items: center;
  padding: 24px 0;
  font-size: var(--pt-fz-body);
  color: var(--pt-t2);
}

.md-note {
  margin-top: 13px;
}

.md-link {
  padding: 0;
  font: inherit;
  font-size: var(--pt-fz-sm);
  font-weight: 500;
  line-height: 1.3;
  color: var(--pt-p);
  white-space: nowrap;
  cursor: pointer;
  background: none;
  border: 0;
}

.md-link:hover {
  text-decoration: underline;
}

/* 一块（库里的文件、种子、订阅）之间是 552 宽的分隔；块的标题 11/700 t3（画板 03 的片段） */
.md-sec {
  padding-top: 11px;
  border-bottom: 1px solid var(--pt-border);
}

.md-sec:last-child {
  border-bottom: 0;
}

.md-sec--files {
  padding: 13px 0 15px;
}

.md-sec--torrents {
  padding-bottom: 7px;
}

.md-sec--subs {
  padding-bottom: 25px;
}

.md-sec__title {
  margin-bottom: 8px;
  font-size: var(--pt-fz-label);
  font-weight: 700;
  line-height: 1.3;
  color: var(--pt-t3);
}

/* 勾选框 14 在左，文字从 x 46 起（框 14 加 8） */
.md-row {
  display: flex;
  gap: 8px;
  align-items: flex-start;
  min-width: 0;
}

.md-row--sub {
  margin: 10px 0 0 22px;
}

.md-check.el-checkbox,
.md-check.el-radio {
  flex: none;
  height: 14px;
  margin: 4px 0 0;
}

/* 单选没有自己的字（标题在右边的文字块里）：空的 label 不占位 */
.md-check.el-radio :deep(.el-radio__label) {
  display: none;
}

.md-row__text {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  min-width: 0;
}

/* 标题 19 高、说明在 y + 20、15 高：一行 35（画板 03 的片段的行高；手机字号大一档时改回按比例） */
.md-row__title {
  font-size: var(--pt-fz-body);
  font-weight: 500;
  line-height: 19px;
  color: var(--pt-t1);
  overflow-wrap: anywhere;
}

.md-row__sub {
  margin-top: 1px;
  font-size: var(--pt-fz-label);
  line-height: 15px;
  color: var(--pt-t3);
  overflow-wrap: anywhere;
}

.md-row__more {
  flex: none;
  margin-top: 2px;
}

.md-empty {
  margin: 0 0 7px;
}

.md-files {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 10px 0 0 22px;
  padding: 8px 0 0;
  list-style: none;
  border-top: 1px dashed var(--pt-border);
}

.md-file {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
}

.md-file__path {
  font-size: var(--pt-fz-sm);
  color: var(--pt-t1);
  word-break: break-all;
}

.md-file__meta {
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

.md-file__meta.is-warn {
  color: var(--pt-warn);
}

/* 种子：一行 54（标题、说明 35，下 11 是从 x 46 起的分隔，再 7 到下一行），最后一行没有分隔 */
.md-tor {
  display: flex;
  gap: 8px;
  align-items: flex-start;
  margin-bottom: 7px;
}

.md-tor__main {
  display: flex;
  flex: 1 1 auto;
  gap: 12px;
  align-items: flex-start;
  min-width: 0;
  padding-bottom: 11px;
  border-bottom: 1px solid var(--pt-border);
}

.md-tor:last-child .md-tor__main {
  border-bottom: 0;
}

.md-guard {
  display: flex;
  gap: 4px;
  align-items: center;
  color: var(--pt-warn);
}

.md-guard__icon {
  flex: none;
}

.md-tor__acts {
  display: flex;
  flex: none;
  gap: 8px;
  align-items: center;
  margin-top: 2px;
}

/* 「连数据一起删 / 只删种子」：120×28，字 12、左右 10（画板 03 的片段） */
.md-how {
  width: 120px;
}

.md-how :deep(.el-select__wrapper) {
  min-height: 28px;
  padding: 0 10px;
  font-size: var(--pt-fz-sm);
  line-height: 20px;
}

/* 「仍然删除」：框在 492、字在 512，字的右边离内容边 16（画板 03 的片段） */
.md-force.el-checkbox {
  --el-checkbox-font-size: var(--pt-fz-sm);
  --el-checkbox-text-color: var(--pt-t2);

  height: 28px;
  margin-right: 16px;
}

.md-force :deep(.el-checkbox__label) {
  padding-left: 6px;
}

/* 订阅：三个单选，一行 46（标题 13、说明 11.5），选中的标题 500 */
.md-subs {
  display: flex;
  flex-direction: column;
  gap: 11px;
  align-items: stretch;
}

.md-sub__text {
  cursor: pointer;
}

.md-sub .md-row__title {
  font-weight: 400;
}

.md-sub .md-row__title.is-on {
  font-weight: 500;
}

/* 页脚：摘要在左，跟着勾选变 */
.md-summary {
  flex: 1 1 auto;
  min-width: 0;
  margin-right: auto;
  font-size: var(--pt-fz-sm);
  line-height: 1.4;
  color: var(--pt-t2);
  /* el-dialog 的页脚默认右对齐：摘要从左边写起 */
  text-align: left;
}

/* 删完以后的每一步 */
.md-steps {
  display: flex;
  flex-direction: column;
  margin: 12px 0 13px;
  padding: 0;
  list-style: none;
}

.md-step {
  display: flex;
  gap: 8px;
  align-items: flex-start;
  min-width: 0;
  padding: 8px 0;
  border-bottom: 1px solid var(--pt-border);
}

.md-step:last-child {
  border-bottom: 0;
}

.md-step__icon {
  flex: none;
  margin-top: 2px;
  color: var(--pt-ok);
}

.md-step.is-fail .md-step__icon {
  color: var(--pt-dang);
}

.md-step__kind {
  flex: none;
  width: 44px;
  font-size: var(--pt-fz-sm);
  line-height: 1.5;
  color: var(--pt-t3);
}

.md-step__body {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  min-width: 0;
}

.md-step__target {
  font-size: var(--pt-fz-body);
  line-height: 1.4;
  color: var(--pt-t1);
  word-break: break-all;
}

.md-step__msg {
  font-size: var(--pt-fz-label);
  line-height: 1.3;
  color: var(--pt-t3);
}

.md-step.is-fail .md-step__msg {
  color: var(--pt-dang);
}

@media (max-width: 768px) {
  .md-summary {
    flex-basis: 100%;
  }

  .md-row__title {
    line-height: 1.4;
  }

  .md-row__sub {
    margin-top: 2px;
    line-height: 1.3;
  }
}
</style>

<style>
/*
 * 删除弹窗的外框（画板 03 的片段）：页头 56（标题 16/700，关闭在右边 24 以内）、正文左右 24、页脚 60。
 * el-dialog 会 teleport 到 body，挂在全局层；只改这一个弹窗，别的弹窗还是 atoms.css 的 42 高页头。
 */
.md-dialog.el-dialog .el-dialog__header {
  height: 57px;
  padding: 0 24px;
}

.md-dialog.el-dialog .el-dialog__title {
  font-size: var(--pt-fz-h2);
  font-weight: 700;
  line-height: 1.3;
}

.md-dialog.el-dialog .el-dialog__headerbtn {
  width: 64px;
  height: 56px;
}

.md-dialog.el-dialog .el-dialog__body {
  padding: 0 24px;
}

.md-dialog.el-dialog .el-dialog__footer {
  height: 60px;
  padding: 0 24px;
}

.md-dialog.el-dialog .el-dialog__footer .el-button {
  min-width: 72px;
  margin-left: 0;
}

/* 手机是全屏页：不留边、不要圆角；左右 16（375 − 32），页脚能放两行（摘要一行、按钮一行） */
.md-dialog.el-dialog.is-fullscreen {
  max-height: none;
  height: 100%;
  border: 0;
  border-radius: 0;
}

@media (max-width: 768px) {
  .md-dialog.el-dialog .el-dialog__header,
  .md-dialog.el-dialog .el-dialog__body {
    padding-right: 16px;
    padding-left: 16px;
  }

  .md-dialog.el-dialog .el-dialog__footer {
    flex-wrap: wrap;
    height: auto;
    padding: 10px 16px calc(10px + env(safe-area-inset-bottom));
  }
}
</style>
