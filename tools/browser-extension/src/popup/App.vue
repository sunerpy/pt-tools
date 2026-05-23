<script setup lang="ts">
import { computed, onMounted, ref } from "vue";

import { initI18n, t, type MessageKey } from "../core/i18n";
import { createMessage, sendToBackground } from "../core/messages";
import { hasRequiredPermissions, requestCorePermissions } from "../core/permissions";
import type {
  AutoCollectPayload,
  BatchSyncResult,
  CollectionSession,
  KnownSiteStatus,
  MessagePayloadMap,
  MessageType,
  PtToolsConnection,
  StatusPayload,
  TabSiteStatus,
  ToggleAutoSyncPayload,
} from "../core/types";
import { createGitHubIssue, createExportZip, downloadZip } from "../modules/export";
import { friendlyErrorMessage } from "../modules/sync";
import { useToast } from "./composables/useToast";
import CookieStatus from "./components/CookieStatus.vue";
import DefaultView from "./components/DefaultView.vue";
import KnownSiteView from "./components/KnownSiteView.vue";
import SettingsPanel from "./components/SettingsPanel.vue";
import ToastStack from "./components/ToastStack.vue";
import UnknownSiteView from "./components/UnknownSiteView.vue";

interface MessageResponse<T> {
  ok: boolean;
  data?: T;
  error?: string;
}

const toast = useToast();

function asMessageResponse<T>(value: unknown): MessageResponse<T> {
  if (typeof value !== "object" || value === null) {
    return { ok: false, error: t("error.invalidResponse") };
  }

  const raw = value as Record<string, unknown>;
  return {
    ok: raw.ok === true,
    data: raw.data as T | undefined,
    error: typeof raw.error === "string" ? raw.error : undefined,
  };
}

function showError(error: unknown, fallbackKey: MessageKey): void {
  const raw = error instanceof Error ? error.message : "";
  const message = raw ? friendlyErrorMessage(raw) : t(fallbackKey);
  toast.error(message);
}

const tabStatus = ref<TabSiteStatus>({ mode: "none" });
const allSites = ref<KnownSiteStatus[]>([]);
const activeSession = ref<CollectionSession | null>(null);
const connection = ref<PtToolsConnection>({
  baseUrl: "http://localhost:8080",
  sessionId: "",
  connected: false,
  lastSync: null,
});
const isSettingsOpen = ref(false);
const permissionsGranted = ref(true);

const busySync = ref(false);
const busyCapture = ref(false);
const busyExport = ref(false);
const busySettings = ref(false);
const busyAutoCollect = ref(false);

async function handleGrantPermissions(): Promise<void> {
  const granted = await requestCorePermissions();
  permissionsGranted.value = granted;
  if (granted) {
    toast.success(t("feedback.permissionGranted"));
    await refreshState();
  }
}

const knownStatus = computed(() =>
  tabStatus.value.mode === "known" ? (tabStatus.value.known ?? null) : null,
);
const unknownStatus = computed(() =>
  tabStatus.value.mode === "unknown" ? (tabStatus.value.unknown ?? null) : null,
);
const showSettingsExpanded = computed(
  () => isSettingsOpen.value || (!knownStatus.value && !unknownStatus.value),
);

async function requestMessage<K extends MessageType, T>(
  type: K,
  payload: MessagePayloadMap[K],
): Promise<T> {
  const response = await sendToBackground(createMessage(type, payload));
  const parsed = asMessageResponse<T>(response);
  if (!parsed.ok || parsed.data === undefined) {
    throw new Error(parsed.error ?? t("feedback.requestFailed"));
  }
  return parsed.data;
}

async function refreshState(): Promise<void> {
  const [nextTabStatus, statusPayload, sites] = await Promise.all([
    requestMessage<"GET_TAB_STATUS", TabSiteStatus>("GET_TAB_STATUS", {}),
    requestMessage<"GET_STATUS", StatusPayload>("GET_STATUS", {}),
    requestMessage<"GET_ALL_SITES_STATUS", KnownSiteStatus[]>("GET_ALL_SITES_STATUS", {}),
  ]);

  tabStatus.value = nextTabStatus;
  activeSession.value = statusPayload.activeSession;
  connection.value = statusPayload.connection;
  allSites.value = sites;
}

function siteNameById(siteId: string): string {
  const site = allSites.value.find((s) => s.site.id === siteId);
  return site?.site.name ?? siteId;
}

