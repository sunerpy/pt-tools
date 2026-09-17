<script setup lang="ts">
import PtIcon from "@/components/PtIcon";
import { ElButton, ElLink } from "element-plus";
import { onMounted, ref } from "vue";

const STORAGE_KEY = "pt_tools_v2_banner_dismissed_v1";
const visible = ref(false);

onMounted(() => {
  try {
    const dismissed = window.localStorage.getItem(STORAGE_KEY);
    if (dismissed !== "1") {
      visible.value = true;
    }
  } catch {
    visible.value = true;
  }
});

function dismiss() {
  visible.value = false;
  try {
    window.localStorage.setItem(STORAGE_KEY, "1");
  } catch {
    // localStorage unavailable (private mode); banner re-shows next visit, accepted trade-off.
  }
}
</script>

<template>
  <div
    v-if="visible"
    class="v2-deprecation-banner"
    data-testid="v2-deprecation-banner"
    role="status"
    aria-live="polite">
    <div class="pt-note v2-deprecation-alert">
      <PtIcon name="info" :size="14" class="pt-note__icon" />
      <div class="v2-deprecation-body">
        <strong class="v2-deprecation-title">pt-tools v2.0 升级完成</strong>
        <p>
          v1 的「批量打开标签页同步」功能已移除。请使用浏览器扩展 popup 中的「一键打开站点」按钮。
          新功能与站点登录管理已迁移至
          <ElLink type="primary" href="/sites" :underline="false">站点与 RSS</ElLink>
          页面。
        </p>
        <div class="v2-deprecation-actions">
          <ElLink
            type="primary"
            href="https://github.com/sunerpy/pt-tools#v20-部署说明"
            target="_blank"
            rel="noopener"
            :underline="false">
            了解 v2 详情
          </ElLink>
          <ElButton
            size="small"
            type="primary"
            plain
            data-testid="v2-deprecation-dismiss"
            @click="dismiss">
            <PtIcon name="check" :size="13" />
            <span style="margin-left: 4px">我知道了</span>
          </ElButton>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.v2-deprecation-banner {
  margin-bottom: var(--pt-space-4);
}

.v2-deprecation-body {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  min-width: 0;
}

.v2-deprecation-title {
  font-size: var(--pt-fz-sm);
  font-weight: 600;
  color: var(--pt-t1);
}

.v2-deprecation-actions {
  display: flex;
  gap: var(--pt-space-3);
  align-items: center;
}
</style>
