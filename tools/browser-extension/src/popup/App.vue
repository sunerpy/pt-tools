<script setup lang="ts">
import { computed, onMounted, ref } from "vue";

import { initI18n, t, type MessageKey } from "../core/i18n";
import { createMessage, sendToBackground } from "../core/messages";
import { hasRequiredPermissions, requestCorePermissions } from "../core/permissions";
import type {
  AutoCollectPayload,
  BatchOpenTabsResult,
  BatchSyncResult,
  CollectionSession,
  ExtensionUpdateCheckResult,
  KnownSiteStatus,
  MessagePayloadMap,
  MessageType,
  PtToolsConnection,
  StatusPayload,
  TabSiteStatus,
  ToggleAutoSyncPayload,
} from "../core/types";
import { createGitHubIssue, createExportZip, downloadZip } from "../modules/export";
import { friendlyErrorMessage, type SiteLoginStateRecord } from "../modules/sync";
import { useToast } from "./composables/useToast";
import KnownSiteView from "./components/KnownSiteView.vue";
import MySites from "./components/MySites.vue";
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
/** 主页面是「当前页 + 我的站点」，设置单独一页（右上角的按钮） */
const view = ref<"main" | "settings">("main");
const permissionsGranted = ref(true);

/** 三样东西分开读、谁先回来先显示谁：以前等它们全回来才渲染，打开弹窗要好几秒 */
const sitesLoading = ref(true);
const connChecking = ref(true);
const logins = ref<SiteLoginStateRecord[]>([]);
const loginError = ref("");
const syncingId = ref("");

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
const connLabel = computed(() => {
  if (!connection.value.baseUrl?.trim()) return t("conn.notConfigured");
  if (connChecking.value) return t("conn.checking");
  return connection.value.connected ? t("conn.connected") : t("conn.disconnected");
});
const connTone = computed(() =>
  connChecking.value ? "muted" : connection.value.connected ? "ok" : "bad",
);
const connHost = computed(() => {
  try {
    return new URL(connection.value.baseUrl).host;
  } catch {
    return "";
  }
});

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

async function loadStatus(): Promise<void> {
  const statusPayload = await requestMessage<"GET_STATUS", StatusPayload>("GET_STATUS", {});
  activeSession.value = statusPayload.activeSession;
  connection.value = { ...statusPayload.connection, connected: connection.value.connected };
}

async function loadTab(): Promise<void> {
  tabStatus.value = await requestMessage<"GET_TAB_STATUS", TabSiteStatus>("GET_TAB_STATUS", {});
}

async function loadSites(): Promise<void> {
  try {
    allSites.value = await requestMessage<"GET_ALL_SITES_STATUS", KnownSiteStatus[]>(
      "GET_ALL_SITES_STATUS",
      {},
    );
  } finally {
    sitesLoading.value = false;
  }
}

/** 页头的连接状态：单独 ping 一次 pt-tools（最多 4 秒），连上以后再取登录状态 */
async function checkConnection(): Promise<void> {
  connChecking.value = true;
  try {
    const conn = await requestMessage<"CHECK_CONNECTION", PtToolsConnection>(
      "CHECK_CONNECTION",
      {},
    );
    connection.value = conn;
  } catch {
    connection.value = { ...connection.value, connected: false };
  } finally {
    connChecking.value = false;
  }
  if (connection.value.connected) {
    await loadLogins();
  } else if (connection.value.baseUrl?.trim()) {
    loginError.value = t("conn.disconnected");
  }
}

async function loadLogins(): Promise<void> {
  try {
    logins.value = await requestMessage<"GET_SITE_LOGIN_STATE", SiteLoginStateRecord[]>(
      "GET_SITE_LOGIN_STATE",
      {},
    );
    loginError.value = "";
  } catch (error: unknown) {
    loginError.value = error instanceof Error ? friendlyErrorMessage(error.message) : "";
  }
}

/** 同步、开关这类操作之后刷新：当前页和站点列表，连接状态不必每次重测 */
async function refreshState(): Promise<void> {
  await Promise.all([loadStatus(), loadTab(), loadSites()]);
}

function siteNameById(siteId: string): string {
  const site = allSites.value.find((s) => s.site.id === siteId);
  return site?.site.name ?? siteId;
}

async function handleSyncSite(siteId: string): Promise<void> {
  busySync.value = true;
  syncingId.value = siteId;
  try {
    const result = await requestMessage<"SYNC_SITE_COOKIES", { ok: true; warning?: string }>(
      "SYNC_SITE_COOKIES",
      { siteId },
    );
    await refreshState();
    if (result.warning) {
      toast.warning(t("feedback.syncedWithWarning", siteNameById(siteId), result.warning));
    } else {
      toast.success(t("feedback.syncSuccess", siteNameById(siteId)));
    }
    void loadLogins();
  } catch (error: unknown) {
    showError(error, "feedback.syncFailed");
  } finally {
    busySync.value = false;
    syncingId.value = "";
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
    void checkConnection();
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
    void checkConnection();
    if (result.connected) {
      toast.success(t("feedback.settingsSaved"));
      view.value = "main";
    } else {
      toast.warning(t("feedback.settingsSavedNotConnected", payload.baseUrl));
    }
  } catch (error: unknown) {
    showError(error, "feedback.settingsFailed");
  } finally {
    busySettings.value = false;
  }
}

