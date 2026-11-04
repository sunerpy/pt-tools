<script setup lang="ts">
/*
 * 首页的「继续观看」（画板 02 的卡 242×182、07 的横滑卡 240×188）：媒体服务器里没看完的，点了在服务器里接着看（新标签页）。
 * 没有配 Emby、Jellyfin（或没选用户）时整块不显示。桌面只显示一行，放不下的不显示。
 */
import type { HomeBlock, MediaResumeGroup, MediaResumeItem } from "@/api";
import PtIcon from "@/components/PtIcon";
import HomeSection from "@/components/home/HomeSection.vue";
import { useIsMobile } from "@/composables/useIsMobile";
import { resumeIdentity, resumeImage, resumeItems, resumeSub } from "@/utils/home";
import { posterURL } from "@/utils/media";
import { computed, ref } from "vue";

const props = defineProps<{
  block: HomeBlock<MediaResumeGroup[]> | null;
  /** 第一台服务器网页的地址（区块标题的「在 Jellyfin 打开」） */
  serverUrl?: string;
}>();

const isMobile = useIsMobile();
const failed = ref(new Set<string>());

const groups = computed(() => props.block?.data ?? null);
/** 没有选了用户的 Emby、Jellyfin 时整块不显示；取不到时照样显示原因 */
const shown = computed(() => Boolean(props.block?.error) || (groups.value?.length ?? 0) > 0);
const items = computed(() => resumeItems(groups.value));
const errors = computed(() =>
  (groups.value ?? []).filter((g) => g.error).map((g) => `${g.name}：${g.error}`),
);
const server = computed(() => groups.value?.[0]?.name ?? "");
const action = computed(() => {
  if (!props.serverUrl || !server.value) return "";
  return isMobile.value ? server.value : `在 ${server.value} 打开`;
});

function image(it: MediaResumeItem): { src: string; poster: boolean } | null {
  const img = resumeImage(it);
  if (!img || failed.value.has(img.path)) return null;
  return { src: posterURL(img.path, img.poster ? "w342" : "w300"), poster: img.poster };
}

function onImgError(it: MediaResumeItem) {
  const img = resumeImage(it);
  if (!img) return;
  const next = new Set(failed.value);
  next.add(img.path);
  failed.value = next;
}

function key(it: MediaResumeItem): string {
  return `${it.server_id}-${it.item_id}`;
}
</script>

<template>
  <HomeSection
    v-if="shown"
    title="继续观看"
    :tag="resumeIdentity(groups)"
    :action="action"
    :href="serverUrl"
    data-testid="hm-resume">
    <p v-if="block?.error" class="hm-empty">取不到：{{ block.error }}</p>
    <template v-else>
      <p v-if="!items.length" class="hm-empty" data-testid="hm-resume-empty">
        {{ errors.length ? `取不到：${errors.join("；")}` : "没有没看完的" }}
      </p>
      <div v-else class="hm-resume" :class="{ 'is-mobile': isMobile }">
        <a
          v-for="it in items"
          :key="key(it)"
          :href="it.open_url || undefined"
          target="_blank"
          rel="noopener noreferrer"
          class="hm-rc"
          :data-testid="`hm-rc-${key(it)}`">
          <div class="hm-rc__img">
            <img
              v-if="image(it)"
              :src="image(it)!.src"
              :class="{ 'is-poster': image(it)!.poster }"
              alt=""
              loading="lazy"
              @error="onImgError(it)" />
            <div v-else class="hm-rc__ph" />
            <span class="hm-rc__play"><PtIcon name="play" :size="18" /></span>
            <span class="hm-rc__track"
              ><span
                class="hm-rc__fill"
                :style="{ width: `${Math.min(100, Math.max(0, it.percent))}%` }"
            /></span>
          </div>
          <div class="hm-rc__title" :title="it.title">{{ it.title }}</div>
          <div class="hm-rc__sub">{{ resumeSub(it) }}</div>
        </a>
      </div>
    </template>
  </HomeSection>
</template>

<style scoped>
.hm-empty {
  margin: 0;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
}

/* 桌面：按 242 宽自动排，只显示一行（多出来的行高为 0、被裁掉） */
.hm-resume {
  display: grid;
  grid-template-rows: auto;
  grid-template-columns: repeat(auto-fill, minmax(242px, 1fr));
  grid-auto-rows: 0;
  gap: 0 17px;
  overflow: hidden;
}

.hm-rc {
  display: flex;
  flex-direction: column;
  height: 182px;
  overflow: hidden;
  color: inherit;
  text-decoration: none;
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-lg);
  box-shadow: var(--pt-shadow-sm);
}

.hm-rc:focus-visible {
  outline: 2px solid var(--pt-p);
  outline-offset: 2px;
}

.hm-rc__img {
  position: relative;
  flex: 0 0 136px;
  height: 136px;
  overflow: hidden;
  background: var(--pt-hover);
}

.hm-rc__img img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.hm-rc__img img.is-poster {
  object-position: center 30%;
}

.hm-rc__ph {
  height: 100%;
}

.hm-rc__play {
  position: absolute;
  top: 48px;
  left: calc(50% - 20px);
  display: grid;
  width: 40px;
  height: 40px;
  place-items: center;
  color: #fff;
  background: rgb(0 0 0 / 0.45);
  border-radius: 50%;
}

.hm-rc__track {
  position: absolute;
  right: 0;
  bottom: 0;
  left: 0;
  height: 4px;
  background: rgb(255 255 255 / 0.35);
}

.hm-rc__fill {
  display: block;
  height: 100%;
  background: var(--pt-p-hover);
}

.hm-rc__title {
  margin: 9px 12px 0;
  overflow: hidden;
  font-size: var(--pt-fz-body);
  font-weight: 500;
  line-height: 17px;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hm-rc:hover .hm-rc__title {
  color: var(--pt-p);
}

.hm-rc__sub {
  margin: 1px 12px 0;
  overflow: hidden;
  font-size: var(--pt-fz-label);
  line-height: 15px;
  color: var(--pt-t3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 手机：横滑卡 240×188 */
.hm-resume.is-mobile {
  display: flex;
  gap: 12px;
  margin: 0 -16px;
  padding: 0 16px;
  overflow-x: auto;
  scroll-padding: 0 16px;
  scroll-snap-type: x mandatory;
  scrollbar-width: none;
}

.hm-resume.is-mobile::-webkit-scrollbar {
  display: none;
}

.hm-resume.is-mobile .hm-rc {
  flex: 0 0 240px;
  height: 188px;
  scroll-snap-align: start;
}

.hm-resume.is-mobile .hm-rc__img {
  flex-basis: 135px;
  height: 135px;
}

.hm-resume.is-mobile .hm-rc__play {
  top: 47px;
}
</style>
