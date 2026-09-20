<script setup lang="ts">
import PtIcon from "@/components/PtIcon";
import { ElLink } from "element-plus";
import { onMounted, ref } from "vue";

import { useIsMobile } from "@/composables/useIsMobile";

const STORAGE_KEY = "pt_tools_v2_banner_dismissed_v1";
const visible = ref(false);

/*
 * 窄屏默认收起正文，点「详情」就地展开 —— 不是隐藏。
 *
 * 一次评审指出：正文在 ≤768 被 `display: none` 藏掉之后，剩下那个链接指向 README 的
 * `#v20-部署说明`，而 README 里**没有这一节**，移动用户于是什么都读不到。
 * 「压缩高度」不能变成「拿掉信息」，所以改成可展开。
 */
const isMobile = useIsMobile();
const detailOpen = ref(false);

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
          窄屏默认收起这段正文（375 宽下它要排五行、约 110px，占掉 812 高的 14%，
          而画板 30 的移动稿里首张行卡就在 y=140、板上根本没有常驻通告），
          但**收起不是拿掉** —— 点后面那个「详情」就地展开。
        -->
        <span v-if="!isMobile || detailOpen" class="v2-deprecation-detail">
          v1 的「批量打开标签页同步」功能已移除，请使用浏览器扩展 popup 中的「一键打开站点」按钮；
          新功能与站点登录管理已迁移至
          <ElLink type="primary" href="/sites" :underline="false">站点与 RSS</ElLink>
          页面。
        </span>
        <button
          v-if="isMobile && !detailOpen"
          type="button"
          class="v2-deprecation-toggle"
          @click="detailOpen = true">
          详情
        </button>
        <!--
          锚点改成 README 里确实存在的「快速开始」：原来那个 `#v20-部署说明`
          在 README 的标题集合里不存在，点过去落在页首。
        -->
        <ElLink
          v-else
          class="v2-deprecation-more"
          type="primary"
          href="https://github.com/sunerpy/pt-tools#快速开始"
          target="_blank"
          rel="noopener">
          部署说明
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

/* 正文段：窄屏由 v-if 控制收起/展开，样式上和句子其余部分一致（顺着排） */
.v2-deprecation-detail {
  color: inherit;
}

/* 窄屏的「详情」是按钮不是链接：它就地展开正文，不跳出去 */
.v2-deprecation-toggle {
  padding: 0;
  margin-left: 6px;
  font-size: inherit;
  color: var(--pt-p);
  text-decoration: underline;
  cursor: pointer;
  background: none;
  border: 0;
  border-radius: var(--pt-r-sm);
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
