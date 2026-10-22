<script setup lang="ts">
/*
 * AI 助手（路线图 M19）：和 pt-tools 内置的 AI 助手对话。助手在后端运行，经 MCP 工具查数据、做操作；
 * 改动类操作（暂停、删除、推送种子，订阅，整理入库）每次都要在这里点「确认」才执行。
 * 进度经 SSE 推过来：浏览器断线自己重连（带 Last-Event-ID），事件丢了（type 为 reset）就重新读会话。
 *
 * 界面按设计稿「AI 助手对话页」方向 B（居中阅读）：会话贴着侧栏一列（AssistantConvList），
 * 对话居中 720 宽、没有卡片和头像（Element-Plus-X 的 BubbleList 负责滚动与跟随），
 * 每次回答的过程是一条时间线，回答完自动折成一行（AssistantTurn）；输入框浮在底部（AssistantComposer，
 * XSender 加附件、模型与推理等级）。整页撑满主区，只有对话那一列自己滚动。手机上先列会话，点进去看对话。
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
import AssistantConvList from "@/components/assistant/AssistantConvList.vue";
import AssistantTurn from "@/components/assistant/AssistantTurn.vue";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
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
  fileTypeOf,
  MAX_ATTACHMENTS,
} from "@/utils/assistant";
import { ElMessage, ElMessageBox } from "element-plus";
import { BubbleList, FilesCard } from "vue-element-plus-x";
import { computed, onBeforeUnmount, onMounted, ref } from "vue";

const isMobile = useIsMobile();
const ds = useDataState();
const status = ref<AssistantStatus | null>(null);
const list = ref<AssistantConversation[]>([]);
/** 打开的会话；null 是还没发第一条消息的新对话 */
const currentId = ref<number | null>(null);
const chat = ref<ChatState | null>(null);
const sending = ref(false);
const deciding = ref(0);
/** 手机上：看对话（否则看会话列表） */
const showChat = ref(false);
/** 事件流断了且浏览器不再重连（会话失效之类） */
const streamLost = ref(false);
const composer = ref<InstanceType<typeof AssistantComposer> | null>(null);
let source: EventSource | null = null;

/* ---- 模型、推理等级与附件 ---- */

const selModel = ref("");
const selEffort = ref<AssistantEffort>("");
const models = ref<string[]>([]);
const modelsLoading = ref(false);
const modelsError = ref("");
let modelsLoaded = false;
const attachments = ref<ComposerAttachment[]>([]);
let uidSeq = 0;

const defaultModel = computed(() => status.value?.model ?? "");

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
  const users = (chat.value?.messages ?? []).filter((m) => m.role === "user");
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

const items = computed(() =>
  chat.value ? chatItems(chat.value.messages, chat.value.confirmations) : [],
);
const running = computed(() => chat.value?.running ?? false);
const current = computed(() => list.value.find((c) => c.id === currentId.value) ?? null);
const canConfirm = computed(() => status.value?.can_confirm ?? false);

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
  if (!s?.running) return out;
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

const liveData = computed(() =>
  chat.value
    ? {
        reasoning: chat.value.partialReasoning,
        text: chat.value.partial,
        tool: chat.value.tool,
        compacting: chat.value.compacting,
      }
    : null,
);

async function load() {
  const pending = ds.run(async () => {
    const [st, convs] = await Promise.all([assistantApi.status(), assistantApi.conversations()]);
    return { st, convs };
  });
  const got = await pending;
  if (ds.isStale(pending) || !got) return;
  status.value = got.st;
  list.value = got.convs;
  selModel.value = got.st.model;
  if (currentId.value === null && got.convs.length > 0 && !isMobile.value) {
    await openConversation(got.convs[0]!.id);
  } else if (currentId.value === null) {
    chat.value = emptyChat();
  }
}

function emptyChat(): ChatState {
  return {
    messages: [],
    confirmations: [],
    running: false,
    runId: "",
    partial: "",
    partialReasoning: "",
    tool: null,
    lastSeq: 0,
    error: "",
    usage: null,
    compacting: false,
  };
}

function closeStream() {
  source?.close();
  source = null;
}

