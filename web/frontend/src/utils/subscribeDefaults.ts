/*
 * 新建订阅时按媒体库带出下载设置：媒体库里设的下载器、保存目录、分类、标签，在新建订阅时填进表单。
 * 这里都是纯函数，不依赖界面：网页的订阅弹窗按它们带出，手机的追剧编辑页也用同一份，不另写一套。规则：
 *   - 只填没动过的项；自己输入、清空或选了别的项以后换库也不动，可以点「换成媒体库的」恢复
 *   - 换库时来自库的项跟着换，新库没设的项清空；没有生效的库时来自库的项清空
 *   - 库里的下载器停用了、删了或读不到下载器列表时不带出它，推送用设置里的默认下载器
 *   - 修改已有的订阅时一律当作自己填的，换了库只在点「套用」时换
 * 服务器在保存与推送订阅时都不补这些值：带不带出只看这里。
 */
import type { DownloaderSetting, MediaLibrary, OrganizeSettings } from "@/api";

export type DownloadField = "downloader_id" | "save_path" | "category" | "tags";

export const DOWNLOAD_FIELDS: readonly DownloadField[] = [
  "downloader_id",
  "save_path",
  "category",
  "tags",
];

export interface DownloadValues {
  downloader_id: number;
  save_path: string;
  category: string;
  tags: string;
}

/** 一项现在的值从哪来：none 没带出过；library 来自媒体库（note 是写在输入框下面的说明）；user 自己填的 */
export type FieldSource =
  | { kind: "none" }
  | { kind: "user" }
  | { kind: "library"; libraryID: number; note: string };

export type Sources = Record<DownloadField, FieldSource>;

export interface DefaultsForm {
  values: DownloadValues;
  sources: Sources;
}

/** 媒体库设的下载器能不能用：unset 没设；ok 启用着；disabled 停用了；deleted 删了；unknown 读不到下载器列表 */
export type DownloaderState = "unset" | "ok" | "disabled" | "deleted" | "unknown";

export interface LibraryDefaults {
  libraryID: number;
  library: string;
  /** 要带出的值：库里的下载器用不了时是 0 */
  values: DownloadValues;
  downloader: DownloaderState;
  /** 下载器的名字（删了的、没设的、读不到列表时为空） */
  downloaderName: string;
}

/** 带出用到的媒体库字段（网页的 MediaLibrary 与手机 App API 的库都有） */
export type DefaultsLibrary = Pick<MediaLibrary, "id" | "name"> &
  Partial<
    Pick<
      MediaLibrary,
      "download_downloader_id" | "download_save_path" | "download_category" | "download_tags"
    >
  >;

/** 带出用到的下载器字段 */
export type DefaultsDownloader = Pick<DownloaderSetting, "id" | "name" | "enabled">;

export type OrganizeScope = Pick<
  OrganizeSettings,
  "downloaders" | "categories" | "tags" | "save_paths"
>;

export function blankValues(): DownloadValues {
  return { downloader_id: 0, save_path: "", category: "", tags: "" };
}

function allSources(kind: "none" | "user"): Sources {
  return {
    downloader_id: { kind },
    save_path: { kind },
    category: { kind },
    tags: { kind },
  };
}

/** newSources 是新建订阅打开时：四项都还没带出过 */
export function newSources(): Sources {
  return allSources("none");
}

/** userSources 是修改已有的订阅时：四项都当作自己填的 */
export function userSources(): Sources {
  return allSources("user");
}

/** effectiveLibrary 是带出设置用的库：选了具体的库用它；选「自动」（0）时用草稿里按类型会整理进的库；都没有时为 null */
export function effectiveLibrary<L extends Pick<MediaLibrary, "id">>(
  libraries: readonly L[],
  libraryID: number,
  autoLibraryID: number,
): L | null {
  const id = libraryID > 0 ? libraryID : autoLibraryID;
  if (id <= 0) return null;
  return libraries.find((l) => l.id === id) ?? null;
}

