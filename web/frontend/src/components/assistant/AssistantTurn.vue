<script setup lang="ts">
/*
 * AI 助手的一次回答（设计稿「AI 助手对话页」方向 B 居中阅读）：没有气泡与头像，像读文档。
 * 上面是「过程」：进行中展开成一条时间线（思考、工具调用、各自的用时），回答完折成一行摘要，点开还能看；
 * 用户自己点过就按用户的。等确认的写操作的卡片不折，确认、拒绝都在卡片上。下面是回答，回答下面是复制与模型。
 */
import type { AssistantConfirmation } from "@/api";
import PtIcon from "@/components/PtIcon";
import {
  argsText,
  CONFIRM_LABELS,
  type ChatTurn,
  decidedByText,
  durationText,
  type LiveStep,
  liveHeadline,
  pendingConfirmations,
  processSummary,
  renderAnswer,
  renderPartial,
  resultPreview,
  resultText,
  type TimelineEntry,
  timeline,
  toolLabel,
  turnAnswer,
  turnMeta,
} from "@/utils/assistant";
import { formatShortDateTime } from "@/utils/format";
import { ElMessage } from "element-plus";
import { computed, ref, useId } from "vue";

const props = defineProps<{
  turn: ChatTurn;
  /** 这次回答还在进行 */
  running: boolean;
  /** 进行中的这一步：推理内容、文字与正在执行的工具（只有进行中的那次回答有） */
  live?: LiveStep | null;
  canConfirm: boolean;
  /** 正在提交的确认（按钮转圈） */
  deciding: number;
}>();

const emit = defineEmits<{ decide: [c: AssistantConfirmation, approve: boolean] }>();

const entries = computed(() => timeline(props.turn, props.running, props.live));
const answer = computed(() => turnAnswer(props.turn));
const pending = computed(() => pendingConfirmations(props.turn));
const meta = computed(() => turnMeta(props.turn));
const liveText = computed(() => (props.running ? (props.live?.text ?? "") : ""));
/** 进行中时就算还没有思考与工具也显示那一行（「正在思考…」「正在压缩上下文…」），发出去以后马上有反应 */
const hasProcess = computed(() => entries.value.length > 0 || props.running);

/** 用户点过以后按用户的；没点过时进行中展开、结束了折起来 */
const manual = ref<boolean | null>(null);
const bodyId = `as-proc-${useId()}`;
const open = computed(() => manual.value ?? props.running);
function toggle() {
  manual.value = !open.value;
}

const headText = computed(() =>
  props.running
    ? liveHeadline(props.turn, props.live ?? null, pending.value.length > 0)
    : processSummary(props.turn) || "过程",
);

/** 长的思考先只露四行，点「展开全文」看全部 */
const LONG_THINK = 240;
const expandedThink = ref<string[]>([]);
function toggleThink(key: string) {
  expandedThink.value = expandedThink.value.includes(key)
    ? expandedThink.value.filter((k) => k !== key)
    : [...expandedThink.value, key];
}
function clamped(e: TimelineEntry): boolean {
  return !e.live && (e.text?.length ?? 0) > LONG_THINK && !expandedThink.value.includes(e.key);
}

function entryIcon(e: TimelineEntry): string {
  if (e.kind === "think") return e.live ? "loader-circle" : "lightbulb";
  if (e.kind === "say") return "message-square-text";
  switch (e.status) {
    case "ok":
      return "circle-check";
    case "wait":
      return "circle-pause";
    case "error":
      return "circle-x";
    case "skipped":
      return "circle";
    default:
      return "loader-circle";
  }
}

function entryTitle(e: TimelineEntry): string {
  if (e.kind === "think") return e.live ? "正在思考" : "思考";
  if (e.kind === "say") return "说明";
  return toolLabel(e.call?.name ?? "");
}