function connect(id: number, after: number) {
  closeStream();
  streamLost.value = false;
  const es = new EventSource(assistantApi.eventsURL(id, after));
  es.onmessage = (e: MessageEvent<string>) => {
    if (currentId.value !== id || !chat.value) return;
    let ev: AssistantEvent;
    try {
      ev = JSON.parse(e.data) as AssistantEvent;
    } catch {
      return;
    }
    const next = applyEvent(chat.value, ev);
    if (next === null) {
      void openConversation(id);
      return;
    }
    chat.value = next;
    if (ev.type === "run_finished" || ev.type === "run_started") void refreshList();
  };
  es.onerror = () => {
    if (es.readyState === EventSource.CLOSED && currentId.value === id) streamLost.value = true;
  };
  source = es;
}

async function openConversation(id: number) {
  currentId.value = id;
  showChat.value = true;
  try {
    const d = await assistantApi.conversation(id);
    if (currentId.value !== id) return;
    chat.value = chatStateFrom(d);
    restoreOptions();
    connect(id, d.last_seq);
  } catch (e) {
    if (currentId.value !== id) return;
    ElMessage.error(errText(e, "读取会话失败"));
  }
}

async function refreshList() {
  try {
    list.value = await assistantApi.conversations();
  } catch {
    /* 列表刷新失败不打扰：对话还在 */
  }
}

function newChat() {
  closeStream();
  currentId.value = null;
  chat.value = emptyChat();
  showChat.value = true;
  composer.value?.clear();
}

