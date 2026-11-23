<script setup lang="ts">
/*
 * 一个会话的对话区：消息（或者空对话的欢迎页）加底部的输入框。读会话、接事件流、发消息、停止、确认写操作都在这里，
 * AI 助手对话页（views/Assistant.vue）与每一页右下角的悬浮窗（AssistantFloat.vue）共用它。
 *
 * 打开哪个会话由外面经 v-model:conversation-id 给，null 是还没发第一条消息的新对话；
 * 新对话发出第一条消息时这里建会话，再把编号交回去（created，然后 update:conversationId）。
 * 读到的会话记成「最近打开的会话」：悬浮窗打开时接着它。会话已经删了（404）就换成新对话，不报错。
 *
 * 版式按设计稿「AI 助手对话页」方向 B（居中阅读）：对话居中 720 宽、没有卡片和头像（Element-Plus-X 的 BubbleList
 * 负责滚动与跟随），每次回答的过程是一条时间线，回答完自动折成一行（AssistantTurn）；输入框浮在底部（AssistantComposer）。
 * compact 是悬浮窗里的收紧版式：边距小一些，推荐的问题少一些、排一列。
 *
 * active 为假时（悬浮窗收起）：没在回答就断开事件流，在回答就等它回答完再断开 —— 每个标签页不一直占着一条连接
 * （局域网里多半是 HTTP/1.1，一个站点最多 6 条）；重新看见时从读到的最后一个事件接着读，丢了的话服务端会叫重新读会话。
 */
import {
  ApiError,
  assistantApi,
  type AssistantConfirmation,
  type AssistantConversation,
  type AssistantEffort,
  type AssistantEvent,
  type AssistantStatus,
} from "@/api";
import AssistantComposer from "@/components/assistant/AssistantComposer.vue";
import AssistantTurn from "@/components/assistant/AssistantTurn.vue";
import PtIcon from "@/components/PtIcon";
import {
  applyEvent,
  attachmentDesc,
  attachProblem,
  type ChatItem,
  chatItems,
  type ChatState,
  chatStateFrom,
  type ChatTurn,
  type ComposerAttachment,
  emptyChatState,
  fileTypeOf,
  MAX_ATTACHMENTS,
  rememberConversation,
} from "@/utils/assistant";
import { ElMessage } from "element-plus";
import { BubbleList, FilesCard } from "vue-element-plus-x";
import { computed, onBeforeUnmount, ref, watch } from "vue";

const props = withDefaults(
  defineProps<{
    status: AssistantStatus;
    /** 悬浮窗里的收紧版式 */
    compact?: boolean;
    /** 看得见（悬浮窗收起时为假） */
    active?: boolean;
  }>(),
  { compact: false, active: true },
);

/** 打开的会话；null 是还没发第一条消息的新对话 */
const conversationId = defineModel<number | null>("conversationId", { default: null });

const emit = defineEmits<{
  /** 新对话的第一条消息建好了会话（会话列表把它放到最上面） */
  created: [conv: AssistantConversation];
  /** 读到了打开的会话（悬浮窗的标题用它） */
  opened: [conv: AssistantConversation];
  /** 一次回答开始或结束了（会话列表刷新「回答中」） */
  activity: [];
  /** 在不在回答、有几个写操作等确认（悬浮窗收起时按钮上提示） */
  progress: [p: { running: boolean; pending: number }];
}>();

/** 这里打开着的会话（外面的 v-model 跟着它）；null 是新对话 */
const cur = ref<number | null>(null);
const chat = ref<ChatState>(emptyChatState());
const sending = ref(false);
const deciding = ref(0);
/** 事件流断了且浏览器不再重连（会话失效之类） */
const streamLost = ref(false);
const composer = ref<InstanceType<typeof AssistantComposer> | null>(null);
let source: EventSource | null = null;

/* ---- 模型、推理等级与附件 ---- */

const selModel = ref(props.status.model);
const selEffort = ref<AssistantEffort>("");
const models = ref<string[]>([]);
const modelsLoading = ref(false);
const modelsError = ref("");
let modelsLoaded = false;
const attachments = ref<ComposerAttachment[]>([]);
let uidSeq = 0;

const defaultModel = computed(() => props.status.model ?? "");

// 设置里换了默认模型（悬浮窗在设置页上也一直开着）：选着旧默认的跟着换，自己选过别的不动
watch(defaultModel, (now, before) => {
  if (selModel.value === before) selModel.value = now;
});

