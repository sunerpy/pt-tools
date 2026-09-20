<script setup lang="ts">
import PtIcon from "@/components/PtIcon";
import { ElLink } from "element-plus";
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
    <!--
      一条流式的通告，不是「标题 + 段落 + 动作行」三层。
      画板的页面板上没有任何常驻横幅（通告在画板里是 58 高的 toast 或移动端 40 高的 alert），
      而这条按三层排会占掉 96px —— 每次打开首页先看 96px 的公告再看数据。
      文字一个字没删，只是让它顺着排、动作跟在句末，高度减半。
    -->
    <div class="pt-note v2-deprecation-alert">
      <PtIcon name="info" :size="14" class="pt-note__icon" />
      <span class="v2-deprecation-body">
        <strong class="pt-note__title">pt-tools v2.0 升级完成</strong>
        <span class="v2-deprecation-sep">·</span>
        <!--
          窄屏只留标题 + 「了解 v2 详情」：375 宽下这段正文要排五行、约 110px，
          占掉 812 高的 14%，而画板 30 的移动稿里首行行卡就在 y=140、板上根本没有常驻通告。
          正文没有删，它在 `了解 v2 详情` 指向的文档里，标题也仍然说清了这是什么。
        -->
        <span class="v2-deprecation-detail">
          v1 的「批量打开标签页同步」功能已移除，请使用浏览器扩展 popup 中的「一键打开站点」按钮；
          新功能与站点登录管理已迁移至
          <ElLink type="primary" href="/sites" :underline="false">站点与 RSS</ElLink>
          页面。
        </span>
        <ElLink
          class="v2-deprecation-more"
          type="primary"
          href="https://github.com/sunerpy/pt-tools#v20-部署说明"
          target="_blank"
          rel="noopener"
          :underline="false">
          了解 v2 详情
        </ElLink>
      </span>
      <button
        type="button"
        class="v2-deprecation-x"
        aria-label="我知道了"
        data-testid="v2-deprecation-dismiss"
        @click="dismiss">
        <PtIcon name="check" :size="14" />
      </button>
    </div>
  </div>
</template>

<style scoped>
.v2-deprecation-banner {
  margin-bottom: var(--pt-space-4);
}

/* 顺着排，不分层：标题、正文、链接在同一段里流动 */
.v2-deprecation-body {
  min-width: 0;
}

.v2-deprecation-sep {
  margin: 0 4px;
  color: var(--pt-t4);
}

.v2-deprecation-more {
  margin-left: 6px;
}

@media (max-width: 768px) {
  .v2-deprecation-detail {
    display: none;
  }
}

/* 关闭钮与其他通告条上的 × 同一套（dash__note-x）：24×24、hover 才显底 */
.v2-deprecation-x {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  padding: 0;
  color: var(--pt-t3);
  cursor: pointer;
  background: none;
  border: 0;
  border-radius: var(--pt-r-sm);
}

.v2-deprecation-x:hover {
  color: var(--pt-t1);
  background: var(--pt-hover);
}
</style>
