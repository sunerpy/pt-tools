<script setup lang="ts">
/*
 * 首页的「待处理」（画板 02 的待处理条 1144×48、07 的待处理卡）：每类一个 chip，点了去对应的页面并带上筛选；
 * 「全部」打开右侧抽屉（手机是全屏），一行一件事。媒体服务器里删了的与库文件不见了的在抽屉里处理：「处理」打开删除弹窗，
 * 「忽略」把这一行的问题都忽略掉。
 */
import {
  type HomeBlock,
  type HomeTodo,
  type HomeTodoGroup,
  type HomeTodoItem,
  organizeApi,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import MediaDeleteDialog from "@/components/media/MediaDeleteDialog.vue";
import { useIsMobile } from "@/composables/useIsMobile";
import { isIssueTodo, todoIcon, todoTotal } from "@/utils/home";
import type { DeleteTarget } from "@/utils/mediaDelete";
import { ElMessage } from "element-plus";
import { computed, ref } from "vue";
import { useRouter } from "vue-router";

const props = defineProps<{ block: HomeBlock<HomeTodo> | null }>();
const emit = defineEmits<{ changed: [] }>();

const router = useRouter();
const isMobile = useIsMobile();

const groups = computed(() => props.block?.data?.groups ?? null);
const total = computed(() => todoTotal(groups.value));
const empty = computed(() => groups.value !== null && groups.value.length === 0);

const drawerOpen = ref(false);
/** 抽屉里只看这一类；空串看全部 */
const drawerKind = ref("");
const drawerItems = computed(() => {
  const all = props.block?.data?.items ?? [];
  return drawerKind.value ? all.filter((it) => it.kind === drawerKind.value) : all;
});
const drawerTitle = computed(() => {
  if (!drawerKind.value) return "待处理";
  return groups.value?.find((g) => g.kind === drawerKind.value)?.label ?? "待处理";
});

function openDrawer(kind = "") {
  drawerKind.value = kind;
  drawerOpen.value = true;
}

function onChip(g: HomeTodoGroup) {
  if (g.target === "drawer") openDrawer(g.kind);
  else void router.push(g.target);
}

function go(it: HomeTodoItem) {
  if (!it.target || it.target === "drawer") return;
  drawerOpen.value = false;
  void router.push(it.target);
}

// ---- 媒体服务器里删了的、库文件不见了的：处理与忽略 ----
const deleteOpen = ref(false);
const deleteTarget = ref<DeleteTarget | null>(null);
const busy = ref("");

function rowKey(it: HomeTodoItem, i: number): string {
  return `${it.kind}-${it.issue_ids?.join(",") ?? ""}-${i}`;
}

function handle(it: HomeTodoItem) {
  if (!it.media_type || !it.tmdb_id) return;
  deleteTarget.value = {
    selector: {
      media_type: it.media_type,
      tmdb_id: it.tmdb_id,
      season: it.media_type === "tv" ? (it.season ?? 0) : 0,
      episodes: it.media_type === "tv" && it.episodes?.length ? [...it.episodes] : undefined,
    },
    title: it.title.replace(/ 第 \d+ 季$/, ""),
    preset: { kind: "issues" },
  };
  deleteOpen.value = true;
}

async function dismiss(it: HomeTodoItem, key: string) {
  const ids = it.issue_ids ?? [];
  if (!ids.length || busy.value) return;
  busy.value = key;
  try {
    for (const id of ids) await organizeApi.dismissIssue(id);
    ElMessage.success("已忽略");
    emit("changed");
  } catch (e) {
    ElMessage.error((e as Error)?.message || "忽略失败");
  } finally {
    busy.value = "";
  }
}

function onDeleted() {
  emit("changed");
}
</script>

<template>
  <div class="hm-todo" :class="{ 'is-mobile': isMobile, 'is-empty': empty }" data-testid="hm-todo">
    <PtIcon
      :name="empty ? 'circle-check' : 'inbox'"
      :size="16"
      class="hm-todo__icon"
      :class="{ 'is-ok': empty }" />
    <span class="hm-todo__title">待处理</span>
    <span v-if="total" class="hm-todo__count" data-testid="hm-todo-count">{{ total }}</span>
    <button
      v-if="isMobile && groups?.length"
      type="button"
      class="hm-todo__all"
      data-testid="hm-todo-all"
      @click="openDrawer()">
      全部<PtIcon name="chevron-right" :size="14" />
    </button>
    <div class="hm-todo__chips">
      <span v-if="block?.error" class="hm-todo__note" data-testid="hm-todo-error"
        >取不到：{{ block.error }}</span
      >
      <span v-else-if="!block" class="hm-todo__note">加载中…</span>
      <span v-else-if="empty" class="hm-todo__note" data-testid="hm-todo-empty"
        >没有要处理的事</span
      >
      <button
        v-for="g in groups ?? []"
        :key="g.kind"
        type="button"
        class="hm-chip"
        :class="`is-${g.tone}`"
        :data-testid="`hm-chip-${g.kind}`"
        @click="onChip(g)">
        <i />{{ g.label }}
      </button>
    </div>
    <button
      v-if="!isMobile && groups?.length"
      type="button"
      class="hm-todo__all"
      data-testid="hm-todo-all"
      @click="openDrawer()">
      全部<PtIcon name="chevron-right" :size="14" />
    </button>

    <el-drawer
      v-model="drawerOpen"
      :title="drawerTitle"
      :size="isMobile ? '100%' : 440"
      append-to-body
      class="hm-todo-drawer"
      data-testid="hm-todo-drawer">
      <p v-if="!drawerItems.length" class="hm-todo__note">没有要处理的事</p>
      <ul v-else class="hm-rows">
        <li
          v-for="(it, i) in drawerItems"
          :key="rowKey(it, i)"
          class="hm-row"
          :data-testid="`hm-row-${it.kind}-${i}`">
          <PtIcon :name="todoIcon(it.kind)" :size="16" class="hm-row__icon" />
          <div class="hm-row__text">
            <div class="hm-row__title" :title="it.title">{{ it.title }}</div>
            <div v-if="it.detail" class="hm-row__detail" :title="it.detail">{{ it.detail }}</div>
          </div>
          <template v-if="isIssueTodo(it)">
            <el-button
              size="small"
              :disabled="!it.tmdb_id"
              :data-testid="`hm-row-handle-${i}`"
              @click="handle(it)"
              >处理</el-button
            >
            <el-button
              size="small"
              :loading="busy === rowKey(it, i)"
              :data-testid="`hm-row-dismiss-${i}`"
              @click="dismiss(it, rowKey(it, i))"
              >忽略</el-button
            >
          </template>
          <el-button
            v-else-if="it.target"
            size="small"
            :data-testid="`hm-row-go-${i}`"
            @click="go(it)"
            >去处理</el-button
          >
        </li>
      </ul>
    </el-drawer>
    <MediaDeleteDialog v-model="deleteOpen" :target="deleteTarget" @done="onDeleted" />
  </div>
</template>

<style scoped>
.hm-todo {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  min-height: 48px;
  padding: 10px 16px;
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-lg);
  box-shadow: var(--pt-shadow-sm);
}