/** libraryDefaults 是媒体库要带出的值；downloaders 为 null 表示读不到下载器列表 */
export function libraryDefaults(
  lib: DefaultsLibrary,
  downloaders: readonly DefaultsDownloader[] | null,
): LibraryDefaults {
  const id = lib.download_downloader_id || 0;
  let state: DownloaderState = "unset";
  let name = "";
  if (id > 0) {
    if (!downloaders) {
      state = "unknown";
    } else {
      const d = downloaders.find((x) => x.id === id);
      if (!d) state = "deleted";
      else {
        state = d.enabled ? "ok" : "disabled";
        name = d.name;
      }
    }
  }
  return {
    libraryID: lib.id,
    library: lib.name,
    values: {
      downloader_id: state === "ok" ? id : 0,
      save_path: (lib.download_save_path ?? "").trim(),
      category: (lib.download_category ?? "").trim(),
      tags: (lib.download_tags ?? "").trim(),
    },
    downloader: state,
    downloaderName: name,
  };
}

/** usableDownloader 报告媒体库设的下载器能不能带出（没设也算：带出的是「用设置里的默认下载器」） */
function usableDownloader(d: LibraryDefaults): boolean {
  return d.downloader === "ok" || d.downloader === "unset";
}

const EMPTY_NOTE: Record<Exclude<DownloadField, "downloader_id">, string> = {
  save_path: "没设，按下载器的默认目录",
  category: "没设，不加分类",
  tags: "没设，不加标签",
};

function noteFor(d: LibraryDefaults, field: DownloadField): string {
  const lib = `媒体库「${d.library}」`;
  if (field === "downloader_id") {
    switch (d.downloader) {
      case "ok":
        return `来自${lib}`;
      case "unset":
        return `${lib}没设下载器，用设置里的默认下载器`;
      case "disabled":
        return `${lib}设的下载器「${d.downloaderName}」已停用，用设置里的默认下载器`;
      case "deleted":
        return `${lib}设的下载器已删除，用设置里的默认下载器`;
      default:
        return `读不到下载器列表，没有带出${lib}设的下载器`;
    }
  }
  return d.values[field] ? `来自${lib}` : `${lib}${EMPTY_NOTE[field]}`;
}

function fromLibrary(d: LibraryDefaults, field: DownloadField): FieldSource {
  return { kind: "library", libraryID: d.libraryID, note: noteFor(d, field) };
}

function setValue(
  values: DownloadValues,
  field: DownloadField,
  value: number | string,
): DownloadValues {
  return { ...values, [field]: value };
}

/** bringOut 按媒体库带出：自己填的项不动，其余的换成库的值（d 为 null 时清空） */
export function bringOut(form: DefaultsForm, d: LibraryDefaults | null): DefaultsForm {
  let values = { ...form.values };
  const sources = { ...form.sources };
  for (const field of DOWNLOAD_FIELDS) {
    if (sources[field].kind === "user") continue;
    if (d) {
      values = setValue(values, field, d.values[field]);
      sources[field] = fromLibrary(d, field);
    } else {
      values = setValue(values, field, blankValues()[field]);
      sources[field] = { kind: "none" };
    }
  }
  return { values, sources };
}

/** markUser 记下这一项是自己填的（输入、清空、选了别的）；值由调用方先改好 */
export function markUser(form: DefaultsForm, field: DownloadField): DefaultsForm {
  return { values: form.values, sources: { ...form.sources, [field]: { kind: "user" } } };
}

/** resetField 是「换成媒体库的」：这一项换回库的值、来源记成媒体库 */
export function resetField(
  form: DefaultsForm,
  field: DownloadField,
  d: LibraryDefaults | null,
): DefaultsForm {
  if (!d) return form;
  return {
    values: setValue(form.values, field, d.values[field]),
    sources: { ...form.sources, [field]: fromLibrary(d, field) },
  };
}

/** applyAll 是修改订阅时的「套用」：四项都换成库的值；库的下载器用不了时下载器保留订阅原来的 */
export function applyAll(form: DefaultsForm, d: LibraryDefaults | null): DefaultsForm {
  if (!d) return form;
  let next = form;
  for (const field of DOWNLOAD_FIELDS) {
    if (field === "downloader_id" && !usableDownloader(d)) continue;
    next = resetField(next, field, d);
  }
  return next;
}

