import { defineStore } from "pinia";
import { ref, computed } from "vue";
import {
  type MediaLibraryConfig,
  type ProviderCredential,
  type ConnectorConfig,
  type ScrapeTask,
  type LLMProvider,
  type LLMGenerateRequest,
  type NFOResult,
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
  const pendingTasks = computed(() =>
    tasks.value.filter((t) => t.state === "pending" || t.state === "retrying"),
  );
  const failedTasks = computed(() => tasks.value.filter((t) => t.state === "failed"));

  // Actions
  async function fetchLibraries() {
    try {
      loading.value = true;
      error.value = null;
      const res = await scraperApi.listLibraries();
      libraries.value = Array.isArray(res) ? res : [];
    } catch (err) {
      error.value = err instanceof Error ? err.message : "Failed to fetch libraries";
      libraries.value = [];
    } finally {
      loading.value = false;
    }
  }

  async function fetchLibrary(id: number) {
    const lib = await scraperApi.getLibrary(id);
    currentLibrary.value = lib;
    return lib;
  }

  async function createLibrary(req: Partial<MediaLibraryConfig>) {
    const lib = await scraperApi.createLibrary(
      req as Omit<MediaLibraryConfig, "id" | "created_at" | "updated_at">,
    );
    libraries.value.push(lib);
    return lib;
  }

  async function updateLibrary(id: number, req: Partial<MediaLibraryConfig>) {
    const updated = await scraperApi.updateLibrary(id, req);
    const idx = libraries.value.findIndex((l) => l.id === id);
    if (idx >= 0) libraries.value[idx] = updated;
    if (currentLibrary.value?.id === id) currentLibrary.value = updated;
    return updated;
  }

  async function deleteLibrary(id: number) {
    await scraperApi.deleteLibrary(id);
    libraries.value = libraries.value.filter((l) => l.id !== id);
    if (currentLibrary.value?.id === id) currentLibrary.value = null;
  }

  async function triggerScrape(req: {
    library_id?: number;
    media_path: string;
    type: "movie" | "tv" | "episode";
  }) {
    const task = await scraperApi.triggerScrape(req);
    tasks.value.unshift(task);
    startTaskPolling();
    return task;
  }

  async function fetchTasks(filter?: { state?: string; library_id?: number; limit?: number }) {
    try {
      tasksLoading.value = true;
      error.value = null;
      const res = await scraperApi.listTasks(filter);
      tasks.value = Array.isArray(res) ? res : [];
      if (runningTasks.value.length > 0) startTaskPolling();
    } catch (err) {
      error.value = err instanceof Error ? err.message : "Failed to fetch tasks";
      tasks.value = [];
    } finally {
      tasksLoading.value = false;
    }
  }

  async function cancelTask(id: number) {
    await scraperApi.cancelTask(id);
    tasks.value = tasks.value.filter((t) => t.id !== id);
  }

  async function fetchProviders() {
    try {
      const res = await scraperApi.listProviders();
      providers.value = Array.isArray(res) ? res : [];
    } catch {
      providers.value = [];
    }
  }

  async function setProviderCredential(name: string, cred: Record<string, string | number>) {
    const credential = await scraperApi.setProviderCredential(name, cred);
    const idx = providers.value.findIndex((p) => p.provider === name);
    if (idx >= 0) providers.value[idx] = credential;
    else providers.value.push(credential);
    return credential;
  }

  async function fetchConnectors() {
    try {
      const res = await scraperApi.listConnectors();
      connectors.value = Array.isArray(res) ? res : [];
    } catch {
      connectors.value = [];
    }
  }

  async function testConnector(id: number) {
    return scraperApi.testConnector(id);
  }

  async function fetchSettings() {
    try {
      settings.value = await scraperApi.getSettings();
    } catch {
      settings.value = {};
    }
  }

  async function saveSettings(req: Record<string, unknown>) {
    await scraperApi.updateSettings(req);
    settings.value = req;
  }

  async function fetchLLMProviders() {
    try {
      const res = await scraperApi.listLLMProviders();
      llmProviders.value = Array.isArray(res) ? res : [];
    } catch {
      llmProviders.value = [];
    }
  }

  async function llmGenerate(req: LLMGenerateRequest): Promise<NFOResult> {
    return scraperApi.llmGenerate(req);
  }

  async function llmValidate(req: { provider_id: number; api_key: string }) {
    return scraperApi.llmValidate(req);
  }

  function startTaskPolling(intervalMs = 2000) {
    if (taskPollTimer) return;
    taskPollTimer = window.setInterval(async () => {
      if (runningTasks.value.length === 0 && pendingTasks.value.length === 0) {
        stopTaskPolling();
        return;
      }
      try {
        const active = [...runningTasks.value, ...pendingTasks.value];
        for (const task of active) {
          const updated = await scraperApi.getTask(task.id);
          const idx = tasks.value.findIndex((t) => t.id === task.id);
          if (idx >= 0) tasks.value[idx] = updated;
        }
      } catch (err) {
        console.error("Failed to poll task status:", err);
      }
    }, intervalMs);
  }

  function stopTaskPolling() {
    if (taskPollTimer) {
      clearInterval(taskPollTimer);
      taskPollTimer = null;
    }
  }

  return {
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
    runningTasks,
    pendingTasks,
    failedTasks,
    fetchLibraries,
    fetchLibrary,
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
