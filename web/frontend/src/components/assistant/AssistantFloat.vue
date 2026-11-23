<script setup lang="ts">
/*
 * 每一页右下角的 AI 助手：AI 助手启用以后，除了对话页本身，每一页的右下角都有一颗悬浮按钮，
 * 点开是一个悬浮的对话窗，在哪一页都能问；对话窗右上角的「全屏」进 AI 助手页，接着同一个会话（/assistant?c=<编号>）。
 *
 * - 对话窗打开的是最近打开的会话（对话页与这里共用，存在浏览器里），没有的话是新对话；「新对话」另起一个。
 * - 不是模态框：开着它照样能点页面。收起时对话窗留在页面里（打了一半的字、选的模型都还在）；
 *   正在回答时事件流不断，按钮上转一圈，有写操作等确认时按钮上标个数，回答完就断开（每个标签页不一直占一条连接）。
 * - 按钮挡住了右下角的东西（分页、表格最后一行的操作）时，按住它上下拖开；位置记在浏览器里，拖动不算点击。
 * - 手机上对话窗是从底部升起的一整块，盖住底栏，点上面露出的那一条收起。
 * - Esc 收起，焦点交回按钮；正在录音时 Esc 只取消录音（输入框自己处理）。
 */
import type { AssistantConversation } from "@/api";
import AssistantThread from "@/components/assistant/AssistantThread.vue";
import PtIcon from "@/components/PtIcon";
import { assistantStatus, refreshAssistantStatus } from "@/composables/useAssistantStatus";
import { lastConversation } from "@/utils/assistant";
import { computed, nextTick, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";

const route = useRoute();
const router = useRouter();

/** 对话页自己就是全屏的对话，不放；路由还没解析出来时也先不放（冷启动直接打开对话页时不闪一下） */
const available = computed(
  () =>
    assistantStatus.value?.enabled === true &&
    route.matched.length > 0 &&
    route.name !== "assistant",
);
const status = computed(() => assistantStatus.value);

const open = ref(false);
/** 第一次打开以后才挂对话窗：没点开过的页面不读会话、不连事件流 */
const mounted = ref(false);
const convId = ref<number | null>(null);
const convTitle = ref("");
const progress = ref({ running: false, pending: 0 });
const thread = ref<InstanceType<typeof AssistantThread> | null>(null);
const fab = ref<HTMLButtonElement | null>(null);
const rail = ref<HTMLElement | null>(null);

const fabLabel = computed(() => {
  if (progress.value.pending) return `AI 助手：${progress.value.pending} 个操作等你确认`;
  if (progress.value.running) return "AI 助手：正在回答";
  return "打开 AI 助手";
});

async function show() {
  if (!mounted.value) {
    // 第一次打开（或者从对话页回来以后）：接着最近打开的会话
    convId.value = lastConversation();
    mounted.value = true;
  }
  open.value = true;
  await nextTick();
  thread.value?.focus();
}

function hide() {
  open.value = false;
  void nextTick(() => fab.value?.focus());
}

function newChat() {
  thread.value?.newChat();
  thread.value?.focus();
}

/** 进 AI 助手页，接着这个会话（还没发第一条消息的话是新对话） */
function fullScreen() {
  const c = convId.value;
  void router.push({ path: "/assistant", query: { c: c === null ? "new" : String(c) } });
}

function onOpened(conv: AssistantConversation) {
  convTitle.value = conv.title;
}

watch(convId, (id) => {
  if (id === null) convTitle.value = "";
});

// 进了对话页、或者 AI 助手被关掉：对话窗整个拆掉（事件流跟着断开）；回来以后再打开时重新接最近打开的会话
watch(available, (a) => {
  if (a) return;
  open.value = false;
  mounted.value = false;
  progress.value = { running: false, pending: 0 };
});

function onKeydown(e: KeyboardEvent) {
  if (e.key !== "Escape" || e.defaultPrevented) return;
  if ((e.currentTarget as HTMLElement).querySelector(".as-mic.is-recording")) return;
  e.preventDefault();
  hide();
}

/* ---- 按住上下拖动 ---- */

const LIFT_KEY = "assistant-fab-lift";
/** 移动不到这么远松手算点击：手指按下去多少会抖几个像素 */
const DRAG_SLOP = 6;

function readLift(): number {
  try {
    const n = Number(localStorage.getItem(LIFT_KEY));
    return Number.isFinite(n) && n > 0 ? Math.round(n) : 0;
  } catch {
    return 0;
  }
}

/** 按钮比默认位置抬高了多少像素（CSS 里再按轨道的高度封顶，窗口变矮时不会跑出去） */
const lift = ref(readLift());
let drag: { id: number; y: number; from: number; max: number; moved: boolean } | null = null;
/** 拖完松手时浏览器还会补一个 click：吞掉它，不当成点开 */
let swallowClick = false;

function onPointerDown(e: PointerEvent) {
  swallowClick = false;
  if (e.button !== 0 || !fab.value || !rail.value) return;
  const max = Math.max(0, rail.value.clientHeight - fab.value.offsetHeight);
  drag = { id: e.pointerId, y: e.clientY, from: Math.min(lift.value, max), max, moved: false };
  fab.value.setPointerCapture?.(e.pointerId);
}

function onPointerMove(e: PointerEvent) {
  if (!drag || e.pointerId !== drag.id) return;
  const dy = e.clientY - drag.y;
  if (!drag.moved && Math.abs(dy) < DRAG_SLOP) return;
  drag.moved = true;
  lift.value = Math.round(Math.min(drag.max, Math.max(0, drag.from - dy)));
}

function endDrag(e: PointerEvent, cancelled: boolean) {
  if (!drag || e.pointerId !== drag.id) return;
  const moved = drag.moved;
  drag = null;
  if (!moved) return;
  swallowClick = !cancelled;
  try {
    localStorage.setItem(LIFT_KEY, String(lift.value));
  } catch {
    /* 存不了就只在这一次记着 */
  }
}

function onFabClick() {
  if (swallowClick) {
    swallowClick = false;
    return;
  }
  void show();
}

onMounted(() => void refreshAssistantStatus());
</script>

<template>
  <template v-if="available && status">
    <!-- 轨道是按钮能上下拖动的范围（右边一条，透明、不接点击）；按钮在里面按 lift 往上抬 -->
    <div ref="rail" class="as-fab-rail">
      <button
        v-show="!open"
        ref="fab"
        type="button"
        class="as-fab"
        :class="{ 'is-running': progress.running }"
        :style="{ '--as-fab-lift': `${lift}px` }"
        :aria-label="fabLabel"
        :title="fabLabel"
        data-testid="as-fab"
        @pointerdown="onPointerDown"
        @pointermove="onPointerMove"
        @pointerup="endDrag($event, false)"
        @pointercancel="endDrag($event, true)"
        @click="onFabClick">
        <PtIcon name="sparkles" :size="20" />
        <span v-if="progress.running" class="as-fab__ring" aria-hidden="true" />
        <span v-if="progress.pending" class="as-fab__badge" data-testid="as-fab-badge">{{
          progress.pending
        }}</span>
      </button>
    </div>

    <div
      v-if="mounted && open"
      class="as-float-scrim"
      aria-hidden="true"
      data-testid="as-float-scrim"
      @click="hide" />

    <Transition name="as-float">
      <section
        v-if="mounted"
        v-show="open"
        class="as-float"
        role="dialog"
        aria-modal="false"
        aria-labelledby="as-float-title"
        data-testid="as-float"
        @keydown="onKeydown">
        <header class="as-float__head">
          <span class="as-float__mark" aria-hidden="true"
            ><PtIcon name="sparkles" :size="15"
          /></span>
          <div class="as-float__heading">
            <h2 id="as-float-title" class="as-float__title">AI 助手</h2>
            <span class="as-float__sub" data-testid="as-float-sub">{{
              convTitle || "新对话"
            }}</span>
          </div>
          <div class="as-float__acts">
            <el-tooltip content="新对话" placement="bottom">
              <button
                type="button"
                class="as-float__btn"
                aria-label="新对话"
                data-testid="as-float-new"
                @click="newChat">
                <PtIcon name="square-pen" :size="16" />
              </button>
            </el-tooltip>
            <el-tooltip content="全屏：在 AI 助手页打开这个会话" placement="bottom">
              <button
                type="button"
                class="as-float__btn"
                aria-label="全屏打开"
                data-testid="as-float-full"
                @click="fullScreen">
                <PtIcon name="maximize-2" :size="16" />
              </button>
            </el-tooltip>
            <el-tooltip content="收起（Esc）" placement="bottom">
              <button
                type="button"
                class="as-float__btn"
                aria-label="收起"
                data-testid="as-float-close"
                @click="hide">
                <PtIcon name="chevron-down" :size="16" />
              </button>
            </el-tooltip>
          </div>
        </header>

        <AssistantThread
          ref="thread"
          v-model:conversation-id="convId"
          :status="status"
          compact
          :active="open"
          @opened="onOpened"
          @created="onOpened"
          @progress="(p) => (progress = p)" />
      </section>
    </Transition>
  </template>
</template>

<style scoped>
/*
 * 层次：盖在页面内容、手机底栏（30）上面，在导航抽屉的遮罩（40）与侧栏（50）下面 ——
 * 窄屏拉出导航抽屉时抽屉照样盖住它。Element 的下拉框、提示与确认框是 2000 起，照样在它上面。
 */
.as-fab-rail {
  position: fixed;
  z-index: 35;
  top: 72px;
  right: 24px;
  bottom: calc(var(--pt-status-h) + 16px);
  width: 48px;
  pointer-events: none;
}

.as-fab {
  position: absolute;
  right: 0;
  bottom: min(var(--as-fab-lift, 0px), calc(100% - 48px));
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 48px;
  height: 48px;
  padding: 0;
  border: 0;
  border-radius: 50%;
  background: var(--pt-p);
  color: var(--pt-on-p);
  box-shadow: var(--pt-shadow-lg);
  cursor: pointer;
  pointer-events: auto;
  /* 按住拖动时不让页面跟着滚 */
  touch-action: none;
  user-select: none;
}

.as-fab:hover {
  background: var(--pt-p-hover);
}

.as-fab:focus-visible {
  outline: 2px solid var(--pt-p);
  outline-offset: 3px;
}

/* 正在回答：按钮外面转一圈 */
.as-fab__ring {
  position: absolute;
  inset: -4px;
  border: 2px solid color-mix(in srgb, var(--pt-p) 22%, transparent);
  border-top-color: var(--pt-p);
  border-radius: 50%;
  animation: as-fab-spin 1s linear infinite;
  pointer-events: none;
}

/* 有写操作等确认：右上角标个数 */
.as-fab__badge {
  position: absolute;
  top: -3px;
  right: -3px;
  min-width: 20px;
  height: 20px;
  padding: 0 6px;
  border-radius: 10px;
  background: var(--pt-dang);
  box-shadow: 0 0 0 2px var(--pt-surface);
  color: var(--pt-on-solid);
  font-size: var(--pt-fz-label);
  font-weight: 600;
  line-height: 20px;
  text-align: center;
}

@keyframes as-fab-spin {
  to {
    transform: rotate(360deg);
  }
}

.as-float-scrim {
  display: none;
}

.as-float {
  position: fixed;
  z-index: 37;
  right: 24px;
  bottom: calc(var(--pt-status-h) + 16px);
  display: flex;
  flex-direction: column;
  width: min(440px, calc(100vw - 48px));
  height: min(720px, calc(100dvh - var(--pt-status-h) - 88px));
  overflow: hidden;
  border: 1px solid var(--pt-border);
  border-radius: 16px;
  background: var(--pt-surface);
  box-shadow: var(--pt-shadow-xl);
  transform-origin: bottom right;
}

.as-float__head {
  display: flex;
  flex: none;
  gap: 10px;
  align-items: center;
  min-height: 52px;
  padding: 0 8px 0 14px;
  border-bottom: 1px solid var(--pt-border);
}

.as-float__mark {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: var(--pt-p-soft);
  color: var(--pt-p);
}

.as-float__heading {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  min-width: 0;
}

.as-float__title {
  margin: 0;
  font-size: var(--pt-fz-body-lg);
  font-weight: 600;
  line-height: 1.3;
  color: var(--pt-t1);
}

.as-float__sub {
  overflow: hidden;
  font-size: var(--pt-fz-label);
  line-height: 1.4;
  color: var(--pt-t3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.as-float__acts {
  display: flex;
  flex: none;
  gap: 2px;
}

.as-float__btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  padding: 0;
  border: 0;
  border-radius: var(--pt-r-md);
  background: transparent;
  color: var(--pt-t2);
  cursor: pointer;
}

.as-float__btn:hover {
  background: var(--pt-hover);
  color: var(--pt-t1);
}

.as-float__btn:focus-visible {
  outline: 2px solid var(--pt-p);
  outline-offset: 1px;
}

.as-float-enter-active,
.as-float-leave-active {
  transition:
    opacity var(--pt-dur-fast) ease,
    transform var(--pt-dur-fast) ease;
}

.as-float-enter-from,
.as-float-leave-to {
  opacity: 0;
  transform: translateY(8px) scale(0.98);
}

@media (prefers-reduced-motion: reduce) {
  .as-fab__ring {
    animation: none;
  }

  .as-float-enter-active,
  .as-float-leave-active {
    transition: none;
  }
}

/* 手机：按钮在底栏上面；对话窗是从底部升起的一整块，盖住底栏，上面留一条看得见页面、点它收起 */
@media (max-width: 768px) {
  .as-fab-rail {
    top: 64px;
    right: 16px;
    bottom: calc(
      var(--pt-m-statusrow-h) + var(--pt-m-tabbar-h) + env(safe-area-inset-bottom) + 16px
    );
  }

  .as-float-scrim {
    position: fixed;
    z-index: 36;
    inset: 0;
    display: block;
    background: color-mix(in srgb, var(--pt-scrim) 44%, transparent);
  }

  .as-float {
    right: 0;
    bottom: 0;
    left: 0;
    width: auto;
    height: calc(100dvh - 48px);
    padding-bottom: env(safe-area-inset-bottom);
    border-bottom: 0;
    border-radius: 16px 16px 0 0;
    transform-origin: bottom center;
  }

  .as-float__head {
    min-height: 56px;
  }

  /* 手机上的图标钮按 44 的触控下限 */
  .as-float__btn {
    width: 44px;
    height: 44px;
  }

  .as-float-enter-from,
  .as-float-leave-to {
    transform: translateY(24px);
  }
}
</style>
