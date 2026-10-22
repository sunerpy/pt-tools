// @vitest-environment happy-dom
/*
 * 钉子：追剧定时的两种新写法。每隔一段时间（数字 + 单位，存成分钟）、cron 表达式（常用写法点一下填进去）；
 * 改定时时现算接下来几次什么时候搜，cron 写错时把后端说的哪一段不对显示出来；保存只发这种定时用得到的字段。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, ref } from "vue";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  profiles: vi.fn(async () => ({ items: [] })),
  create: vi.fn(),
  update: vi.fn(),
  previewSchedule: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    subscribeApi: { ...real.subscribeApi, ...api },
    downloadersApi: { list: vi.fn(async () => []) },
    sitesApi: { list: vi.fn(async () => ({})) },
    organizeApi: { ...real.organizeApi, libraries: vi.fn(async () => []) },
    mediaApi: { ...real.mediaApi, settings: vi.fn(async () => ({ image_base_url: "" })) },
  };
});

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  document.body.innerHTML = "";
  vi.clearAllMocks();
});

const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid="${id}"]`);

/*
 * 后端回的是带时区的时间，弹窗按浏览器的时区显示。模拟的时间按本机时区造（月份从 1 起），
 * 断言里的钟点在哪个时区跑都成立 —— CI 在 UTC 上跑，写死 +08:00 的话会差 8 小时。
 */
const at = (y: number, mon: number, d: number, hh: number, mm = 0) =>
  new Date(y, mon - 1, d, hh, mm).toISOString();

function openOptions(id: string): () => HTMLElement[] {
  const sel = q(id)!;
  (sel.querySelector<HTMLElement>(".el-select__wrapper") ?? sel).click();
  const listId = sel.querySelector("input")?.getAttribute("aria-controls");
  return () => [
    ...(document
      .getElementById(listId ?? "")
      ?.querySelectorAll<HTMLElement>(".el-select-dropdown__item") ?? []),
  ];
}

async function pick(id: string, text: string) {
  const options = openOptions(id);
  await vi.waitFor(() => {
    const opt = options().find((o) => (o.textContent ?? "").trim() === text);
    expect(opt).toBeTruthy();
    opt!.click();
  });
}

function fill(id: string, v: string) {
  const el = q(id)!;
  const i = (
    el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement
      ? el
      : el.querySelector("input, textarea")
  ) as HTMLInputElement;
  i.value = v;
  i.dispatchEvent(new Event("input"));
}

async function mountDialog(sub?: Record<string, unknown>) {
  const Dialog = (await import("./SubscribeDialog.vue")).default;
  const Host = defineComponent({
    setup() {
      const open = ref(true);
      return () =>
        h(Dialog, {
          modelValue: open.value,
          "onUpdate:modelValue": (v: boolean) => (open.value = v),
          ...(sub ? { sub: sub as never } : { target: { kind: "tv", tmdb_id: 7, title: "乘风" } }),
        });
    },
  });
  view = mountView(Host);
  await vi.waitFor(() => expect(q("sd-schedule")).not.toBeNull());
}

describe("追剧定时：每隔一段时间与 cron", () => {
  it("每隔一段时间：数字 + 单位存成分钟，预览接下来几次", async () => {
    api.previewSchedule.mockResolvedValue({
      next: [at(2026, 10, 24, 20), at(2026, 10, 25, 2)],
    });
    await mountDialog();
    await pick("sd-schedule", "每隔一段时间");
    await vi.waitFor(() => expect(q("sd-interval")).not.toBeNull());
    fill("sd-interval", "2");
    await vi.waitFor(() =>
      expect(api.previewSchedule).toHaveBeenLastCalledWith(
        expect.objectContaining({
          schedule: { kind: "interval", interval_minutes: 120 },
          media_type: "tv",
        }),
      ),
    );
    await vi.waitFor(() => expect(q("sd-schedule-preview")!.textContent).toContain("接下来"));
    expect(q("sd-schedule-preview")!.textContent).toContain("周六 20:00");
    expect(q("sd-at")).toBeNull();
  });

  it("cron：常用写法点一下填进去；写错时显示哪一段不对；保存只发 cron", async () => {
    const { ApiError } = await import("@/api");
    api.previewSchedule.mockImplementation(
      async ({ schedule }: { schedule: { cron?: string } }) => {
        if (schedule.cron?.includes("25"))
          throw new ApiError(400, "参数不对: 第 2 段（小时）「25」不在 0 到 23 之间");
        return { next: [at(2026, 10, 23, 22)] };
      },
    );
    api.create.mockResolvedValue({ id: 9 });
    await mountDialog();
    await pick("sd-schedule", "cron 表达式（自定义）");
    await vi.waitFor(() => expect(q("sd-cron")).not.toBeNull());
    q("sd-cron-preset-0 22 * * 5; 0 9,13 * * 6")!.click();
    await vi.waitFor(() =>
      expect(api.previewSchedule).toHaveBeenLastCalledWith(
        expect.objectContaining({ schedule: { kind: "cron", cron: "0 22 * * 5; 0 9,13 * * 6" } }),
      ),
    );
    await vi.waitFor(() => expect(q("sd-schedule-preview")!.textContent).toContain("周五 22:00"));

    fill("sd-cron", "0 25 * * *");
    await vi.waitFor(() =>
      expect(q("sd-schedule-preview")!.textContent).toContain(
        "定时写得不对：第 2 段（小时）「25」不在 0 到 23 之间",
      ),
    );

    fill("sd-cron", " 0 20 * * * ");
    await vi.waitFor(() => expect(q("sd-schedule-preview")!.textContent).not.toContain("第 2 段"));
    q("sd-save")!.click();
    await vi.waitFor(() => expect(api.create).toHaveBeenCalledTimes(1));
    expect(api.create.mock.calls[0]![0].schedule).toEqual({ kind: "cron", cron: "0 20 * * *" });
  });

  it("修改已经完成的追剧：写明现在不会搜索，定时等它改回在找才用上", async () => {
    api.previewSchedule.mockResolvedValue({ next: [at(2026, 10, 23, 22)] });
    await mountDialog({
      id: 4,
      media_type: "tv",
      tmdb_id: 7,
      title: "乘风",
      year: 2026,
      season: 4,
      status: "done",
      profile_id: 0,
      sites: [],
      downloader_id: 0,
      category: "",
      tags: "",
      save_path: "",
      upgrade: false,
      lock_version: true,
      lock_group: "HHWEB",
      lock_resolution: "1080p",
      schedule: { kind: "cron", cron: "0 22 * * 5" },
    });
    await vi.waitFor(() => expect(q("sd-not-searching")).not.toBeNull());
    expect(q("sd-not-searching")!.textContent).toContain("已经完成，现在不会搜索");
  });
});
