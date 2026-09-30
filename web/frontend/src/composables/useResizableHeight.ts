import { ref, type Ref } from "vue";

/**
 * 可拖拽调高的面板 —— 用户统计的站点表（用户原话「站点区域面板尺寸偏小，需要扩大；
 * 支持下拉 / 上拉拖拽方式动态调整面板高度」）。
 *
 * 两种状态：
 *   - 自适应（manual = null）：内容少时贴合内容，多时封顶到调用方给的上限（通常按视口算）；
 *   - 手动（manual = 数字）：用户拖过或用键盘调过，就是这个高度，存进 localStorage。
 * 双击把手或按 Enter 回到自适应。
 *
 * 纯计算放在导出的函数里（clampHeight / parseStoredHeight / heightForKey），可以单测；
 * 指针拖拽本身在 PtResizeHandle 里。
 */

export interface HeightBounds {
  min: number;
  max: number;
}

export function clampHeight(h: number, { min, max }: HeightBounds): number {
  const hi = Math.max(min, max);
  return Math.round(Math.min(hi, Math.max(min, h)));
}

/** localStorage 里存的是像素数；读不出、不是有限数就当没存（回到自适应） */
export function parseStoredHeight(raw: string | null, bounds: HeightBounds): number | null {
  if (raw == null || raw === "" || raw === "auto") return null;
  const n = Number(raw);
  if (!Number.isFinite(n) || n <= 0) return null;
  return clampHeight(n, bounds);
}

/**
 * 键盘调高：↑ / ↓ 一步，Shift 四步，Home / End 到最矮 / 最高。
 * 返回 undefined 表示这个键不归把手管（让它继续冒泡，例如 Tab）。
 */
export function heightForKey(
  key: string,
  shift: boolean,
  current: number,
  bounds: HeightBounds,
  step = 24,
): number | undefined {
  const s = shift ? step * 4 : step;
  switch (key) {
    case "ArrowUp":
      return clampHeight(current - s, bounds);
    case "ArrowDown":
      return clampHeight(current + s, bounds);
    case "Home":
      return clampHeight(bounds.min, bounds);
    case "End":
      return clampHeight(bounds.max, bounds);
    default:
      return undefined;
  }
}

export interface UseResizableHeight {
  /** null = 自适应；数字 = 用户定的高度 */
  manual: Ref<number | null>;
  /** 拖动过程中 persist = false，只改值；松手（或键盘一步）才落盘 */
  set: (h: number, persist?: boolean) => void;
  reset: () => void;
}

export function useResizableHeight(
  storageKey: string,
  bounds: () => HeightBounds,
): UseResizableHeight {
  let initial: number | null = null;
  try {
    initial = parseStoredHeight(window.localStorage.getItem(storageKey), bounds());
  } catch {
    /* 隐私模式读不了就按自适应 */
  }
  const manual = ref<number | null>(initial);

  function persist() {
    try {
      if (manual.value == null) window.localStorage.removeItem(storageKey);
      else window.localStorage.setItem(storageKey, String(manual.value));
    } catch {
      /* 存不下就只在本次会话里生效 */
    }
  }

  return {
    manual,
    set(h: number, save = true) {
      manual.value = clampHeight(h, bounds());
      if (save) persist();
    },
    reset() {
      manual.value = null;
      persist();
    },
  };
}
