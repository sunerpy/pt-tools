<script setup lang="ts">
import { computed } from "vue";

import { t } from "../../core/i18n";

const props = withDefaults(
  defineProps<{
    status: "valid" | "expiring" | "expired" | "unverified" | "missing";
    days: number | null;
    compact?: boolean;
  }>(),
  {
    compact: false,
  },
);

const text = computed(() => {
  if (props.status === "valid") {
    if (props.days === null) {
      return t("cookie.valid");
    }
    return t("cookie.validDays", props.days);
  }

  if (props.status === "expiring") {
    return props.days === null ? t("cookie.expiring") : t("cookie.expiringDays", props.days);
  }

  if (props.status === "expired") {
    return t("cookie.expired");
  }

  if (props.status === "unverified") {
    return t("cookie.unverified");
  }

  return t("cookie.missing");
});

const tone = computed(() => {
  if (props.status === "valid") {
    return "ok";
  }
  if (props.status === "expiring") {
    return "warn";
  }
  if (props.status === "unverified") {
    return "info";
  }
  return "bad";
});
</script>

<template>
  <span
    class="cookie-status"
    :class="[tone, { compact: props.compact }]"
    :title="props.status === 'unverified' ? t('cookie.unverifiedHint') : undefined"
    >{{ text }}</span
  >
</template>