async function loadModels() {
  if (modelsLoaded || modelsLoading.value) return;
  modelsLoading.value = true;
  try {
    const got = await assistantApi.models();
    models.value = got.models;
    modelsError.value = got.error ? `读不到服务商的模型列表：${got.error}` : "";
    modelsLoaded = !got.error;
  } catch (e) {
    modelsError.value = errText(e, "读不到模型列表");
  } finally {
    modelsLoading.value = false;
  }
}

/** 打开会话时沿用这个会话最后一次选的模型与推理等级 */
function restoreOptions() {
  const users = chat.value.messages.filter((m) => m.role === "user");
  const last = users[users.length - 1];
  selModel.value = last?.model || defaultModel.value;
  selEffort.value = last?.effort ?? "";
}

function addFiles(files: File[]) {
  for (const file of files) {
    if (attachments.value.length >= MAX_ATTACHMENTS) {
      ElMessage.warning(`一条消息最多 ${MAX_ATTACHMENTS} 个附件`);
      return;
    }
    const problem = attachProblem(file);
    if (problem) {
      ElMessage.warning(problem);
      continue;
    }
    const a: ComposerAttachment = {
      uid: `a${++uidSeq}`,
      name: file.name,
      kind: file.type.startsWith("image/") ? "image" : "text",
      size: file.size,
      status: "uploading",
      file,
    };
    attachments.value = [...attachments.value, a];
    void upload(a, file);
  }
}

async function upload(a: ComposerAttachment, file: File) {
  try {
    const got = await assistantApi.upload(file);
    patchAttachment(a.uid, { status: "done", id: got.id, name: got.name, kind: got.kind });
  } catch (e) {
    const msg = errText(e, "上传失败");
    patchAttachment(a.uid, { status: "error", error: msg });
    ElMessage.error(`「${a.name}」${msg}`);
  }
}

function patchAttachment(uid: string, patch: Partial<ComposerAttachment>) {
  attachments.value = attachments.value.map((x) => (x.uid === uid ? { ...x, ...patch } : x));
}

function removeAttachment(uid: string) {
  const a = attachments.value.find((x) => x.uid === uid);
  attachments.value = attachments.value.filter((x) => x.uid !== uid);
  // 传了还没发的从服务端也删掉；删不掉不要紧，一天以后自己清掉
  if (a?.id) void assistantApi.removeAttachment(a.id).catch(() => undefined);
}

/* ---- 会话与事件流 ---- */

const items = computed(() => chatItems(chat.value.messages, chat.value.confirmations));
const running = computed(() => chat.value.running);
const canConfirm = computed(() => props.status.can_confirm);
const pending = computed(
  () => chat.value.confirmations.filter((c) => c.status === "pending").length,
);

/** BubbleList 的一项：用户消息靠右，回答与提示靠左；都不要气泡底色，内容自己排 */
interface Bubble {
  key: string;
  placement: "start" | "end";
  noStyle: true;
  item: ChatItem;
  /** 这一项是进行中的那次回答 */
  live: boolean;
}

const bubbles = computed<Bubble[]>(() => {
  const s = chat.value;
  const out: Bubble[] = items.value.map((it) => ({
    key: it.key,
    placement: it.kind === "user" ? "end" : "start",
    noStyle: true,
    item: it,
    live: false,
  }));
  if (!s.running) return out;
  // 进行中的回答：接在这次运行最后那次回答上；还没有回答的话另起一项
  const last = out[out.length - 1];
  if (last && last.item.kind === "turn" && last.item.runId === s.runId) {
    last.live = true;
  } else {
    const turn: ChatTurn = { kind: "turn", key: `live-${s.runId}`, runId: s.runId, steps: [] };
    out.push({ key: turn.key, placement: "start", noStyle: true, item: turn, live: true });
  }
  return out;
});

const liveData = computed(() => ({
  reasoning: chat.value.partialReasoning,
  text: chat.value.partial,
  tool: chat.value.tool,
  compacting: chat.value.compacting,
}));

function closeStream() {
  source?.close();
  source = null;
}

function connect(id: number, after: number) {
  closeStream();
  streamLost.value = false;
  const es = new EventSource(assistantApi.eventsURL(id, after));
  es.onmessage = (e: MessageEvent<string>) => {
    if (cur.value !== id) return;
    let ev: AssistantEvent;
    try {
      ev = JSON.parse(e.data) as AssistantEvent;
    } catch {
      return;
    }
    const next = applyEvent(chat.value, ev);
    if (next === null) {
      void open(id);
      return;
    }
    chat.value = next;
    if (ev.type === "run_finished" || ev.type === "run_started") emit("activity");
    // 收起时只为这一次回答留着事件流：回答完就断开
    if (ev.type === "run_finished" && !props.active && source === es) closeStream();
  };
  es.onerror = () => {
    if (es.readyState === EventSource.CLOSED && cur.value === id) streamLost.value = true;
  };
  source = es;
}

