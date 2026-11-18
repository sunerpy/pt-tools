<script setup lang="ts">
/*
 * 定时同步与 CookieCloud 的设置（M22）。设置都在 background 里存与校验，这里只发消息；
 * CookieCloud 的密码弹窗拿不到，只知道有没有设置，留空就不改。
 */
import { computed, onMounted, reactive, ref } from "vue";

import { runHadProblems } from "../../background/scheduled-sync";
import { t } from "../../core/i18n";
import { createMessage, sendToBackground } from "../../core/messages";
import type {
  MessagePayloadMap,
  MessageType,
  SyncRunSummary,
  SyncSettingsView,
} from "../../core/types";
import { useToast } from "../composables/useToast";

const MIN_INTERVAL = 10;
const MAX_INTERVAL = 1440;

interface UploadResponse {
  result?: { uploaded: boolean; domains: number; cookies: number };
  needsOverwrite?: boolean;
  message?: string;
  settings: SyncSettingsView;
}

interface TestResponse {
  status: "empty" | "ok";
  domains: number;
  updatedAt: string | null;
}

const toast = useToast();
const view = ref<SyncSettingsView | null>(null);
const busy = ref(false);
const overwritePrompt = ref(false);

const schedule = reactive({ enabled: false, intervalMinutes: 60, allSites: false });
const cc = reactive({
  enabled: false,
  serverUrl: "",
  uuid: "",
  password: "",
  cryptoType: "aes-128-cbc-fixed" as "aes-128-cbc-fixed" | "legacy",
});

async function request<K extends MessageType, T>(
  type: K,
  payload: MessagePayloadMap[K],
): Promise<T> {
  const response = (await sendToBackground(createMessage(type, payload))) as {
    ok?: boolean;
    data?: T;
    error?: string;
  } | null;
  if (!response?.ok || response.data === undefined) {
    throw new Error(response?.error ?? t("feedback.requestFailed"));
  }
  return response.data;
}

function apply(next: SyncSettingsView): void {
  view.value = next;
  schedule.enabled = next.schedule.enabled;
  schedule.intervalMinutes = next.schedule.intervalMinutes;
  schedule.allSites = next.schedule.allSites;
  cc.enabled = next.cookieCloud.enabled;
  cc.serverUrl = next.cookieCloud.serverUrl;
  cc.uuid = next.cookieCloud.uuid;
  cc.cryptoType = next.cookieCloud.cryptoType;
  cc.password = "";
}

function fail(error: unknown): void {
  toast.error(error instanceof Error ? error.message : t("feedback.requestFailed"));
}

async function run(fn: () => Promise<void>): Promise<void> {
  busy.value = true;
  try {
    await fn();
  } catch (error: unknown) {
    fail(error);
  } finally {
    busy.value = false;
  }
}

onMounted(() => {
  void run(async () => {
    apply(await request<"GET_SYNC_SETTINGS", SyncSettingsView>("GET_SYNC_SETTINGS", {}));
  });
});

function saveSchedule(): Promise<void> {
  return run(async () => {
    apply(
      await request<"SAVE_SYNC_SCHEDULE", SyncSettingsView>("SAVE_SYNC_SCHEDULE", {
        enabled: schedule.enabled,
        intervalMinutes: Number(schedule.intervalMinutes),
        allSites: schedule.allSites,
      }),
    );
    toast.success(t("schedule.saved"));
  });
}

function runNow(): Promise<void> {
  return run(async () => {
    const next = await request<"RUN_SYNC_NOW", SyncSettingsView>("RUN_SYNC_NOW", {});
    apply(next);
    if (runHadProblems(next.lastRun)) {
      toast.warning(t("schedule.ranWithProblems"));
    } else {
      toast.success(t("schedule.ran"));
    }
  });
}

/** requestOrigin 申请访问 CookieCloud 服务地址：要在点击的同一个调用里先调（浏览器要求用户手势），已经有权限时直接返回 true。 */
async function requestOrigin(server: string): Promise<boolean> {
  let origin: string;
  try {
    origin = new URL(server.trim()).origin;
  } catch {
    // 地址不对时由 background 写明原因
    return true;
  }
  if (!origin.startsWith("http")) {
    return true;
  }
  return chrome.permissions.request({ origins: [`${origin}/*`] });
}

