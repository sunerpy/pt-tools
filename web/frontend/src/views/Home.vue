<script setup lang="ts">
/*
 * 首页（媒体中心 M8；首页 NAS 概览 H2 改成方案 A，规格 docs/design/home-nas-spec.md）：最上面满宽的 NAS 概览，
 * 下面是今天播出、待处理，两栏的左栏是继续观看、最近入库、这周还要播，右栏是下载器与状态。
 * 数据来自 GET /api/home，每块单独给数据或错误，一块取不到不影响别的；服务端缓存 30 秒，「刷新」不用缓存。
 * NAS 概览另外每 30 秒取 GET /api/nas（只读内存）；下载器速度跟着外壳每 30 秒取的 transfer-stats 更新。
 * 打开网页先到这里（/ 重定向到 /home）。
 */
import { type HomeData, homeApi } from "@/api";
import PtIcon from "@/components/PtIcon";
import HomeDownloaders from "@/components/home/HomeDownloaders.vue";
import HomeNasOverview from "@/components/home/HomeNasOverview.vue";
import HomeRecent from "@/components/home/HomeRecent.vue";
import HomeResume from "@/components/home/HomeResume.vue";
import HomeToday from "@/components/home/HomeToday.vue";
import HomeTodo from "@/components/home/HomeTodo.vue";
import HomeWeek from "@/components/home/HomeWeek.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import { useIsMobile } from "@/composables/useIsMobile";
import { useNasHome } from "@/composables/useNasHome";
import { useTmdbImageBase } from "@/composables/useTmdbImageBase";
import { headline, resumeItems, todoTotal } from "@/utils/home";
import { NAS_LOCAL_ID } from "@/utils/nas";
import { computed, onMounted, ref } from "vue";

const isMobile = useIsMobile();
useTmdbImageBase();

const data = ref<HomeData | null>(null);
const loading = ref(false);
const error = ref("");
/** 点「刷新」的次数：NAS 概览跟着重取 GET /api/nas */
const refreshes = ref(0);
let seq = 0;

async function load(refresh = false) {
  const n = ++seq;
  if (refresh) refreshes.value += 1;
  loading.value = true;
  error.value = "";
  try {
    const got = await homeApi.get(refresh);
    if (n === seq) data.value = got;
  } catch (e) {
    if (n === seq) error.value = (e as Error)?.message || "读取首页失败";
  } finally {
    if (n === seq) loading.value = false;
  }
}

onMounted(() => void load());

const nas = useNasHome(
  () => data.value?.nas ?? null,
  () => refreshes.value,
);
/** NAS 概览画的是本机来源时，下载盘与媒体库在概览里，右栏不再列一遍 */
const showStorage = computed(() => nas.home.value?.source?.id !== NAS_LOCAL_ID);

const sub = computed(() => {
  const d = data.value;
  if (!d) return error.value ? "" : "加载中…";
  return headline(
    d.calendar.data?.today ?? null,
    todoTotal(d.todo.data?.groups ?? null),
    isMobile.value,
  );
});

const resumeCount = computed(() => {
  const groups = data.value?.resume.data;
  return groups ? resumeItems(groups).length : null;
});

/** 继续观看「在 Jellyfin 打开」：第一台有继续观看的服务器的网页 */
const resumeServerURL = computed(() => {
  const first = data.value?.resume.data?.[0];
  if (!first) return undefined;
  return data.value?.status.data?.servers.find((s) => s.id === first.server_id)?.open_url;
});
</script>

<template>
  <div class="hm" :class="{ 'is-mobile': isMobile }" data-testid="home">
    <PtHeadSub>{{ sub }}</PtHeadSub>
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button
        v-if="!isMobile"
        :loading="loading && Boolean(data)"
        data-testid="hm-refresh"
        @click="load(true)">
        <PtIcon name="refresh-cw" :size="15" /><span>刷新</span>
      </el-button>
    </Teleport>

    <div v-if="error && !data" class="pt-note pt-note--warn" data-testid="hm-error">
      <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
      <span>{{ error }}</span>
      <el-button size="small" @click="load(true)">重试</el-button>
    </div>

    <HomeNasOverview
      :home="nas.home.value"
      :view="nas.view.value"
      :failure="nas.failure.value"
      :now="nas.now.value"
      :storage="data?.storage ?? null"
      :downloaders="data?.downloaders ?? null" />
    <HomeToday :block="data?.calendar ?? null" />
    <HomeTodo :block="data?.todo ?? null" @changed="load(true)" />

    <div class="hm-cols">
      <div class="hm-main">
        <HomeResume :block="data?.resume ?? null" :server-url="resumeServerURL" />
        <HomeRecent :block="data?.recent ?? null" />
        <HomeWeek :block="data?.calendar ?? null" />
      </div>
      <HomeDownloaders
        :downloaders="data?.downloaders ?? null"
        :storage="data?.storage ?? null"
        :status="data?.status ?? null"
        :resume-count="resumeCount"
        :show-storage="showStorage" />
    </div>

    <div v-if="isMobile" class="hm-mrefresh">
      <el-button :loading="loading && Boolean(data)" data-testid="hm-refresh" @click="load(true)">
        <PtIcon name="refresh-cw" :size="15" /><span>刷新</span>
      </el-button>
    </div>
  </div>
</template>

<style scoped>
.hm {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
  padding: var(--pt-pad);
  container-type: inline-size;
}

/* 两栏：左栏继续观看、最近入库、这周还要播，右栏 368 宽的下载器与状态；内容宽不到 1100 时右栏落到下面 */
.hm-cols {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 368px;
  gap: 16px;
  align-items: start;
}

.hm-main {
  display: flex;
  flex-direction: column;
  gap: 20px;
  min-width: 0;
}

@container (max-width: 1099px) {
  .hm-cols {
    grid-template-columns: minmax(0, 1fr);
  }
}

/* 手机：外壳的内容列已经左右内缩 16、上面 12（再让 4，标题落在顶栏下 16）；块与块之间统一 24（画板 07 的偏离 ④） */
.hm.is-mobile {
  gap: 24px;
  padding: 4px 0 16px;
}

.hm.is-mobile .hm-cols,
.hm.is-mobile .hm-main {
  gap: 24px;
}

.hm-mrefresh {
  display: flex;
  justify-content: center;
}
</style>
