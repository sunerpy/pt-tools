import { ElMessage } from "element-plus";
import { defineStore } from "pinia";
import { ref } from "vue";
import { logLevelApi } from "../api";

export const useLogLevelStore = defineStore("logLevel", () => {
  // 当前日志级别
  const currentLevel = ref("info");
  // 可用的日志级别列表
  const availableLevels = ref<string[]>(["debug", "info", "warn", "error"]);
  // 加载状态
  const loading = ref(false);

  // 获取当前日志级别
  async function fetchLogLevel() {
    try {
      loading.value = true;
      const response = await logLevelApi.get();
      currentLevel.value = response.level;
      availableLevels.value = response.levels;
    } catch (error) {
      ElMessage.error("获取日志级别失败");
      console.error("Failed to fetch log level:", error);
    } finally {
      loading.value = false;
    }
  }

  // 设置日志级别
  async function setLogLevel(level: string) {
    try {
      loading.value = true;
      const response = await logLevelApi.set(level);
      currentLevel.value = response.level;
      ElMessage.success(`日志级别已更改为: ${level}`);
    } catch (error) {
      ElMessage.error("设置日志级别失败");
      console.error("Failed to set log level:", error);
    } finally {
      loading.value = false;
    }
  }

  return {
    currentLevel,
    availableLevels,
    loading,
    fetchLogLevel,
    setLogLevel,
  };
});
