/*
 * 删除一部片（媒体中心 M7，画板 03 的片段「删除这一季」）用到的纯函数：弹窗标题、库文件那一行的说明、种子行的文字、
 * 打开时默认勾哪些、请求体与底部的摘要。能不能删都由服务端判断（执行时还会现读下载器、重新判断一遍），这里只照预览拼。
 */
import type {
  DeleteFile,
  DeletePreview,
  DeleteRequest,
  DeleteResult,
  DeleteSelector,
  DeleteStep,
  DeleteSubscriptionAction,
  DeleteTorrent,
  MediaIssue,
  MediaKind,
  TorrentKey,
} from "@/api";
import { epCode, epList, shortSize } from "@/utils/entrySearch";
import { modeLabel } from "@/utils/media";
import { type IssueRef, itemTorrentSpan, torrentData } from "@/utils/mediaItem";

/** 从哪里打开的：大图的「删除」、种子行的「删除这个种子」（只勾这一个）、问题的「处理」（只删有问题的集） */
export type DeletePreset =
  | { kind: "item" }
  | { kind: "torrent"; key: TorrentKey }
  | { kind: "issues" };

export interface DeleteTarget {
  selector: DeleteSelector;
  /** 条目名，写在标题的书名号里 */
  title: string;
  preset: DeletePreset;
}

const pad = (n: number) => String(n).padStart(2, "0");

/** 弹窗标题：「删除《长夜将尽》第 2 季」「删除《长夜将尽》第 2 季 E03」「删除《长夜将尽》特别篇」「删除《沙丘2》」 */
export function deleteTitle(t: DeleteTarget): string {
  const sel = t.selector;
  const head = `删除《${t.title}》`;
  if (sel.media_type !== "tv") return head;
  const season = sel.season === 0 ? "特别篇" : `第 ${sel.season} 季`;
  return sel.episodes?.length ? `${head}${season} ${epList(sel.episodes)}` : `${head}${season}`;
}

/** 选的范围怎么说：「这部电影」「这一季」「这几集」 */
export function scopeText(sel: DeleteSelector): string {
  if (sel.media_type !== "tv") return "这部电影";
  return sel.episodes?.length ? "这几集" : "这一季";
}

/**
 * 「处理」要删的范围：电影是整部；剧集是第一个问题的那一季里有问题的集，其中有不知道是哪一集的就是整季。
 * 问题按季、集排好（服务端排的），别的季的问题处理完这一季以后再处理。
 */
export function issueSelector(
  kind: MediaKind,
  tmdbID: number,
  issues: readonly IssueRef[],
): DeleteSelector {
  if (kind !== "tv") return { media_type: kind, tmdb_id: tmdbID, season: 0 };
  const season = issues[0]?.season ?? 0;
  const eps = issues.filter((i) => (i.season ?? 0) === season).map((i) => i.episode ?? 0);
  const episodes = eps.some((n) => n <= 0) ? [] : [...new Set(eps)].sort((a, b) => a - b);
  return { media_type: "tv", tmdb_id: tmdbID, season, episodes };
}

/** 「处理」打开时写在最上面的问题：「S02E03 在 Jellyfin 里被删了」「S02E05 库里的文件不见了」（电影不写集） */
export function issueLine(is: MediaIssue): string {
  const where =
    is.media_type === "tv"
      ? is.episode > 0
        ? `S${pad(is.season)}${epCode(is.episode)} `
        : `S${pad(is.season)} `
      : "";
  if (is.kind === "server_removed") return `${where}在 ${is.server_name || "媒体服务器"} 里被删了`;
  return `${where}库里的文件不见了`;
}

/** 种子在弹窗里的键：下载器编号与小写 hash（同一个 hash 在两台下载器上是两个种子，各自判断） */
export function torrentKey(t: { downloader_id: number; hash: string }): string {
  return `${t.downloader_id}|${t.hash.toLowerCase()}`;
}

/** 一个种子在弹窗里怎么选：on 是删，force 是受保护的勾了「仍然删除」，data 是连数据一起删 */
export interface TorrentPick {
  on: boolean;
  force: boolean;
  data: boolean;
}

