<script setup lang="ts">
/*
 * AI 助手的输入框（设计稿「AI 助手对话页」方向 B：浮在对话下面的一张圆角卡片）：
 * Element-Plus-X 的 XSender（Enter 发送、Shift+Enter 换行，输入法组字时的 Enter 不发），
 * 上面一排是要发的附件（FilesCard），下面一排左边是添加附件、选模型、选推理等级（两个圆角下拉框），右边是圆形的发送 / 停止。
 * 附件可以点回形针选、粘贴、拖进来；能不能发、传没传完由页面管，这里只把文件交上去。
 * 配了语音输入（Voltip）时多一个麦克风：点一下开始录、再点一下停下，录音经 pt-tools 交给 Voltip 识别，文字插在输入框的光标处。
 */
import { ApiError, assistantApi, type AssistantEffort, type AssistantUsage } from "@/api";
import AssistantUsageMeter from "@/components/assistant/AssistantUsageMeter.vue";
import PtIcon from "@/components/PtIcon";
import { useIsMobile } from "@/composables/useIsMobile";
import {
  attachmentDesc,
  ATTACH_ACCEPT,
  type ComposerAttachment,
  EFFORT_OPTIONS,
  fileTypeOf,
  MAX_ATTACHMENTS,
} from "@/utils/assistant";
import { VOICE_MAX_SECONDS, VoiceRecorder, voiceUnavailable } from "@/utils/voiceRecorder";
import { ElMessage } from "element-plus";
import { FilesCard, XSender } from "vue-element-plus-x";
import { computed, onBeforeUnmount, ref, watch } from "vue";

const props = defineProps<{
  running: boolean;
  sending: boolean;
  attachments: ComposerAttachment[];
  models: string[];
  modelsLoading: boolean;
  modelsError: string;
  /** 设置里的模型（下拉框里标「默认」） */
  defaultModel: string;
  /** 这个会话的上下文用量（新对话没有） */
  usage?: AssistantUsage | null;
  /** 配了语音输入（Voltip） */
  voice?: boolean;
}>();

const model = defineModel<string>("model", { required: true });
const effort = defineModel<AssistantEffort>("effort", { required: true });

const emit = defineEmits<{
  submit: [text: string];
  cancel: [];
  files: [files: File[]];
  remove: [uid: string];
  /** 打开模型下拉框：页面这时才去读服务商的模型列表 */
  "load-models": [];
}>();

const sender = ref<InstanceType<typeof XSender> | null>(null);
/** 手机上一行放不下「推理：不思考」：只写等级，灯泡图标说明是推理 */
const isMobile = useIsMobile();
const fileInput = ref<HTMLInputElement | null>(null);
const hasText = ref(false);
const dragging = ref(false);

const uploading = computed(() => props.attachments.some((a) => a.status === "uploading"));
const ready = computed(() => props.attachments.some((a) => a.status === "done"));
const canSend = computed(
  () => !props.running && !props.sending && !uploading.value && (hasText.value || ready.value),
);
const full = computed(() => props.attachments.length >= MAX_ATTACHMENTS);
const attachTip = computed(() =>
  full.value
    ? `一条消息最多 ${MAX_ATTACHMENTS} 个附件`
    : "添加图片或文本文件，也可以粘贴、拖进来；图片只随这一条消息发，要选能看图的模型",
);

function text(): string {
  return sender.value?.getModelValue().text.trim() ?? "";
}

function onChange() {
  hasText.value = text() !== "";
}

/*
 * XSender 收到输入以后等下一帧才把字记进它自己的模型，Enter 却是按下那一刻就发：刚打完字马上按 Enter，
 * 最后几个字还没记进去（自动输入时实测丢了最后一个字）。先等一帧再读。
 */
async function submit() {
  await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
  const t = text();
  hasText.value = t !== "";
  if (!canSend.value) return;
  emit("submit", t);
}

function pick() {
  fileInput.value?.click();
}

function onPicked(e: Event) {
  const input = e.target as HTMLInputElement;
  if (input.files?.length) emit("files", [...input.files]);
  input.value = "";
}

function onPaste(_first: File, list: FileList) {
  emit("files", [...list]);
}

function onDrop(e: DragEvent) {
  dragging.value = false;
  const files = e.dataTransfer?.files;
  if (files?.length) emit("files", [...files]);
}

