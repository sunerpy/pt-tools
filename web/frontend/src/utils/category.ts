/*
 * 搜索结果的分类归桶 —— 画板 15 的 bar-88 分段器只有五档
 * （全部 / 电影 / 剧集 / 动漫 / 音乐），而各站点回的分类名是自己那一套：
 *
 *   M-Team   `电影/HD`、`影剧/综艺/HD`、`纪录片`、`音乐(无损)`、`演唱会`、
 *            `PC游戏`、`TV游戏`、`动画`、`H-Anime`、`H-Comic`…
 *            （site/v2/mtorrent_driver.go 的 mteamCategoryMap）
 *   HDDolby  `Movies/SD`、`TV/HD`、`Documentary`、`Animation`、`Music`、
 *            `Sports`、`Games`…（site/v2/hddolby_driver.go 的 getCategoryName）
 *   Unit3D   站点自己配的 `Category.Name`，内容不可枚举
 *   NexusPHP 从列表页的分类图标上刮下来的文字，同样不可枚举
 *
 * 所以归桶必须按**规则**而不是简单的子串包含，而且顺序有讲究。两个真实的坑：
 *
 *   ① 漏收：`影剧/综艺/HD` 里没有「剧集」二字，`Animation` 里没有「动漫」；
 *      只写「剧集」「动漫」两个词会把这两大类整片漏掉。
 *   ② 误收：`TV游戏` 含 `tv`，裸的子串匹配会把 TV 游戏归进「剧集」。
 *      所以 `tv` 只在后面跟分隔符或到头时才算命中（`TV/HD` 命中，`TV游戏` 不命中）。
 *
 * 归不进任何一桶的（纪录片、运动、游戏、软件、电子书…）在选中具体档位时不显示 ——
 * 五个固定桶的设计本身就是这个含义，不为它们编一个「其他」档。
 */

/** 桶 ID。空串是「全部」，不参与匹配 */
export type CategoryBucket = "" | "movie" | "tv" | "anime" | "music";

export interface CategoryOption {
  label: string;
  value: CategoryBucket;
}

/** 画板 bar-88 的五个固定档位，顺序照画板 */
export const CATEGORY_OPTIONS: readonly CategoryOption[] = [
  { label: "全部", value: "" },
  { label: "电影", value: "movie" },
  { label: "剧集", value: "tv" },
  { label: "动漫", value: "anime" },
  { label: "音乐", value: "music" },
];

const LABEL_OF: Record<CategoryBucket, string> = {
  "": "全部分类",
  movie: "电影",
  tv: "剧集",
  anime: "动漫",
  music: "音乐",
};

/**
 * 匹配规则，**按顺序**试，第一条命中即停。顺序不是随意的：
 * 动漫要排在剧集前面（`TV动画` 属动漫），音乐要排在电影前面
 * （`音乐(无损)` 不含电影词，但把顺序写死能防以后加词时打架）。
 */
const RULES: readonly { bucket: Exclude<CategoryBucket, "">; test: RegExp }[] = [
  { bucket: "anime", test: /动漫|动画|卡通|anime|animation|comic/i },
  { bucket: "music", test: /音乐|原声|music|flac|ape|mp3|album/i },
  { bucket: "movie", test: /电影|movies?|film/i },
  {
    bucket: "tv",
    /*
     * `\btv(?=[\s/\-_.]|$)`：`TV/HD`、`TV - HD`、末尾的 `TV` 都算，`TV游戏` 不算。
     * 中文侧收「影剧 / 剧集 / 电视剧 / 连续剧 / 综艺」—— M-Team 把综艺和影剧归在一类。
     */
    test: /影剧|剧集|电视剧|连续剧|综艺|series|\bshow\b|\btv(?=[\s/\-_.]|$)/i,
  },
];

/** 把站点回的分类名归进画板那四个桶；归不进去返回空串 */
export function bucketOf(category: string | undefined | null): CategoryBucket {
  const name = (category ?? "").trim();
  if (!name) return "";
  for (const rule of RULES) {
    if (rule.test.test(name)) return rule.bucket;
  }
  return "";
}

/**
 * 按「分类 + 标签」归桶。
 *
 * 为什么要看标签：Gazelle 的搜索响应里**根本没有分类字段**
 * （`site/v2/gazelle_driver.go` 的 ParseSearch 因此不填 `Category`），
 * 它用 group 上的 `tags` 表达内容类型 —— 走 Gazelle 的站点（内置 MooKo）分类恒为空，
 * 一选具体档位就被全部筛掉，画板那条分段器对它等于失效。
 *
 * 顺序是「分类优先、标签兜底」：分类是站点明确给出的归类，比标签准。
 * 标签里归不进任何桶的（音乐站的流派标签 electronic / jazz 之类）照旧返回空串。
 */
export function bucketOfItem(item: {
  category?: string | null;
  tags?: string[] | null;
}): CategoryBucket {
  const byCategory = bucketOf(item.category);
  if (byCategory) return byCategory;
  for (const tag of item.tags ?? []) {
    const byTag = bucketOf(tag);
    if (byTag) return byTag;
  }
  return "";
}

/** 桶 ID → 给人看的标签。用于「已保存的搜索」那类要回显条件的地方 */
export function categoryLabel(bucket: string | undefined): string {
  if (!bucket) return LABEL_OF[""];
  return LABEL_OF[bucket as CategoryBucket] ?? bucket;
}