/** 弹窗里的勾选：库里的文件（pt-tools 放进去的）、导入前就有的文件、每个种子、订阅怎么办 */
export interface DeleteChoice {
  files: boolean;
  existing: boolean;
  torrents: Record<string, TorrentPick>;
  subscription: DeleteSubscriptionAction;
}

/**
 * 库里的文件分三类：placed 是 pt-tools 放进去的（勾「删除库里的文件」就删）；existing 是导入前就有的（是唯一的一份，
 * 要另外勾）；locked 是不能在这里删的（移动整理的）。
 */
export function fileGroups(files: readonly DeleteFile[]): {
  placed: DeleteFile[];
  existing: DeleteFile[];
  locked: DeleteFile[];
} {
  const out = {
    placed: [] as DeleteFile[],
    existing: [] as DeleteFile[],
    locked: [] as DeleteFile[],
  };
  for (const f of files) {
    if (!f.deletable) out.locked.push(f);
    else if (f.existing) out.existing.push(f);
    else out.placed.push(f);
  }
  return out;
}

/**
 * 选「暂停」时真的会暂停的订阅有几个（同服务端）：在找的；剧集已经完成的也算（不暂停的话台账会把删掉的集当成缺的）。
 * 已经暂停的、还没确认的、完成了的电影订阅不动。
 */
export function pausable(p: DeletePreview, kind: MediaKind): number {
  return p.subscriptions.filter(
    (s) => s.status === "active" || (s.status === "done" && kind === "tv"),
  ).length;
}

/**
 * 打开时默认勾的：大图与「处理」打开的勾上库里的文件、能删的种子（种子里还有这次没选的集的不勾）、暂停订阅（没有要暂停的订阅时
 * 保留）；「删除这个种子」只勾那一个（受保护的也不勾，要先勾「仍然删除」），订阅保留。导入前就有的文件、受保护的种子一律不勾。
 */
export function defaultChoice(
  p: DeletePreview,
  preset: DeletePreset,
  kind: MediaKind,
): DeleteChoice {
  const one = preset.kind === "torrent" ? torrentKey(preset.key) : "";
  const torrents: Record<string, TorrentPick> = {};
  for (const t of p.torrents) {
    const k = torrentKey(t);
    const on = preset.kind === "torrent" ? k === one && t.deletable : t.deletable && !t.covers_more;
    torrents[k] = { on, force: false, data: true };
  }
  return {
    files: preset.kind !== "torrent" && fileGroups(p.files).placed.length > 0,
    existing: false,
    torrents,
    subscription: preset.kind !== "torrent" && pausable(p, kind) > 0 ? "pause" : "keep",
  };
}

/** 连数据删时，同一台下载器里还用着这份数据、这次又不删的种子（名字）：有的话只删种子，保留数据 */
export function sharedBy(t: DeleteTorrent, c: DeleteChoice): string[] {
  return t.shares
    .filter((s) => !c.torrents[torrentKey({ downloader_id: t.downloader_id, hash: s.hash })]?.on)
    .map((s) => s.name || s.hash);
}

/** 按勾选拼请求体：受保护的种子勾了「仍然删除」才删，并按（下载器编号, hash）写进确认；共用数据的只删种子 */
export function deleteRequest(
  sel: DeleteSelector,
  p: DeletePreview,
  c: DeleteChoice,
): DeleteRequest {
  const g = fileGroups(p.files);
  const files = [...(c.files ? g.placed : []), ...(c.existing ? g.existing : [])];
  const torrents: DeleteRequest["torrents"] = [];
  const confirm: TorrentKey[] = [];
  for (const t of p.torrents) {
    const pick = c.torrents[torrentKey(t)];
    if (!pick?.on || (!t.deletable && !pick.force)) continue;
    const key = { downloader_id: t.downloader_id, hash: t.hash };
    torrents.push({ ...key, delete_data: pick.data && !sharedBy(t, c).length });
    if (!t.deletable) confirm.push(key);
  }
  return {
    ...sel,
    history_ids: files.map((f) => f.id),
    allow_existing: c.existing && g.existing.length > 0,
    torrents,
    confirm_protected: confirm,
    subscription: p.subscriptions.length ? c.subscription : "keep",
  };
}

