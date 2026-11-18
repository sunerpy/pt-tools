<script setup lang="ts">
/*
 * 首页的「今天播出」（画板 02 的卡片 216×200、07 的横滑卡 240×196 加翻页圆点）：今天播出的集与上映的电影，
 * 按台账状态写状态牌、副行与动作（播放、搜资源、看版本，或一句说明）。桌面一行排不下时横向滚，两端有翻页按钮。
 */
import type { HomeBlock, HomeCalendar } from "@/api";
import PtIcon from "@/components/PtIcon";
import HomeSection from "@/components/home/HomeSection.vue";
import { useIsMobile } from "@/composables/useIsMobile";
import { homeItemLink, todayCard, todayTap } from "@/utils/home";
import { posterURL } from "@/utils/media";
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";

const props = defineProps<{ block: HomeBlock<HomeCalendar> | null }>();

const isMobile = useIsMobile();
const now = ref(new Date());
const failed = ref(new Set<string>());

const data = computed(() => props.block?.data ?? null);
const items = computed(() => data.value?.today ?? []);
const cards = computed(() =>
  items.value.map((e) => ({
    e,
    card: todayCard(e, now.value),
    key: `${e.subscription_id}-${e.season ?? 0}-${e.episode ?? 0}`,
  })),
);

/** 计数：「5 集」「1 部」「4 集 · 1 部」 */
const count = computed(() => {
  if (!data.value) return "";
  const eps = items.value.filter((e) => e.media_type === "tv").length;
  const movies = items.value.length - eps;
  const parts = [];
  if (eps) parts.push(`${eps} 集`);
  if (movies) parts.push(`${movies} 部`);
  return parts.join(" · ");
});

const noSubs = computed(() => data.value !== null && data.value.subscriptions === 0);

function image(path?: string): string {
  if (!path || failed.value.has(path)) return "";
  return posterURL(path, "w300");
}

function onImgError(path?: string) {
  if (!path) return;
  const next = new Set(failed.value);
  next.add(path);
  failed.value = next;
}

// ---- 桌面：一行排不下时两端的翻页按钮 ----
const row = ref<HTMLElement | null>(null);
const canPrev = ref(false);
const canNext = ref(false);

function measure() {
  const el = row.value;
  if (!el) return;
  canPrev.value = el.scrollLeft > 1;
  canNext.value = el.scrollLeft + el.clientWidth < el.scrollWidth - 1;
  if (isMobile.value) updateDot();
}

function page(dir: 1 | -1) {
  const el = row.value;
  if (!el) return;
  el.scrollBy({ left: dir * el.clientWidth, behavior: "smooth" });
}

// ---- 手机：翻页圆点（当前这张是最左边完整露出的那张） ----
const dot = ref(0);
const CARD_STEP = 240 + 12;

function updateDot() {
  const el = row.value;
  if (!el) return;
  dot.value = Math.min(cards.value.length - 1, Math.max(0, Math.round(el.scrollLeft / CARD_STEP)));
}

let observer: ResizeObserver | null = null;

/** 翻页按钮跟着这一行的宽度：这一行是数据到了才出现的，出现、换掉时重新观察 */
function observe(el: HTMLElement | null, old?: HTMLElement | null) {
  if (typeof ResizeObserver !== "function") return;
  observer ??= new ResizeObserver(measure);
  if (old) observer.unobserve(old);
  if (el) observer.observe(el);
}

onMounted(() => {
  observe(row.value);
  measure();
});
onBeforeUnmount(() => observer?.disconnect());

watch(
  row,
  (el, old) => {
    observe(el, old);
    measure();
  },
  { flush: "post" },
);

// 数据换了（刷新）时按现在的时间重算「播出 6 小时」
watch(
  () => props.block,
  () => {
    now.value = new Date();
  },
);

// 卡片数或桌面、手机换了时重新算翻页按钮与圆点；分开写两个来源，值没变时不触发
watch([() => cards.value.length, isMobile], () => {
  void nextTick(measure);
});
</script>

