<script setup lang="ts">
/*
 * 首页右栏（首页 NAS 概览 H2，规格 docs/design/home-nas-spec.md）：「下载器」（上下行总速度与最近几拍的速度柱、
 * 各台下载器，手机不列）与「状态」（体检、备份与媒体服务器三行）。原来的「NAS 状态」面板拆成这里与上面的 NAS 概览。
 * 首页的 NAS 概览没在画「存储」时（来源不是本机，或者一个来源都没有），下载盘与媒体库的剩余列在这里，不从首页消失。
 * 速度跟着外壳每 30 秒取的 transfer-stats 更新，别的跟着首页的接口；各部分单独出错，在那一部分写原因。
 */
import type { HomeBlock, HomeDisk, HomeDownloaders, HomeStatus } from "@/api";
import PtIcon from "@/components/PtIcon";
import HomeSection from "@/components/home/HomeSection.vue";
import PtBars from "@/components/ui/PtBars.vue";
import PtProgress from "@/components/ui/PtProgress.vue";
import { useHomeSpeed } from "@/composables/useHomeSpeed";
import { useIsMobile } from "@/composables/useIsMobile";
import { useRuntimeStore } from "@/stores/runtime";
import {
  type StatusLine,
  backupLine,
  diskBar,
  diskText,
  downloaderLine,
  healthLine,
  mobileDisks,
  serverLine,
  speedHint,
  speedLabels,
  speedText,
} from "@/utils/home";
import { computed } from "vue";

const props = defineProps<{
  downloaders: HomeBlock<HomeDownloaders> | null;
  storage: HomeBlock<HomeDisk[]> | null;
  status: HomeBlock<HomeStatus> | null;
  /** 继续观看的条数（媒体服务器那一行的说明；没取到时为 null） */
  resumeCount: number | null;
  /** 列不列下载盘与媒体库（NAS 概览的本机来源已经画了时不列） */
  showStorage: boolean;
}>();

const isMobile = useIsMobile();
const runtime = useRuntimeStore();

const { dls, history, lastAt, down, up, downBars, upBars } = useHomeSpeed(() => props.downloaders);
const labels = computed(() => speedLabels(dls.value, isMobile.value));
const hint = computed(() => speedHint(history.value));

/** 每台下载器的速度：外壳这一拍取到的那台用外壳的 */
const downloaderRows = computed(() =>
  (dls.value?.downloaders ?? []).map((d) => {
    const live = runtime.downloaderStats.find((s) => s.downloader_id === d.id);
    if (!live || runtime.stale) return d;
    return {
      ...d,
      online: live.reachable,
      down: live.download_speed,
      up: live.upload_speed,
      error: live.reachable ? undefined : (live.error ?? d.error),
    };
  }),
);
const dlCount = computed(() => (dls.value ? `${dls.value.downloaders.length} 台` : undefined));

const disks = computed(() => {
  const list = props.storage?.data ?? [];
  return isMobile.value ? mobileDisks(list) : list;
});

const lines = computed<StatusLine[]>(() => {
  const st = props.status?.data;
  if (!st) return [];
  return [healthLine(st.health), backupLine(st.backup), serverLine(st.servers, props.resumeCount)];
});
</script>

