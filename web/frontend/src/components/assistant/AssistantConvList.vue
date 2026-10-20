<script setup lang="ts">
/*
 * AI 助手的会话列表（设计稿「AI 助手对话页」方向 B）：贴着侧栏的一列，上面是搜索框，
 * 会话按「今天 / 昨天 / 近 7 天 / 更早」分组，每项一行标题、一行时间（正在回答的带一个圆点）。
 * 打开的那项是白底加边框；删除按钮鼠标移上去或键盘移到这一项时出现，触屏上一直在。
 */
import type { AssistantConversation } from "@/api";
import PtIcon from "@/components/PtIcon";
import { CONV_GROUPS, convGroup, convTime } from "@/utils/assistant";
import { computed, ref } from "vue";

const props = defineProps<{
  items: AssistantConversation[];
  active: number | null;
}>();

const emit = defineEmits<{
  open: [id: number];
  remove: [c: AssistantConversation];
}>();

const query = ref("");

const groups = computed(() => {
  const q = query.value.trim().toLowerCase();
  const now = new Date();
  const hit = props.items.filter((c) => !q || titleOf(c).toLowerCase().includes(q));
  return CONV_GROUPS.map((name) => ({
    name,
    items: hit
      .filter((c) => convGroup(c.updated_at, now) === name)
      .map((c) => ({ conv: c, time: convTime(c.updated_at, now) })),
  })).filter((g) => g.items.length > 0);
});

function titleOf(c: AssistantConversation): string {
  return c.title || "新对话";
}
</script>

<template>
  <section class="as-convs" aria-label="会话" data-testid="as-convs">
    <label class="as-convs__search">
      <PtIcon name="search" :size="14" class="as-convs__search-icon" />
      <input
        v-model="query"
        type="search"
        placeholder="搜索会话"
        aria-label="搜索会话"
        data-testid="as-conv-search" />
    </label>

    <div class="as-convs__scroll">
      <p v-if="items.length === 0" class="as-convs__empty">还没有会话：在右边问一句试试。</p>
      <p v-else-if="groups.length === 0" class="as-convs__empty">
        没有标题里带「{{ query.trim() }}」的会话
      </p>
      <div v-for="g in groups" :key="g.name">
        <h3 class="as-convs__label">{{ g.name }}</h3>
        <ul class="as-convs__list">
          <li
            v-for="{ conv: c, time } in g.items"
            :key="c.id"
            class="as-conv"
            :class="{ 'is-active': c.id === active }">
            <button
              type="button"
              class="as-conv__main"
              :aria-current="c.id === active ? 'true' : undefined"
              :data-testid="`as-conv-${c.id}`"
              @click="emit('open', c.id)">
              <span class="as-conv__title">{{ titleOf(c) }}</span>
              <span class="as-conv__meta">
                <template v-if="c.running">
                  <span class="as-conv__dot" aria-hidden="true" />
                  <span class="as-conv__state">回答中</span>
                </template>
                <span>{{ time }}</span>
              </span>
            </button>
            <button
              type="button"
              class="as-conv__del"
              :aria-label="`删除会话 ${titleOf(c)}`"
              title="删除会话"
              :data-testid="`as-del-${c.id}`"
              @click="emit('remove', c)">
              <PtIcon name="trash-2" :size="14" />
            </button>
          </li>
        </ul>
      </div>
    </div>
  </section>
</template>

<style scoped>
.as-convs {
  display: flex;
  flex-direction: column;
  min-height: 0;
  background: color-mix(in srgb, var(--pt-hover) 45%, var(--pt-surface));
}

.as-convs__search {
  display: flex;
  flex: none;
  gap: 8px;
  align-items: center;
  height: 32px;
  margin: 14px 12px 4px;
  padding: 0 10px;
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-md);
  background: var(--pt-surface);
  cursor: text;
}

.as-convs__search:focus-within {
  border-color: var(--pt-p);
}

.as-convs__search-icon {
  flex: none;
  color: var(--pt-t3);
}

.as-convs__search input {
  flex: 1 1 auto;
  min-width: 0;
  padding: 0;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--pt-t1);
  font: inherit;
  font-size: var(--pt-fz-sm);
}

.as-convs__search input::placeholder {
  color: var(--pt-t3);
}

.as-convs__scroll {
  flex: 1 1 auto;
  min-height: 0;
  padding: 0 6px 12px;
  overflow-y: auto;
  overscroll-behavior: contain;
}

.as-convs__empty {
  margin: 12px 10px;
  font-size: var(--pt-fz-sm);
  line-height: 1.7;
  color: var(--pt-t3);
}

.as-convs__label {
  margin: 12px 10px 6px;
  font-size: var(--pt-fz-label);
  font-weight: 500;
  color: var(--pt-t3);
}

.as-convs__list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.as-conv {
  position: relative;
  border: 1px solid transparent;
  border-radius: var(--pt-r-lg);
}

.as-conv:hover {
  background: var(--pt-hover);
}

.as-conv.is-active {
  border-color: var(--pt-border);
  background: var(--pt-surface);
  box-shadow: var(--pt-shadow-sm);
}

.as-conv__main {
  display: flex;
  flex-direction: column;
  gap: 2px;
  width: 100%;
  padding: 7px 36px 7px 11px;
  border: 0;
  border-radius: inherit;
  background: transparent;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.as-conv__main:focus-visible,
.as-conv__del:focus-visible {
  outline: 2px solid var(--pt-p);
  outline-offset: -2px;
}

.as-conv__title {
  overflow: hidden;
  font-size: var(--pt-fz-body);
  line-height: 1.5;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.as-conv.is-active .as-conv__title {
  font-weight: 500;
}

.as-conv__meta {
  display: flex;
  gap: 4px;
  align-items: center;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

.as-conv__dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--pt-p);
}

.as-conv__state {
  margin-right: 6px;
  color: var(--pt-p);
}

.as-conv__del {
  position: absolute;
  top: 50%;
  right: 6px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  padding: 0;
  border: 0;
  border-radius: var(--pt-r-md);
  background: transparent;
  color: var(--pt-t3);
  opacity: 0;
  transform: translateY(-50%);
  cursor: pointer;
}

.as-conv:hover .as-conv__del,
.as-conv:focus-within .as-conv__del {
  opacity: 1;
}

.as-conv__del:hover {
  background: var(--pt-dang-soft);
  color: var(--pt-dang);
}

/* 触屏没有悬停：删除按钮一直在 */
@media (hover: none) {
  .as-conv__del {
    opacity: 1;
  }
}
</style>
