import { defineStore } from "pinia";
import { ref, computed } from "vue";
import {
  type MediaLibraryConfig,
  type ProviderCredential,
  type ConnectorConfig,
  type ScrapeTask,
  type ScrapeSearchResult,
  type LLMProvider,
  type LLMGenerateRequest,
  type LLMGenerateResult,
  scraperApi,
} from "../api";

export const useScraperStore = defineStore("scraper", () => {
  // State
  const libraries = ref<MediaLibraryConfig[]>([]);
  const currentLibrary = ref<MediaLibraryConfig | null>(null);
  const tasks = ref<ScrapeTask[]>([]);
  const providers = ref<ProviderCredential[]>([]);
  const connectors = ref<ConnectorConfig[]>([]);
  const settings = ref<Record<string, unknown>>({});
  const llmProviders = ref<LLMProvider[]>([]);

  const loading = ref(false);
  const tasksLoading = ref(false);
  const error = ref<string | null>(null);

  let taskPollTimer: number | null = null;

  // Computed
  const runningTasks = computed(() => tasks.value.filter((t) => t.state === "running"));

  // Actions
  async function fetchLibraries() {
    try {
      loading.value = true;
      error.value = null;
      libraries.value = await scraperApi.listLibraries();
    } catch (err) {
      error.value = err instanceof Error ? err.message : "Failed to fetch libraries";
      libraries.value = [];
    } finally {
      loading.value = false;
    }
  }

  async function createLibrary(req: Omit<MediaLibraryConfig, "id" | "created_at" | "updated_at">) {
    try {
      loading.value = true;
      error.value = null;
      const lib = await scraperApi.createLibrary(req);
      libraries.value.push(lib);
      return lib;
    } catch (err) {
      error.value = err instanceof Error ? err.message : "Failed to create library";
      throw err;
    } finally {
      loading.value = false;
    }
  }

  async function updateLibrary(id: number, req: Partial<MediaLibraryConfig>) {
    try {
      loading.value = true;
      error.value = null;
      const updated = await scraperApi.updateLibrary(id, req);
      const idx = libraries.value.findIndex((l) => l.id === id);
      if (idx >= 0) {
        libraries.value[idx] = updated;
      }
      if (currentLibrary.value?.id === id) {
        currentLibrary.value = updated;
      }
      return updated;
    } catch (err) {
      error.value = err instanceof Error ? err.message : "Failed to update library";
      throw err;
    } finally {
      loading.value = false;
    }
  }

  async function deleteLibrary(id: number) {
    try {
      loading.value = true;
      error.value = null;
      await scraperApi.deleteLibrary(id);
      libraries.value = libraries.value.filter((l) => l.id !== id);
      if (currentLibrary.value?.id === id) {
        currentLibrary.value = null;
      }
    } catch (err) {
      error.value = err instanceof Error ? err.message : "Failed to delete library";
      throw err;
    } finally {
      loading.value = false;
    }
  }

  async function triggerScrape(req: {
    library_id: number;
    media_key: string;
    media_type: string;
    source_ids?: string[];
  }) {
    try {
      error.value = null;
      const task = await scraperApi.triggerScrape(req);
      tasks.value.push(task);
      startTaskPolling();
      return task;
    } catch (err) {
      error.value = err instanceof Error ? err.message : "Failed to trigger scrape";
      throw err;
    }
  }

  async function fetchTasks(params?: URLSearchParams) {
    try {
      tasksLoading.value = true;
      error.value = null;
      const response = await scraperApi.listTasks(params);
      tasks.value = response.items;
      if (runningTasks.value.length > 0) {
        startTaskPolling();
      }
    } catch (err) {
      error.value = err instanceof Error ? err.message : "Failed to fetch tasks";
      tasks.value = [];
    } finally {
      tasksLoading.value = false;
    }
  }

  async function cancelTask(id: number) {
    try {
      error.value = null;
      await scraperApi.cancelTask(id);
      const task = tasks.value.find((t) => t.id === id);
      if (task) {
        task.state = "canceled";
      }
    } catch (err) {
      error.value = err instanceof Error ? err.message : "Failed to cancel task";
      throw err;
    }
  }

  async function fetchProviders() {
    try {
      error.value = null;
      providers.value = await scraperApi.listProviders();
    } catch (err) {
      error.value = err instanceof Error ? err.message : "Failed to fetch providers";
      providers.value = [];
    }
  }

  async function setProviderCredential(name: string, cred: Record<string, string | number>) {
    try {
      error.value = null;
      const credential = await scraperApi.setProviderCredential(name, cred);
      const idx = providers.value.findIndex((p) => p.provider_name === name);
      if (idx >= 0) {
        providers.value[idx] = credential;
      } else {
        providers.value.push(credential);
      }
      return credential;
    } catch (err) {
      error.value = err instanceof Error ? err.message : "Failed to set provider credential";
      throw err;
    }
  }

  async function fetchConnectors() {
    try {
      error.value = null;
      connectors.value = await scraperApi.listConnectors();
    } catch (err) {
      error.value = err instanceof Error ? err.message : "Failed to fetch connectors";
      connectors.value = [];
    }
  }

  async function testConnector(id: number) {
    try {
      error.value = null;
      const result = await scraperApi.testConnector(id);
      return result;
    } catch (err) {
      error.value = err instanceof Error ? err.message : "Failed to test connector";
      throw err;
    }
  }

  async function fetchSettings() {
    try {
      error.value = null;
      settings.value = await scraperApi.getSettings();
    } catch (err) {
      error.value = err instanceof Error ? err.message : "Failed to fetch settings";
      settings.value = {};
    }
  }

  async function saveSettings(req: Record<string, unknown>) {
    try {
      loading.value = true;
      error.value = null;
      await scraperApi.updateSettings(req);
      settings.value = req;
    } catch (err) {
      error.value = err instanceof Error ? err.message : "Failed to save settings";
      throw err;
    } finally {
      loading.value = false;
    }
  }

  async function fetchLLMProviders() {
    try {
      error.value = null;
      llmProviders.value = await scraperApi.listLLMProviders();
    } catch (err) {
      error.value = err instanceof Error ? err.message : "Failed to fetch LLM providers";
      llmProviders.value = [];
    }
  }

  async function llmGenerate(req: LLMGenerateRequest) {
    try {
      error.value = null;
      const result = await scraperApi.llmGenerate(req);
      return result;
    } catch (err) {
      error.value = err instanceof Error ? err.message : "LLM generation failed";
      throw err;
    }
  }

  async function llmValidate(req: { provider_id: number; api_key: string }) {
    try {
      error.value = null;
      const result = await scraperApi.llmValidate(req);
      return result;
    } catch (err) {
      error.value = err instanceof Error ? err.message : "LLM validation failed";
      throw err;
    }
  }

  // Task polling (2s interval)
  function startTaskPolling() {
    if (taskPollTimer) {
      return; // Already polling
    }

    taskPollTimer = window.setInterval(async () => {
      if (runningTasks.value.length === 0) {
        stopTaskPolling();
        return;
      }

      try {
        for (const task of runningTasks.value) {
          const updated = await scraperApi.getTask(task.id);
          const idx = tasks.value.findIndex((t) => t.id === task.id);
          if (idx >= 0) {
            tasks.value[idx] = updated;
          }
        }
      } catch (err) {
        console.error("Failed to poll task status:", err);
      }
    }, 2000);
  }

  function stopTaskPolling() {
    if (taskPollTimer) {
      clearInterval(taskPollTimer);
      taskPollTimer = null;
    }
  }

  return {
    // State
    libraries,
    currentLibrary,
    tasks,
    providers,
    connectors,
    settings,
    llmProviders,
    loading,
    tasksLoading,
    error,

    // Computed
    runningTasks,

    // Actions
    fetchLibraries,
    createLibrary,
    updateLibrary,
    deleteLibrary,
    triggerScrape,
    fetchTasks,
    cancelTask,
    fetchProviders,
    setProviderCredential,
    fetchConnectors,
    testConnector,
    fetchSettings,
    saveSettings,
    fetchLLMProviders,
    llmGenerate,
    llmValidate,
    startTaskPolling,
    stopTaskPolling,
  };
});
