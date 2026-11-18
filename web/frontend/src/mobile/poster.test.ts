// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h } from "vue";
import { mountView, type MountedView } from "@/test-mount";
const api = vi.hoisted(() => ({ request: vi.fn(), imageAction: vi.fn() }));
vi.mock("./bridge", async (original) => ({
  ...(await original<typeof import("./bridge")>()),
  request: api.request,
  imageAction: api.imageAction,
}));
vi.mock("@/components/exportPoster/posterImage", () => ({
  ensurePosterFonts: async () => {},
  loadPosterLogos: async () => new Map(),
  posterToBlob: async () => new Blob(["png"], { type: "image/png" }),
}));
vi.mock("@/components/exportPoster/ExportPoster.vue", () => ({
  default: defineComponent({
    setup(_, { expose }) {
      expose({ posterEl: () => document.createElement("div"), fonts: () => [] });
      return () => h("div", "Preview");
    },
  }),
}));
let view: MountedView | undefined;
afterEach(() => {
  view?.unmount();
  view = undefined;
  document.body.innerHTML = "";
  vi.clearAllMocks();
});
beforeEach(() => {
  api.request.mockImplementation(async (path: string) =>
    path.includes("summary")
      ? { sites: [] }
      : {
          siteCount: 1,
          perSiteStats: [],
          totalUploaded: 1,
          totalDownloaded: 0,
          totalSeeding: 0,
          totalLeeching: 0,
          totalBonus: 0,
          averageRatio: 0,
          lastUpdate: 0,
        },
  );
});
async function save() {
  const Page = (await import("./PosterPage.vue")).default;
  view = mountView(Page);
  await vi.waitFor(() => expect(document.body.textContent).toContain("保存图片"));
  [...document.querySelectorAll<HTMLButtonElement>("button")]
    .find((b) => b.textContent?.includes("保存图片"))!
    .click();
  await vi.waitFor(() => expect(api.imageAction).toHaveBeenCalled());
}
describe("native poster export", () => {
  it("does not report canceled save as success", async () => {
    api.imageAction.mockResolvedValue({ canceled: true, saved: false });
    await save();
    expect(document.body.textContent).not.toContain("图片已保存");
  });
  it("reports success only after native confirmed saving", async () => {
    api.imageAction.mockResolvedValue({ saved: true });
    await save();
    await vi.waitFor(() => expect(document.body.textContent).toContain("图片已保存"));
  });
});