/** 有没有要做的：删库文件、删种子，或者删除订阅、暂停在找的订阅 */
export function canDelete(req: DeleteRequest, p: DeletePreview): boolean {
  return (
    req.history_ids.length > 0 ||
    req.torrents.length > 0 ||
    (p.subscriptions.length > 0 && req.subscription === "delete") ||
    (req.subscription === "pause" && pausable(p, req.media_type) > 0)
  );
}

/** 订阅的叫法：追剧的写「追剧」，别的写「订阅」 */
const subNoun = (p: DeletePreview) => (p.subscriptions.some((s) => s.follow) ? "追剧" : "订阅");

export interface SubscriptionOption {
  value: DeleteSubscriptionAction;
  label: string;
  desc: string;
}

/** 没有要暂停的订阅时，「暂停」那一项写明为什么不用暂停 */
function noPauseWhy(p: DeletePreview): string {
  const status = new Set(p.subscriptions.map((s) => s.status));
  const only = status.size === 1 ? [...status][0] : "";
  if (only === "paused") return "不用暂停：已经暂停了";
  if (only === "pending") return "不用暂停：待确认的订阅不会去下";
  if (only === "done") return "不用暂停：已经完成了，不会再去下";
  return "不用暂停：现在不会去下";
}

/** 订阅的三个选项（画板 03 的片段）：暂停（推荐）、删除、保留；没有要暂停的订阅时「暂停」那一项写明不用暂停 */
export function subscriptionOptions(p: DeletePreview, kind: MediaKind): SubscriptionOption[] {
  const noun = subNoun(p);
  const tv = kind === "tv";
  let pauseDesc = tv
    ? "推荐：不暂停的话，台账会把删掉的集当成缺的，再去下一遍"
    : "推荐：不暂停的话，还在找的订阅会再去下一遍";
  if (!pausable(p, kind)) pauseDesc = noPauseWhy(p);
  return [
    { value: "pause", label: `暂停${noun}`, desc: pauseDesc },
    { value: "delete", label: "删除订阅", desc: tv ? "以后不再找这一季" : "以后不再找这部电影" },
    {
      value: "keep",
      label: noun === "追剧" ? "保留，继续追" : "保留订阅",
      desc: "只删这次选的文件和种子",
    },
  ];
}

/** 底部的摘要，跟着勾选变：「将删除 6 个库文件、1 个种子（连数据），暂停追剧」 */
export function deleteSummary(req: DeleteRequest, p: DeletePreview): string {
  const parts: string[] = [];
  if (req.history_ids.length) parts.push(`${req.history_ids.length} 个库文件`);
  const n = req.torrents.length;
  if (n) {
    const data = req.torrents.filter((t) => t.delete_data).length;
    const how = data === n ? "连数据" : data === 0 ? "只删种子" : `${data} 个连数据`;
    parts.push(`${n} 个种子（${how}）`);
  }
  let sub = "";
  if (req.subscription === "pause" && pausable(p, req.media_type)) sub = `暂停${subNoun(p)}`;
  else if (p.subscriptions.length && req.subscription === "delete") sub = "删除订阅";
  if (!parts.length) return sub ? `只${sub}` : "没有选要删的";
  return `将删除 ${parts.join("、")}${sub ? `，${sub}` : ""}`;
}

const MODE_ALL: Record<string, string> = {
  hardlink: "都是硬链接",
  symlink: "都是软链接",
  copy: "都是复制出来的",
};

/** 库里那些文件的总大小（不在了的不算） */
const sizeOf = (files: readonly DeleteFile[]) =>
  files.reduce((n, f) => n + (f.state === "missing" ? 0 : f.size), 0);

/**
 * 「删除库里的文件」那一行的说明：「6 个文件 · 41.2 GB · 都是硬链接，种子里的数据不受影响」；notes 是另起一行的补充
 * （换成了别的文件的只删记录、不在了的、移动整理的不在这里删）。没有能删的文件时写明为什么，勾选框不能勾。
 */