function saveCookieCloud(): Promise<void> {
  // 先申请权限（同一个点击里），再保存
  const granted = cc.serverUrl.trim() ? requestOrigin(cc.serverUrl) : Promise.resolve(true);
  return run(async () => {
    if (!(await granted)) {
      throw new Error(t("cc.permissionDenied"));
    }
    apply(
      await request<"SAVE_COOKIECLOUD", SyncSettingsView>("SAVE_COOKIECLOUD", {
        enabled: cc.enabled,
        serverUrl: cc.serverUrl,
        uuid: cc.uuid,
        cryptoType: cc.cryptoType,
        ...(cc.password ? { password: cc.password } : {}),
      }),
    );
    toast.success(t("cc.saved"));
  });
}

function testCookieCloud(): Promise<void> {
  return run(async () => {
    const result = await request<"TEST_COOKIECLOUD", TestResponse>("TEST_COOKIECLOUD", {});
    if (result.status === "empty") {
      toast.success(t("cc.testEmpty"));
    } else {
      toast.success(t("cc.testOk", result.domains));
    }
  });
}

function upload(overwrite: boolean): Promise<void> {
  overwritePrompt.value = false;
  return run(async () => {
    const response = await request<"UPLOAD_COOKIECLOUD", UploadResponse>("UPLOAD_COOKIECLOUD", {
      overwrite,
    });
    apply(response.settings);
    if (response.needsOverwrite) {
      overwritePrompt.value = true;
      return;
    }
    if (response.result) {
      toast.success(t("cc.uploaded", response.result.domains, response.result.cookies));
    }
  });
}

function formatTime(iso: string | null): string {
  if (!iso) {
    return "";
  }
  const date = new Date(iso);
  return Number.isNaN(date.getTime()) ? iso : date.toLocaleString();
}

function runLines(run: SyncRunSummary): string[] {
  const lines: string[] = [];
  const pt = run.ptTools;
  const missing = pt.missing ?? [];
  lines.push(
    pt.skipped
      ? t("schedule.summaryPtSkipped", pt.skipped)
      : t("schedule.summaryPt", pt.synced.length, pt.unchanged, missing.length, pt.failed.length),
  );
  if (pt.error) {
    lines.push(pt.error);
  }
  for (const f of pt.failed) {
    lines.push(`· ${f.siteId}: ${f.error}`);
  }
  if (missing.length > 0) {
    lines.push(t("schedule.summaryPtMissing", missing.join(", ")));
  }
  const c = run.cookieCloud;
  if (c.error) {
    lines.push(t("schedule.summaryCcError", c.error));
  } else if (c.skipped) {
    lines.push(t("schedule.summaryCcSkipped"));
  } else if (c.uploaded) {
    lines.push(t("schedule.summaryCc", c.domains, c.cookies));
  } else {
    lines.push(t("schedule.summaryCcUnchanged"));
  }
  return lines;
}

const lastRunLines = computed((): string[] =>
  view.value?.lastRun ? runLines(view.value.lastRun) : [],
);
const intervalLabel = computed((): string => t("schedule.interval", MIN_INTERVAL, MAX_INTERVAL));
</script>

