/*
 * 数字从旧值过渡到新值（系统体检页 H1 的计数、环形图中间的数）。时长取 --pt-dur-slow；
 * 系统要求减少动态效果、或环境里没有 requestAnimationFrame 时直接到位。过渡中途来了新值，从当前显示的值接着走。
 */
import { getCurrentScope, onScopeDispose, readonly, ref, watch, type Ref } from "vue";

const REDUCED = "(prefers-reduced-motion: reduce)";

function reducedMotion(): boolean {
  return typeof window.matchMedia === "function" && window.matchMedia(REDUCED).matches;
}

/** 读一个时长令牌（`150ms`、`0.4s`），读不到时用 fallback（毫秒） */
export function tokenMs(name: string, fallback: number): number {
  if (typeof getComputedStyle !== "function") return fallback;
  const raw = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  const m = /^([\d.]+)(ms|s)$/.exec(raw);
  if (!m) return fallback;
  return m[2] === "s" ? Number(m[1]) * 1000 : Number(m[1]);
}

/** 缓出：开头快、到终点慢下来 */
function easeOut(k: number): number {
  return 1 - (1 - k) ** 3;
}

export function useTweenedNumber(source: () => number, from = 0): Readonly<Ref<number>> {
  const shown = ref(from);
  let raf = 0;
  const stop = () => {
    if (raf) cancelAnimationFrame(raf);
    raf = 0;
  };
  watch(
    source,
    (to) => {
      stop();
      const start = shown.value;
      if (start === to || reducedMotion() || typeof requestAnimationFrame !== "function") {
        shown.value = to;
        return;
      }
      const dur = tokenMs("--pt-dur-slow", 400);
      let t0 = -1;
      const step = (t: number) => {
        if (t0 < 0) t0 = t;
        const k = Math.min(1, (t - t0) / dur);
        shown.value = start + (to - start) * easeOut(k);
        raf = k < 1 ? requestAnimationFrame(step) : 0;
      };
      raf = requestAnimationFrame(step);
    },
    { immediate: true },
  );
  if (getCurrentScope()) onScopeDispose(stop);
  return readonly(shown);
}