<template>
  <HomeSection
    title="今天播出"
    :count="count"
    :action="noSubs ? '去探索' : '看日历'"
    :to="noSubs ? '/media/explore' : '/media/calendar'"
    data-testid="hm-today">
    <p v-if="block?.error" class="hm-empty" data-testid="hm-today-error">
      取不到：{{ block.error }}
    </p>
    <p v-else-if="!block" class="hm-empty">加载中…</p>
    <p v-else-if="!cards.length" class="hm-empty" data-testid="hm-today-empty">
      {{ noSubs ? "还没有追剧" : "今天没有要播的集" }}
    </p>
    <div v-else class="hm-today" :class="{ 'is-mobile': isMobile }">
      <button
        v-if="!isMobile && canPrev"
        type="button"
        class="hm-today__page is-prev"
        aria-label="上一页"
        data-testid="hm-today-prev"
        @click="page(-1)">
        <PtIcon name="chevron-left" :size="16" />
      </button>
      <div ref="row" class="hm-today__row" data-testid="hm-today-row" @scroll.passive="measure">
        <template v-for="{ e, card, key } in cards" :key="key">
          <!-- 手机：整张卡点了去播放、搜资源或详情（见 todayTap） -->
          <component
            :is="todayTap(e).href ? 'a' : 'router-link'"
            v-if="isMobile"
            class="hm-tc is-tap"
            :class="`is-${e.state}`"
            v-bind="
              todayTap(e).href
                ? { href: todayTap(e).href, target: '_blank', rel: 'noopener noreferrer' }
                : { to: todayTap(e).to }
            "
            :data-testid="`hm-tc-${key}`">
            <div class="hm-tc__img">
              <img
                v-if="image(card.image)"
                :src="image(card.image)"
                alt=""
                loading="lazy"
                @error="onImgError(card.image)" />
              <div v-else class="hm-tc__ph"><PtIcon name="calendar-days" :size="22" /></div>
              <span class="hm-tc__state" :class="`is-${card.state.tone}`"
                ><i />{{ card.state.label }}</span
              >
            </div>
            <div class="hm-tc__title" :title="e.title">{{ e.title }}</div>
            <div class="hm-tc__sub" :title="card.sub">
              {{ card.time ? `${card.sub} · ${card.time}` : card.sub }}
            </div>
          </component>
          <div v-else class="hm-tc" :class="`is-${e.state}`" :data-testid="`hm-tc-${key}`">
            <router-link
              :to="homeItemLink(e.media_type, e.tmdb_id, e.season, e.episode)"
              class="hm-tc__img"
              tabindex="-1"
              aria-hidden="true">
              <img
                v-if="image(card.image)"
                :src="image(card.image)"
                alt=""
                loading="lazy"
                @error="onImgError(card.image)" />
              <div v-else class="hm-tc__ph"><PtIcon name="calendar-days" :size="22" /></div>
              <span class="hm-tc__state" :class="`is-${card.state.tone}`"
                ><i />{{ card.state.label }}</span
              >
              <span v-if="card.time" class="hm-tc__time">{{ card.time }}</span>
            </router-link>
            <router-link
              :to="homeItemLink(e.media_type, e.tmdb_id, e.season, e.episode)"
              class="hm-tc__title"
              :title="e.title">
              {{ e.title }}
            </router-link>
            <div class="hm-tc__sub" :title="card.sub">{{ card.sub }}</div>
            <a
              v-if="card.action.kind === 'play'"
              :href="card.action.href"
              target="_blank"
              rel="noopener noreferrer"
              class="hm-tc__act"
              :data-testid="`hm-tc-act-${key}`">
              <PtIcon :name="card.action.icon!" :size="13" />{{ card.action.text }}
            </a>
            <router-link
              v-else-if="card.action.to"
              :to="card.action.to"
              class="hm-tc__act"
              :data-testid="`hm-tc-act-${key}`">
              <PtIcon :name="card.action.icon!" :size="13" />{{ card.action.text }}
            </router-link>
            <div v-else class="hm-tc__note" :data-testid="`hm-tc-act-${key}`">
              {{ card.action.text }}
            </div>
          </div>
        </template>
      </div>
      <button
        v-if="!isMobile && canNext"
        type="button"
        class="hm-today__page is-next"
        aria-label="下一页"
        data-testid="hm-today-next"
        @click="page(1)">
        <PtIcon name="chevron-right" :size="16" />
      </button>
      <div
        v-if="isMobile && cards.length > 1"
        class="hm-dots"
        aria-hidden="true"
        data-testid="hm-today-dots">
        <span
          v-for="(c, i) in cards"
          :key="c.key"
          class="hm-dots__dot"
          :class="{ 'is-on': i === dot }" />
      </div>
    </div>
  </HomeSection>
</template>

<style scoped>
.hm-empty {
  margin: 0;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
}

.hm-today {
  position: relative;
}

.hm-today__row {
  display: flex;
  gap: 16px;
  overflow-x: auto;
  scroll-snap-type: x mandatory;
  scrollbar-width: none;
}

.hm-today__row::-webkit-scrollbar {
  display: none;
}

.hm-today__page {
  position: absolute;
  top: 46px;
  z-index: 2;
  display: grid;
  width: 30px;
  height: 30px;
  place-items: center;
  color: var(--pt-t1);
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: 50%;
  box-shadow: var(--pt-shadow-md);
  cursor: pointer;
}

