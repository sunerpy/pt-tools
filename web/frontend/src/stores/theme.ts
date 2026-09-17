import { defineStore } from "pinia";
import { computed, ref, watch } from "vue";

/**
 * 主题由两个互相独立的轴决定：
 *   palette —— 配色方案，对应 Penpot 设计稿的四个方向（styles/theme.scss ①）
 *   mode    —— 明暗，auto 表示跟随系统 prefers-color-scheme
 *
 * 落到 DOM 上仍然是 html.dark / html.light 加 html[data-theme-style]，
 * 沿用旧的属性名是有意的：既有 7000 多行页面样式和 Element Plus 的
 * dark/css-vars.css 都挂在这两个选择器上，换名字要连带改一大片。
 */
export type ThemePalette = "cockpit" | "atlas" | "deck" | "halo";
export type ThemeMode = "light" | "dark" | "auto";

export interface PaletteOption {
  value: ThemePalette;
  label: string;
  hint: string;
  /** 选择器里的色板：强调色 + 内容底色 + 外壳色 */
  swatch: [string, string, string];
}

export const PALETTES: readonly PaletteOption[] = [
  {
    value: "cockpit",
    label: "驾驶舱",
    hint: "浅色内容 + 深色外壳 · 青色强调",
    swatch: ["#0e8fa8", "#f2f4f7", "#171a1f"],
  },
  {
    value: "atlas",
    label: "图册",
    hint: "纸感浅色 · 紫色强调 · 圆角更柔",
    swatch: ["#7c3aed", "#f7f8fa", "#1b1d26"],
  },
  {
    value: "deck",
    label: "甲板",
    hint: "近黑无彩 · 高对比 · 强调色即文字色",
    swatch: ["#fafafa", "#121214", "#08080a"],
  },
  {
    value: "halo",
    label: "光晕",
    hint: "暖调 · 珊瑚橙强调 · 大圆角",
    swatch: ["#ff8a4c", "#1a1714", "#16120f"],
  },
] as const;

export const MODES: readonly { value: ThemeMode; label: string; icon: string }[] = [
  { value: "light", label: "明亮", icon: "sun" },
  { value: "dark", label: "黑暗", icon: "moon" },
  { value: "auto", label: "跟随系统", icon: "monitor" },
] as const;

const PALETTE_VALUES = PALETTES.map((p) => p.value);

/**
 * 旧版本存的是 default/ocean/graphite/contrast/emerald，按性格就近落到新配色，
 * 免得升级后用户看到一个没有任何 token 生效的裸界面。
 */
const LEGACY_PALETTE: Record<string, ThemePalette> = {
  default: "cockpit",
  ocean: "cockpit",
  contrast: "atlas",
  graphite: "deck",
  emerald: "cockpit",
};

const STORAGE_PALETTE = "theme-style";
const STORAGE_MODE = "theme";

function readPalette(): ThemePalette {
  const raw = localStorage.getItem(STORAGE_PALETTE);
  if (raw && (PALETTE_VALUES as string[]).includes(raw)) return raw as ThemePalette;
  if (raw && LEGACY_PALETTE[raw]) return LEGACY_PALETTE[raw];
  return "cockpit";
}

function readMode(): ThemeMode {
  const raw = localStorage.getItem(STORAGE_MODE);
  if (raw === "light" || raw === "dark" || raw === "auto") return raw;
  // 历史默认是黑暗模式，保持不变，避免老用户升级后界面突然变白
  return "dark";
}

function systemPrefersDark(): boolean {
  return typeof window.matchMedia === "function"
    ? window.matchMedia("(prefers-color-scheme: dark)").matches
    : true;
}

export const useThemeStore = defineStore("theme", () => {
  const palette = ref<ThemePalette>(readPalette());
  const mode = ref<ThemeMode>(readMode());
  const systemDark = ref(systemPrefersDark());

  // auto 模式下要跟着系统切，所以监听整个会话，而不是只在启动时读一次
  if (typeof window.matchMedia === "function") {
    const mql = window.matchMedia("(prefers-color-scheme: dark)");
    const onChange = (e: MediaQueryListEvent) => {
      systemDark.value = e.matches;
    };
    if (typeof mql.addEventListener === "function") {
      mql.addEventListener("change", onChange);
    } else if (typeof mql.addListener === "function") {
      // Safari < 14 / 旧 WebView（打包成 app 时会遇到）
      mql.addListener(onChange);
    }
  }

  const isDark = computed(() => (mode.value === "auto" ? systemDark.value : mode.value === "dark"));

  watch(
    [isDark, palette],
    ([dark, p]) => {
      const root = document.documentElement;
      root.classList.toggle("dark", dark);
      root.classList.toggle("light", !dark);
      root.setAttribute("data-theme-style", p);
      // 让浏览器原生控件（滚动条、表单、地址栏）跟着走
      root.style.colorScheme = dark ? "dark" : "light";
    },
    { immediate: true },
  );

  watch(mode, (value) => localStorage.setItem(STORAGE_MODE, value), { immediate: true });
  watch(palette, (value) => localStorage.setItem(STORAGE_PALETTE, value), { immediate: true });

  function setMode(value: ThemeMode) {
    if (value === "light" || value === "dark" || value === "auto") mode.value = value;
  }

  /** 点击式切换：auto 先落到与当前观感相反的一侧，再在明暗之间来回 */
  function toggle() {
    setMode(isDark.value ? "light" : "dark");
  }

  function setPalette(value: string) {
    if ((PALETTE_VALUES as string[]).includes(value)) palette.value = value as ThemePalette;
  }

  return {
    palette,
    mode,
    isDark,
    setMode,
    setPalette,
    toggle,
    palettes: PALETTES,
    modes: MODES,
  };
});
