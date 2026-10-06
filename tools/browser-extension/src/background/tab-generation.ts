/**
 * 每个标签页的解析代次。同一个 tab 的多次 onUpdated 解析没有先后约束：慢的旧解析
 * （比如还在等内容脚本检测）可能最后才回来，把已经离开 PT 站的标签页又标回 PT 站。
 * 解析开始时领一个代次，提交前确认自己仍是最新的那一次。
 */
export function createTabGenerations() {
  const generations = new Map<number, number>();
  return {
    /** 开始一次解析，返回「这次解析是否仍是该标签页最新的一次」 */
    begin(tabId: number): () => boolean {
      const generation = (generations.get(tabId) ?? 0) + 1;
      generations.set(tabId, generation);
      return () => generations.get(tabId) === generation;
    },
    forget(tabId: number): void {
      generations.delete(tabId);
    },
  };
}
