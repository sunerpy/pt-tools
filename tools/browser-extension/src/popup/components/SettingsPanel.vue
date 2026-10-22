<script setup lang="ts">
/*
 * 设置页（弹窗右上角的「设置」）：pt-tools 的地址与登录、定时同步与 CookieCloud、检查扩展更新。
 * 批量同步、一键打开站点和登录状态挪到了主页面的「我的站点」里。
 */
import { reactive, watch } from "vue";

import { t } from "../../core/i18n";
import SyncSettings from "./SyncSettings.vue";
import type { PtToolsConnection } from "../../core/types";

const props = defineProps<{
  connection: PtToolsConnection;
  busy: boolean;
}>();

const emit = defineEmits<{
  test: [payload: { baseUrl: string; username?: string; password?: string }];
  save: [payload: { baseUrl: string }];
  checkUpdate: [];
}>();

const form = reactive({
  baseUrl: props.connection.baseUrl || "http://localhost:8080",
  username: "",
  password: "",
});

watch(
  () => props.connection.baseUrl,
  (value) => {
    if (value) {
      form.baseUrl = value;
    }
  },
);

function handleSave(): void {
  emit("save", { baseUrl: form.baseUrl.trim() });
  form.username = "";
  form.password = "";
}

function handleTest(): void {
  const payload: { baseUrl: string; username?: string; password?: string } = {
    baseUrl: form.baseUrl.trim(),
  };

  if (form.username.trim() && form.password.trim()) {
    payload.username = form.username.trim();
    payload.password = form.password.trim();
  }

  emit("test", payload);
}
</script>

<template>
  <div class="settings-panel">
    <section class="card">
      <h2 class="settings-head">{{ t("settings.connection") }}</h2>
      <label class="field">
        <span>{{ t("settings.ptToolsUrl") }}</span>
        <input v-model="form.baseUrl" type="url" :placeholder="t('settings.urlPlaceholder')" />
      </label>
      <label class="field">
        <span>{{ t("settings.username") }}</span>
        <input
          v-model="form.username"
          type="text"
          autocomplete="off"
          :placeholder="t('settings.usernamePlaceholder')" />
      </label>
      <label class="field">
        <span>{{ t("settings.password") }}</span>
        <input
          v-model="form.password"
          type="password"
          autocomplete="off"
          :placeholder="t('settings.passwordPlaceholder')" />
      </label>
      <div class="row-actions">
        <button type="button" class="btn" :disabled="busy" @click="handleSave">
          {{ busy ? t("settings.saving") : t("settings.save") }}
        </button>
        <button type="button" class="btn primary" :disabled="busy" @click="handleTest">
          {{ busy ? t("settings.testing") : t("settings.testConnection") }}
        </button>
      </div>
    </section>

    <SyncSettings />

    <div class="update-section">
      <button
        type="button"
        class="btn ghost"
        data-testid="check-update-btn"
        :disabled="busy"
        @click="emit('checkUpdate')">
        检查扩展更新
      </button>
    </div>
  </div>
</template>

<style scoped>
.settings-head {
  margin: 0 0 8px;
  font-size: 14px;
  color: var(--primary);
}
</style>
