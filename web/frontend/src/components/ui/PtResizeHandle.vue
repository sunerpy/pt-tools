<script setup lang="ts">
import { onBeforeUnmount, ref } from "vue";
import { heightForKey } from "../../composables/useResizableHeight";
/**
 * 面板底边的拖拽把手（role="separator"，横向分隔条）。
 *
 * 父组件收到 resize 时要**同步**把传回来的 height 更新成新值（乐观更新），再让 ResizeObserver
 * 按实际渲染校正：否则 height 要等表格重排之后才变，连按方向键时后几下读到的都是旧值
 * （实测 ↑ 按三次只生效一次）。
 *
 * 指针：按住上下拖，实时发 resize，松手发 commit；
 * 键盘：↑ / ↓ 一步（Shift 四步）、Home / End 到最矮 / 最高，Enter 恢复默认；双击也恢复默认。
 *
 * 它只管「从哪个高度开始、拖了多少」：起点是父组件给的当前高度，落点由父组件决定写到哪
 * （它知道表格现在多高、上下限多少）。命中区就是这条 14px 高的带，不用伪元素负 inset 去扩 ——
 * 那种写法在验收的裁切检测里被抓过两次（设计文档 §30.3）。
 */
const props = defineProps<{
  /** 面板当前的实际高度（px） */
  height: number;
  min: number;
  max: number;
  /** 读屏名称，例如「站点表高度」 */
  label: string;
}>();

const emit = defineEmits<{ resize: [h: number]; commit: [h: number]; reset: [] }>();

const dragging = ref(false);
let startY = 0;
let startH = 0;
let last = 0;
let frame = 0;

const clamp = (h: number) => Math.round(Math.min(props.max, Math.max(props.min, h)));

function onPointerDown(e: PointerEvent) {
  if (e.button !== 0) return;
  e.preventDefault();
  try {
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
  } catch {
    /* 合成的指针事件没有活动指针可捕获；拖动照样靠 pointermove 走 */
  }
  dragging.value = true;
  startY = e.clientY;
  startH = props.height;
  last = clamp(startH);
}

function onPointerMove(e: PointerEvent) {
  if (!dragging.value) return;
  const h = clamp(startH + (e.clientY - startY));
  if (h === last) return;
  last = h;
  cancelAnimationFrame(frame);
  frame = requestAnimationFrame(() => emit("resize", h));
}

function onPointerUp(e: PointerEvent) {
  if (!dragging.value) return;
  dragging.value = false;
  try {
    (e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId);
  } catch {
    /* 同上 */
  }
  cancelAnimationFrame(frame);
  emit("resize", last);
  emit("commit", last);
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === "Enter") {
    e.preventDefault();
    emit("reset");
    return;
  }
  const h = heightForKey(e.key, e.shiftKey, props.height, { min: props.min, max: props.max });
  if (h === undefined) return;
  e.preventDefault();
  emit("resize", h);
  emit("commit", h);
}

onBeforeUnmount(() => cancelAnimationFrame(frame));
</script>

<template>
  <div
    class="pt-resize"
    :class="{ 'is-dragging': dragging }"
    role="separator"
    aria-orientation="horizontal"
    tabindex="0"
    :aria-label="label"
    :aria-valuenow="Math.round(height)"
    :aria-valuemin="min"
    :aria-valuemax="max"
    :title="`${label}：上下拖动调整，双击恢复默认`"
    data-testid="pt-resize-handle"
    @pointerdown="onPointerDown"
    @pointermove="onPointerMove"
    @pointerup="onPointerUp"
    @pointercancel="onPointerUp"
    @dblclick="emit('reset')"
    @keydown="onKeydown">
    <span class="pt-resize__grip" aria-hidden="true" />
  </div>
</template>

<style scoped>
.pt-resize {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  height: 14px;
  cursor: row-resize;
  touch-action: none;
  user-select: none;
  background: var(--pt-surface);
  border-bottom: 1px solid var(--pt-border);
}

/* 4 高的抓手：平时是 borderStrong，悬停 / 拖动时加宽并换成强调色 —— 看得出「这里能拖」 */
.pt-resize__grip {
  width: 40px;
  height: 4px;
  background: var(--pt-border-strong);
  border-radius: 2px;
  transition:
    width var(--pt-dur-fast) ease,
    background-color var(--pt-dur-fast) ease;
}

.pt-resize:hover .pt-resize__grip,
.pt-resize.is-dragging .pt-resize__grip {
  width: 64px;
  background: var(--pt-p);
}

.pt-resize:focus-visible {
  outline: 2px solid var(--pt-p);
  outline-offset: -2px;
}
</style>