/** 本地预览图：传图片时用浏览器里的文件，不用再下载一次 */
const previews = new Map<string, string>();
function previewOf(a: ComposerAttachment): string | undefined {
  if (a.kind !== "image" || !a.file) return undefined;
  let url = previews.get(a.uid);
  if (!url) {
    url = URL.createObjectURL(a.file);
    previews.set(a.uid, url);
  }
  return url;
}
watch(
  () => props.attachments.map((a) => a.uid),
  (uids) => {
    for (const [uid, url] of previews) {
      if (!uids.includes(uid)) {
        URL.revokeObjectURL(url);
        previews.delete(uid);
      }
    }
  },
);
onBeforeUnmount(() => {
  for (const url of previews.values()) URL.revokeObjectURL(url);
  previews.clear();
});

/** Element Plus 的下拉框把空串当成没选（显示占位文字）：「默认」在下拉框里写成 default，交出去时换回空串 */
const effortSel = computed({
  get: () => effort.value || "default",
  set: (v: string) => {
    effort.value = (v === "default" ? "" : v) as AssistantEffort;
  },
});

const modelOptions = computed(() => {
  const list = props.models.length ? [...props.models] : [];
  if (props.defaultModel && !list.includes(props.defaultModel)) list.unshift(props.defaultModel);
  if (model.value && !list.includes(model.value)) list.push(model.value);
  return list;
});

/* ---- 语音输入 ---- */

type VoiceState = "idle" | "recording" | "transcribing";
const voiceState = ref<VoiceState>("idle");
const voiceSeconds = ref(0);
const voiceLevel = ref(0);
let recorder: VoiceRecorder | null = null;
let voiceTimer: ReturnType<typeof setInterval> | undefined;

const voiceTip = computed(() => {
  if (voiceState.value === "recording") return "点一下停止并识别（Esc 取消）";
  if (voiceState.value === "transcribing") return "正在识别…";
  return voiceUnavailable() || "语音输入：点一下开始说话（Voltip 识别）";
});

const voiceClock = computed(() => {
  const s = Math.floor(voiceSeconds.value);
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
});

function stopVoiceTimer() {
  if (voiceTimer) clearInterval(voiceTimer);
  voiceTimer = undefined;
}

async function toggleVoice() {
  if (voiceState.value === "transcribing") return;
  if (voiceState.value === "recording") {
    await finishVoice();
    return;
  }
  const why = voiceUnavailable();
  if (why) {
    ElMessage.warning(why);
    return;
  }
  const rec = new VoiceRecorder();
  rec.onLevel = (l) => (voiceLevel.value = l);
  try {
    await rec.start();
  } catch (e) {
    await rec.cancel();
    ElMessage.error(micError(e));
    return;
  }
  recorder = rec;
  voiceSeconds.value = 0;
  voiceState.value = "recording";
  voiceTimer = setInterval(() => {
    voiceSeconds.value = rec.seconds;
    if (rec.seconds >= VOICE_MAX_SECONDS) {
      ElMessage.info(`一段最多录 ${VOICE_MAX_SECONDS / 60} 分钟，先识别这一段`);
      void finishVoice();
    }
  }, 250);
}

async function finishVoice() {
  const rec = recorder;
  recorder = null;
  stopVoiceTimer();
  if (!rec) return;
  voiceState.value = "transcribing";
  try {
    const wav = await rec.stop();
    if (!wav) {
      ElMessage.warning("没有录到声音");
      return;
    }
    const got = (await assistantApi.transcribe(wav)).trim();
    if (!got) {
      ElMessage.info("没有听到说话");
      return;
    }
    // XSender 的 setText 是在光标处插入：前后都是英文字母或数字时（多半光标在末尾）中间补一个空格，中文不补
    const sep = /[A-Za-z0-9]$/.test(text()) && /^[A-Za-z0-9]/.test(got) ? " " : "";
    sender.value?.setText(sep + got);
    hasText.value = true;
    sender.value?.focus("end");
  } catch (e) {
    ElMessage.error(e instanceof ApiError ? e.message : "识别失败");
  } finally {
    voiceState.value = "idle";
    voiceLevel.value = 0;
  }
}

async function cancelVoice() {
  const rec = recorder;
  recorder = null;
  stopVoiceTimer();
  voiceState.value = "idle";
  voiceLevel.value = 0;
  await rec?.cancel();
}

function micError(e: unknown): string {
  const name = e instanceof DOMException ? e.name : "";
  if (name === "NotAllowedError" || name === "SecurityError") {
    return "没有麦克风权限：在浏览器地址栏左边的网站设置里允许使用麦克风";
  }
  if (name === "NotFoundError") return "没有找到麦克风";
  if (name === "NotReadableError") return "麦克风被别的程序占用了";
  return "打不开麦克风";
}