<template>
  <div class="hm-side" data-testid="hm-side">
    <HomeSection title="下载器" :count="dlCount" action="任务列表" to="/tasks">
      <div class="hm-card" data-testid="hm-dl-card">
        <!-- 速度 -->
        <div class="hm-side__part hm-speed" data-testid="hm-speed">
          <p v-if="downloaders?.error" class="hm-side__err">取不到：{{ downloaders.error }}</p>
          <template v-else>
            <div class="hm-speed__col" :title="hint">
              <div class="hm-speed__label">
                <PtIcon name="download" :size="14" class="hm-speed__icon is-down" />
                <span>{{ labels.down }}</span>
              </div>
              <div class="hm-speed__value" data-testid="hm-speed-down">{{ speedText(down) }}</div>
              <PtBars
                :values="downBars"
                hue="var(--pt-kpi-1)"
                :count="20"
                :bar-width="isMobile ? 4 : 5"
                :gap="3"
                :height="isMobile ? 24 : 26"
                baseline="zero"
                :push-key="lastAt" />
            </div>
            <div class="hm-speed__col" :title="hint">
              <div class="hm-speed__label">
                <PtIcon name="upload" :size="14" class="hm-speed__icon is-up" />
                <span>{{ labels.up }}</span>
              </div>
              <div class="hm-speed__value" data-testid="hm-speed-up">{{ speedText(up) }}</div>
              <PtBars
                :values="upBars"
                hue="var(--pt-kpi-2)"
                :count="20"
                :bar-width="isMobile ? 4 : 5"
                :gap="3"
                :height="isMobile ? 24 : 26"
                baseline="zero"
                :push-key="lastAt" />
            </div>
          </template>
        </div>

        <!-- 各台下载器（手机不列：速度里已经有了） -->
        <div v-if="!isMobile" class="hm-side__part hm-dls" data-testid="hm-downloaders">
          <p v-if="downloaders?.error" class="hm-side__err">取不到：{{ downloaders.error }}</p>
          <p v-else-if="dls && !downloaderRows.length" class="hm-side__err">没有启用的下载器</p>
          <div v-for="d in downloaderRows" :key="d.id" class="hm-dl" :data-testid="`hm-dl-${d.id}`">
            <i class="hm-dl__dot" :class="d.online ? 'is-ok' : 'is-dang'" />
            <span class="hm-dl__name" :title="d.name">{{ d.name }}</span>
            <span
              class="hm-dl__speed"
              :class="{ 'is-off': !d.online }"
              :title="downloaderLine(d)"
              >{{ downloaderLine(d) }}</span
            >
          </div>
        </div>
      </div>
    </HomeSection>

    <!-- 存储：NAS 概览没画本机的存储时列在这里 -->
    <HomeSection v-if="showStorage" title="存储" count="下载盘与媒体库">
      <div class="hm-card hm-side__part hm-storage" data-testid="hm-storage">
        <p v-if="storage?.error" class="hm-side__err">取不到：{{ storage.error }}</p>
        <p v-else-if="!storage" class="hm-side__err">加载中…</p>
        <p v-else-if="!disks.length" class="hm-side__err">没有下载盘与媒体库</p>
        <div
          v-for="(d, i) in disks"
          :key="`${d.kind}-${d.path}-${i}`"
          class="hm-disk"
          :data-testid="`hm-disk-${i}`">
          <div class="hm-disk__head">
            <span class="hm-disk__name" :title="`${d.label} ${d.path}`">
              <!-- 空格放在 span 外面：span 里开头的空白会被模板编译去掉，名字和路径就连在一起了 -->
              {{ d.label }} <span v-if="d.path" class="hm-disk__path">{{ d.path }}</span>
            </span>
            <span
              class="hm-disk__free"
              :class="{ 'is-dang': d.low, 'is-err': d.error }"
              :title="d.error ? diskText(d) : undefined">
              {{ diskText(d, isMobile) }}
            </span>
          </div>
          <PtProgress
            v-if="!d.error && diskBar(d).percent !== null"
            :percent="diskBar(d).percent!"
            :tone="diskBar(d).tone"
            class="hm-disk__bar" />
        </div>
      </div>
    </HomeSection>

    <HomeSection title="状态" action="系统体检" to="/system/check">
      <div class="hm-card hm-side__part hm-lines" data-testid="hm-lines">
        <p v-if="status?.error" class="hm-side__err">取不到：{{ status.error }}</p>
        <div v-for="l in lines" :key="l.key" class="hm-line" :data-testid="`hm-line-${l.key}`">
          <span class="hm-line__ic" :class="`is-${l.tone}`">
            <PtIcon :name="l.icon" :size="15" />
          </span>
          <div class="hm-line__text">
            <div class="hm-line__title" :title="l.title">{{ l.title }}</div>
            <div v-if="!isMobile" class="hm-line__desc" :title="l.desc">{{ l.desc }}</div>
          </div>
          <a
            v-if="l.href && l.right"
            :href="l.href"
            target="_blank"
            rel="noopener noreferrer"
            class="hm-line__act">
            {{ l.right }}
          </a>
          <router-link
            v-else-if="l.to && l.right"
            :to="l.to"
            class="hm-line__right"
            :class="l.rightKind === 'action' ? 'hm-line__act' : 'hm-line__time'">
            {{ l.right }}
          </router-link>
        </div>
      </div>
    </HomeSection>
  </div>
</template>

<style scoped>
/* 右栏：三块从上往下，块间 20（同左栏的区块间距） */
.hm-side {
  display: flex;
  flex-direction: column;
  gap: 20px;
  min-width: 0;
}

/* 卡：白底、1px 描边、圆角 8，没有阴影（同首页别的区块卡） */
.hm-card {
  overflow: hidden;
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-lg);
}

.hm-side__part {
  padding: 12px 16px;
}

.hm-side__part + .hm-side__part {
  border-top: 1px solid var(--pt-border);
}

.hm-side__err {
  margin: 0;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
}

/* 速度：两列 */
.hm-speed {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
  padding-top: 13px;
  padding-bottom: 16px;
}

.hm-speed .hm-side__err {
  grid-column: 1 / -1;
}

/* 一列：标签太长时省略，不撑宽这一列 */
.hm-speed__col {
  min-width: 0;
}

.hm-speed__label {
  display: flex;
  gap: 6px;
  align-items: center;
  min-width: 0;
  font-size: var(--pt-fz-label);
  font-weight: 500;
  color: var(--pt-t3);
  white-space: nowrap;
}

.hm-speed__label span {
  overflow: hidden;
  text-overflow: ellipsis;
}

