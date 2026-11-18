/**
 * 海报转 PNG、海报用的字体与站点图标。
 *
 * 导出的 PNG 就是预览那份 DOM 画出来的（modern-screenshot：克隆节点、内联计算后的样式，放进 SVG 的
 * foreignObject 再画到画布上），预览与导出不再是两套排版。画进 SVG 的内容取不了外部资源，所以：
 * - 站点图标先取成 data URL（loadPosterLogos），海报里直接用；
 * - 打包进来的两种字体（只有拉丁字符）在导出时作为 data URL 的 @font-face 一起交进去（font.cssText），
 *   中文用系统字体。字族名单独起（PT Poster Serif / Mono），不和本站其他地方的等宽字混用。
 */
import serifUrl from "@fontsource/source-serif-4/files/source-serif-4-latin-500-normal.woff2?url";
import mono400Url from "@fontsource/jetbrains-mono/files/jetbrains-mono-latin-400-normal.woff2?url";
import mono700Url from "@fontsource/jetbrains-mono/files/jetbrains-mono-latin-700-normal.woff2?url";

import { getAvatarColor } from "@/utils/format";

import { fallbackCells, type PosterLogoData } from "./model";

export type PosterFont = "serif" | "mono";

interface FontFile {
  family: string;
  weight: string;
  url: string;
}

const FONT_FILES: Record<PosterFont, FontFile[]> = {
  serif: [{ family: "PT Poster Serif", weight: "500", url: serifUrl }],
  mono: [
    { family: "PT Poster Mono", weight: "400", url: mono400Url },
    { family: "PT Poster Mono", weight: "700", url: mono700Url },
  ],
};

let fontsAdded: Promise<void> | null = null;

/** 把两种字体登记进 document.fonts（只登记一次），等它们加载完；不支持 FontFace 的环境直接跳过 */
export function ensurePosterFonts(): Promise<void> {
  if (fontsAdded) return fontsAdded;
  if (typeof FontFace === "undefined" || typeof document === "undefined" || !document.fonts) {
    fontsAdded = Promise.resolve();
    return fontsAdded;
  }
  const faces = Object.values(FONT_FILES)
    .flat()
    .map((f) => new FontFace(f.family, `url(${f.url}) format("woff2")`, { weight: f.weight }));
  for (const face of faces) document.fonts.add(face);
  fontsAdded = Promise.all(faces.map((face) => face.load().catch(() => face))).then(() => {});
  return fontsAdded;
}

const fontCss = new Map<string, Promise<string>>();

async function fontFaceCss(f: FontFile): Promise<string> {
  const res = await fetch(f.url);
  if (!res.ok) throw new Error(`字体取不到（HTTP ${res.status}）`);
  const data = await blobToDataUrl(new Blob([await res.arrayBuffer()], { type: "font/woff2" }));
  return `@font-face{font-family:"${f.family}";src:url(${data}) format("woff2");font-weight:${f.weight};font-style:normal;font-display:block;}`;
}

/** 导出时交给 modern-screenshot 的 @font-face（data URL）；取不到字体时退回系统字体，不让导出失败 */
async function embedCss(fonts: PosterFont[]): Promise<string> {
  const parts = await Promise.all(
    fonts.flatMap((name) =>
      FONT_FILES[name].map((f) => {
        const key = `${f.family}/${f.weight}`;
        if (!fontCss.has(key))
          fontCss.set(
            key,
            fontFaceCss(f).catch(() => ""),
          );
        return fontCss.get(key)!;
      }),
    ),
  );
  return parts.join("\n");
}

/** 预览里排成一行的文字：导出前在原节点上做记号，克隆出来的节点据此改成不折行 */
const ONE_LINE = "data-poster-one-line";

/**
 * 元素里的文字在预览里是不是只占一行：逐段文字取行框，后一段既回到前一段的左边、又落到它下面，
 * 就是换了行（上下叠放的两块字也算）。量不到行框（例如测试环境）时当成不是一行，不做处理。
 */
function isOneLine(el: HTMLElement): boolean {
  const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
  const range = document.createRange();
  let prev: DOMRect | null = null;
  for (let n = walker.nextNode(); n; n = walker.nextNode()) {
    if (!n.textContent?.trim()) continue;
    range.selectNodeContents(n);
    for (const r of Array.from(range.getClientRects())) {
      if (r.width <= 0 || r.height <= 0) continue;
      if (prev && r.left < prev.right - 1 && r.top > prev.top + 1) return false;
      prev = r;
    }
  }
  return prev !== null;
}

/**
 * modern-screenshot 把每个元素的计算宽高原样写进克隆：宽度刚好包住文字的元素（弹性布局里的标题、
 * 角落的小标签），导出时字只要比预览宽一点点（手机 WebView 里画进 SVG 的字与页面上的宽度不完全一样），
 * 最后一个字就折到下一行，高度却还是一行，压在下面的文字上。所以预览里排成一行的，导出也不许折行。
 */
function markOneLineText(root: HTMLElement): HTMLElement[] {
  const marked: HTMLElement[] = [];
  for (const el of root.querySelectorAll("*")) {
    if (!(el instanceof HTMLElement) || !el.textContent?.trim()) continue;
    const style = getComputedStyle(el);
    if (style.whiteSpace !== "normal" || ["inline", "contents", "none"].includes(style.display))
      continue;
    if (!isOneLine(el)) continue;
    el.setAttribute(ONE_LINE, "");
    marked.push(el);
  }
  return marked;
}

