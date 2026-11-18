/**
 * 试搜索里「选标题里的一段」（追剧方案 3.2.5）：标题按空格、点、下划线、括号、逗号、竖线、斜杠切成小块，用户点起止两块，
 * 取原标题里的这一段。括号、连字符、点照原样保留，所以这一段一定在这个种子的标题里，加进标题包含以后至少这个种子收。
 */

/** 一小块：text 是原文，start、end 是它在标题里的位置（end 不含） */
export interface TitleToken {
  text: string;
  start: number;
  end: number;
}

// 不是分隔符的连续字符算一块；连字符不切（「Atmos-CHDWEB」是一块）
const TOKEN = /[^\s._()[\]{}【】（）「」《》<>,，|/]+/gu;

export function splitTitle(title: string): TitleToken[] {
  const out: TitleToken[] = [];
  for (const m of title.matchAll(TOKEN)) {
    const start = m.index ?? 0;
    out.push({ text: m[0], start, end: start + m[0].length });
  }
  return out;
}

/** 第 from 块到第 to 块（先后不论）之间的原文；块号不对时为空串 */
export function segmentText(
  title: string,
  tokens: readonly TitleToken[],
  from: number,
  to: number,
): string {
  const a = tokens[Math.min(from, to)];
  const b = tokens[Math.max(from, to)];
  if (!a || !b) return "";
  return title.slice(a.start, b.end);
}

// 集号：S02E03、E03、EP03（前后不能紧挨着字母或数字，「x265-E1A」这类不算）、第 3 集、第三话、第 2 期
const EPISODE = [
  /S\d{1,3}\s*E\d{1,4}/i,
  /(?:^|[^A-Za-z0-9])EP?\d{1,4}(?![A-Za-z0-9])/i,
  /第\s*[0-9一二三四五六七八九十百千零两]+\s*[集话話期]/,
];

/** 这一段里有集号：加进包含以后只收这一集（或这几集） */
export function hasEpisode(text: string): boolean {
  return EPISODE.some((re) => re.test(text));
}