.hm-speed__icon.is-down {
  color: var(--pt-kpi-1);
}

.hm-speed__icon.is-up {
  color: var(--pt-kpi-2);
}

.hm-speed__value {
  margin: 4px 0 3px;
  font-size: var(--pt-fz-display);
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  line-height: 24px;
  color: var(--pt-t1);
}

.hm-dls {
  padding-bottom: 9px;
}

/* 下载器：一台一行，行距 26（名字 18 高，圆点在名字顶下 6） */
.hm-dl {
  display: flex;
  gap: 7px;
  align-items: flex-start;
  min-height: 26px;
}

.hm-dl__dot {
  flex: 0 0 auto;
  width: 7px;
  height: 7px;
  margin-top: 6px;
  border-radius: 50%;
}

.hm-dl__dot.is-ok {
  background: var(--pt-ok);
}

.hm-dl__dot.is-dang {
  background: var(--pt-dang);
}

/* 名字不让给速度：连不上时右边那句原因很长，截原因不截名字 */
.hm-dl__name {
  flex: 0 0 auto;
  max-width: 50%;
  min-width: 0;
  overflow: hidden;
  font-size: var(--pt-fz-body);
  font-weight: 700;
  line-height: 18px;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hm-dl__speed {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  font-size: var(--pt-fz-label);
  line-height: 18px;
  font-variant-numeric: tabular-nums;
  color: var(--pt-t2);
  text-align: right;
  text-overflow: ellipsis;
  white-space: pre;
}

.hm-dl__speed.is-off {
  color: var(--pt-t3);
}

/* 存储：一块盘一行，行距 42（名字 17、条在名字顶下 22） */
.hm-storage {
  padding-bottom: 16px;
}

.hm-disk + .hm-disk {
  margin-top: 15px;
}

.hm-disk__head {
  display: flex;
  gap: 12px;
  align-items: baseline;
  min-width: 0;
}

.hm-disk__name {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  font-size: var(--pt-fz-body);
  font-weight: 500;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hm-disk__path {
  font-size: var(--pt-fz-label);
  font-weight: 400;
  color: var(--pt-t3);
}

.hm-disk__free {
  flex: 0 0 auto;
  max-width: 55%;
  overflow: hidden;
  font-size: var(--pt-fz-label);
  font-variant-numeric: tabular-nums;
  color: var(--pt-t2);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hm-disk__free.is-dang {
  color: var(--pt-dang);
}

.hm-disk__free.is-err {
  color: var(--pt-t3);
}

.hm-disk__bar {
  margin-top: 5px;
}

/* 状态行：一行 52，图标在 28 的浅底方块里，标题与说明两行，右侧时间或动作 */
.hm-lines {
  padding: 0;
}

.hm-lines .hm-side__err {
  padding: 12px 16px;
}

.hm-line {
  display: flex;
  gap: 10px;
  align-items: center;
  min-height: 52px;
  padding: 8px 16px;
}

.hm-line + .hm-line {
  border-top: 1px solid var(--pt-border);
}

.hm-line__ic {
  display: grid;
  flex: 0 0 auto;
  place-items: center;
  width: 28px;
  height: 28px;
  color: var(--pt-t2);
  background: var(--pt-hover);
  border-radius: var(--pt-r-md);
}

.hm-line__ic.is-ok {
  color: var(--pt-ok);
  background: var(--pt-ok-soft);
}

.hm-line__ic.is-warn {
  color: var(--pt-warn);
  background: var(--pt-warn-soft);
}

.hm-line__ic.is-dang {
  color: var(--pt-dang);
  background: var(--pt-dang-soft);
}

.hm-line__text {
  flex: 1 1 auto;
  min-width: 0;
}

.hm-line__title {
  overflow: hidden;
  font-size: var(--pt-fz-body);
  font-weight: 600;
  line-height: 18px;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hm-line__desc {
  margin-top: 1px;
  overflow: hidden;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hm-line__right,
.hm-line__act {
  flex: 0 0 auto;
  font-size: var(--pt-fz-label);
  text-decoration: none;
}

.hm-line__time {
  color: var(--pt-t3);
}

.hm-line__act {
  font-weight: 500;
  color: var(--pt-p);
}

.hm-line__act:hover,
.hm-line__time:hover {
  text-decoration: underline;
}

/* 手机（画板 07 的区块卡 343 宽）：内边距 14，状态行只有标题、一行 48 */
@media (max-width: 768px) {
  .hm-side {
    gap: 24px;
  }

  .hm-side__part {
    padding: 11px 14px 16px;
  }

  .hm-speed {
    gap: 18px;
    padding: 13px 14px 14px;
  }

  .hm-lines {
    padding: 0;
  }

  .hm-line {
    min-height: 48px;
    padding: 8px 14px;
  }

  .hm-line__right,
  .hm-line__act {
    font-size: var(--pt-fz-sm);
  }
}
</style>