<template>
  <div class="sync-settings" data-testid="sync-settings">
    <section class="batch-sync-section">
      <div class="batch-header">
        <span class="batch-title">{{ t("schedule.title") }}</span>
      </div>
      <label class="check-row">
        <input v-model="schedule.enabled" type="checkbox" data-testid="schedule-enabled" />
        <span>{{ t("schedule.enable") }}</span>
      </label>
      <label class="field sync-field">
        <span>{{ intervalLabel }}</span>
        <input
          v-model.number="schedule.intervalMinutes"
          type="number"
          :min="MIN_INTERVAL"
          :max="MAX_INTERVAL"
          step="10"
          data-testid="schedule-interval" />
      </label>
      <label class="check-row">
        <input v-model="schedule.allSites" type="checkbox" data-testid="schedule-all-sites" />
        <span>{{ t("schedule.allSites") }}</span>
      </label>
      <p class="sync-hint">{{ t("schedule.allSitesHint") }}</p>
      <div class="row-actions">
        <button
          type="button"
          class="btn"
          :disabled="busy"
          data-testid="schedule-save"
          @click="saveSchedule">
          {{ t("schedule.save") }}
        </button>
        <button
          type="button"
          class="btn"
          :disabled="busy"
          data-testid="schedule-run"
          @click="runNow">
          {{ t("schedule.runNow") }}
        </button>
      </div>
      <div class="sync-status" data-testid="schedule-last-run">
        <template v-if="view?.lastRun">
          <div>{{ t("schedule.lastRun", formatTime(view.lastRun.at)) }}</div>
          <div v-for="(line, i) in lastRunLines" :key="i">{{ line }}</div>
        </template>
        <div v-else>{{ t("schedule.never") }}</div>
      </div>
    </section>

    <section class="batch-sync-section">
      <div class="batch-header">
        <span class="batch-title">{{ t("cc.title") }}</span>
      </div>
      <p class="sync-hint">{{ t("cc.desc") }}</p>
      <label class="check-row">
        <input v-model="cc.enabled" type="checkbox" data-testid="cc-enabled" />
        <span>{{ t("cc.enable") }}</span>
      </label>
      <label class="field sync-field">
        <span>{{ t("cc.server") }}</span>
        <input
          v-model="cc.serverUrl"
          type="url"
          autocomplete="off"
          placeholder="https://cookiecloud.example.com"
          data-testid="cc-server" />
      </label>
      <label class="field sync-field">
        <span>{{ t("cc.uuid") }}</span>
        <input v-model="cc.uuid" type="text" autocomplete="off" data-testid="cc-uuid" />
      </label>
      <label class="field sync-field">
        <span>{{ t("cc.password") }}</span>
        <input
          v-model="cc.password"
          type="password"
          autocomplete="new-password"
          :placeholder="
            view?.cookieCloud.hasPassword ? t('cc.passwordSet') : t('cc.passwordPlaceholder')
          "
          data-testid="cc-password" />
      </label>
      <label class="field sync-field">
        <span>{{ t("cc.crypto") }}</span>
        <select v-model="cc.cryptoType" class="sync-select" data-testid="cc-crypto">
          <option value="aes-128-cbc-fixed">{{ t("cc.cryptoFixed") }}</option>
          <option value="legacy">{{ t("cc.cryptoLegacy") }}</option>
        </select>
      </label>
      <div class="row-actions">
        <button
          type="button"
          class="btn"
          :disabled="busy"
          data-testid="cc-save"
          @click="saveCookieCloud">
          {{ t("cc.save") }}
        </button>
        <button
          type="button"
          class="btn"
          :disabled="busy"
          data-testid="cc-test"
          @click="testCookieCloud">
          {{ t("cc.test") }}
        </button>
      </div>
      <button
        type="button"
        class="btn primary"
        :disabled="busy"
        data-testid="cc-upload"
        @click="upload(false)">
        {{ t("cc.upload") }}
      </button>
      <div class="sync-status" data-testid="cc-status">
        <div v-if="view?.cookieCloud.lastUploadAt">
          {{
            t(
              "cc.lastUpload",
              formatTime(view.cookieCloud.lastUploadAt),
              view.cookieCloud.domains,
              view.cookieCloud.cookies,
            )
          }}
        </div>
        <div v-if="view?.cookieCloud.lastError" class="sync-error">
          {{
            t("cc.lastError", formatTime(view.cookieCloud.lastErrorAt), view.cookieCloud.lastError)
          }}
        </div>
      </div>
    </section>

    <div
      v-if="overwritePrompt"
      class="confirm-overlay"
      role="dialog"
      aria-modal="true"
      data-testid="cc-overwrite">
      <div class="confirm-dialog">
        <p class="confirm-title">{{ t("cc.overwriteTitle") }}</p>
        <p class="confirm-text">{{ t("cc.overwriteConfirm") }}</p>
        <div class="row-actions">
          <button type="button" class="btn" @click="overwritePrompt = false">
            {{ t("cc.cancel") }}
          </button>
          <button
            type="button"
            class="btn primary"
            data-testid="cc-overwrite-confirm"
            @click="upload(true)">
            {{ t("cc.overwrite") }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.sync-field {
  margin-top: 6px;
}

.sync-select {
  height: 32px;
  border: 1px solid var(--line);
  border-radius: 8px;
  padding: 0 6px;
  background: var(--primary-faint);
  color: var(--text);
  font-size: 12px;
}

.sync-hint {
  margin: 4px 0 6px;
  font-size: 11px;
  line-height: 1.5;
  color: var(--muted);
}

.sync-status {
  margin-top: 6px;
  font-size: 11px;
  line-height: 1.5;
  color: var(--muted);
  word-break: break-word;
}

.sync-error {
  color: var(--danger, #c0392b);
}

.confirm-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.4);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.confirm-dialog {
  background: var(--surface, #fff);
  color: var(--text, #1a1a1a);
  padding: 16px;
  border-radius: 8px;
  max-width: 320px;
  box-shadow: 0 6px 24px rgba(0, 0, 0, 0.2);
}

.confirm-title {
  margin: 0 0 6px;
  font-size: 13px;
  font-weight: 600;
}

.confirm-text {
  margin: 0 0 12px;
  font-size: 12px;
  line-height: 1.5;
}
</style>
