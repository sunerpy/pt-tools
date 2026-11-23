<script setup lang="ts">
/*
 * AI 助手（路线图 M19）：和 pt-tools 内置的 AI 助手对话。助手在后端运行，经 MCP 工具查数据、做操作；
 * 改动类操作（暂停、删除、推送种子，订阅，整理入库）每次都要在这里点「确认」才执行。
 *
 * 界面按设计稿「AI 助手对话页」方向 B（居中阅读）：会话贴着侧栏一列（AssistantConvList），右边是对话
 * （AssistantThread：读会话、接事件流、发消息与确认都在它里面，每一页右下角的悬浮窗用的也是它）。
 * 整页撑满主区，只有对话那一列自己滚动。手机上先列会话，点进去看对话。
 *
 * 地址里的 ?c=<编号> 打开这个会话（悬浮窗的「全屏」就是这样进来的），?c=new 是新对话；
 * 打开着的会话写回地址（不加历史记录），刷新以后还在这个会话。
 */
import { assistantApi, type AssistantConversation, type AssistantStatus } from "@/api";
import AssistantConvList from "@/components/assistant/AssistantConvList.vue";
import AssistantThread from "@/components/assistant/AssistantThread.vue";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { conversationFromQuery, lastConversation, rememberConversation } from "@/utils/assistant";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";

const route = useRoute();
const router = useRouter();
const isMobile = useIsMobile();
const ds = useDataState();
const status = ref<AssistantStatus | null>(null);
const list = ref<AssistantConversation[]>([]);
/** 打开的会话；null 是还没发第一条消息的新对话 */
const currentId = ref<number | null>(null);
/** 手机上：看对话（否则看会话列表） */
const showChat = ref(false);
const thread = ref<InstanceType<typeof AssistantThread> | null>(null);

const current = computed(() => list.value.find((c) => c.id === currentId.value) ?? null);

async function load() {
  const pending = ds.run(async () => {
    const [st, convs] = await Promise.all([assistantApi.status(), assistantApi.conversations()]);
    return { st, convs };
  });
  const got = await pending;
  if (ds.isStale(pending) || !got) return;
  status.value = got.st;
  list.value = got.convs;
  const want = conversationFromQuery(route.query.c);
  if (want === "new") {
    showChat.value = true;
  } else if (want !== null) {
    openConversation(want);
  } else if (currentId.value === null && got.convs.length > 0 && !isMobile.value) {
    openConversation(got.convs[0]!.id);
  }
}

/** 打开一个会话：对话那一栏跟着 currentId 去读 */
function openConversation(id: number) {
  currentId.value = id;
  showChat.value = true;
}

/** 点了会话列表里的一项。手机上点回列表里正打开着的那个，也要回到对话 */
function onOpen(id: number) {
  if (id === currentId.value) showChat.value = true;
  else openConversation(id);
}

function onCreated(conv: AssistantConversation) {
  list.value = [conv, ...list.value.filter((c) => c.id !== conv.id)];
}

async function refreshList() {
  try {
    list.value = await assistantApi.conversations();
  } catch {
    /* 列表刷新失败不打扰：对话还在 */
  }
}

function newChat() {
  currentId.value = null;
  showChat.value = true;
  thread.value?.newChat();
}

function backToList() {
  showChat.value = false;
}

function errText(e: unknown, fallback: string): string {
  return e instanceof Error && e.message ? e.message : fallback;
}

async function remove(c: AssistantConversation) {
  if (c.running) {
    ElMessage.warning("这个会话正在回答：先停止再删");
    return;
  }
  try {
    await ElMessageBox.confirm(
      `删除会话「${c.title || "新对话"}」？消息、附件与待确认记录一起删掉。`,
      "删除会话",
      {
        type: "warning",
        confirmButtonText: "删除",
        cancelButtonText: "取消",
      },
    );
  } catch {
    return;
  }
  try {
    await assistantApi.remove(c.id);
    list.value = list.value.filter((x) => x.id !== c.id);
    if (lastConversation() === c.id) rememberConversation(null);
    if (currentId.value === c.id) newChat();
    ElMessage.success("已删除");
  } catch (e) {
    ElMessage.error(errText(e, "删除失败"));
  }
}

// 打开着的会话写回地址：刷新以后还在这个会话。用 replace，不往浏览器历史里加一条
watch(currentId, (id) => {
  const c = id === null ? "new" : String(id);
  if (route.query.c === c) return;
  void router.replace({ query: { ...route.query, c } });
});

// 地址里的 ?c= 被换了（手动改地址之类）：跟着打开。写回地址引起的变化与 currentId 一致，什么都不做
watch(
  () => route.query.c,
  (v) => {
    if (!status.value?.enabled) return;
    const want = conversationFromQuery(v);
    if (want === null) return;
    if (want === "new") {
      if (currentId.value !== null) newChat();
    } else if (want !== currentId.value) {
      openConversation(want);
    }
  },
);

onMounted(load);
</script>