function backToList() {
  showChat.value = false;
}

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
    if (currentId.value === null) {
      const res = await assistantApi.create({ message: text, ...opts });
      composer.value?.clear();
      attachments.value = [];
      list.value = [res.conversation, ...list.value];
      await openConversation(res.conversation.id);
    } else {
      const id = currentId.value;
      const res = await assistantApi.send(id, text, opts);
      composer.value?.clear();
      attachments.value = [];
      // 先把这条消息放上去（事件流里的同一条按编号去重）。事件流可能比这个回应先到、连 run_finished 都到了：
      // 这次运行的 run_started 已经处理过（runId 对上）就不再改运行状态，免得卡在「回答中」
      if (chat.value && currentId.value === id) {
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
  if (currentId.value === null) return;
  try {
    await assistantApi.cancel(currentId.value);
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
  if (!chat.value) return;
  const rest = chat.value.confirmations.filter((x) => x.id !== c.id);
  chat.value = { ...chat.value, confirmations: [...rest, c].sort((a, b) => a.id - b.id) };
}

async function remove(c: AssistantConversation) {
  if (c.running) {
    ElMessage.warning("这个会话正在回答：先停止再删");
    return;
  }
  try {
    await ElMessageBox.confirm(
      `删除会话「${c.title || "新对话"}」？消息、附件与待确认记录一起删掉。`,
      "删除会话",
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
    await assistantApi.remove(c.id);
    list.value = list.value.filter((x) => x.id !== c.id);
    if (currentId.value === c.id) newChat();
    ElMessage.success("已删除");
  } catch (e) {
    ElMessage.error(errText(e, "删除失败"));
  }
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

function usePrompt(label: string) {
  composer.value?.setText(label);
  composer.value?.focus();
}

onMounted(load);
onBeforeUnmount(closeStream);
</script>

<template>
  <div class="as-page" data-testid="as-page">
    <PtHeadSub
      >用自然语言查站点数据、下载器与种子，搜索、推送、订阅、整理入库、改配置；改动类操作每次都要你确认才执行</PtHeadSub
    >
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <div v-show="!isMobile || !showChat" class="as-acts">
        <el-button data-testid="as-settings" @click="$router.push('/assistant/settings')">
          <PtIcon name="settings" :size="15" /><span>设置</span>
        </el-button>
        <el-button type="primary" data-testid="as-new" @click="newChat">
          <PtIcon name="square-pen" :size="15" /><span>新对话</span>
        </el-button>
      </div>
    </Teleport>

    <PtDataState
      v-if="!status"
      class="as-state"
      :state="ds.state.value"
      :sub="ds.errorText.value"
      data-testid="as-state" />

    <div v-else-if="!status.enabled" class="pt-note pt-note--warn as-off" data-testid="as-disabled">
      <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
      <span
        >AI 助手没有启用：先到
        <router-link to="/assistant/settings" data-testid="as-go-settings"
          >系统 → AI 助手设置</router-link
        >
        选服务商、填 API Key 并打开。</span
      >
    </div>

    <div v-else class="as-split">
      <AssistantConvList
        v-show="!isMobile || !showChat"
        class="as-split__list"
        :items="list"
        :active="currentId"
        @open="(id: number) => id !== currentId && openConversation(id)"
        @remove="remove" />

      <section
        v-show="!isMobile || showChat"
        class="as-chat"
        :aria-label="current?.title || '新对话'"
        data-testid="as-chat">
        <div v-if="isMobile" class="as-chat__bar">
          <el-button link data-testid="as-back" @click="backToList">
            <PtIcon name="arrow-left" :size="14" /><span>会话</span>
          </el-button>
          <span class="as-chat__title">{{ current?.title || "新对话" }}</span>
        </div>

        <div class="as-chat__body" data-testid="as-msgs">
          <div v-if="bubbles.length === 0" class="as-hello" data-testid="as-hello">
            <div class="as-hello__col">
              <span class="as-hello__mark" aria-hidden="true"
                ><PtIcon name="sparkles" :size="22"
              /></span>
              <h2 class="as-hello__title">有什么可以帮你？</h2>
              <p class="as-hello__desc">
                我能查站点数据与下载器、搜索并推送种子、管理订阅，把下载完的电影与剧集整理进媒体库，也能帮你改配置；改动类操作每次都要你确认。
              </p>
              <div class="as-hello__prompts">
                <button
                  v-for="p in PROMPTS"
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
          <div v-if="chat?.error" class="as-dock__note is-error" data-testid="as-error">
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
            :usage="chat?.usage"
            :voice="status?.voice ?? false"
            @submit="send"
            @cancel="cancel"
            @files="addFiles"
            @remove="removeAttachment"
            @load-models="loadModels" />
        </div>
      </section>
    </div>
  </div>
</template>

<style>
/*
 * 这一页撑满主区、只让对话那一列滚动：外壳的 .pt-shell__inner 只有 min-height，
 * 里面的高度算不出来（同 .pt-shell.is-immersive 的做法，这里只对这一页生效）。
 */
.pt-shell__inner:has(> .as-page) {
  height: 100%;
}

/* 会话那一列贴着侧栏：窄一点的桌面上外壳给 inner 的左右 16 内缩，这一页不要（手机照旧） */
@media (min-width: 769px) {
  .pt-shell__inner:has(> .as-page) {
    padding-inline: 0;
  }
}
</style>

<style scoped>
.as-page {
  display: flex;
  flex: 1 1 0;
  flex-direction: column;
  min-height: 0;
}

.as-state,
.as-off {
  margin: var(--pt-pad);
}

.as-acts {
  display: flex;
  gap: 8px;
}

/* 桌面上两个按钮落进页头的操作区：页头自己排，这一层不占地方 */
.pt-head__acts .as-acts {
  display: contents;
}

.as-split {
  display: grid;
  flex: 1 1 0;
  grid-template-columns: 268px minmax(0, 1fr);
  min-height: 0;
}

.as-split__list {
  border-right: 1px solid var(--pt-border);
}

.as-chat {
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
  background: var(--pt-surface);
}

.as-chat__bar {
  display: flex;
  flex: none;
  gap: 8px;
  align-items: center;
  min-height: 40px;
  padding: 0 4px 8px;
}

.as-chat__title {
  overflow: hidden;
  font-size: var(--pt-fz-body);
  font-weight: 500;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 对话：滚动区占满这一列（滚动条贴着右边），每一项居中 720 宽 */
.as-chat__body {
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

@media (max-width: 768px) {
  .as-split {
    grid-template-columns: minmax(0, 1fr);
  }

  /* 手机上会话列表与对话各是一张白卡片（灰画布上的行卡规格），列表按内容高 */
  .as-split__list {
    align-self: start;
    max-height: 100%;
    border: 1px solid var(--pt-border);
    border-radius: var(--pt-r-lg);
    background: var(--pt-surface);
  }

  .as-acts {
    margin-bottom: 12px;
  }

  .as-acts > .el-button {
    flex: 1 1 0;
  }

  .as-acts > .el-button + .el-button {
    margin-left: 0;
  }

  .as-chat {
    border: 1px solid var(--pt-border);
    border-radius: var(--pt-r-lg);
    padding: 6px 12px 0;
  }

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
}
</style>
