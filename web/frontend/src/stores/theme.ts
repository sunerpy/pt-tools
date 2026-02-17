import { defineStore } from "pinia";
import { ref, watch } from "vue";

type ThemeStyle = "default" | "ocean" | "graphite" | "contrast" | "emerald";

export const useThemeStore = defineStore("theme", () => {
  // 默认为黑暗模式：只有明确设置为 'light' 时才使用亮色模式
  const storedTheme = localStorage.getItem("theme");
  const isDark = ref(storedTheme !== "light");

  const storedThemeStyle = localStorage.getItem("theme-style") as ThemeStyle | null;
  const themeStyle = ref<ThemeStyle>(
    storedThemeStyle === "ocean" ||
      storedThemeStyle === "graphite" ||
      storedThemeStyle === "contrast" ||
      storedThemeStyle === "emerald"
      ? storedThemeStyle
      : "default",
  );

  watch(
    isDark,
    (value) => {
      localStorage.setItem("theme", value ? "dark" : "light");
      document.documentElement.classList.toggle("dark", value);
      document.documentElement.classList.toggle("light", !value);
    },
    { immediate: true },
  );

  watch(
    themeStyle,
    (value) => {
      localStorage.setItem("theme-style", value);
      document.documentElement.setAttribute("data-theme-style", value);
    },
    { immediate: true },
  );

  function toggle() {
    isDark.value = !isDark.value;
  }

  function setThemeStyle(value: string) {
    if (
      value === "ocean" ||
      value === "graphite" ||
      value === "contrast" ||
      value === "emerald" ||
      value === "default"
    ) {
      themeStyle.value = value;
    }
  }

  return { isDark, toggle, themeStyle, setThemeStyle };
});