function onVoiceKey(e: KeyboardEvent) {
  if (e.key === "Escape" && voiceState.value === "recording") void cancelVoice();
}
window.addEventListener("keydown", onVoiceKey);
onBeforeUnmount(() => {
  window.removeEventListener("keydown", onVoiceKey);
  void cancelVoice();
});

defineExpose({
  /** 发出去以后清空输入框 */
  clear: () => {
    sender.value?.clear();
    hasText.value = false;
  },
  /** 填进一段文字（点了推荐的问题、发送失败放回去） */
  setText: (t: string) => {
    sender.value?.setText(t);
    hasText.value = t.trim() !== "";
  },
  focus: () => sender.value?.focus("end"),
});
</script>

<template>
  <div
    class="as-composer"
    :class="{ 'is-dragging': dragging }"
    data-testid="as-composer"
    @dragover.prevent="dragging = true"
    @dragleave.prevent="dragging = false"
    @drop.prevent="onDrop">
    <XSender
      ref="sender"
      class="as-sender"
      variant="updown"
      placeholder="问点什么，Enter 发送，Shift+Enter 换行"
      :max-length="4000"
      @change="onChange"
      @submit="submit"
      @cancel="emit('cancel')"
      @paste-file="onPaste">
      <template v-if="attachments.length" #header>
        <div class="as-attach" data-testid="as-attachments">
          <!-- 删除按钮自己画：FilesCard 自带的只在鼠标悬停时出现，手机与键盘都点不到 -->
          <div
            v-for="a in attachments"
            :key="a.uid"
            class="as-attach__item"
            :data-testid="`as-attachment-${a.uid}`">
            <FilesCard
              :uid="a.uid"
              :name="a.name"
              :file-size="a.size"
              :file-type="fileTypeOf(a)"
              :description="a.status === 'error' ? undefined : attachmentDesc(a)"
              icon-color="var(--pt-p)"
              :thumb-url="previewOf(a)"
              :url="previewOf(a)"
              :status="a.status"
              :error-tip="a.error"
              img-variant="square" />
            <button
              type="button"
              class="as-attach__del"
              :aria-label="`删除附件 ${a.name}`"
              :data-testid="`as-attachment-del-${a.uid}`"
              @click="emit('remove', a.uid)">
              <PtIcon name="x" :size="12" />
            </button>
          </div>
        </div>
      </template>
      <template #prefix>
        <div class="as-tools">
          <el-tooltip :content="attachTip" placement="top">
            <button
              type="button"
              class="as-round"
              :disabled="full || running"
              aria-label="添加附件"
              data-testid="as-attach"
              @click="pick">
              <PtIcon name="paperclip" :size="16" />
            </button>
          </el-tooltip>
          <input
            ref="fileInput"
            class="as-file"
            type="file"
            multiple
            :accept="ATTACH_ACCEPT"
            data-testid="as-file"
            @change="onPicked" />
          <el-select
            v-model="model"
            class="as-pill as-model-select"
            filterable
            allow-create
            default-first-option
            :loading="modelsLoading"
            :no-data-text="modelsError || '没有可选的模型：可以直接输入模型名'"
            aria-label="模型"
            data-testid="as-model"
            @visible-change="(open: boolean) => open && emit('load-models')">
            <template #prefix><PtIcon name="cpu" :size="14" /></template>
            <el-option v-for="m in modelOptions" :key="m" :value="m" :label="m">
              <span>{{ m }}</span>
              <span v-if="m === defaultModel" class="as-model-default">默认</span>
            </el-option>
          </el-select>
          <el-select
            v-model="effortSel"
            class="as-pill as-effort-select"
            aria-label="推理等级"
            data-testid="as-effort">
            <template #prefix><PtIcon name="lightbulb" :size="14" /></template>
            <el-option
              v-for="o in EFFORT_OPTIONS"
              :key="o.value || 'default'"
              :value="o.value || 'default'"
              :label="isMobile ? o.label : `推理：${o.label}`" />
          </el-select>
        </div>
      </template>
      <template #action-list>
        <AssistantUsageMeter v-if="usage" :usage="usage" class="as-usage" />
        <el-tooltip v-if="voice" :content="voiceTip" placement="top">
          <button
            type="button"
            class="as-mic"
            :class="`is-${voiceState}`"
            :disabled="voiceState === 'transcribing' || (running && voiceState === 'idle')"
            :aria-label="voiceState === 'recording' ? '停止录音并识别' : '语音输入'"
            :aria-pressed="voiceState === 'recording'"
            data-testid="as-mic"
            @click="toggleVoice">
            <PtIcon
              :name="
                voiceState === 'recording'
                  ? 'square'
                  : voiceState === 'transcribing'
                    ? 'loader-circle'
                    : 'mic'
              "
              :size="15"
              :class="{ 'as-spin': voiceState === 'transcribing' }" />
            <span
              v-if="voiceState === 'recording'"
              class="as-mic__clock"
              data-testid="as-mic-clock"
              >{{ voiceClock }}</span
            >
            <span
              v-if="voiceState === 'recording'"
              class="as-mic__level"
              :style="{ transform: `scaleY(${Math.min(1, 0.2 + voiceLevel * 6)})` }"
              aria-hidden="true" />
          </button>
        </el-tooltip>
        <button
          v-if="running"
          type="button"
          class="as-go is-stop"
          aria-label="停止回答"
          title="停止回答"
          data-testid="as-stop"
          @click="emit('cancel')">
          <PtIcon name="pause" :size="15" />
        </button>
        <button
          v-else
          type="button"
          class="as-go"
          :disabled="!canSend"
          :aria-busy="sending"
          aria-label="发送"
          title="发送"
          data-testid="as-send"
          @click="submit">
          <PtIcon
            :name="sending ? 'loader-circle' : 'send'"
            :size="16"
            :class="{ 'as-spin': sending }" />
        </button>
      </template>
    </XSender>
    <p class="as-composer__note">对话、附件与工具结果会发给所选的模型服务商</p>
  </div>