/** 让外面的 v-model 跟上这里打开着的会话 */
function syncModel(id: number | null) {
  if (conversationId.value !== id) conversationId.value = id;
}

async function open(id: number) {
  closeStream();
  cur.value = id;
  syncModel(id);
  rememberConversation(id);
  try {
    const d = await assistantApi.conversation(id);
    if (cur.value !== id) return;
    chat.value = chatStateFrom(d);
    restoreOptions();
    emit("opened", d);
    if (props.active || d.running) connect(id, d.last_seq);
  } catch (e) {
    if (cur.value !== id) return;
    if (e instanceof ApiError && e.status === 404) {
      // 会话已经删了（在别的页面或者别的设备上），或者地址里的编号不对：换成新对话
      rememberConversation(null);
      reset();
      return;
    }
    ElMessage.error(errText(e, "读取会话失败"));
  }
}

/** 换成新对话：断开事件流、清掉消息（输入框里打了一半的字留着，由 newChat 决定清不清） */
function reset() {
  closeStream();
  streamLost.value = false;
  cur.value = null;
  chat.value = emptyChatState();
  syncModel(null);
}

function newChat() {
  reset();
  composer.value?.clear();
}

// 外面换了会话（点了会话列表、地址里的 ?c=）：跟着打开；是这里自己交出去的编号就不用再读一遍
watch(
  conversationId,
  (id) => {
    if (id === cur.value) return;
    if (id === null) reset();
    else void open(id);
  },
  { immediate: true },
);

watch(
  () => props.active,
  (active) => {
    if (!active) {
      if (!chat.value.running) closeStream();
      return;
    }
    if (cur.value !== null && !source) connect(cur.value, chat.value.lastSeq);
  },
);

watch([running, pending], ([r, n]) => emit("progress", { running: r, pending: n }));

function errText(e: unknown, fallback: string): string {
  return e instanceof Error && e.message ? e.message : fallback;
}

/** 发消息带的选项：没改过的不带（模型跟着设置走，推理等级用模型的默认） */
function sendOptions() {
  const opts: { model?: string; effort?: AssistantEffort; attachment_ids?: number[] } = {};
  if (selModel.value && selModel.value !== defaultModel.value) opts.model = selModel.value;
  if (selEffort.value) opts.effort = selEffort.value;
  const ids = attachments.value.filter((a) => a.status === "done" && a.id).map((a) => a.id!);
  if (ids.length) opts.attachment_ids = ids;
  return opts;
}

async function send(text: string) {
  if (sending.value || running.value) return;
  const opts = sendOptions();
  if (!text && !opts.attachment_ids) return;
  sending.value = true;
  try {
    if (cur.value === null) {
      const res = await assistantApi.create({ message: text, ...opts });
      composer.value?.clear();
      attachments.value = [];
      emit("created", res.conversation);
      await open(res.conversation.id);
    } else {
      const id = cur.value;
      const res = await assistantApi.send(id, text, opts);
      composer.value?.clear();
      attachments.value = [];
      // 先把这条消息放上去（事件流里的同一条按编号去重）。事件流可能比这个回应先到、连 run_finished 都到了：
      // 这次运行的 run_started 已经处理过（runId 对上）就不再改运行状态，免得卡在「回答中」
      if (cur.value === id) {
        const others = chat.value.messages.filter((m) => m.id !== res.message.id);
        const seen = chat.value.runId === res.run_id;
        chat.value = {
          ...chat.value,
          messages: [...others, res.message].sort((a, b) => a.id - b.id),
          ...(seen ? {} : { running: true, runId: res.run_id }),
        };
      }
    }
  } catch (e) {
    ElMessage.error(errText(e, "发送失败"));
  } finally {
    sending.value = false;
  }
}

async function cancel() {
  if (cur.value === null) return;
  try {
    await assistantApi.cancel(cur.value);
  } catch (e) {
    ElMessage.error(errText(e, "停止失败"));
  }
}

