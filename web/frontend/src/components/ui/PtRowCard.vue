<script setup lang="ts">
/**
 * 行卡 —— 移动端替代桌面表格的那一行（设计文档 §9）。
 *
 * 稿上的结构是「两行文本 + 一条进度 + 一个主操作」，不做横向滚动表格：
 * 手机上横向滚的表格既看不到列头，又和页面本身的纵向滚动打架。
 *
 *   ┌─────────────────────────────────────────┐
 *   │ [lead] 标题………………………………  [status] │
 *   │        第二行 meta（次要信息）           │
 *   │        ▓▓▓▓▓▓▓░░░░░  progress（可选）   │
 *   │ ────────────────────────────────────    │
 *   │ [actions]                      （可选） │
 *   └─────────────────────────────────────────┘
 *
 * 做成组件而不是各页抄一份 CSS：十几个列表页都要用，卡片留白、圆角、分隔线
 * 抄十几遍必然漂移。页面只负责往插槽里填内容和语义色。
 */
withDefaults(
  defineProps<{
    /** 整卡可点时传 true：加 hover 反馈并允许键盘聚焦 */
    interactive?: boolean;
  }>(),
  { interactive: false },
);
</script>

<template>
  <article class="pt-rowcard" :class="{ 'is-interactive': interactive }">
    <header class="pt-rowcard__top">
      <span v-if="$slots.lead" class="pt-rowcard__lead">
        <slot name="lead" />
      </span>
      <div class="pt-rowcard__body">
        <div class="pt-rowcard__title">
          <slot name="title" />
        </div>
        <div v-if="$slots.meta" class="pt-rowcard__meta">
          <slot name="meta" />
        </div>
      </div>
      <span v-if="$slots.status" class="pt-rowcard__status">
        <slot name="status" />
      </span>
    </header>

    <div v-if="$slots.progress" class="pt-rowcard__progress">
      <slot name="progress" />
    </div>

    <footer v-if="$slots.actions" class="pt-rowcard__actions">
      <slot name="actions" />
    </footer>
  </article>
</template>

<style scoped>
.pt-rowcard {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  padding: var(--pt-space-3);
  background: var(--pt-raised);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-lg);
}

.pt-rowcard.is-interactive:active {
  background: var(--pt-hover);
}

.pt-rowcard__top {
  display: flex;
  gap: var(--pt-space-3);
  align-items: flex-start;
  min-width: 0;
}

.pt-rowcard__lead {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  /* 触控目标 ≥44（§9）：lead 里常放勾选框或头像按钮 */
  min-width: var(--pt-m-touch);
  min-height: var(--pt-m-touch);
  justify-content: center;
  margin: calc(var(--pt-space-2) * -1) 0 calc(var(--pt-space-2) * -1) calc(var(--pt-space-2) * -1);
}

/*
 * 只把 44×44 留给外框是不够的：el-checkbox 自己的可点区域实测只有 14×32
 * （它是个 label，宽度贴着那个小方块），手指点空白处不会生效。所以把控件本身撑满外框。
 * 只针对交互控件，不动头像这类纯展示内容 —— 那些有自己的尺寸，拉伸会变形。
 */
.pt-rowcard__lead > :slotted(.el-checkbox),
.pt-rowcard__lead > :slotted(.el-radio),
.pt-rowcard__lead > :slotted(button) {
  display: flex;
  align-items: center;
  justify-content: center;
  /* 直接给足尺寸，不用 100% —— 外框只有 min-height，height:100% 会解析成 auto，
     控件反而塌回内容高度 */
  min-width: var(--pt-m-touch);
  min-height: var(--pt-m-touch);
  /* Element 给 el-checkbox 留了 margin-right，会把它推偏 */
  margin: 0;
}

.pt-rowcard__body {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

/* 第一行：标题。长标题在手机上折两行还是比省略号有用，所以只限两行 */
.pt-rowcard__title {
  display: -webkit-box;
  overflow: hidden;
  font-size: var(--pt-fz-body);
  font-weight: 600;
  line-height: var(--pt-lh-body);
  color: var(--pt-t1);
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

/* 第二行：次要信息，横向排开，放不下就换行 */
.pt-rowcard__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 4px var(--pt-space-3);
  align-items: center;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

/*
 * meta 与 actions 里的内容来自插槽，带的是调用页的 scope id，不是本组件的 ——
 * 所以必须用 :slotted()，写成 `.pt-rowcard__meta > *` 会被编译成
 * `> *[data-v-本组件]`，一条都匹配不上，是死规则。
 */
.pt-rowcard__meta > :slotted(*) {
  display: inline-flex;
  gap: 4px;
  align-items: center;
  min-width: 0;
}

.pt-rowcard__status {
  flex: 0 0 auto;
}

.pt-rowcard__progress {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.pt-rowcard__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pt-space-2);
  align-items: center;
  padding-top: var(--pt-space-2);
  border-top: 1px solid var(--pt-border);
}

/*
 * 主操作撑满一行更好点；同排多个时各自等分，高度按 §9 的触控目标 ≥44。
 * margin:0 是为了压掉 Element 的 `.el-button + .el-button { margin-left: 12px }`，
 * 间距交给 flex gap。同样必须走 :slotted()。
 */
.pt-rowcard__actions :slotted(.el-button) {
  flex: 1;
  min-width: 96px;
  min-height: var(--pt-m-touch);
  margin: 0;
}

/*
 * 只管直接塞进来的 el-button。像 el-dropdown 那样自己包一层容器的，
 * 由调用页决定容器怎么参与等分 —— 在这里再写一条会和页面的同权重规则打架，
 * 谁赢取决于组件 CSS 的注入顺序。
 */
</style>
