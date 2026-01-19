import { defineStore } from "pinia";
import { ref, watch } from "vue";

export const useThemeStore = defineStore("theme", () => {
  // 默认为黑暗模式：只有明确设置为 'light' 时才使用亮色模式
  const storedTheme = localStorage.getItem("theme");
  const isDark = ref(storedTheme !== "light");

  watch(
    isDark,
    (value) => {
      localStorage.setItem("theme", value ? "dark" : "light");
      document.documentElement.classList.toggle("dark", value);
      document.documentElement.classList.toggle("light", !value);
    },
    { immediate: true },
  );

  function toggle() {
    isDark.value = !isDark.value;
  }

  return { isDark, toggle };
});
