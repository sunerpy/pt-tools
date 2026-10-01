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
 * 还有第三个坑，比上面两个更伤人：**有的站点根本不返回分类**。
 * Gazelle 的 ParseSearch（site/v2/gazelle_driver.go）不填 `Category`，只带 group 的
 * `tags`，而内置 MooKo 的真实样本里 tags 是 `["剧情","悬疑","传记"]` 这种**题材词** ——
 * 它既不是分类，也归不进任何一桶。把这种行和「站点说了、但不属于五桶」的行
 * （M-Team 的 `纪录片`、`TV游戏`）当成同一回事，一选具体档位就会把 MooKo 的结果整片筛掉，
 * 而「站点命中」卡用的是未筛选的后端计数，于是画面变成「MooKo 12 条」配一张空表格。
 *
 * 所以归桶是**三态**（见 `CategoryVerdict`）：命中某桶 / `other` / `unknown`。
 * 三者在筛选里的待遇不同：
 *   命中     留下
 *   other    丢掉（站点表过态，纪录片与游戏照旧不在五个固定桶里）
 *   unknown  默认也丢掉，但**必须被数出来并可一键请回**（见 `matchesCategory`）——
 *            「站点没提供分类」不等于「不属于这个分类」，可它也不该在每个档位下都冒出来。
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
  /*
   * `ape`（Monkey's Audio 的格式名）必须是独立的词：裸子串会把「Escape Room」「Landscape」
   * 这类单词归进音乐。`\b` 只认 ASCII 单词字符，所以「APE无损」「无损/APE」照样命中。
   */
  { bucket: "music", test: /音乐|原声|music|flac|\bape\b|mp3|album/i },
  { bucket: "movie", test: /电影|movies?|film/i },
  {
    bucket: "tv",
    /*
     * `\b(?:hd)?tv(?=[\s/\-_.]|$)`：`TV/HD`、`TV - HD`、末尾的 `TV` 都算，`TV游戏` 不算。
     * `HDTV` 单列一个可选前缀：tv 紧跟在 D 后面没有词边界，只写 `\btv` 会把这一整类漏掉。
     * 中文侧收「影剧 / 剧集 / 电视剧 / 连续剧 / 综艺」—— M-Team 把综艺和影剧归在一类。
     */
    test: /影剧|剧集|电视剧|连续剧|综艺|series|\bshow\b|\b(?:hd)?tv(?=[\s/\-_.]|$)/i,
  },
];

/**
 * 把一个分类名（或标签）归进画板那四个桶；归不进去返回空串。
 *
 * 这是**单个字符串**的匹配器，空串只表示「这个名字认不出来」，不表示「站点没给分类」。
 * 要判断一条搜索结果，用下面的 `verdictOfItem` —— 那两件事必须分开。
 */
export function bucketOf(category: string | undefined | null): CategoryBucket {
  const name = (category ?? "").trim();
  if (!name) return "";
  for (const rule of RULES) {
    if (rule.test.test(name)) return rule.bucket;
  }
  return "";
}

/**
 * 一条结果的归桶结论。
 *
 * `other` 与 `unknown` 必须分开，因为筛选时要区别对待：
 *   `other`   站点**给了**分类，只是不在五桶里（`纪录片`、`TV游戏`、`Sports`…）
 *   `unknown` 站点连分类都没给，标签也认不出内容类型（Gazelle / MooKo）
 */
export type CategoryVerdict = Exclude<CategoryBucket, ""> | "other" | "unknown";

/**
 * 按「分类 + 标签」判定一条结果的归桶结论。
 *
 * 顺序是「分类优先、标签兜底」：分类是站点明确给出的归类，比标签准。
 * 只有分类是空的时候才翻标签 —— 标签兜底是为 Gazelle 那种没有分类字段的响应准备的，
 * 不是用来推翻站点自己的归类。
 *
 * 站点给了分类却归不进任何桶，就是 `other`（这是站点的表态，可以照它筛）。
 * 分类是空的且没有一个标签能归桶，就是 `unknown`（这不是表态，是信息缺失）。
 */
export function verdictOfItem(item: {
  category?: string | null;
  tags?: string[] | null;
}): CategoryVerdict {
  const named = (item.category ?? "").trim();
  if (named) {
    const byCategory = bucketOf(named);
    return byCategory === "" ? "other" : byCategory;
  }
  for (const tag of item.tags ?? []) {
    const byTag = bucketOf(tag);
    if (byTag !== "") return byTag;
  }
  return "unknown";
}

/**
 * 选中具体档位时这一行留不留。
 *
 * 默认**只留命中这个桶的**：档位选「音乐」就该只看到音乐，否则分段器对 Gazelle 那类
 * 站点等于没有作用 —— 一次评审的原话是「只是从『所有分类下消失』改成『所有分类下都出现』」，
 * 判得对。
 *
 * `includeUnknown` 用来把「站点没提供分类」的行请回来。它不是默认值，但也不能没有：
 * MooKo 那种站点一条分类都不给，默认严格筛掉之后它的结果会整片消失，而「站点命中」卡
 * 报的是未筛选的后端计数，画面就成了「MooKo 12 条」配一张空表格。所以调用方（搜索页）
 * 始终把「被筛掉了几条没有分类的行」写在表格上方，并给一个一键请回来的开关：
 * 筛得干净，但没有悄悄丢东西。
 */
export function matchesCategory(
  item: { category?: string | null; tags?: string[] | null },
  bucket: CategoryBucket,
  options: { includeUnknown?: boolean } = {},
): boolean {
  if (!bucket) return true;
  const verdict = verdictOfItem(item);
  if (verdict === bucket) return true;
  return verdict === "unknown" && options.includeUnknown === true;
}

/** 桶 ID → 给人看的标签。用于「已保存的搜索」那类要回显条件的地方 */
export function categoryLabel(bucket: string | undefined): string {
  if (!bucket) return LABEL_OF[""];
  return LABEL_OF[bucket as CategoryBucket] ?? bucket;
}