async function decide(c: AssistantConfirmation, approve: boolean) {
  deciding.value = c.id;
  try {
    const got = await assistantApi.decide(c.id, approve);
    upsertConfirmation(got);
  } catch (e) {
    if (e instanceof ApiError && e.status === 409) {
      try {
        upsertConfirmation(JSON.parse(e.message) as AssistantConfirmation);
      } catch {
        /* 回的不是 JSON：等事件流 */
      }
      ElMessage.info("这个操作已经处理过了");
    } else {
      ElMessage.error(errText(e, "提交失败"));
    }
  } finally {
    deciding.value = 0;
  }
}

function upsertConfirmation(c: AssistantConfirmation) {
  const rest = chat.value.confirmations.filter((x) => x.id !== c.id);
  chat.value = { ...chat.value, confirmations: [...rest, c].sort((a, b) => a.id - b.id) };
}

/* ---- 空对话的欢迎页 ---- */

const PROMPTS = [
  {
    key: "sites",
    icon: "chart-line",
    label: "我的站点数据怎么样",
    description: "上传、分享率、魔力与保号",
  },
  {
    key: "dl",
    icon: "hard-drive",
    label: "下载器现在有多少在下载",
    description: "速度、剩余空间与种子",
  },
  {
    key: "search",
    icon: "search",
    label: "搜一下 沙丘 2160p，推送到默认下载器",
    description: "搜索与推送都要你确认",
  },
  {
    key: "subs",
    icon: "bookmark",
    label: "我的订阅还缺哪些集",
    description: "订阅进度与立即搜索",
  },
  {
    key: "lib",
    icon: "folder-open",
    label: "最近整理入库失败的有哪些",
    description: "整理历史、原因与重新整理",
  },
  {
    key: "config",
    icon: "sliders-horizontal",
    label: "打开 hdsky 的自动签到",
    description: "改配置：先看改前改后，确认才保存",
  },
];

/** 悬浮窗里地方小：只放前四个 */
const prompts = computed(() => (props.compact ? PROMPTS.slice(0, 4) : PROMPTS));

function usePrompt(label: string) {
  composer.value?.setText(label);
  composer.value?.focus();
}

onBeforeUnmount(closeStream);

defineExpose({
  /** 另起一个新对话（清掉输入框） */
  newChat,
  /** 把焦点放进输入框 */
  focus: () => composer.value?.focus(),
});
</script>

<template>
  <div class="as-thread" :class="{ 'is-compact': compact }">
    <div class="as-thread__body" data-testid="as-msgs">
      <div v-if="bubbles.length === 0" class="as-hello" data-testid="as-hello">
        <div class="as-hello__col">
          <span class="as-hello__mark" aria-hidden="true"
            ><PtIcon name="sparkles" :size="compact ? 18 : 22"
          /></span>
          <h2 class="as-hello__title">有什么可以帮你？</h2>
          <p class="as-hello__desc">
            我能查站点数据与下载器、搜索并推送种子、管理订阅，把下载完的电影与剧集整理进媒体库，也能帮你改配置；改动类操作每次都要你确认。
          </p>
          <div class="as-hello__prompts">
            <button
              v-for="p in prompts"
              :key="p.key"
              type="button"
              class="as-prompt"
              :data-testid="`as-prompt-${p.key}`"
              @click="usePrompt(p.label)">
              <PtIcon :name="p.icon" :size="16" class="as-prompt__icon" />
              <span class="as-prompt__text">
                <span class="as-prompt__label">{{ p.label }}</span>
                <span class="as-prompt__desc">{{ p.description }}</span>
              </span>
            </button>
          </div>
        </div>
      </div>
      <BubbleList
        v-else
        class="as-list"
        :list="bubbles"
        item-key="key"
        :virtual="false"
        max-height="100%"
        btn-color="var(--pt-p)">
        <template #content="{ item }">
          <div v-if="item.item.kind === 'user'" class="as-user" data-testid="as-user">
            <div v-if="item.item.message.content" class="as-user__text">
              {{ item.item.message.content }}
            </div>
            <div v-if="item.item.message.attachments?.length" class="as-user__files">
              <FilesCard
                v-for="a in item.item.message.attachments"
                :key="a.id"
                :uid="a.id"
                :name="a.name"
                :file-size="a.size"
                :file-type="fileTypeOf(a)"
                :description="attachmentDesc(a)"
                icon-color="var(--pt-p)"
                :url="assistantApi.attachmentURL(a.id)"
                :thumb-url="a.kind === 'image' ? assistantApi.attachmentURL(a.id) : undefined"
                img-variant="square"
                :data-testid="`as-file-${a.id}`" />
            </div>
          </div>
          <div
            v-else-if="item.item.kind === 'notice'"
            class="as-notice"
            :class="{ 'is-error': item.item.message.is_error }"
            data-testid="as-notice">
            <PtIcon
              :name="item.item.message.is_error ? 'circle-alert' : 'info'"
              :size="13"
              aria-hidden="true" />
            <span>{{ item.item.message.content }}</span>
          </div>
          <AssistantTurn
            v-else
            :turn="item.item"
            :running="item.live"
            :live="item.live ? liveData : null"
            :can-confirm="canConfirm"
            :deciding="deciding"
            @decide="decide" />
        </template>
      </BubbleList>
    </div>

    <div class="as-dock">
      <div v-if="chat.error" class="as-dock__note is-error" data-testid="as-error">
        <PtIcon name="circle-alert" :size="14" aria-hidden="true" />
        <span>{{ chat.error }}</span>
      </div>
      <div v-if="streamLost" class="as-dock__note is-warn" data-testid="as-lost">
        <PtIcon name="triangle-alert" :size="14" aria-hidden="true" />
        <span>和 pt-tools 的连接断了：刷新页面重新连接（登录过期的话要重新登录）。</span>
      </div>
      <AssistantComposer
        ref="composer"
        v-model:model="selModel"
        v-model:effort="selEffort"
        :running="running"
        :sending="sending"
        :attachments="attachments"
        :models="models"
        :models-loading="modelsLoading"
        :models-error="modelsError"
        :default-model="defaultModel"
        :usage="chat.usage"
        :voice="status.voice ?? false"
        :compact="compact"
        @submit="send"
        @cancel="cancel"
        @files="addFiles"
        @remove="removeAttachment"
        @load-models="loadModels" />
    </div>
  </div>
