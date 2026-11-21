import { describe, expect, it, vi } from "vitest";

import { t } from "../core/i18n";
import { autoCollect, type CollectorDeps } from "../modules/collector/auto-collector";
import { createSingleRun } from "./single-run";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

const busy = () => new Error(t("error.autoCollectBusy"));

/*
 * 一键采集要一两分钟：关掉弹窗再打开又点一次，原来会同时跑第二次，
 * 两次抢同一个请求记录器的注册、各自读改写同一个采集会话。
 */
describe("createSingleRun", () => {
  it("上一次没结束时再调用直接拒绝，不跑第二个任务，第一次照常完成", async () => {
    const run = createSingleRun(busy);
    const gate = deferred<string>();
    const first = run(() => gate.promise);
    const second = vi.fn(async () => "second");

    await expect(run(second)).rejects.toThrow(t("error.autoCollectBusy"));
    expect(second).not.toHaveBeenCalled();

    gate.resolve("first");
    await expect(first).resolves.toBe("first");
  });

  it("上一次成功结束以后可以再跑", async () => {
    const run = createSingleRun(busy);
    await run(async () => 1);
    await expect(run(async () => 2)).resolves.toBe(2);
  });

  it("上一次出错结束以后也可以再跑", async () => {
    const run = createSingleRun(busy);
    await expect(
      run(async () => {
        throw new Error("site down");
      }),
    ).rejects.toThrow("site down");
    await expect(run(async () => "again")).resolves.toBe("again");
  });

  it("包着一键采集：第一次还在取页面时第二次被拒绝、不发任何请求，第一次结束后能再采", async () => {
    const run = createSingleRun(busy);
    const requested: string[] = [];
    const gate = deferred<void>();
    let blockFirst = true;
    const deps: CollectorDeps = {
      fetch: async (input) => {
        requested.push(String(input));
        if (blockFirst) {
          blockFirst = false;
          await gate.promise;
        }
        return new Response(`<a href="/torrents/7">t</a><a href="/users/alice">alice</a>`, {
          headers: { "content-type": "text/html" },
        });
      },
      sleep: async () => undefined,
    };
    const collect = () => run(() => autoCollect("https://u3d.example", "Unit3D", undefined, deps));

    const first = collect();
    await vi.waitFor(() => expect(requested).toHaveLength(1));
    await expect(collect()).rejects.toThrow(t("error.autoCollectBusy"));
    expect(requested).toHaveLength(1);

    gate.resolve();
    const firstPages = (await first).pages.map((page) => page.pageType);
    expect(firstPages).toEqual(["index", "search", "detail", "userinfo"]);

    const again = await collect();
    expect(again.pages.map((page) => page.pageType)).toEqual(firstPages);
  });
});
