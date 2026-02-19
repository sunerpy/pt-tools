<script setup lang="ts">
import { computed } from "vue";

import { t } from "../../core/i18n";
import type { CollectionSession, SiteSchema, UnknownSiteStatus } from "../../core/types";

const props = defineProps<{
  status: UnknownSiteStatus;
  session: CollectionSession | null;
  busyCapture: boolean;
  busyAutoCollect: boolean;
}>();

const emit = defineEmits<{
  capture: [];
  autoCollect: [payload: { siteOrigin: string; schema: SiteSchema }];
}>();

const requiredTypes = ["search", "detail", "userinfo"] as const;

const typeLabels: Record<string, string> = {
  search: t("unknown.pageSearch"),
  detail: t("unknown.pageDetail"),
  userinfo: t("unknown.pageUserinfo"),
};

const typeHints: Record<string, string> = {
  search: t("unknown.hintSearch"),
  detail: t("unknown.hintDetail"),
  userinfo: t("unknown.hintUserinfo"),
};

const completedTypes = computed(() => {
  const set = new Set<string>();
  for (const page of props.session?.pages ?? []) {
    set.add(page.pageType);
  }
  return set;
});

const progress = computed(() => {
  const done = requiredTypes.filter((type) => completedTypes.value.has(type)).length;
  return `${done}/${requiredTypes.length}`;
});

const currentTypeMatch = computed(() => {
  const pt = props.status.pageType;
  if (pt === "search" || pt === "detail" || pt === "userinfo") return pt;
  return null;
});

const isCurrentAlreadyCaptured = computed(() => {
  if (!currentTypeMatch.value) return false;
  return completedTypes.value.has(currentTypeMatch.value);
});

const captureButtonText = computed(() => {
  if (props.busyCapture) return t("unknown.capturing");
  if (!currentTypeMatch.value) return t("unknown.captureUnknown");
  const label = typeLabels[currentTypeMatch.value] ?? currentTypeMatch.value;
  if (isCurrentAlreadyCaptured.value) return t("unknown.recaptureBtn", label);
  return t("unknown.captureBtn", label);
});

const siteOrigin = computed(() => {
  try {
    return new URL(props.status.url).origin;
  } catch {
    return "";
  }
});

const supportsAutoCollect = computed(() =>
  ["NexusPHP", "Unit3D", "Gazelle", "HDDolby", "Rousi"].includes(props.status.detectedSchema),
);

function handleAutoCollect(): void {
  if (!siteOrigin.value) return;
  emit("autoCollect", { siteOrigin: siteOrigin.value, schema: props.status.detectedSchema });
}
</script>

<template>
  <article class="card unknown-card">
    <header class="card-header">
      <h3>{{ t("unknown.title") }}</h3>
    </header>

    <p class="line-item">
      <span>{{ t("unknown.detectedSchema") }}</span
      ><strong>{{ status.detectedSchema }}</strong>
    </p>
    <p class="line-item">
      <span>{{ t("unknown.progress") }}</span
      ><strong>{{ progress }}</strong>
    </p>

    <button
      v-if="supportsAutoCollect"
      type="button"
      class="btn primary"
      :disabled="busyAutoCollect"
      @click="handleAutoCollect">
      {{ busyAutoCollect ? t("unknown.autoCollecting") : t("unknown.autoCollect") }}
    </button>

    <details class="manual-section">
      <summary class="manual-toggle">{{ t("unknown.manualCapture") }}</summary>

      <div class="step-list">
        <div
          v-for="type in requiredTypes"
          :key="type"
          class="step-item"
          :class="{
            done: completedTypes.has(type),
            active: currentTypeMatch === type && !completedTypes.has(type),
          }">
          <span class="step-icon">{{ completedTypes.has(type) ? "✅" : "⬜" }}</span>
          <span class="step-label">{{ typeLabels[type] }}</span>
          <span v-if="!completedTypes.has(type)" class="step-hint">{{ typeHints[type] }}</span>
        </div>
      </div>

      <button type="button" class="btn" :disabled="busyCapture" @click="emit('capture')">
        {{ captureButtonText }}
      </button>
    </details>
  </article>
</template>