.hm-todo__icon {
  flex: 0 0 auto;
  color: var(--pt-t2);
}

.hm-todo__icon.is-ok {
  color: var(--pt-ok);
}

.hm-todo__title {
  flex: 0 0 auto;
  font-size: var(--pt-fz-body);
  font-weight: 700;
  color: var(--pt-t1);
}

.hm-todo__count {
  flex: 0 0 auto;
  height: 18px;
  margin-left: 4px;
  padding: 0 6px;
  font-size: var(--pt-fz-label);
  font-variant-numeric: tabular-nums;
  line-height: 18px;
  color: var(--pt-t3);
  background: var(--pt-hover);
  border-radius: var(--pt-r-sm);
}

.hm-todo__chips {
  display: flex;
  flex: 1 1 0;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  min-width: 0;
  margin-left: 12px;
}

.hm-todo__note {
  margin: 0;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
}

.hm-todo__all {
  display: inline-flex;
  flex: 0 0 auto;
  gap: 2px;
  align-items: center;
  padding: 0;
  font-family: inherit;
  font-size: var(--pt-fz-sm);
  font-weight: 500;
  color: var(--pt-p);
  background: transparent;
  border: 0;
  cursor: pointer;
}

.hm-todo__all:hover {
  text-decoration: underline;
}

.hm-todo__all:focus-visible,
.hm-chip:focus-visible {
  border-radius: var(--pt-r-sm);
  outline: 2px solid var(--pt-p);
  outline-offset: 2px;
}

.hm-chip {
  display: inline-flex;
  gap: 5px;
  align-items: center;
  height: 26px;
  padding: 0 10px;
  font-family: inherit;
  font-size: var(--pt-fz-sm);
  font-weight: 500;
  white-space: nowrap;
  border: 0;
  border-radius: 13px;
  cursor: pointer;
}

.hm-chip i {
  width: 5px;
  height: 5px;
  background: currentcolor;
  border-radius: 50%;
}

.hm-chip.is-dang {
  color: var(--pt-dang);
  background: var(--pt-dang-soft);
}

.hm-chip.is-warn {
  color: var(--pt-warn);
  background: var(--pt-warn-soft);
}

.hm-chip.is-primary {
  color: var(--pt-p);
  background: var(--pt-p-soft);
}

.hm-chip.is-info {
  color: var(--pt-info);
  background: var(--pt-info-soft);
}

.hm-chip:hover {
  filter: brightness(0.96);
}

/* 手机：343 宽的卡，标题行一行，chip 从下一行起换行 */
.hm-todo.is-mobile {
  padding: 11px 14px 7px;
}

.hm-todo.is-mobile .hm-todo__all {
  margin-left: auto;
}

/* chip 从 14,42 起，行距 36，最后一行底下留 8 */
.hm-todo.is-mobile .hm-todo__chips {
  flex-basis: 100%;
  gap: 6px 8px;
  margin: 4px 0 0;
}

.hm-todo.is-mobile .hm-chip {
  height: 30px;
  border-radius: 15px;
}

.hm-rows {
  margin: 0;
  padding: 0;
  list-style: none;
}

.hm-row {
  display: flex;
  gap: 8px;
  align-items: center;
  min-height: 48px;
  padding: 6px 0;
  border-bottom: 1px solid var(--pt-border);
}

.hm-row:last-child {
  border-bottom: 0;
}

.hm-row__icon {
  flex: 0 0 auto;
  margin-right: 4px;
  color: var(--pt-t2);
}

.hm-row__text {
  flex: 1 1 auto;
  min-width: 0;
}

.hm-row__title {
  overflow: hidden;
  font-size: var(--pt-fz-body);
  font-weight: 500;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hm-row__detail {
  margin-top: 2px;
  overflow: hidden;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hm-row :deep(.el-button + .el-button) {
  margin-left: 0;
}
</style>
