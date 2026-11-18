<script setup lang="ts">
/*
 * 首页的「这周还要播」（画板 02 的 760×116 面板四列、07 的行卡四行）：明天起 6 天播出的集与上映的电影，只列前四条，
 * 其余看日历。点一条进条目详情。桌面是带卡头的面板；手机的标题在卡外（同别的区块标题），卡里只有四行。
 */
import type { CalendarEntry, HomeBlock, HomeCalendar } from "@/api";
import PtIcon from "@/components/PtIcon";
import HomeSection from "@/components/home/HomeSection.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import { useIsMobile } from "@/composables/useIsMobile";
import { homeItemLink, weekLine } from "@/utils/home";
import { computed } from "vue";

const props = defineProps<{ block: HomeBlock<HomeCalendar> | null }>();

/** 只列前几条：桌面四列、手机四行 */
const WEEK_MAX = 4;

const isMobile = useIsMobile();
const entries = computed(() => (props.block?.data?.week ?? []).slice(0, WEEK_MAX));

const TITLE = "这周还要播";
const wrap = computed(() =>
  isMobile.value
    ? { is: HomeSection, props: { title: TITLE, action: "看日历", to: "/media/calendar" } }
    : { is: PtPanel, props: { title: TITLE, icon: "calendar-days", padding: "none" as const } },
);

function key(e: CalendarEntry): string {
  return `${e.subscription_id}-${e.season ?? 0}-${e.episode ?? 0}-${e.air_date}`;
}
</script>

<template>
  <component :is="wrap.is" v-bind="wrap.props" data-testid="hm-week">
    <template v-if="!isMobile" #actions>
      <router-link to="/media/calendar" class="hm-week__link">
        看日历<PtIcon name="chevron-right" :size="14" />
      </router-link>
    </template>
    <div :class="{ 'hm-mcard': isMobile }">
      <p v-if="block?.error" class="hm-week__empty">取不到：{{ block.error }}</p>
      <p v-else-if="!block" class="hm-week__empty">加载中…</p>
      <p v-else-if="!entries.length" class="hm-week__empty" data-testid="hm-week-empty">
        这周没有要播的集
      </p>
      <div v-else class="hm-week" :class="{ 'is-mobile': isMobile }">
        <router-link
          v-for="e in entries"
          :key="key(e)"
          :to="homeItemLink(e.media_type, e.tmdb_id, e.season, e.episode)"
          class="hm-wk"
          :data-testid="`hm-wk-${key(e)}`">
          <span class="hm-wk__date">{{ weekLine(e).date }}</span>
          <span class="hm-wk__body">
            <span class="hm-wk__title" :title="weekLine(e).title">{{ weekLine(e).title }}</span>
            <span class="hm-wk__note">{{ weekLine(e).note }}</span>
          </span>
        </router-link>
      </div>
    </div>
  </component>
</template>

<style scoped>
.hm-week__link {
  display: inline-flex;
  gap: 2px;
  align-items: center;
  font-size: var(--pt-fz-sm);
  font-weight: 500;
  color: var(--pt-p);
  text-decoration: none;
}

.hm-week__link:hover {
  text-decoration: underline;
}

.hm-week__empty {
  margin: 0;
  padding: 16px;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
}

/* 桌面：四列等分，列间有竖线（第一列前没有） */
.hm-week {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  padding: 12px 0 20px 16px;
}

.hm-wk {
  position: relative;
  display: flex;
  flex-direction: column;
  min-width: 0;
  padding: 0 24px 0 0;
  color: inherit;
  text-decoration: none;
}

.hm-wk:last-child {
  padding-right: 16px;
}

.hm-wk + .hm-wk::before {
  position: absolute;
  top: 2px;
  left: -12px;
  width: 1px;
  height: 52px;
  background: var(--pt-border);
  content: "";
}

.hm-wk:focus-visible {
  border-radius: var(--pt-r-sm);
  outline: 2px solid var(--pt-p);
  outline-offset: 2px;
}

.hm-wk__date {
  font-size: var(--pt-fz-label);
  font-weight: 500;
  line-height: 15px;
  color: var(--pt-t3);
}

.hm-wk__body {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.hm-wk__title {
  margin-top: 3px;
  overflow: hidden;
  font-size: var(--pt-fz-body);
  font-weight: 500;
  line-height: 17px;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hm-wk:hover .hm-wk__title {
  color: var(--pt-p);
}

.hm-wk__note {
  margin-top: 3px;
  overflow: hidden;
  font-size: var(--pt-fz-label);
  line-height: 15px;
  color: var(--pt-t3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 手机：卡外是区块标题，卡是行卡样式（白底、1px 描边、圆角 8，没有阴影） */
.hm-mcard {
  overflow: hidden;
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-lg);
}

/* 手机：一行一条（日期在左 82 宽），行高 52，行间分隔 */
.hm-week.is-mobile {
  display: block;
  padding: 0 14px;
}

/* 行里：日期在行首下 17，标题 9、说明 28（画板 07 的这周还要播卡） */
.hm-week.is-mobile .hm-wk {
  box-sizing: border-box;
  flex-direction: row;
  align-items: flex-start;
  height: 52px;
  padding: 9px 0 0;
  border-bottom: 1px solid var(--pt-border);
}

.hm-week.is-mobile .hm-wk:last-child {
  border-bottom: 0;
}

.hm-week.is-mobile .hm-wk + .hm-wk::before {
  display: none;
}

.hm-week.is-mobile .hm-wk__date {
  flex: 0 0 82px;
  margin-top: 8px;
}

.hm-week.is-mobile .hm-wk__title {
  margin-top: 0;
}
</style>
