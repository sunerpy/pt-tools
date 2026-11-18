/*
 * 首页的下载器总速度（首页 NAS 概览 H2）：右栏的「下载器」与本机来源时 NAS 概览的「下载器速度」卡共用。
 * 外壳每 30 秒取 transfer-stats（runtime store），这一拍取到了就用它（更新），否则用首页接口带的那一份。
 */
import type { HomeBlock, HomeDownloaders, SpeedSample } from "@/api";
import { useRuntimeStore } from "@/stores/runtime";
import { computed } from "vue";

export function useHomeSpeed(block: () => HomeBlock<HomeDownloaders> | null) {
  const runtime = useRuntimeStore();
  const dls = computed(() => block()?.data ?? null);

  /** 速度柱：外壳取到过采样就用外壳的（比首页接口给的新） */
  const history = computed<SpeedSample[]>(() =>
    runtime.speedHistory.length ? runtime.speedHistory : (dls.value?.speed_history ?? []),
  );
  /** 最新一拍的时间：变了就把新的一拍从右边推进来 */
  const lastAt = computed(() => history.value[history.value.length - 1]?.at);

  const down = computed(() =>
    !runtime.stale && runtime.downloadSpeed !== null
      ? runtime.downloadSpeed
      : (dls.value?.down ?? 0),
  );
  const up = computed(() =>
    !runtime.stale && runtime.uploadSpeed !== null ? runtime.uploadSpeed : (dls.value?.up ?? 0),
  );

  /** 柱的值：没有采样时只画当前这一拍 */
  const downBars = computed(() => {
    const v = history.value.map((s) => s.down);
    return v.length ? v : [down.value];
  });
  const upBars = computed(() => {
    const v = history.value.map((s) => s.up);
    return v.length ? v : [up.value];
  });

  return { dls, history, lastAt, down, up, downBars, upBars };
}