</template>

<style scoped>
.as-composer {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

/*
 * 卡片：白底、圆角 20、浅边框加一层柔和的阴影（画板 composer）。
 * XSender 的边框画在 ::after 上、聚焦时加粗成 2px 主色：换成 1px 主色，阴影不变。
 * 它自己的选择器都是三到六级类名，下面的覆盖要比它多一级。
 */
.as-sender {
  width: 100%;
}

.as-composer :deep(.elx-x-sender),
.as-composer :deep(.elx-x-sender:focus-within) {
  border-radius: 20px;
  background: var(--pt-surface);
  box-shadow:
    0 8px 24px -6px color-mix(in srgb, var(--pt-t1) 14%, transparent),
    0 0 0 1px color-mix(in srgb, var(--pt-t1) 4%, transparent);
}

.as-composer :deep(.elx-x-sender::after) {
  border-width: 1px;
  border-color: var(--pt-border);
}

.as-composer :deep(.elx-x-sender:focus-within::after) {
  border-width: 1px;
  border-color: var(--pt-p);
}

.as-composer.is-dragging :deep(.elx-x-sender::after) {
  border: 2px dashed var(--pt-p);
}

.as-composer .as-sender :deep(.elx-x-sender__header .elx-x-sender__header-container) {
  border-bottom: 0;
}

.as-composer .as-sender :deep(.elx-x-sender__content) {
  padding: 4px 12px 10px;
}

.as-composer .as-sender :deep(.elx-x-sender__content .elx-x-sender__chat-room .elx-x-sender__chat) {
  font-size: var(--pt-fz-body);
  line-height: 20px;
}

.as-composer .as-sender :deep(.chat-rich-text) {
  max-height: 220px;
  padding: 8px 2px;
  color: var(--pt-t1);
}

/* Element-Plus-X 把占位文字写成粗体（五个类名的选择器），看着像已经写了字：换成普通的占位样式，选择器要比它多一级。
   颜色用 t3：t4 只给装饰与禁用态（paletteContrast.test.ts） */
.as-composer
  .as-sender
  :deep(
    .elx-x-sender__content .elx-x-sender__chat-room .elx-x-sender__chat .chat-placeholder-wrap
  ) {
  font-weight: 400;
  color: var(--pt-t3);
}

.as-composer .as-sender :deep(.elx-x-sender__content .elx-x-sender__prefix) {
  height: 34px;
  padding-left: 0;
}

.as-composer .as-sender :deep(.elx-x-sender__content .elx-x-sender__action-list) {
  height: 34px;
}

.as-attach {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  padding: 12px 14px 0;
}

.as-attach__item {
  position: relative;
  --elx-files-card-bg: var(--pt-surface);
  --elx-files-card-border-color: var(--pt-border);
}

.as-attach__item :deep(.elx-files-card-content .elx-files-card-name) {
  font-size: var(--pt-fz-body);
  font-weight: 500;
}

.as-attach__item :deep(.elx-files-card-content .elx-files-card-description) {
  font-size: var(--pt-fz-label);
}

.as-attach__del {
  position: absolute;
  top: -6px;
  right: -6px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  padding: 0;
  border: 1px solid var(--pt-border);
  border-radius: 50%;
  background: var(--pt-surface);
  color: var(--pt-t2);
  cursor: pointer;
}

.as-attach__del:hover {
  color: var(--pt-dang);
}

.as-attach__del:focus-visible,
.as-round:focus-visible,
.as-go:focus-visible {
  outline: 2px solid var(--pt-p);
  outline-offset: 2px;
}

.as-tools {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}

.as-file {
  display: none;
}

/* 添加附件：没有底色的圆形图标按钮 */
.as-round {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  padding: 0;
  border: 0;
  border-radius: 50%;
  background: transparent;
  color: var(--pt-t2);
  cursor: pointer;
}

.as-round:hover:not(:disabled) {
  background: var(--pt-hover);
  color: var(--pt-t1);
}

.as-round:disabled {
  color: var(--pt-t4);
  cursor: not-allowed;
}

/* 模型与推理等级：浅灰底的圆角下拉框，没有边框 */
.as-pill :deep(.el-select__wrapper) {
  min-height: 30px;
  padding: 0 10px 0 12px;
  border-radius: 15px;
  background: var(--pt-hover);
  box-shadow: none;
  font-size: var(--pt-fz-sm);
}

.as-pill :deep(.el-select__wrapper.is-hovering:not(.is-focused)),
.as-pill :deep(.el-select__wrapper:hover) {
  box-shadow: none;
}

.as-pill :deep(.el-select__wrapper.is-focused) {
  box-shadow: 0 0 0 1px var(--pt-p) inset;
}

.as-pill :deep(.el-select__prefix) {
  color: var(--pt-t2);
}

.as-pill :deep(.el-select__placeholder:not(.is-transparent)) {
  color: var(--pt-t1);
}

.as-model-select {
  width: 190px;
}

.as-effort-select {
  width: 132px;
}

.as-model-default {
  margin-left: 8px;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
}

/* 发送 / 停止：主色的圆形按钮 */
.as-go {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  padding: 0;
  border: 0;
  border-radius: 50%;
  background: var(--pt-p);
  color: var(--pt-on-p);
  cursor: pointer;
}

.as-go:hover:not(:disabled) {
  background: var(--pt-p-hover);
}

.as-go:disabled {
  background: var(--pt-hover);
  color: var(--pt-t4);
  cursor: not-allowed;
}

.as-go.is-stop {
  background: var(--pt-dang-soft);
  color: var(--pt-dang);
}

.as-go.is-stop:hover {
  background: var(--pt-dang);
  color: var(--pt-on-solid);
}

.as-usage {
  margin-right: 4px;
}

/* 麦克风：没录时是没有底色的圆形按钮；录音时变红、带时长与音量 */
.as-mic {
  display: inline-flex;
  gap: 6px;
  align-items: center;
  justify-content: center;
  min-width: 34px;
  height: 34px;
  margin-right: 6px;
  padding: 0 9px;
  border: 0;
  border-radius: 17px;
  background: transparent;
  color: var(--pt-t2);
  font: inherit;
  font-size: var(--pt-fz-label);
  font-variant-numeric: tabular-nums;
  cursor: pointer;
}

.as-mic:hover:not(:disabled) {
  background: var(--pt-hover);
  color: var(--pt-t1);
}

.as-mic:disabled {
  color: var(--pt-t4);
  cursor: not-allowed;
}

.as-mic:focus-visible {
  outline: 2px solid var(--pt-p);
  outline-offset: 2px;
}

/* 录音中鼠标停在上面也保持红色（悬停的样式选择器更具体，要一起写上） */
.as-mic.is-recording,
.as-mic.is-recording:hover:not(:disabled) {
  background: var(--pt-dang-soft);
  color: var(--pt-dang);
}

/* 时长的位数会变：按最长的 m:ss 留宽，按钮不跟着抖 */
.as-mic__clock {
  min-width: 28px;
  text-align: left;
}

.as-mic__level {
  width: 3px;
  height: 14px;
  border-radius: 2px;
  background: currentColor;
  transform-origin: center;
  transition: transform 0.1s linear;
}

.as-composer__note {
  margin: 0;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
  text-align: center;
}

.as-spin {
  animation: as-spin 1s linear infinite;
}

@keyframes as-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (prefers-reduced-motion: reduce) {
  .as-spin {
    animation: none;
  }
}

/* 手机上回形针、模型、推理等级与发送要放在一行（390 宽的屏幕上输入框里约 266 宽） */
@media (max-width: 768px) {
  .as-model-select {
    width: 124px;
  }

  .as-effort-select {
    width: 92px;
  }
}
</style>