.hm-today__page.is-prev {
  left: -12px;
}

.hm-today__page.is-next {
  right: -12px;
}

.hm-today__page:focus-visible {
  outline: 2px solid var(--pt-p);
  outline-offset: 2px;
}

/*
 * 画板 02 按 1144 宽画：一行五张 216、间距 16。首页加了 NAS 概览以后桌面总比视口高，滚动区的竖向滚动条让出 10，
 * 内容只剩 1134：卡跟着缩到 214，五张仍是一行放下；更窄时按 206 横滑（两端有翻页按钮）。
 */
.hm-tc {
  position: relative;
  display: flex;
  flex: 0 0 max(206px, calc((100% - 64px) / 5));
  flex-direction: column;
  height: 200px;
  overflow: hidden;
  scroll-snap-align: start;
  color: inherit;
  text-decoration: none;
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-lg);
  box-shadow: var(--pt-shadow-sm);
}

.hm-tc__img {
  position: relative;
  display: block;
  flex: 0 0 122px;
  height: 122px;
  overflow: hidden;
  background: var(--pt-hover);
}

.hm-tc__img img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.hm-tc__ph {
  display: grid;
  height: 100%;
  place-items: center;
  color: var(--pt-t3);
}

.hm-tc__state {
  position: absolute;
  top: 8px;
  left: 8px;
  display: inline-flex;
  gap: 5px;
  align-items: center;
  height: 20px;
  padding: 0 7px;
  font-size: var(--pt-fz-label);
  font-weight: 500;
  color: var(--pt-t2);
  background: color-mix(in srgb, var(--pt-surface) 94%, transparent);
  border-radius: var(--pt-r-sm);
}

.hm-tc__state i {
  width: 5px;
  height: 5px;
  background: currentcolor;
  border-radius: 50%;
}

.hm-tc__state.is-ok {
  color: var(--pt-ok);
}

.hm-tc__state.is-warn {
  color: var(--pt-warn);
}

.hm-tc__state.is-info {
  color: var(--pt-info);
}

.hm-tc__state.is-primary {
  color: var(--pt-p);
}

.hm-tc__time {
  position: absolute;
  right: 8px;
  bottom: 8px;
  height: 20px;
  padding: 0 6px;
  font-size: var(--pt-fz-label);
  font-weight: 500;
  line-height: 20px;
  color: #fff;
  background: rgb(0 0 0 / 0.55);
  border-radius: var(--pt-r-sm);
}

.hm-tc__title {
  display: block;
  margin: 9px 12px 0;
  overflow: hidden;
  font-size: var(--pt-fz-body);
  font-weight: 700;
  line-height: 17px;
  color: var(--pt-t1);
  text-decoration: none;
  text-overflow: ellipsis;
  white-space: nowrap;
}

a.hm-tc__title:hover {
  color: var(--pt-p);
}

.hm-tc__sub {
  margin: 3px 12px 0;
  overflow: hidden;
  font-size: var(--pt-fz-label);
  line-height: 15px;
  color: var(--pt-t3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hm-tc__act,
.hm-tc__note {
  display: inline-flex;
  gap: 5px;
  align-items: center;
  margin: 5px 12px 0;
  overflow: hidden;
  font-size: var(--pt-fz-sm);
  line-height: 16px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hm-tc__act {
  align-self: flex-start;
  font-weight: 500;
  color: var(--pt-p);
  text-decoration: none;
}

.hm-tc__act:hover {
  text-decoration: underline;
}

.hm-tc__note {
  color: var(--pt-t3);
}

/* 手机：横滑卡 240×196，没有动作行（整张卡可点），时刻并进副行 */
.hm-today.is-mobile .hm-today__row {
  gap: 12px;
  margin: 0 -16px;
  padding: 0 16px;
  scroll-padding: 0 16px;
}

.hm-today.is-mobile .hm-tc {
  flex-basis: 240px;
  height: 196px;
}

.hm-today.is-mobile .hm-tc__img {
  flex-basis: 135px;
  height: 135px;
}

.hm-tc.is-tap:active {
  background: var(--pt-hover);
}

/* 翻页圆点 6×6、一个占 12（画板 07 的 today dot 1–5），在卡片下 8、整排居中 */
.hm-dots {
  display: flex;
  gap: 6px;
  justify-content: center;
  margin-top: 8px;
}

.hm-dots__dot {
  width: 6px;
  height: 6px;
  background: var(--pt-border-strong);
  border-radius: 50%;
}

.hm-dots__dot.is-on {
  background: var(--pt-t2);
}
</style>
