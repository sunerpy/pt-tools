<script setup lang="ts">
import { ElAlert, ElButton, ElIcon, ElLink } from "element-plus";
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
    <ElAlert type="info" :closable="false" show-icon class="v2-deprecation-alert">
      <template #title>
        <span class="v2-deprecation-title">pt-tools v2.0 升级完成</span>
      </template>
      <template #default>
        <div class="v2-deprecation-body">
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
              <ElIcon><Close /></ElIcon>
              <span style="margin-left: 4px">我知道了</span>
            </ElButton>
          </div>
        </div>
      </template>
    </ElAlert>
  </div>
</template>

<style scoped>
.v2-deprecation-banner {
  margin: 8px 16px 0;
}

.v2-deprecation-alert {
  border-radius: 8px;
}

.v2-deprecation-title {
  font-weight: 600;
  font-size: 14px;
}

.v2-deprecation-body {
  display: flex;
  flex-direction: column;
  gap: 8px;
  font-size: 13px;
  line-height: 1.5;
}

.v2-deprecation-actions {
  display: flex;
  gap: 12px;
  align-items: center;
}
</style>