function sameField(a: DownloadValues, b: DownloadValues, field: DownloadField): boolean {
  if (field === "downloader_id") return a.downloader_id === b.downloader_id;
  return a[field].trim() === b[field].trim();
}

/** differs 是和媒体库的值不一样的项（库的下载器用不了时不算下载器） */
export function differs(values: DownloadValues, d: LibraryDefaults): DownloadField[] {
  return DOWNLOAD_FIELDS.filter((field) => {
    if (field === "downloader_id" && !usableDownloader(d)) return false;
    return !sameField(values, d.values, field);
  });
}

/** sameValues 报告两组值一样（字符串去掉首尾空白再比）；未保存的基准在带出完成以后再取 */
export function sameValues(a: DownloadValues, b: DownloadValues): boolean {
  return DOWNLOAD_FIELDS.every((field) => sameField(a, b, field));
}

/** sourceNote 是写在这一项下面的说明：来自媒体库时写明是哪个库、库没设时推送怎么办；自己填的与没带出过的为空 */
export function sourceNote(form: DefaultsForm, field: DownloadField): string {
  const s = form.sources[field];
  return s.kind === "library" ? s.note : "";
}

function isWindowsPath(p: string): boolean {
  return p.startsWith("\\") || /^[A-Za-z]:/.test(p);
}

/** underPath 和整理范围的判断一样（models/transfer.go 的 matchPathPrefix）：按整段目录比，Windows 路径不分大小写、认两种斜杠 */
function underPath(src: string, prefix: string): boolean {
  const raw = prefix.trim();
  if (!raw) return false;
  const win = isWindowsPath(raw) || isWindowsPath(src);
  let s = src;
  let p = raw;
  if (win) {
    s = s.replaceAll("\\", "/");
    p = p.replaceAll("\\", "/");
  }
  p = p.replace(/\/+$/, "");
  if (p === "") return s.startsWith("/");
  if (s.length < p.length) return false;
  const head = s.slice(0, p.length);
  const same = win ? head.toLowerCase() === p.toLowerCase() : head === p;
  return same && (s.length === p.length || s[p.length] === "/");
}

function sameText(a: string, b: string): boolean {
  return a.toLowerCase() === b.toLowerCase();
}

function splitTags(raw: string): string[] {
  return raw
    .split(",")
    .map((t) => t.trim())
    .filter(Boolean);
}

/**
 * scopeMisses 是按这些值推送的种子不在自动整理范围里的地方（和整理的判断一样：每一项空着不限，不空时满足其中之一）。
 * 四项都空着时不提醒；下载器与保存目录空着时推送用默认的，这里不知道是哪个，不算。
 */
export function scopeMisses(scope: OrganizeScope | null | undefined, v: DownloadValues): string[] {
  if (!scope || sameValues(v, blankValues())) return [];
  const out: string[] = [];
  const downloaders = scope.downloaders ?? [];
  if (downloaders.length && v.downloader_id > 0 && !downloaders.includes(v.downloader_id)) {
    out.push("下载器不在范围里");
  }
  const categories = scope.categories ?? [];
  const category = v.category.trim();
  if (categories.length) {
    const list = categories.join("、");
    if (!category) out.push(`没设分类，范围限了分类（${list}）`);
    else if (!categories.some((c) => sameText(c, category))) {
      out.push(`分类「${category}」不在范围里（${list}）`);
    }
  }
  const tags = scope.tags ?? [];
  if (tags.length) {
    const list = tags.join("、");
    const have = splitTags(v.tags);
    if (!have.length) out.push(`没设标签，范围限了标签（${list}）`);
    else if (!tags.some((want) => have.some((h) => sameText(h, want)))) {
      out.push(`标签不在范围里（${list}）`);
    }
  }
  const paths = scope.save_paths ?? [];
  const savePath = v.save_path.trim();
  if (paths.length && savePath && !paths.some((p) => underPath(savePath, p))) {
    out.push(`保存目录不在范围里（${paths.join("、")}）`);
  }
  return out;
}