export function filesView(
  files: readonly DeleteFile[],
  sel: DeleteSelector,
): { line: string; notes: string[]; enabled: boolean } {
  const { placed, existing, locked } = fileGroups(files);
  const notes: string[] = [];
  if (locked.length) notes.push(`${locked.length} 个是移动整理的，是唯一的一份，不在这里删`);
  if (!placed.length) {
    const line =
      existing.length || locked.length
        ? "没有 pt-tools 放进去的文件"
        : `库里没有${scopeText(sel)}的文件`;
    return { line, notes, enabled: false };
  }
  const modes = [...new Set(placed.map((f) => f.mode))];
  const how =
    modes.length === 1
      ? (MODE_ALL[modes[0]!] ?? `都是${modeLabel(modes[0])}`)
      : modes
          .map((m) => `${modeLabel(m)} ${placed.filter((f) => f.mode === m).length} 个`)
          .join("、");
  const size = sizeOf(placed);
  const line = [
    `${placed.length} 个文件`,
    size > 0 ? shortSize(size) : "",
    `${how}，种子里的数据不受影响`,
  ]
    .filter(Boolean)
    .join(" · ");
  const changed = placed.filter((f) => f.state === "changed").length;
  const missing = placed.filter((f) => f.state === "missing").length;
  if (changed) notes.unshift(`${changed} 个换成了别的文件，只删记录、留着文件`);
  if (missing) notes.unshift(`${missing} 个已经不在了，删的是记录`);
  return { line, notes, enabled: true };
}

/** 「也删导入前就有的文件」那一行的说明：「3 个文件 · 12.1 GB · 导入前就在库里，删了就没有了」 */
export function existingLine(files: readonly DeleteFile[]): string {
  const { existing } = fileGroups(files);
  const size = sizeOf(existing);
  return [
    `${existing.length} 个文件`,
    size > 0 ? shortSize(size) : "",
    "导入前就在库里，删了就没有了",
  ]
    .filter(Boolean)
    .join(" · ");
}

/** 「看文件」里一个文件的第二行：「S02E01 · 剧集库 · 硬链接 · 6.8 GB」 */
export function fileMeta(f: DeleteFile): string {
  const parts: string[] = [];
  if (f.episode > 0) {
    const ep =
      f.episode_end > f.episode
        ? `${epCode(f.episode)}–${epCode(f.episode_end)}`
        : epCode(f.episode);
    parts.push(`S${pad(f.season)}${ep}`);
  }
  if (f.library) parts.push(f.library);
  parts.push(modeLabel(f.mode));
  if (f.size > 0) parts.push(shortSize(f.size));
  return parts.join(" · ");
}

/** 种子行的标题：「OurBits · S02E01–E04 · qb」（站点 · 集 · 下载器；电影写版本） */
export function deleteTorrentTitle(t: DeleteTorrent): string {
  return [t.site || "下载器里", itemTorrentSpan(t) || t.version || "", t.downloader]
    .filter(Boolean)
    .join(" · ");
}

/** 能删的种子的说明：「分享率 1.31 · 做种 18 天 · H&R 已满足」 */
export function deleteTorrentSub(t: DeleteTorrent): string {
  const parts = [torrentData(t)];
  if (t.reason === "H&R 已满足") parts.push(t.reason);
  return parts.join(" · ");
}

/** 受保护的种子的说明：「受保护：H&R 要求做种 7 天，还差 2 天」 */
export function protectedText(t: DeleteTorrent): string {
  return `受保护：${t.why || t.protection || t.reason.replace(/^受保护：/, "")}`;
}

const STEP_KIND: Record<DeleteStep["kind"], string> = {
  subscription: "订阅",
  torrent: "种子",
  file: "库文件",
};

export function stepKindLabel(kind: DeleteStep["kind"]): string {
  return STEP_KIND[kind] ?? kind;
}

/** 删完以后的一句话：都成功了，或者几步没做成 */
export function resultText(r: DeleteResult): string {
  if (!r.steps.length) return "没有要做的";
  if (!r.failed) return `删完了，${r.steps.length} 步都做成了`;
  return `${r.failed} 步没做成，原因写在下面；做成了的不受影响`;
}