</template>

<style scoped>
.as-thread {
  display: flex;
  flex: 1 1 0;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
}

/* 对话：滚动区占满这一列（滚动条贴着右边），每一项居中 720 宽 */
.as-thread__body {
  position: relative;
  flex: 1 1 0;
  min-height: 0;
}

.as-list {
  position: absolute;
  inset: 0;
}

.as-list :deep(.elx-bubble-list__list) {
  gap: 28px;
  padding: 24px 24px 32px;
}

.as-list :deep(.elx-bubble) {
  width: 100%;
  max-width: 720px;
  margin-inline: auto;
}

/* 回答占满这一列：过程展开、收起时不跟着内容变宽变窄（Bubble 默认把内容限在 500 宽） */
.as-list :deep(.elx-bubble--start .elx-bubble__content) {
  width: 100%;
  max-width: none;
}

.as-list :deep(.elx-bubble-list__back-to-bottom-default) {
  width: 36px;
  height: 36px;
  border-color: var(--pt-border);
  background: var(--pt-surface);
  color: var(--pt-t2);
}

/* 用户的消息靠右，最宽八成：上限放在气泡外层（它的宽度是整行），放在里面会按内容自己的宽度算、早早折行 */
.as-list :deep(.elx-bubble--end .elx-bubble__content) {
  max-width: 80%;
}

.as-user {
  display: flex;
  flex-direction: column;
  gap: 8px;
  align-items: flex-end;
}

.as-user__text {
  padding: 9px 14px;
  border-radius: 18px;
  background: var(--pt-hover);
  color: var(--pt-t1);
  font-size: var(--pt-fz-body);
  line-height: 1.65;
  white-space: pre-wrap;
  word-break: break-word;
}

.as-user__files {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  justify-content: flex-end;
  --elx-files-card-bg: var(--pt-surface);
  --elx-files-card-border-color: var(--pt-border);
}

/* 附件卡片的文件名 13/500、说明 11（FilesCard 默认 14 与 12） */
.as-user__files :deep(.elx-files-card-content .elx-files-card-name) {
  font-size: var(--pt-fz-body);
  font-weight: 500;
}

.as-user__files :deep(.elx-files-card-content .elx-files-card-description) {
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

.as-notice {
  display: flex;
  gap: 6px;
  align-items: center;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
}

.as-notice.is-error {
  color: var(--pt-dang);
}

/* 空对话：居中的一段说明加几个推荐的问题 */
.as-hello {
  position: absolute;
  inset: 0;
  display: flex;
  overflow-y: auto;
  padding: 24px;
}

.as-hello__col {
  display: flex;
  flex-direction: column;
  gap: 10px;
  align-items: center;
  width: 100%;
  max-width: 720px;
  margin: auto;
  text-align: center;
}

.as-hello__mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  border-radius: 50%;
  background: var(--pt-p-soft);
  color: var(--pt-p);
}

