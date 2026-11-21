/**
 * 同一时间只跑一个任务：上一次还在跑时再调用，直接用 busy() 给的错误拒绝，不排队、不并发；
 * 上一次结束（成功或出错）以后可以再跑。
 *
 * 一键采集用它：关掉弹窗再打开又点一次，第二次要被拒绝。两次同时跑会抢同一个请求记录器的注册
 * （tab-recorder.ts 开始时清掉残留的注册、结束时注销），还会各自读改写同一个采集会话。
 * 状态只放在 service worker 的内存里：worker 被停掉时采集也一起停了，重启以后自然可以再采，
 * 不会因为存在存储里的标记卡在「正在采集」。
 */
export function createSingleRun(busy: () => Error): <T>(task: () => Promise<T>) => Promise<T> {
  let running = false;
  return async <T>(task: () => Promise<T>): Promise<T> => {
    if (running) {
      throw busy();
    }
    running = true;
    try {
      return await task();
    } finally {
      running = false;
    }
  };
}