/**
 * 克隆出来的节点：带记号的连同里面的子元素都改成不折行（子元素的克隆会自带 text-wrap: wrap，
 * 靠继承压不住），记号本身不进图。
 */
export function keepOneLine(cloned: Node): void {
  if (cloned.nodeType !== Node.ELEMENT_NODE) return;
  const el = cloned as HTMLElement;
  if (!el.hasAttribute(ONE_LINE)) return;
  el.removeAttribute(ONE_LINE);
  for (const node of [el, ...Array.from(el.querySelectorAll<HTMLElement>("*"))])
    node.style?.setProperty("white-space", "nowrap");
}

/**
 * 把海报节点画成 PNG（2 倍）。宽高按节点自己的布局尺寸给：预览为了放进窄屏整体缩放过，
 * getBoundingClientRect 量到的是缩放后的大小。
 */
export async function posterToBlob(node: HTMLElement, fonts: PosterFont[]): Promise<Blob> {
  const { domToBlob } = await import("modern-screenshot");
  await ensurePosterFonts();
  const cssText = await embedCss(fonts);
  const marked = markOneLineText(node);
  try {
    return await domToBlob(node, {
      scale: 2,
      type: "image/png",
      width: node.offsetWidth,
      height: node.offsetHeight,
      backgroundColor: null,
      timeout: 15000,
      // 没有要打包的字体时也要给一个 cssText：不给时它会去扫整页的 @font-face（Element 的图标字体等）
      font: { cssText: cssText || "/* system fonts only */" },
      onCloneEachNode: keepOneLine,
    });
  } finally {
    for (const el of marked) el.removeAttribute(ONE_LINE);
  }
}

function blobToDataUrl(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result));
    reader.onerror = () => reject(reader.error ?? new Error("读不了图片"));
    reader.readAsDataURL(blob);
  });
}

function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image();
    img.onload = () => resolve(img);
    img.onerror = () => reject(new Error("图片解码失败"));
    img.src = src;
  });
}

/**
 * 打码色块：图标先画到白底的 16×16 上，再按 4×4 一块求平均 —— 直接缩到 4×4 时浏览器只取样几个像素，
 * 颜色会跳。画布不可用（测试环境）或图标没有尺寸时返回 null，用站点颜色的色块。
 */
function pixelCells(img: HTMLImageElement): string[] | null {
  if (img.naturalWidth <= 1 || img.naturalHeight <= 1) return null;
  const canvas = document.createElement("canvas");
  canvas.width = canvas.height = 16;
  const ctx = canvas.getContext("2d");
  if (!ctx) return null;
  ctx.fillStyle = "#fff";
  ctx.fillRect(0, 0, 16, 16);
  ctx.imageSmoothingEnabled = true;
  ctx.drawImage(img, 0, 0, 16, 16);
  const { data } = ctx.getImageData(0, 0, 16, 16);
  const cells: string[] = [];
  for (let by = 0; by < 4; by++) {
    for (let bx = 0; bx < 4; bx++) {
      let r = 0;
      let g = 0;
      let b = 0;
      for (let y = by * 4; y < by * 4 + 4; y++) {
        for (let x = bx * 4; x < bx * 4 + 4; x++) {
          const i = (y * 16 + x) * 4;
          r += data[i]!;
          g += data[i + 1]!;
          b += data[i + 2]!;
        }
      }
      cells.push(`rgb(${Math.round(r / 16)} ${Math.round(g / 16)} ${Math.round(b / 16)})`);
    }
  }
  return cells;
}

/**
 * 取一个站点的图标。后端取不到图标时回 1×1 的透明占位（响应头 X-Favicon-Placeholder: 1），
 * 这时与网络出错一样用首字母与站点颜色的色块。
 */
export async function loadPosterLogo(
  site: string,
  readDataUrl?: (site: string) => Promise<string>,
): Promise<PosterLogoData> {
  const fallback: PosterLogoData = { src: null, cells: fallbackCells(site, getAvatarColor(site)) };
  try {
    let src: string;
    if (readDataUrl) {
      src = await readDataUrl(site.toLowerCase());
      if (!src.startsWith("data:image/")) return fallback;
    } else {
      const res = await fetch(`/api/favicon/${encodeURIComponent(site.toLowerCase())}`, {
        credentials: "same-origin",
      });
      if (!res.ok || res.headers.get("X-Favicon-Placeholder") === "1") return fallback;
      const blob = await res.blob();
      if (!blob.type.startsWith("image/")) return fallback;
      src = await blobToDataUrl(blob);
    }
    const img = await loadImage(src);
    if (img.naturalWidth === 1 && img.naturalHeight === 1) return fallback;
    return { src, cells: pixelCells(img) ?? fallback.cells };
  } catch {
    return fallback;
  }
}

export async function loadPosterLogos(
  sites: string[],
  readDataUrl?: (site: string) => Promise<string>,
): Promise<Map<string, PosterLogoData>> {
  const entries = await Promise.all(
    sites.map(async (s) => [s.toLowerCase(), await loadPosterLogo(s, readDataUrl)] as const),
  );
  return new Map(entries);
}
