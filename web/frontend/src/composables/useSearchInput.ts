import { onBeforeUnmount, watch } from "vue";

/**
 * 工具栏上的文本筛选框：输入停下一会儿自动查，回车或清空立刻查，同一个词不重复查。
 *
 * 操作审计、RSS 通知日志的搜索框以前只认回车：旁边的分段与下拉都是选了就查，
 * 只有搜索框要按回车，页面上又没写 —— 打完字等着结果，什么都不发生。
 *
 * 用法：`const q = useSearchInput(() => filters.q, handleFilterChange)`，
 * 模板里 `@keyup.enter="q.now" @clear="q.now"`。
 */
export function useSearchInput(source: () => string, apply: () => void, delay = 400) {
  let timer: ReturnType<typeof setTimeout> | undefined;
  /** 最近一次真正拿去查的词：v-model 先于 clear 事件更新，清空时这里已经是空串，watch 就不再排一次 */
  let applied = source();

  function now() {
    clearTimeout(timer);
    timer = undefined;
    applied = source();
    apply();
  }

  watch(source, (value) => {
    clearTimeout(timer);
    timer = value === applied ? undefined : setTimeout(now, delay);
  });

  onBeforeUnmount(() => clearTimeout(timer));

  return { now };
}
