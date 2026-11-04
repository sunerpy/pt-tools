<script setup lang="ts">
/*
 * 首页的「最近入库」（画板 02 的海报 98×147、07 的横滑海报 100×150）：已整理的电影与剧的一季（写最新那一集），
 * 连了媒体服务器时也有服务器里新加的（没有质量角标）。点海报进条目详情。桌面只排一行，放不下的不显示。
 */
import type { HomeBlock, HomeRecent, HomeRecentItem } from "@/api";
import HomeSection from "@/components/home/HomeSection.vue";
import { useIsMobile } from "@/composables/useIsMobile";
import { homeItemLink, recentSub } from "@/utils/home";
import { posterURL } from "@/utils/media";
import { computed, ref } from "vue";

const props = defineProps<{ block: HomeBlock<HomeRecent> | null }>();

const isMobile = useIsMobile();
const failed = ref(new Set<string>());

const data = computed(() => props.block?.data ?? null);
/** 计数：「今天 7」；一项都没有时不写（只写「最近没有入库」） */
const count = computed(() => (data.value?.items.length ? `今天 ${data.value.today_count}` : ""));

function poster(it: HomeRecentItem): string {
  if (!it.poster_path || failed.value.has(it.poster_path)) return "";
  return posterURL(it.poster_path, "w342");
}

function onImgError(it: HomeRecentItem) {
  if (!it.poster_path) return;
  const next = new Set(failed.value);
  next.add(it.poster_path);
  failed.value = next;
}

function key(it: HomeRecentItem): string {
  return `${it.media_type}-${it.tmdb_id}-${it.season ?? 0}`;
}
</script>

<template>
  <HomeSection
    title="最近入库"
    :count="count"
    action="整理历史"
    to="/media/history"
    data-testid="hm-recent">
    <p v-if="block?.error" class="hm-empty">取不到：{{ block.error }}</p>
    <p v-else-if="!block" class="hm-empty">加载中…</p>
    <p v-else-if="!data?.items.length" class="hm-empty" data-testid="hm-recent-empty">
      最近没有入库
    </p>
    <div v-else class="hm-recent" :class="{ 'is-mobile': isMobile }">
      <router-link
        v-for="it in data.items"
        :key="key(it)"
        :to="homeItemLink(it.media_type, it.tmdb_id, it.season)"
        class="hm-pc"
        :data-testid="`hm-pc-${key(it)}`">
        <div class="hm-pc__poster">
          <img v-if="poster(it)" :src="poster(it)" alt="" loading="lazy" @error="onImgError(it)" />
          <span v-else class="hm-pc__ph">{{ it.title.slice(0, 2) }}</span>
          <span v-if="it.quality" class="hm-pc__q">{{ it.quality }}</span>
        </div>
        <div class="hm-pc__title" :title="it.title">{{ it.title }}</div>
        <div class="hm-pc__sub">{{ recentSub(it) }}</div>
      </router-link>
    </div>
  </HomeSection>
</template>

<style scoped>
.hm-empty {
  margin: 0;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
}

/* 桌面：98 宽的海报一行排满，多出来的行高为 0、被裁掉 */
.hm-recent {
  display: grid;
  grid-template-rows: auto;
  grid-template-columns: repeat(auto-fill, 98px);
  grid-auto-rows: 0;
  gap: 0 12px;
  overflow: hidden;
}

.hm-pc {
  display: block;
  min-width: 0;
  color: inherit;
  text-decoration: none;
}

.hm-pc:focus-visible {
  border-radius: var(--pt-r-md);
  outline: 2px solid var(--pt-p);
  outline-offset: 2px;
}

.hm-pc__poster {
  position: relative;
  width: 98px;
  height: 147px;
  overflow: hidden;
  background: var(--pt-hover);
  border-radius: var(--pt-r-md);
}

.hm-pc__poster img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.hm-pc__ph {
  display: grid;
  height: 100%;
  place-items: center;
  font-size: var(--pt-fz-h2);
  font-weight: 700;
  color: var(--pt-t3);
}

.hm-pc__q {
  position: absolute;
  top: 6px;
  left: 6px;
  height: 17px;
  padding: 0 5px;
  font-size: var(--pt-fz-foot);
  font-weight: 700;
  line-height: 17px;
  color: #fff;
  background: rgb(0 0 0 / 0.55);
  border-radius: 3px;
}

.hm-pc__title {
  margin-top: 6px;
  overflow: hidden;
  font-size: var(--pt-fz-sm);
  font-weight: 500;
  line-height: 17px;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hm-pc:hover .hm-pc__title {
  color: var(--pt-p);
}

.hm-pc__sub {
  margin-top: 1px;
  font-size: var(--pt-fz-label);
  line-height: 15px;
  color: var(--pt-t3);
}

/* 手机：横滑海报 100×150 */
.hm-recent.is-mobile {
  display: flex;
  gap: 12px;
  margin: 0 -16px;
  padding: 0 16px;
  overflow-x: auto;
  scroll-padding: 0 16px;
  scroll-snap-type: x mandatory;
  scrollbar-width: none;
}

.hm-recent.is-mobile::-webkit-scrollbar {
  display: none;
}

.hm-recent.is-mobile .hm-pc {
  flex: 0 0 100px;
  scroll-snap-align: start;
}

.hm-recent.is-mobile .hm-pc__poster {
  width: 100px;
  height: 150px;
}
</style>