/** 工具这一行的补充：确认的结论、没有执行的原因 */
function toolNote(e: TimelineEntry): string {
  const c = e.confirmation;
  if (c && c.status !== "pending") {
    if (c.status === "approved" && e.status === "error") return "已确认，没有成功";
    const why = c.status === "expired" ? decidedByText(c) : "";
    return why ? `${CONFIRM_LABELS[c.status]}（${why}）` : CONFIRM_LABELS[c.status];
  }
  switch (e.status) {
    case "wait":
      return "等你确认";
    case "queued":
      return "排队中";
    case "running":
      return "执行中";
    case "skipped":
      return "没有执行";
    case "error":
      return e.result ? "没有成功" : "";
    default:
      return "";
  }
}

function confirmNote(c: AssistantConfirmation): string {
  return `${formatShortDateTime(c.expires_at)} 以前有效`;
}

async function copyAnswer() {
  try {
    await navigator.clipboard.writeText(answer.value);
    ElMessage.success("已复制");
  } catch {
    ElMessage.warning("浏览器不让复制：选中文字手动复制");
  }
}
</script>

<template>
  <div class="as-turn" :data-testid="running ? 'as-live' : `as-${turn.key}`">
    <div v-if="hasProcess" class="as-proc" :class="{ 'is-open': open }">
      <!-- 折起来是一枚圆角标签，展开是一行标题：同一个按钮，点了焦点还在 -->
      <button
        type="button"
        class="as-proc__head"
        :class="{ 'is-live': running }"
        :aria-expanded="open"
        :aria-controls="bodyId"
        data-testid="as-process"
        @click="toggle">
        <PtIcon
          :name="running ? 'loader-circle' : 'sparkles'"
          :size="14"
          :class="{ 'as-spin': running }" />
        <span>{{ headText }}</span>
        <PtIcon :name="open ? 'chevron-up' : 'chevron-right'" :size="13" class="as-proc__caret" />
      </button>
      <ol v-show="open" :id="bodyId" class="as-tl" data-testid="as-process-body">
        <li
          v-for="(e, i) in entries"
          :key="e.key"
          class="as-tl__item"
          :class="[
            `is-${e.kind}`,
            e.status ? `is-${e.status}` : '',
            { 'is-last': i === entries.length - 1 },
          ]"
          :data-testid="
            e.kind === 'think' ? (e.live ? 'as-live-thinking' : 'as-thinking') : undefined
          ">
          <span class="as-tl__dot" aria-hidden="true">
            <PtIcon
              :name="entryIcon(e)"
              :size="16"
              :class="{ 'as-spin': e.live || e.status === 'running' || e.status === 'queued' }" />
          </span>
          <div class="as-tl__body">
            <div class="as-tl__title">
              <span class="as-tl__name">{{ entryTitle(e) }}</span>
              <span v-if="e.seconds != null" class="as-tl__time">{{
                durationText(e.seconds)
              }}</span>
              <span v-if="e.kind === 'tool' && toolNote(e)" class="as-tl__note">{{
                toolNote(e)
              }}</span>
            </div>
            <template v-if="e.kind === 'think'">
              <p class="as-tl__think" :class="{ 'is-clamped': clamped(e) }">{{ e.text }}</p>
              <button
                v-if="!e.live && (e.text?.length ?? 0) > LONG_THINK"
                type="button"
                class="as-tl__more"
                @click="toggleThink(e.key)">
                {{ expandedThink.includes(e.key) ? "收起" : "展开全文" }}
              </button>
            </template>
            <!-- 中间说的话也是模型写的 Markdown：同回答一样经 renderAnswer -->
            <!-- eslint-disable-next-line vue/no-v-html -->
            <div
              v-else-if="e.kind === 'say'"
              class="as-md as-tl__say"
              v-html="renderAnswer(e.text ?? '')" />
            <template v-else>
              <div class="as-tl__args">{{ argsText(e.call?.arguments ?? "{}") }}</div>
              <details
                v-if="e.result"
                class="as-tl__result"
                :class="{ 'is-error': e.result.is_error }">
                <summary>{{ resultPreview(e.result.content) }}</summary>
                <pre>{{ resultText(e.result.content) }}</pre>
              </details>
            </template>
          </div>
        </li>
      </ol>
    </div>

    <div
      v-for="c in pending"
      :key="c.id"
      class="as-confirm"
      role="group"
      :aria-label="`需要你确认：${c.summary}`"
      :data-testid="`as-confirm-${c.id}`">
      <PtIcon name="triangle-alert" :size="16" class="as-confirm__icon" />
      <div class="as-confirm__main">
        <div class="as-confirm__title">需要你确认</div>
        <div class="as-confirm__summary">{{ c.summary }}</div>
        <dl v-if="c.changes?.length" class="as-diff" :data-testid="`as-diff-${c.id}`">
          <div v-for="(ch, i) in c.changes" :key="i" class="as-diff__row">
            <dt class="as-diff__label">{{ ch.label }}</dt>
            <dd class="as-diff__value">
              <template v-if="ch.secret"
                ><span class="as-diff__after">已更新（不显示）</span></template
              >
              <template v-else>
                <span v-if="ch.before" class="as-diff__before">{{ ch.before }}</span>
                <span v-if="ch.before && ch.after" class="as-diff__arrow" aria-label="改成">→</span>
                <span v-if="ch.after" class="as-diff__after">{{ ch.after }}</span>
                <span v-else class="as-diff__gone">删除</span>
              </template>
            </dd>
          </div>
        </dl>
        <div v-if="c.impact?.length" class="as-impact" :data-testid="`as-impact-${c.id}`">
          <div class="as-impact__title">可能的影响</div>
          <ul>
            <li v-for="(it, i) in c.impact" :key="i">{{ it }}</li>
          </ul>
        </div>
        <div class="as-confirm__foot">
          <span class="as-confirm__note">{{ confirmNote(c) }}</span>
          <template v-if="canConfirm">
            <el-button
              size="small"
              :loading="deciding === c.id"
              :data-testid="`as-deny-${c.id}`"
              @click="emit('decide', c, false)"
              >拒绝</el-button
            >
            <el-button
              size="small"
              type="primary"
              :loading="deciding === c.id"
              :data-testid="`as-approve-${c.id}`"
              @click="emit('decide', c, true)"
              >确认执行</el-button
            >
          </template>
          <span v-else class="as-confirm__note">这个入口不能确认改动类操作</span>
        </div>
      </div>
    </div>

    <!-- 回答是模型写的 Markdown：renderAnswer 只留排版标签（没有图片），再经 DOMPurify -->
    <!-- eslint-disable-next-line vue/no-v-html -->
    <div
      v-if="answer"
      class="as-md as-answer"
      data-testid="as-answer"
      v-html="renderAnswer(answer)" />
    <!-- eslint-disable-next-line vue/no-v-html -->
    <div
      v-if="liveText"
      class="as-md as-answer"
      data-testid="as-partial"
      v-html="renderPartial(liveText)" />

    <div v-if="!running && (answer || meta)" class="as-turn__foot">
      <button
        v-if="answer"
        type="button"
        class="as-icon-btn"
        aria-label="复制回答"
        title="复制回答"
        data-testid="as-copy"
        @click="copyAnswer">
        <PtIcon name="copy" :size="14" />
      </button>
      <span v-if="meta" class="as-turn__meta" data-testid="as-meta">{{ meta }}</span>
    </div>
  </div>