async function handleBatchOpen(siteIds: string[]): Promise<void> {
  busySettings.value = true;
  try {
    const result = await requestMessage<"BATCH_OPEN_TABS", BatchOpenTabsResult>("BATCH_OPEN_TABS", {
      siteIds,
      timeoutMs: 150,
    });
    if (result.failed.length > 0) {
      toast.warning(`已打开 ${result.ok.length} 个站点，失败 ${result.failed.length} 个`);
    } else {
      toast.success(`已打开 ${result.ok.length} 个站点页面`);
    }
  } catch (error: unknown) {
    showError(error, "feedback.requestFailed");
  } finally {
    busySettings.value = false;
  }
}

async function handleCheckExtensionUpdate(): Promise<void> {
  busySettings.value = true;
  try {
    const result = await requestMessage<"CHECK_EXTENSION_UPDATE", ExtensionUpdateCheckResult>(
      "CHECK_EXTENSION_UPDATE",
      {},
    );
    if (result.status === "update_available") {
      toast.success(
        `发现扩展更新，当前版本 ${result.currentVersion}，请在浏览器扩展页重载以完成更新`,
      );
    } else if (result.status === "no_update") {
      toast.success(`扩展已是最新版本：${result.currentVersion}`);
    } else if (result.status === "throttled") {
      toast.warning("浏览器限制了更新检查频率，请稍后再试");
    } else {
      toast.warning("当前安装方式不支持自动更新检查，请打开 Releases 页面手动更新");
    }
    if (result.status === "unavailable") {
      void chrome.tabs.create({ url: result.releaseUrl, active: true });
    }
  } catch (error: unknown) {
    showError(error, "feedback.requestFailed");
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
    const warnings = result.warnings ?? [];
    if (warnings.length > 0) {
      toast.warning(
        t(
          "feedback.batchWarnings",
          warnings.length,
          warnings.map((w) => `${siteNameById(w.siteId)}：${w.warning}`).join("；"),
        ),
      );
    }
    void loadLogins();
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
    const result = await requestMessage<
      "AUTO_COLLECT",
      { collected: number; pages: string[]; warnings?: string[] }
    >("AUTO_COLLECT", payload);
    await refreshState();
    toast.success(t("feedback.autoCollected", result.collected, result.pages.join(", ")));
    const warnings = result.warnings ?? [];
    if (warnings.length > 0) {
      toast.warning(t("feedback.autoCollectWarnings", warnings.length, warnings.join(" / ")));
    }
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
    // 各读各的：连接状态、当前页、站点列表谁先回来先显示谁
    const fail = (error: unknown) => showError(error, "feedback.initFailed");
    void loadStatus().catch(fail);
    void loadTab().catch(fail);
    void loadSites().catch(fail);
    void checkConnection();
  } else {
    sitesLoading.value = false;
    connChecking.value = false;
  }

  chrome.runtime.onMessage.addListener((message: unknown) => {
    if (typeof message !== "object" || message === null) {
      return;
    }

    const raw = message as Record<string, unknown>;
    if (raw.type !== "STATUS_UPDATE") {
      return;
    }

    void loadStatus();
    void loadTab();
  });
});
</script>

<template>
  <main class="popup-root">
    <header class="app-header">
      <h1>{{ view === "settings" ? t("app.settingsTitle") : t("app.title") }}</h1>
      <span
        v-if="view === 'main'"
        class="conn-chip"
        :class="`tone-${connTone}`"
        :title="connection.baseUrl"
        data-testid="conn-chip">
        <i class="conn-dot" />{{ connLabel
        }}<template v-if="connHost && !connChecking"> · {{ connHost }}</template>
      </span>
      <button
        type="button"
        class="settings-trigger"
        data-testid="settings-trigger"
        @click="view = view === 'main' ? 'settings' : 'main'">
        {{ view === "main" ? t("app.settings") : t("app.back") }}
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

    <template v-else-if="view === 'settings'">
      <SettingsPanel
        :connection="connection"
        :busy="busySettings"
        @test="handleTestConnection"
        @save="handleSaveSettings"
        @check-update="handleCheckExtensionUpdate" />
    </template>

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

      <MySites
        :sites="allSites"
        :sites-loading="sitesLoading"
        :logins="logins"
        :login-error="loginError"
        :busy="busySync || busySettings"
        :syncing-id="syncingId"
        @sync="handleSyncSite"
        @sync-all="handleBatchSync"
        @open-all="handleBatchOpen" />

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
    </template>

    <ToastStack />
  </main>
</template>
