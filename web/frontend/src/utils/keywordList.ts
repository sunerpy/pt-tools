/**
 * 订阅的标题包含、标题排除（每行或逗号分隔一个词）：试搜索的结果上「只要」「不要」「选标题里的一段」往里加一行。
 * 写法与后端相同（internal/media/subscribe/keywords.go）：按换行与逗号拆，不分大小写，空格、点、下划线连在一起的算一个空格；
 * 去掉首尾空白以后最长 1024 字节。
 */

export const MAX_KEYWORDS_BYTES = 1024;

/** 规整一个词：转小写，空格、点、下划线连在一起的算一个空格 */
export function normalizeKeyword(s: string): string {
  return s.toLowerCase().replace(/[._]/g, " ").split(/\s+/).filter(Boolean).join(" ");
}

/** 拆成规整后的词（去掉空的） */
export function splitKeywords(text: string): string[] {
  return text
    .split(/[\n,，]/)
    .map(normalizeKeyword)
    .filter(Boolean);
}

const byteLength = (s: string) => new TextEncoder().encode(s).length;

export type AddKeywordResult =
  | { ok: true; text: string }
  | { ok: false; reason: "empty" | "separator" | "duplicate" | "too_long"; message: string };

/**
 * 在 text 后面加一行 word：规整以后已经有了的不加；词里有逗号或换行时不加（会被当成几个词）；
 * 加了以后超过 1024 字节时不加。原来的内容不动（只去掉末尾多余的空行与逗号）。
 */
export function addKeyword(text: string, word: string): AddKeywordResult {
  const w = word.trim();
  if (!normalizeKeyword(w)) return { ok: false, reason: "empty", message: "没有可以加的词" };
  if (/[\n,，]/.test(w)) {
    return {
      ok: false,
      reason: "separator",
      message: "这一段里有逗号，会被当成几个词：选一段不带逗号的",
    };
  }
  if (splitKeywords(text).includes(normalizeKeyword(w))) {
    return { ok: false, reason: "duplicate", message: `已经有「${w}」了` };
  }
  const base = text.replace(/[\s,，]+$/, "");
  const next = base ? `${base}\n${w}` : w;
  if (byteLength(next.trim()) > MAX_KEYWORDS_BYTES) {
    return {
      ok: false,
      reason: "too_long",
      message: `加了以后超过 ${MAX_KEYWORDS_BYTES} 字节：先删掉几个词`,
    };
  }
  return { ok: true, text: next };
}