</template>

<style scoped>
.as-turn {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
}

/* 过程：折起来是一枚圆角标签（画板 a1 / process），展开是一行标题加时间线（a2 / live） */
.as-proc {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

/* 展开的时间线和下面的回答之间多留一点 */
.as-proc.is-open {
  margin-bottom: 6px;
}

.as-proc__head {
  display: inline-flex;
  gap: 6px;
  align-items: center;
  align-self: flex-start;
  max-width: 100%;
  min-height: 30px;
  padding: 5px 12px;
  border: 0;
  border-radius: 15px;
  background: var(--pt-hover);
  color: var(--pt-t2);
  font: inherit;
  font-size: var(--pt-fz-sm);
  text-align: left;
  cursor: pointer;
}

.as-proc__head:hover {
  color: var(--pt-t1);
}

.as-proc.is-open .as-proc__head {
  min-height: 0;
  padding: 2px 0;
  background: transparent;
  font-weight: 500;
}

.as-proc__head.is-live,
.as-proc.is-open .as-proc__head.is-live {
  color: var(--pt-p);
}

.as-proc__caret {
  flex: none;
  color: var(--pt-t3);
}

.as-proc__head:focus-visible,
.as-tl__more:focus-visible,
.as-icon-btn:focus-visible {
  outline: 2px solid var(--pt-p);
  outline-offset: 2px;
}

.as-tl {
  margin: 0;
  padding: 0;
  list-style: none;
}

.as-tl__item {
  position: relative;
  display: flex;
  gap: 10px;
  padding-bottom: 16px;
}

.as-tl__item.is-last {
  padding-bottom: 0;
}

/* 圆点之间的竖线 */
.as-tl__item:not(.is-last)::before {
  position: absolute;
  top: 22px;
  bottom: 0;
  left: 8px;
  width: 1px;
  background: var(--pt-border);
  content: "";
}

.as-tl__dot {
  display: inline-flex;
  flex: none;
  padding-top: 2px;
  color: var(--pt-p);
}

.as-tl__item.is-think .as-tl__dot,
.as-tl__item.is-say .as-tl__dot,
.as-tl__item.is-skipped .as-tl__dot {
  color: var(--pt-t3);
}

.as-tl__item.is-ok .as-tl__dot {
  color: var(--pt-ok);
}

.as-tl__item.is-wait .as-tl__dot {
  color: var(--pt-warn);
}

.as-tl__item.is-error .as-tl__dot {
  color: var(--pt-dang);
}

.as-tl__body {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.as-tl__title {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: baseline;
}

.as-tl__name {
  font-size: var(--pt-fz-body);
  font-weight: 500;
  color: var(--pt-t1);
}

.as-tl__item.is-think .as-tl__name,
.as-tl__item.is-say .as-tl__name {
  color: var(--pt-t2);
}

.as-tl__time,
.as-tl__note {
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

.as-tl__item.is-wait .as-tl__note {
  color: var(--pt-warn);
}

.as-tl__item.is-error .as-tl__note {
  color: var(--pt-dang);
}

.as-tl__think {
  margin: 0;
  font-size: var(--pt-fz-sm);
  line-height: 1.55;
  color: var(--pt-t2);
  white-space: pre-wrap;
  word-break: break-word;
}

.as-tl__think.is-clamped {
  display: -webkit-box;
  overflow: hidden;
  -webkit-line-clamp: 4;
  -webkit-box-orient: vertical;
}

.as-tl__more {
  align-self: flex-start;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--pt-p);
  font: inherit;
  font-size: var(--pt-fz-label);
  cursor: pointer;
}

.as-tl__say {
  font-size: var(--pt-fz-body);
  color: var(--pt-t2);
}

.as-tl__args {
  overflow: hidden;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.as-tl__result {
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
}

.as-tl__result summary {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  cursor: pointer;
}

.as-tl__result.is-error summary {
  color: var(--pt-dang);
}

.as-tl__result pre {
  max-height: 260px;
  margin: 6px 0 0;
  padding: 8px 10px;
  overflow: auto;
  border-radius: var(--pt-r-md);
  background: var(--pt-hover);
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-label);
  line-height: 1.55;
  white-space: pre-wrap;
  word-break: break-all;
}

/* 等确认的卡片 */
.as-confirm {
  display: flex;
  gap: 10px;
  padding: 12px 14px;
  border: 1px solid color-mix(in srgb, var(--pt-warn) 35%, transparent);
  border-radius: 10px;
  background: color-mix(in srgb, var(--pt-warn) 8%, var(--pt-surface));
}

.as-confirm__icon {
  flex: none;
  margin-top: 2px;
  color: var(--pt-warn);
}

.as-confirm__main {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.as-confirm__title {
  font-size: var(--pt-fz-body);
  font-weight: 700;
  color: var(--pt-warn);
}

.as-confirm__summary {
  font-size: var(--pt-fz-body);
  line-height: 1.6;
  color: var(--pt-t1);
  word-break: break-word;
}

/* 改动：一行一项，名称在左，改以前（划掉）→ 改以后 */
.as-diff {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 6px 0 0;
  padding: 8px 10px;
  border-radius: var(--pt-r-md);
  background: color-mix(in srgb, var(--pt-surface) 70%, transparent);
}

.as-diff__row {
  display: grid;
  grid-template-columns: minmax(64px, max-content) minmax(0, 1fr);
  gap: 12px;
  align-items: baseline;
}

.as-diff__label {
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
  word-break: break-all;
}

.as-diff__value {
  margin: 0;
  font-size: var(--pt-fz-sm);
  line-height: 1.6;
  color: var(--pt-t1);
  word-break: break-word;
}

.as-diff__before {
  color: var(--pt-t3);
  text-decoration: line-through;
}

.as-diff__arrow {
  margin: 0 6px;
  color: var(--pt-t3);
}

.as-diff__after {
  font-weight: 500;
}

.as-diff__gone {
  margin-left: 6px;
  color: var(--pt-dang);
}

.as-impact {
  margin-top: 6px;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
}

.as-impact__title {
  font-weight: 500;
  color: var(--pt-t2);
}

.as-impact ul {
  margin: 2px 0 0;
  padding-left: 18px;
  line-height: 1.6;
}

.as-confirm__foot {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  margin-top: 4px;
}

.as-confirm__note {
  flex: 1 1 auto;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

/* 回答：正文 13（字号设置放大时跟着变），标题 14 粗体 */
.as-answer {
  font-size: var(--pt-fz-body);
}

.as-md {
  line-height: 1.75;
  color: var(--pt-t1);
  word-break: break-word;
}

.as-md :deep(p),
.as-md :deep(ul),
.as-md :deep(ol),
.as-md :deep(pre),
.as-md :deep(blockquote),
.as-md :deep(table) {
  margin: 0 0 10px;
}

.as-md :deep(> :last-child) {
  margin-bottom: 0;
}

.as-md :deep(ul),
.as-md :deep(ol) {
  padding-left: 22px;
}

.as-md :deep(li + li) {
  margin-top: 2px;
}

.as-md :deep(h1),
.as-md :deep(h2),
.as-md :deep(h3),
.as-md :deep(h4),
.as-md :deep(h5),
.as-md :deep(h6) {
  margin: 4px 0 8px;
  font-size: var(--pt-fz-body-lg);
  font-weight: 700;
}

.as-md :deep(code) {
  padding: 1px 4px;
  border-radius: var(--pt-r-sm);
  background: var(--pt-hover);
  font-family: var(--pt-font-mono);
  font-size: 0.92em;
}

.as-md :deep(pre) {
  padding: 10px 12px;
  overflow-x: auto;
  border-radius: var(--pt-r-md);
  background: var(--pt-hover);
  font-size: var(--pt-fz-sm);
}

.as-md :deep(pre code) {
  padding: 0;
  background: transparent;
}

.as-md :deep(table) {
  display: block;
  max-width: 100%;
  overflow-x: auto;
  border-collapse: collapse;
}

.as-md :deep(th),
.as-md :deep(td) {
  padding: 4px 10px;
  border: 1px solid var(--pt-border);
  font-size: var(--pt-fz-sm);
}

.as-md :deep(blockquote) {
  padding-left: 12px;
  border-left: 3px solid var(--pt-border);
  color: var(--pt-t2);
}

.as-md :deep(a) {
  color: var(--pt-p);
}

.as-turn__foot {
  display: flex;
  gap: 8px;
  align-items: center;
}

.as-icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  padding: 0;
  border: 0;
  border-radius: var(--pt-r-md);
  background: transparent;
  color: var(--pt-t3);
  cursor: pointer;
}

.as-icon-btn:hover {
  background: var(--pt-hover);
  color: var(--pt-t1);
}

.as-turn__meta {
  font-size: var(--pt-fz-label);
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
</style>
