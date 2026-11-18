// @vitest-environment happy-dom
/*
 * 钉子：数字过渡（系统体检页 H1）。从旧值走到新值、终点准确；减少动态效果时直接到位；时长读 --pt-dur-slow。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { effectScope, nextTick, ref } from "vue";
import { tokenMs, useTweenedNumber } from "./useTweenedNumber";

function motion(reduce: boolean) {
  vi.spyOn(window, "matchMedia").mockImplementation(
    (query: string) =>
      ({
        matches: reduce && query.includes("prefers-reduced-motion"),
        media: query,
        addEventListener: () => {},
        removeEventListener: () => {},
      }) as unknown as MediaQueryList,
  );
}

/** 手动推进 requestAnimationFrame：每调一次 frame(t) 跑一拍 */
function fakeFrames() {
  let queue: FrameRequestCallback[] = [];
  vi.stubGlobal("requestAnimationFrame", (cb: FrameRequestCallback) => {
    queue.push(cb);
    return queue.length;
  });
  vi.stubGlobal("cancelAnimationFrame", () => {
    queue = [];
  });
  return (t: number) => {
    const run = queue;
    queue = [];
    run.forEach((cb) => cb(t));
  };
}

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  document.documentElement.style.removeProperty("--pt-dur-slow");
});

describe("数字过渡", () => {
  it("从旧值走到新值，中途是两者之间的值，到时长时正好是新值", async () => {
    motion(false);
    const frame = fakeFrames();
    document.documentElement.style.setProperty("--pt-dur-slow", "400ms");
    const src = ref(0);
    const scope = effectScope();
    const shown = scope.run(() => useTweenedNumber(() => src.value))!;
    expect(shown.value).toBe(0);
    src.value = 10;
    await nextTick();
    frame(1000);
    frame(1200);
    expect(shown.value).toBeGreaterThan(0);
    expect(shown.value).toBeLessThan(10);
    frame(1400);
    expect(shown.value).toBe(10);
    scope.stop();
  });

  it("减少动态效果时直接到位", async () => {
    motion(true);
    fakeFrames();
    const src = ref(3);
    const scope = effectScope();
    const shown = scope.run(() => useTweenedNumber(() => src.value))!;
    expect(shown.value).toBe(3);
    src.value = 7;
    await nextTick();
    expect(shown.value).toBe(7);
    scope.stop();
  });

  it("时长令牌按毫秒或秒读，读不到时用默认值", () => {
    document.documentElement.style.setProperty("--pt-dur-slow", "0.5s");
    expect(tokenMs("--pt-dur-slow", 400)).toBe(500);
    document.documentElement.style.setProperty("--pt-dur-slow", "240ms");
    expect(tokenMs("--pt-dur-slow", 400)).toBe(240);
    expect(tokenMs("--pt-没有这个", 400)).toBe(400);
  });
});