.as-hello__title {
  margin: 4px 0 0;
  font-size: var(--pt-fz-h1);
  font-weight: 700;
  color: var(--pt-t1);
}

.as-hello__desc {
  max-width: 560px;
  margin: 0;
  font-size: var(--pt-fz-body);
  line-height: 1.7;
  color: var(--pt-t3);
}

.as-hello__prompts {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
  width: 100%;
  margin-top: 14px;
}

.as-prompt {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  padding: 12px 14px;
  border: 1px solid var(--pt-border);
  border-radius: 10px;
  background: var(--pt-surface);
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.as-prompt:hover {
  border-color: var(--pt-border-strong);
  background: var(--pt-hover);
}

.as-prompt:focus-visible {
  outline: 2px solid var(--pt-p);
  outline-offset: 2px;
}

.as-prompt__icon {
  flex: none;
  margin-top: 2px;
  color: var(--pt-p);
}

.as-prompt__text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.as-prompt__label {
  font-size: var(--pt-fz-body);
  font-weight: 500;
  color: var(--pt-t1);
}

.as-prompt__desc {
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
}

/* 输入框：浮在对话下面，和对话一样居中 720 宽；上面一层渐隐，滚到下面的字淡出 */
.as-dock {
  position: relative;
  display: flex;
  flex: none;
  flex-direction: column;
  gap: 8px;
  width: 100%;
  max-width: calc(720px + 48px);
  margin: 0 auto;
  padding: 0 24px 6px;
}

.as-dock::before {
  position: absolute;
  right: 0;
  bottom: 100%;
  left: 0;
  height: 24px;
  background: linear-gradient(to bottom, transparent, var(--pt-surface));
  content: "";
  pointer-events: none;
}

.as-dock__note {
  display: flex;
  gap: 6px;
  align-items: flex-start;
  padding: 8px 12px;
  border-radius: var(--pt-r-md);
  font-size: var(--pt-fz-sm);
  line-height: 1.6;
}

.as-dock__note.is-error {
  background: var(--pt-dang-soft);
  color: var(--pt-dang);
}

.as-dock__note.is-warn {
  background: var(--pt-warn-soft);
  color: var(--pt-warn);
}

/*
 * 悬浮窗（约 440 宽）：边距收到 16、问题排一列、欢迎页的字小一号。
 * 回答与用户消息的上限照手机那一档（88%）：这么窄的一列里八成太早折行。
 */
.as-thread.is-compact .as-list :deep(.elx-bubble-list__list) {
  gap: 22px;
  padding: 16px 16px 24px;
}

.as-thread.is-compact .as-list :deep(.elx-bubble--end .elx-bubble__content) {
  max-width: 88%;
}

.as-thread.is-compact .as-hello {
  padding: 16px;
}

.as-thread.is-compact .as-hello__mark {
  width: 36px;
  height: 36px;
}

.as-thread.is-compact .as-hello__title {
  margin: 2px 0 0;
  font-size: var(--pt-fz-h2);
}

.as-thread.is-compact .as-hello__desc {
  font-size: var(--pt-fz-sm);
  line-height: 1.65;
}

.as-thread.is-compact .as-hello__prompts {
  grid-template-columns: minmax(0, 1fr);
  gap: 8px;
  margin-top: 10px;
}

.as-thread.is-compact .as-prompt {
  padding: 10px 12px;
}

.as-thread.is-compact .as-dock {
  max-width: none;
  padding: 0 12px 10px;
}

@media (max-width: 768px) {
  .as-list :deep(.elx-bubble-list__list) {
    gap: 22px;
    padding: 8px 2px 24px;
  }

  .as-list :deep(.elx-bubble--end .elx-bubble__content) {
    max-width: 88%;
  }

  .as-hello {
    padding: 8px 0;
  }

  .as-hello__prompts {
    grid-template-columns: minmax(0, 1fr);
  }

  .as-dock {
    padding: 0 0 4px;
  }

  .as-dock::before {
    display: none;
  }

  /* 手机上的悬浮窗是占满宽度的底部弹层：左右照悬浮窗留 12，不贴边 */
  .as-thread.is-compact .as-list :deep(.elx-bubble-list__list) {
    padding: 12px 12px 20px;
  }

  .as-thread.is-compact .as-hello {
    padding: 12px;
  }

  .as-thread.is-compact .as-dock {
    padding: 0 12px 8px;
  }
}
</style>
