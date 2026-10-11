<script setup lang="ts">
/*
 * AI 助手（路线图 M19）：和 pt-tools 内置的 AI 助手对话。助手在后端运行，经 MCP 工具查数据、做操作；
 * 改动类操作（暂停、删除、推送种子，添加订阅）每次都要在这里点「确认」才执行。
 * 进度经 SSE 推过来：浏览器断线自己重连（带 Last-Event-ID），事件丢了（type 为 reset）就重新读会话。
 * 画板没有这一页：左边会话、右边对话，沿用运行日志页的 pt-cards--side；手机上先列会话，点进去看对话。
 */
import {
  ApiError,
  assistantApi,
  type AssistantConfirmation,
  type AssistantConversation,
  type AssistantEvent,
  type AssistantStatus,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import {
  applyEvent,
  argsText,
  chatItems,
  type ChatState,
  chatStateFrom,
  CONFIRM_LABELS,
  CONFIRM_TONES,
  decidedByText,
  renderAnswer,
  toolLabel,
} from "@/utils/assistant";
import { formatShortDateTime } from "@/utils/format";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from "vue";

const isMobile = useIsMobile();
const ds = useDataState();
const status = ref<AssistantStatus | null>(null);
const list = ref<AssistantConversation[]>([]);
/** 打开的会话；null 是还没发第一条消息的新对话 */
const currentId = ref<number | null>(null);
const chat = ref<ChatState | null>(null);
const input = ref("");
const sending = ref(false);
const deciding = ref(0);
/** 手机上：看对话（否则看会话列表） */
const showChat = ref(false);
/** 事件流断了且浏览器不再重连（会话失效之类） */
const streamLost = ref(false);
const listEl = ref<HTMLElement | null>(null);
let source: EventSource | null = null;

const items = computed(() =>
  chat.value ? chatItems(chat.value.messages, chat.value.confirmations) : [],
);
const running = computed(() => chat.value?.running ?? false);
const current = computed(() => list.value.find((c) => c.id === currentId.value) ?? null);
const canSend = computed(() => !sending.value && !running.value && input.value.trim() !== "");

async function load() {
  const pending = ds.run(async () => {
    const [st, convs] = await Promise.all([assistantApi.status(), assistantApi.conversations()]);
    return { st, convs };
  });
  const got = await pending;
  if (ds.isStale(pending) || !got) return;
  status.value = got.st;
  list.value = got.convs;
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
    tool: null,
    lastSeq: 0,
    error: "",
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
    const stick = nearBottom();
    chat.value = next;
    if (ev.type === "run_finished" || ev.type === "run_started") void refreshList();
    if (stick) void scrollToBottom();
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
    connect(id, d.last_seq);
    await scrollToBottom();
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
  input.value = "";
}

function backToList() {
  showChat.value = false;
}

function errText(e: unknown, fallback: string): string {
  return e instanceof Error && e.message ? e.message : fallback;
}

async function send() {
  const text = input.value.trim();
  if (!text || sending.value || running.value) return;
  sending.value = true;
  try {
    if (currentId.value === null) {
      const res = await assistantApi.create({ message: text });
      input.value = "";
      list.value = [res.conversation, ...list.value];
      await openConversation(res.conversation.id);
    } else {
      const id = currentId.value;
      const res = await assistantApi.send(id, text);
      input.value = "";
      // 先把这条消息放上去（事件流里的同一条按编号去重）
      if (chat.value && currentId.value === id) {
        const others = chat.value.messages.filter((m) => m.id !== res.message.id);
        chat.value = { ...chat.value, messages: [...others, res.message], running: true };
      }
      await scrollToBottom();
    }
  } catch (e) {
    ElMessage.error(errText(e, "发送失败"));
  } finally {
    sending.value = false;
  }
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === "Enter" && !e.shiftKey && !e.isComposing) {
    e.preventDefault();
    void send();
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
  const list = chat.value.confirmations.filter((x) => x.id !== c.id);
  chat.value = { ...chat.value, confirmations: [...list, c].sort((a, b) => a.id - b.id) };
}

async function remove(c: AssistantConversation) {
  try {
    await ElMessageBox.confirm(
      `删除会话「${c.title || "新对话"}」？消息与待确认记录一起删掉。`,
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

function nearBottom(): boolean {
  const el = listEl.value;
  return !el || el.scrollHeight - el.scrollTop - el.clientHeight < 80;
}

async function scrollToBottom() {
  await nextTick();
  const el = listEl.value;
  if (el) el.scrollTop = el.scrollHeight;
}

function confirmNote(c: AssistantConfirmation): string {
  if (c.status === "pending") return `${formatShortDateTime(c.expires_at)} 以前有效`;
  const why = c.status === "expired" ? decidedByText(c) : "";
  return why ? `${CONFIRM_LABELS[c.status]}：${why}` : CONFIRM_LABELS[c.status];
}

const canConfirm = computed(() => status.value?.can_confirm ?? false);

onMounted(load);
onBeforeUnmount(closeStream);
</script>

<template>
  <div class="pt-cards pt-cards--side">
    <PtHeadSub
      >用自然语言查站点数据、下载器与种子，搜索、推送、订阅；改动类操作每次都要你确认才执行</PtHeadSub
    >
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button data-testid="as-settings" @click="$router.push('/assistant/settings')">
        <PtIcon name="settings" :size="15" /><span>设置</span>
      </el-button>
      <el-button type="primary" data-testid="as-new" @click="newChat">
        <PtIcon name="square-pen" :size="15" /><span>新对话</span>
      </el-button>
    </Teleport>

    <PtDataState
      v-if="!status"
      class="pt-cards__full"
      :state="ds.state.value"
      :sub="ds.errorText.value"
      data-testid="as-state" />

    <div
      v-else-if="!status.enabled"
      class="pt-cards__full pt-note pt-note--warn"
      data-testid="as-disabled">
      <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
      <span
        >AI 助手没有启用：先到
        <router-link to="/assistant/settings" data-testid="as-go-settings"
          >系统 → AI 助手设置</router-link
        >
        选服务商、填 API Key 并打开。</span
      >
    </div>

    <template v-else>
      <PtPanel
        v-show="!isMobile || !showChat"
        title="会话"
        icon="messages-square"
        :count="list.length"
        data-testid="as-convs">
        <p v-if="list.length === 0" class="as-empty">没有会话：在右边问一句试试。</p>
        <ul v-else class="as-convlist">
          <li
            v-for="c in list"
            :key="c.id"
            class="as-conv"
            :class="{ 'is-active': c.id === currentId }"
            :data-testid="`as-conv-${c.id}`">
            <button type="button" class="as-conv__open" @click="openConversation(c.id)">
              <span class="as-conv__title">{{ c.title || "新对话" }}</span>
              <span class="as-conv__meta">
                <PtStatusPill v-if="c.running" tone="primary" dot size="sm">回答中</PtStatusPill>
                <span>{{ formatShortDateTime(c.updated_at) }}</span>
              </span>
            </button>
            <el-button
              link
              type="danger"
              :aria-label="`删除会话 ${c.title || '新对话'}`"
              :data-testid="`as-del-${c.id}`"
              :disabled="c.running"
              @click="remove(c)">
              <PtIcon name="trash-2" :size="14" />
            </el-button>
          </li>
        </ul>
      </PtPanel>

      <PtPanel
        v-show="!isMobile || showChat"
        :title="current?.title || '新对话'"
        icon="sparkles"
        data-testid="as-chat">
        <template #actions>
          <el-button v-if="isMobile" link data-testid="as-back" @click="backToList">
            <PtIcon name="arrow-left" :size="14" /><span>会话</span>
          </el-button>
          <span class="as-model">{{ status.model }}</span>
        </template>

        <div ref="listEl" class="as-msgs" data-testid="as-msgs">
          <p v-if="items.length === 0 && !chat?.partial" class="as-hello">
            可以问：「我的站点数据怎么样」「下载器现在有多少在下载」「搜一下 沙丘
            2160p，推送到默认下载器」
          </p>
          <template v-for="it in items" :key="it.message.id">
            <div v-if="it.kind === 'user'" class="as-row as-row--user">
              <div class="as-bubble as-bubble--user" data-testid="as-user">
                {{ it.message.content }}
              </div>
            </div>
            <div
              v-else-if="it.kind === 'notice'"
              class="as-notice"
              :class="{ 'is-error': it.message.is_error }"
              data-testid="as-notice">
              <PtIcon
                :name="it.message.is_error ? 'circle-alert' : 'info'"
                :size="13"
                aria-hidden="true" />
              <span>{{ it.message.content }}</span>
            </div>
            <div v-else class="as-row">
              <div class="as-answer">
                <!-- 回答是模型写的 Markdown：renderAnswer 只留排版标签（没有图片），再经 DOMPurify -->
                <!-- eslint-disable-next-line vue/no-v-html -->
                <div
                  v-if="it.message.content"
                  class="as-md"
                  data-testid="as-answer"
                  v-html="renderAnswer(it.message.content)" />
                <div
                  v-for="cv in it.calls"
                  :key="cv.call.id"
                  class="as-call"
                  :data-testid="`as-call-${cv.call.name}`">
                  <div class="as-call__head">
                    <PtIcon name="wrench" :size="13" aria-hidden="true" />
                    <span class="as-call__name">{{ toolLabel(cv.call.name) }}</span>
                    <span class="as-call__args">{{ argsText(cv.call.arguments) }}</span>
                    <PtStatusPill v-if="cv.result?.is_error" tone="dang" size="sm"
                      >没有成功</PtStatusPill
                    >
                  </div>
                  <div
                    v-if="cv.confirmation"
                    class="as-confirm"
                    :class="`is-${cv.confirmation.status}`"
                    :data-testid="`as-confirm-${cv.confirmation.id}`">
                    <div class="as-confirm__top">
                      <PtStatusPill :tone="CONFIRM_TONES[cv.confirmation.status]" dot size="sm">{{
                        CONFIRM_LABELS[cv.confirmation.status]
                      }}</PtStatusPill>
                      <span class="as-confirm__summary">{{ cv.confirmation.summary }}</span>
                    </div>
                    <div class="as-confirm__foot">
                      <span class="as-confirm__note">{{ confirmNote(cv.confirmation) }}</span>
                      <template v-if="cv.confirmation.status === 'pending' && canConfirm">
                        <el-button
                          size="small"
                          :loading="deciding === cv.confirmation.id"
                          :data-testid="`as-deny-${cv.confirmation.id}`"
                          @click="decide(cv.confirmation, false)"
                          >拒绝</el-button
                        >
                        <el-button
                          size="small"
                          type="primary"
                          :loading="deciding === cv.confirmation.id"
                          :data-testid="`as-approve-${cv.confirmation.id}`"
                          @click="decide(cv.confirmation, true)"
                          >确认执行</el-button
                        >
                      </template>
                    </div>
                  </div>
                  <details v-if="cv.result" class="as-result">
                    <summary>结果</summary>
                    <pre class="as-result__body">{{ cv.result.content }}</pre>
                  </details>
                </div>
              </div>
            </div>
          </template>
          <div v-if="running" class="as-row" data-testid="as-live">
            <div class="as-answer">
              <!-- eslint-disable-next-line vue/no-v-html -->
              <div v-if="chat?.partial" class="as-md" v-html="renderAnswer(chat.partial)" />
              <div class="as-typing" aria-live="polite">
                <PtIcon name="loader-circle" :size="13" class="as-spin" aria-hidden="true" />
                <span>{{ chat?.tool ? `正在${toolLabel(chat.tool.name)}…` : "正在思考…" }}</span>
              </div>
            </div>
          </div>
          <div v-if="chat?.error" class="as-notice is-error" data-testid="as-error">
            <PtIcon name="circle-alert" :size="13" aria-hidden="true" />
            <span>{{ chat.error }}</span>
          </div>
        </div>

        <div v-if="streamLost" class="pt-note pt-note--warn as-lost" data-testid="as-lost">
          <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
          <span>和 pt-tools 的连接断了：刷新页面重新连接（登录过期的话要重新登录）。</span>
        </div>

        <div class="as-compose">
          <el-input
            v-model="input"
            type="textarea"
            :autosize="{ minRows: 2, maxRows: 6 }"
            :maxlength="4000"
            resize="none"
            placeholder="问点什么，Enter 发送，Shift+Enter 换行"
            aria-label="给 AI 助手的消息"
            data-testid="as-input"
            @keydown="onKeydown" />
          <div class="as-compose__acts">
            <span class="as-compose__tip">对话与工具结果会发给所选的模型服务商</span>
            <el-button v-if="running" data-testid="as-stop" @click="cancel">
              <PtIcon name="circle-pause" :size="14" /><span>停止</span>
            </el-button>
            <el-button
              v-else
              type="primary"
              :disabled="!canSend"
              :loading="sending"
              data-testid="as-send"
              @click="send">
              <PtIcon name="send" :size="14" /><span>发送</span>
            </el-button>
          </div>
        </div>
      </PtPanel>
    </template>
  </div>
</template>

<style scoped>
.as-empty,
.as-hello {
  margin: 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--pt-t3);
}

.as-convlist {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: min(64vh, 640px);
  margin: 0;
  padding: 0;
  overflow-y: auto;
  list-style: none;
}

.as-conv {
  display: flex;
  gap: 4px;
  align-items: center;
  border-radius: 8px;
}

.as-conv.is-active {
  background: color-mix(in srgb, var(--pt-p) 10%, transparent);
}

.as-conv__open {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
  min-height: 44px;
  padding: 8px 10px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.as-conv__open:focus-visible {
  outline: 2px solid var(--pt-p);
  outline-offset: -2px;
}

.as-conv__title {
  overflow: hidden;
  font-size: 13px;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.as-conv__meta {
  display: flex;
  gap: 6px;
  align-items: center;
  font-size: 12px;
  color: var(--pt-t3);
}

.as-model {
  font-size: 12px;
  color: var(--pt-t3);
}

.as-msgs {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-height: 320px;
  max-height: min(62vh, 680px);
  padding: 4px 2px 8px;
  overflow-y: auto;
}

.as-row {
  display: flex;
}

.as-row--user {
  justify-content: flex-end;
}

.as-bubble {
  max-width: 82%;
  padding: 8px 12px;
  border-radius: 10px;
  font-size: 13px;
  line-height: 1.65;
  white-space: pre-wrap;
  word-break: break-word;
}

.as-bubble--user {
  background: color-mix(in srgb, var(--pt-p) 12%, transparent);
  color: var(--pt-t1);
}

.as-answer {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
  max-width: 92%;
}

.as-md {
  font-size: 13px;
  line-height: 1.7;
  color: var(--pt-t1);
  word-break: break-word;
}

.as-md :deep(p),
.as-md :deep(ul),
.as-md :deep(ol),
.as-md :deep(pre),
.as-md :deep(blockquote),
.as-md :deep(table) {
  margin: 0 0 8px;
}

.as-md :deep(> :last-child) {
  margin-bottom: 0;
}

.as-md :deep(ul),
.as-md :deep(ol) {
  padding-left: 20px;
}

.as-md :deep(h1),
.as-md :deep(h2),
.as-md :deep(h3),
.as-md :deep(h4),
.as-md :deep(h5),
.as-md :deep(h6) {
  margin: 0 0 6px;
  font-size: 13px;
  font-weight: 600;
}

.as-md :deep(pre) {
  padding: 8px 10px;
  overflow-x: auto;
  border-radius: 6px;
  background: var(--pt-bg-surface-muted);
  font-size: 12px;
}

.as-md :deep(table) {
  display: block;
  max-width: 100%;
  overflow-x: auto;
  border-collapse: collapse;
}

.as-md :deep(th),
.as-md :deep(td) {
  padding: 4px 8px;
  border: 1px solid var(--pt-border);
  font-size: 12px;
}

.as-md :deep(blockquote) {
  padding-left: 10px;
  border-left: 3px solid var(--pt-border);
  color: var(--pt-t2);
}

.as-call {
  padding: 8px 10px;
  border: 1px solid var(--pt-border);
  border-radius: 8px;
  font-size: 12px;
}

.as-call__head {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
  color: var(--pt-t2);
}

.as-call__name {
  font-weight: 500;
  color: var(--pt-t1);
}

.as-call__args {
  min-width: 0;
  color: var(--pt-t3);
  word-break: break-all;
}

.as-confirm {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 8px;
  padding: 10px;
  border-radius: 8px;
  background: var(--pt-bg-surface-muted);
}

.as-confirm.is-pending {
  background: color-mix(in srgb, var(--pt-warn) 12%, transparent);
}

.as-confirm__top {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}

.as-confirm__summary {
  font-size: 13px;
  color: var(--pt-t1);
  word-break: break-all;
}

.as-confirm__foot {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}

.as-confirm__note {
  flex: 1 1 auto;
  color: var(--pt-t3);
}

.as-result {
  margin-top: 6px;
  color: var(--pt-t3);
}

.as-result summary {
  cursor: pointer;
}

.as-result__body {
  max-height: 240px;
  margin: 6px 0 0;
  padding: 8px 10px;
  overflow: auto;
  border-radius: 6px;
  background: var(--pt-bg-surface-muted);
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
}

.as-notice {
  display: flex;
  gap: 6px;
  align-items: center;
  align-self: center;
  font-size: 12px;
  color: var(--pt-t3);
}

.as-notice.is-error {
  align-self: flex-start;
  color: var(--pt-dang);
}

.as-typing {
  display: flex;
  gap: 6px;
  align-items: center;
  font-size: 12px;
  color: var(--pt-t3);
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

.as-lost {
  margin-top: 8px;
}

.as-compose {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 12px;
}

.as-compose__acts {
  display: flex;
  gap: 8px;
  align-items: center;
  justify-content: flex-end;
}

.as-compose__tip {
  flex: 1 1 auto;
  font-size: 12px;
  color: var(--pt-t3);
}
</style>