<template>
  <div class="as-page" data-testid="as-page">
    <PtHeadSub
      >用自然语言查站点数据、下载器与种子，搜索、推送、订阅、整理入库、改配置；改动类操作每次都要你确认才执行</PtHeadSub
    >
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <!--
        没启用时页头不放按钮：以前「新对话」照样是醒目的主按钮，点了只重置本地状态、页面纹丝不动；
        那时唯一能做的事是去设置，入口就是正文中间那颗「去 AI 助手设置」，页头再放一颗是重复。
      -->
      <div v-show="!isMobile || !showChat" class="as-acts">
        <el-button
          v-if="!status || status.enabled"
          data-testid="as-settings"
          @click="$router.push('/assistant/settings')">
          <PtIcon name="settings" :size="15" /><span>设置</span>
        </el-button>
        <el-button v-if="status?.enabled" type="primary" data-testid="as-new" @click="newChat">
          <PtIcon name="square-pen" :size="15" /><span>新对话</span>
        </el-button>
      </div>
    </Teleport>

    <PtDataState
      v-if="!status"
      class="as-state"
      :state="ds.state.value"
      :sub="ds.errorText.value"
      data-testid="as-state" />

    <!-- 没启用：以前整屏只有一条警告横幅，下面全空；现在是一块居中的引导，下一步就是那颗按钮 -->
    <PtDataState
      v-else-if="!status.enabled"
      class="as-off"
      state="empty"
      title="AI 助手还没有启用"
      sub="到 AI 助手设置里选服务商、填 API Key，再打开开关。改动类操作每次都要你确认才执行"
      data-testid="as-disabled">
      <template #action>
        <el-button
          type="primary"
          data-testid="as-go-settings"
          @click="$router.push('/assistant/settings')">
          <PtIcon name="settings" :size="14" /><span>去 AI 助手设置</span>
        </el-button>
      </template>
    </PtDataState>

    <div v-else class="as-split">
      <AssistantConvList
        v-show="!isMobile || !showChat"
        class="as-split__list"
        :items="list"
        :active="currentId"
        @open="onOpen"
        @remove="remove" />

      <section
        v-show="!isMobile || showChat"
        class="as-chat"
        :aria-label="current?.title || '新对话'"
        data-testid="as-chat">
        <div v-if="isMobile" class="as-chat__bar">
          <el-button link data-testid="as-back" @click="backToList">
            <PtIcon name="arrow-left" :size="14" /><span>会话</span>
          </el-button>
          <span class="as-chat__title">{{ current?.title || "新对话" }}</span>
        </div>

        <AssistantThread
          ref="thread"
          v-model:conversation-id="currentId"
          :status="status"
          @created="onCreated"
          @activity="refreshList" />
      </section>
    </div>
  </div>
</template>

<style>
/*
 * 这一页撑满主区、只让对话那一列滚动：外壳的 .pt-shell__inner 只有 min-height，
 * 里面的高度算不出来（同 .pt-shell.is-immersive 的做法，这里只对这一页生效）。
 */
.pt-shell__inner:has(> .as-page) {
  height: 100%;
}

/* 会话那一列贴着侧栏：窄一点的桌面上外壳给 inner 的左右 16 内缩，这一页不要（手机照旧） */
@media (min-width: 769px) {
  .pt-shell__inner:has(> .as-page) {
    padding-inline: 0;
  }
}
</style>

<style scoped>
.as-page {
  display: flex;
  flex: 1 1 0;
  flex-direction: column;
  min-height: 0;
}

.as-state,
.as-off {
  margin: var(--pt-pad);
}

.as-acts {
  display: flex;
  gap: 8px;
}

/* 桌面上两个按钮落进页头的操作区：页头自己排，这一层不占地方 */
.pt-head__acts .as-acts {
  display: contents;
}

.as-split {
  display: grid;
  flex: 1 1 0;
  grid-template-columns: 268px minmax(0, 1fr);
  min-height: 0;
}

.as-split__list {
  border-right: 1px solid var(--pt-border);
}

.as-chat {
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
  background: var(--pt-surface);
}

.as-chat__bar {
  display: flex;
  flex: none;
  gap: 8px;
  align-items: center;
  min-height: 40px;
  padding: 0 4px 8px;
}

.as-chat__title {
  overflow: hidden;
  font-size: var(--pt-fz-body);
  font-weight: 500;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 768px) {
  .as-split {
    grid-template-columns: minmax(0, 1fr);
  }

  /* 手机上会话列表与对话各是一张白卡片（灰画布上的行卡规格），列表按内容高 */
  .as-split__list {
    align-self: start;
    max-height: 100%;
    border: 1px solid var(--pt-border);
    border-radius: var(--pt-r-lg);
    background: var(--pt-surface);
  }

  .as-acts {
    margin-bottom: 12px;
  }

  .as-acts > .el-button {
    flex: 1 1 0;
  }

  .as-acts > .el-button + .el-button {
    margin-left: 0;
  }

  .as-chat {
    border: 1px solid var(--pt-border);
    border-radius: var(--pt-r-lg);
    padding: 6px 12px 0;
  }
}
</style>