async function handleSyncSite(siteId: string): Promise<void> {
  busySync.value = true;
  try {
    await requestMessage<"SYNC_SITE_COOKIES", { ok: true }>("SYNC_SITE_COOKIES", { siteId });
    await refreshState();
    toast.success(t("feedback.syncSuccess", siteNameById(siteId)));
  } catch (error: unknown) {
    showError(error, "feedback.syncFailed");
  } finally {
    busySync.value = false;
  }
}

async function handleToggleAutoSync(payload: ToggleAutoSyncPayload): Promise<void> {
  busySync.value = true;
  try {
    const result = await requestMessage<"TOGGLE_AUTO_SYNC", { enabled: boolean }>(
      "TOGGLE_AUTO_SYNC",
      payload,
    );
    await refreshState();
    const name = siteNameById(payload.siteId);
    toast.info(t(result.enabled ? "feedback.autoSyncEnabled" : "feedback.autoSyncDisabled", name));
  } catch (error: unknown) {
    showError(error, "feedback.autoSyncFailed");
  } finally {
    busySync.value = false;
  }
}

async function handleCapture(): Promise<void> {
  busyCapture.value = true;
  try {
    await requestMessage<"CAPTURE_PAGE", { dispatched: boolean }>("CAPTURE_PAGE", {});
    await refreshState();
    toast.success(t("feedback.captured"));
  } catch (error: unknown) {
    showError(error, "feedback.captureFailed");
  } finally {
    busyCapture.value = false;
  }
}

async function handleExportZip(): Promise<void> {
  if (!activeSession.value) {
    toast.warning(t("feedback.noSession"));
    return;
  }

  busyExport.value = true;
  try {
    const blob = await createExportZip(activeSession.value);
    downloadZip(blob, `${activeSession.value.site.name}-${activeSession.value.id}.zip`);
    toast.success(t("feedback.zipExported"));
  } catch (error: unknown) {
    showError(error, "feedback.exportFailed");
  } finally {
    busyExport.value = false;
  }
}

async function handleCreateIssue(): Promise<void> {
  if (!activeSession.value) {
    toast.warning(t("feedback.noIssueSession"));
    return;
  }

  busyExport.value = true;
  try {
    const blob = await createExportZip(activeSession.value);
    downloadZip(blob, `${activeSession.value.site.name}-${activeSession.value.id}.zip`);
    await createGitHubIssue(activeSession.value, blob);
    toast.success(t("feedback.issueCreated"));
  } catch (error: unknown) {
    showError(error, "feedback.issueFailed");
  } finally {
    busyExport.value = false;
  }
}

async function handleTestConnection(payload: {
  baseUrl: string;
  username?: string;
  password?: string;
}): Promise<void> {
  busySettings.value = true;
  try {
    const result = await requestMessage<"SYNC_COOKIES", PtToolsConnection>("SYNC_COOKIES", payload);
    await refreshState();
    if (result.connected) {
      toast.success(t("feedback.connectionSuccess"));
    } else {
      toast.warning(t("feedback.settingsSavedNotConnected", payload.baseUrl));
    }
  } catch (error: unknown) {
    showError(error, "feedback.connectionFailed");
  } finally {
    busySettings.value = false;
  }
}

async function handleSaveSettings(payload: { baseUrl: string }): Promise<void> {
  busySettings.value = true;
  try {
    const result = await requestMessage<"SYNC_COOKIES", PtToolsConnection>("SYNC_COOKIES", {
      baseUrl: payload.baseUrl,
    });
    await refreshState();
    if (result.connected) {
      toast.success(t("feedback.settingsSaved"));
    } else {
      toast.warning(t("feedback.settingsSavedNotConnected", payload.baseUrl));
    }
  } catch (error: unknown) {
    showError(error, "feedback.settingsFailed");
  } finally {
    busySettings.value = false;
  }
}

async function handleBatchSync(siteIds: string[]): Promise<void> {
  busySettings.value = true;
  try {
    const result = await requestMessage<"BATCH_SYNC_SITES", BatchSyncResult>("BATCH_SYNC_SITES", {
      siteIds,
    });
    await refreshState();
    const total = result.synced.length + result.failed.length;
    if (total === 0) {
      toast.warning(t("feedback.noSyncable"));
    } else if (result.failed.length === 0) {
      toast.success(t("feedback.batchAllSynced", result.synced.length));
    } else if (result.synced.length === 0) {
      toast.error(
        t(
          "feedback.batchAllFailed",
          result.failed.length,
          result.failed.map((f) => siteNameById(f.siteId)).join(", "),
        ),
      );
    } else {
      toast.warning(
        t(
          "feedback.batchPartial",
          result.synced.length,
          result.failed.length,
          result.failed.map((f) => siteNameById(f.siteId)).join(", "),
        ),
      );
    }
  } catch (error: unknown) {
    showError(error, "feedback.batchSyncFailed");
  } finally {
    busySettings.value = false;
  }
}

