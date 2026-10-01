import { onBeforeUnmount, onMounted, readonly, type Ref, ref } from "vue";

/**
 * 移动端断点，和 shell.css 里换外壳的那一个（≤768）保持一致。
 * 两处都写死 768 是有意的：CSS 不能读 JS 常量，改了这里要同时改 shell.css 的媒体查询。
 */
export const MOBILE_MAX_WIDTH = 768;
const MOBILE_QUERY = `(max-width: ${MOBILE_MAX_WIDTH}px)`;

/**
 * 当前是否移动端视口。
 *
 * 用 matchMedia 而不是 resize + innerWidth：媒体查询变化只在跨越断点时触发一次，
 * 而 resize 会在拖动窗口时每帧回调，逼得每个页面自己去做节流。
 *
 * 为什么需要它：设计文档 §9 要求桌面表格在移动端一律降级成行卡，不做横向滚动表格。
 * 这个判断有十几个列表页要用，各写一份 resize 监听既重复又容易漏掉解绑。
 */
export function useIsMobile(): Readonly<Ref<boolean>> {
  const isMobile = ref(matches());
  let mql: MediaQueryList | null = null;

  function matches(): boolean {
    return typeof window.matchMedia === "function"
      ? window.matchMedia(MOBILE_QUERY).matches
      : false;
  }

  function onChange(e: MediaQueryListEvent) {
    isMobile.value = e.matches;
  }

  onMounted(() => {
    if (typeof window.matchMedia !== "function") return;
    mql = window.matchMedia(MOBILE_QUERY);
    mql.addEventListener("change", onChange);
    // 挂载前视口可能已经变过（比如路由切换时转屏），对齐一次
    isMobile.value = mql.matches;
  });

  onBeforeUnmount(() => {
    mql?.removeEventListener("change", onChange);
    mql = null;
  });

  return readonly(isMobile);
}