async function handleAutoCollect(payload: AutoCollectPayload): Promise<void> {
  busyAutoCollect.value = true;
  toast.info(t("feedback.autoCollecting"));
  try {
    const result = await requestMessage<"AUTO_COLLECT", { collected: number; pages: string[] }>(
      "AUTO_COLLECT",
      payload,
    );
    await refreshState();
    toast.success(t("feedback.autoCollected", result.collected, result.pages.join(", ")));
  } catch (error: unknown) {
    showError(error, "feedback.autoCollectFailed");
  } finally {
    busyAutoCollect.value = false;
  }
}

onMounted(async () => {
  initI18n();
  permissionsGranted.value = await hasRequiredPermissions();

  if (permissionsGranted.value) {
    void refreshState().catch((error: unknown) => {
      showError(error, "feedback.initFailed");
    });
  }

  chrome.runtime.onMessage.addListener((message: unknown) => {
    if (typeof message !== "object" || message === null) {
      return;
    }

    const raw = message as Record<string, unknown>;
    if (raw.type !== "STATUS_UPDATE") {
      return;
    }

    void refreshState();
  });
});
</script>

<template>
  <main class="popup-root">
    <header class="app-header">
      <h1>{{ t("app.title") }}</h1>
      <button type="button" class="settings-trigger" @click="isSettingsOpen = !isSettingsOpen">
        {{ t("app.settings") }}
      </button>
    </header>

    <section v-if="!permissionsGranted" class="card permission-card">
      <h3>{{ t("permission.title") }}</h3>
      <p class="muted-line">{{ t("permission.desc") }}</p>
      <ul class="permission-list">
        <li>{{ t("permission.cookie") }}</li>
        <li>{{ t("permission.tabs") }}</li>
      </ul>
      <button type="button" class="btn primary" @click="handleGrantPermissions">
        {{ t("permission.grant") }}
      </button>
    </section>

    <template v-else>
      <section v-if="knownStatus" class="view-shell">
        <KnownSiteView
          :status="knownStatus"
          :busy="busySync"
          :busy-capture="busyCapture"
          :busy-auto-collect="busyAutoCollect"
          @sync="handleSyncSite"
          @toggle-auto-sync="handleToggleAutoSync"
          @capture="handleCapture"
          @auto-collect="handleAutoCollect" />
      </section>

      <section v-else-if="unknownStatus" class="view-shell">
        <UnknownSiteView
          :status="unknownStatus"
          :session="activeSession"
          :busy-capture="busyCapture"
          :busy-auto-collect="busyAutoCollect"
          @capture="handleCapture"
          @auto-collect="handleAutoCollect" />
      </section>

      <section v-else class="view-shell">
        <DefaultView :sites="allSites" :connection="connection" />
      </section>

      <section
        v-if="activeSession && activeSession.pages.length > 0"
        class="global-section export-section">
        <h2>{{ t("collect.title") }}</h2>
        <p class="line-item">
          <span>{{ activeSession.site.name }} ({{ activeSession.site.schema }})</span>
          <strong>{{ t("collect.pages", activeSession.pages.length) }}</strong>
        </p>
        <div class="row-actions">
          <button type="button" class="btn" :disabled="busyExport" @click="handleExportZip">
            {{ busyExport ? t("collect.processing") : t("collect.exportZip") }}
          </button>
          <button type="button" class="btn ghost" :disabled="busyExport" @click="handleCreateIssue">
            {{ t("collect.createIssue") }}
          </button>
        </div>
      </section>

      <section class="global-section">
        <div class="section-header" @click="isSettingsOpen = !isSettingsOpen">
          <h2>{{ t("settings.title") }}</h2>
          <span class="toggle-icon">{{ showSettingsExpanded ? "▾" : "▸" }}</span>
        </div>
        <SettingsPanel
          v-show="showSettingsExpanded"
          :connection="connection"
          :busy="busySettings"
          :sites="allSites"
          @test="handleTestConnection"
          @save="handleSaveSettings"
          @batch-sync="handleBatchSync" />
        <div v-if="knownStatus && !showSettingsExpanded" class="quick-cookie-status">
          <span class="quick-cookie-label">{{ t("known.cookieStatus") }}</span>
          <CookieStatus
            :status="knownStatus.cookieStatus"
            :days="knownStatus.cookieExpireDays"
            compact />
        </div>
      </section>
    </template>

    <ToastStack />
  </main>
</template>
