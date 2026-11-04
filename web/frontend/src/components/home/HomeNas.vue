<script setup lang="ts">
/*
 * 首页的「NAS 状态」（画板 02 的 368×588 面板、07 的 343×368 行卡）：上下行速度与最近几拍的速度柱、各块盘的剩余、
 * 各台下载器（手机不列）、体检、备份与媒体服务器三条状态行。速度跟着外壳每 30 秒取的 transfer-stats 更新，
 * 别的跟着首页的接口；各部分单独出错，在那一部分写原因。
 */
import type { HomeBlock, HomeDisk, HomeDownloaders, HomeStatus, SpeedSample } from "@/api";
import PtIcon from "@/components/PtIcon";
import HomeSection from "@/components/home/HomeSection.vue";
import PtBars from "@/components/ui/PtBars.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtProgress from "@/components/ui/PtProgress.vue";
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
}>();

const isMobile = useIsMobile();
const runtime = useRuntimeStore();

const dls = computed(() => props.downloaders?.data ?? null);

/** 速度柱：外壳取到的新采样比首页接口给的新时用外壳的 */
const history = computed<SpeedSample[]>(() =>
  runtime.speedHistory.length ? runtime.speedHistory : (dls.value?.speed_history ?? []),
);

/** 总速度：外壳这一拍取到了就用它（更新），否则用首页接口给的 */
const down = computed(() =>
  !runtime.stale && runtime.downloadSpeed !== null ? runtime.downloadSpeed : (dls.value?.down ?? 0),
);
const up = computed(() =>
  !runtime.stale && runtime.uploadSpeed !== null ? runtime.uploadSpeed : (dls.value?.up ?? 0),
);

const downBars = computed(() => {
  const v = history.value.map((s) => s.down);
  return v.length ? v : [down.value];
});
const upBars = computed(() => {
  const v = history.value.map((s) => s.up);
  return v.length ? v : [up.value];
});
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

const disks = computed(() => {
  const list = props.storage?.data ?? [];
  return isMobile.value ? mobileDisks(list) : list;
});

const lines = computed<StatusLine[]>(() => {
  const st = props.status?.data;
  if (!st) return [];
  return [healthLine(st.health), backupLine(st.backup), serverLine(st.servers, props.resumeCount)];
});

function lineKey(l: StatusLine): string {
  return l.key;
}

/* 桌面是带卡头的面板（画板 02）；手机的标题在卡外，同别的区块标题（画板 07） */
const TITLE = "NAS 状态";
const wrap = computed(() =>
  isMobile.value
    ? { is: HomeSection, props: { title: TITLE, action: "系统体检", to: "/system/check" } }
    : { is: PtPanel, props: { title: TITLE, icon: "hard-drive", padding: "none" as const } },
);
</script>

<template>
  <component :is="wrap.is" v-bind="wrap.props" data-testid="hm-nas">
    <template v-if="!isMobile" #actions>
      <router-link to="/system/check" class="hm-nas__link">
        系统体检<PtIcon name="chevron-right" :size="14" />
      </router-link>
    </template>
    <div :class="{ 'hm-mcard': isMobile }">
      <!-- 速度 -->
      <div class="hm-nas__part hm-speed" data-testid="hm-speed">
        <p v-if="downloaders?.error" class="hm-nas__err">取不到：{{ downloaders.error }}</p>
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
              :gap="isMobile ? 3 : 3"
              :height="isMobile ? 24 : 26"
              baseline="zero" />
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
              :gap="isMobile ? 3 : 3"
              :height="isMobile ? 24 : 26"
              baseline="zero" />
          </div>
        </template>
      </div>

      <!-- 存储 -->
      <div class="hm-nas__part hm-storage" data-testid="hm-storage">
        <div v-if="!isMobile" class="hm-nas__group">存储</div>
        <p v-if="storage?.error" class="hm-nas__err">取不到：{{ storage.error }}</p>
        <p v-else-if="!storage" class="hm-nas__err">加载中…</p>
        <p v-else-if="!disks.length" class="hm-nas__err">没有下载盘与媒体库</p>
        <div
          v-for="(d, i) in disks"
          :key="`${d.kind}-${d.path}-${i}`"
          class="hm-disk"
          :data-testid="`hm-disk-${i}`">
          <div class="hm-disk__head">
            <span class="hm-disk__name" :title="`${d.label} ${d.path}`">
              {{ d.label }}<span v-if="d.path" class="hm-disk__path"> {{ d.path }}</span>
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

      <!-- 下载器（手机不列：速度里已经有了） -->
      <div v-if="!isMobile" class="hm-nas__part hm-dls" data-testid="hm-downloaders">
        <div class="hm-nas__group">下载器</div>
        <p v-if="downloaders?.error" class="hm-nas__err">取不到：{{ downloaders.error }}</p>
        <p v-else-if="dls && !downloaderRows.length" class="hm-nas__err">没有启用的下载器</p>
        <div v-for="d in downloaderRows" :key="d.id" class="hm-dl" :data-testid="`hm-dl-${d.id}`">
          <i class="hm-dl__dot" :class="d.online ? 'is-ok' : 'is-dang'" />
          <span class="hm-dl__name" :title="d.name">{{ d.name }}</span>
          <span class="hm-dl__speed" :class="{ 'is-off': !d.online }" :title="downloaderLine(d)">{{
            downloaderLine(d)
          }}</span>
        </div>
      </div>

      <!-- 状态行 -->
      <div class="hm-nas__part hm-lines" data-testid="hm-lines">
        <p v-if="status?.error" class="hm-nas__err">取不到：{{ status.error }}</p>
        <div v-for="l in lines" :key="lineKey(l)" class="hm-line" :data-testid="`hm-line-${l.key}`">
          <PtIcon :name="l.icon" :size="16" class="hm-line__icon" :class="`is-${l.tone}`" />
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
    </div>
  </component>
</template>

<style scoped>
.hm-nas__link {
  display: inline-flex;
  gap: 2px;
  align-items: center;
  font-size: var(--pt-fz-sm);
  font-weight: 500;
  color: var(--pt-p);
  text-decoration: none;
}

.hm-nas__link:hover {
  text-decoration: underline;
}

.hm-nas__part {
  padding: 12px 16px;
}

.hm-nas__part + .hm-nas__part {
  border-top: 1px solid var(--pt-border);
}

.hm-nas__group {
  margin-bottom: 8px;
  font-size: var(--pt-fz-label);
  font-weight: 700;
  color: var(--pt-t3);
}

.hm-nas__err {
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

/* 各部分的底落在画板的分隔线上（130、296、392） */
.hm-storage {
  padding-bottom: 20px;
}

.hm-dls {
  padding-bottom: 9px;
}

.hm-speed .hm-nas__err {
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

/* 磁盘：一块一行，行距 42（名字 17、条在名字顶下 22） */
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

/* 状态行：一行 58，图标 16、标题（顶在行首）、说明在标题下 20，右侧时间或动作 */
.hm-lines {
  padding-top: 13px;
  padding-bottom: 7px;
}

.hm-line {
  position: relative;
  display: flex;
  gap: 8px;
  align-items: flex-start;
  min-height: 58px;
}

.hm-line__icon {
  flex: 0 0 auto;
  margin-top: 2px;
  color: var(--pt-t2);
}

.hm-line__icon.is-ok {
  color: var(--pt-ok);
}

.hm-line__icon.is-warn {
  color: var(--pt-warn);
}

.hm-line__icon.is-dang {
  color: var(--pt-dang);
}

.hm-line__text {
  flex: 1 1 auto;
  min-width: 0;
}

.hm-line__title {
  overflow: hidden;
  font-size: var(--pt-fz-body);
  font-weight: 500;
  line-height: 18px;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hm-line__desc {
  margin-top: 2px;
  overflow: hidden;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hm-line__right,
.hm-line__act {
  flex: 0 0 auto;
  margin-top: 2px;
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

/* 手机：卡外是区块标题，卡是行卡样式（白底、1px 描边、圆角 8，没有阴影） */
.hm-mcard {
  overflow: hidden;
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-lg);
}

/* 手机（画板 07 的 NAS 状态卡 343×368）：速度两列 x 14 与 180、条 148×24 在 60，分隔在 98 与 200；盘 110 起、行距 42；
   状态行 214 起、一行 50，标题在行首下 12，没有说明行 */
@media (max-width: 768px) {
  .hm-nas__part {
    padding: 11px 14px 20px;
  }

  .hm-speed {
    gap: 18px;
    padding: 13px 14px 14px;
  }

  .hm-lines {
    padding: 13px 14px 3px;
  }

  .hm-line {
    min-height: 50px;
    padding-top: 12px;
  }

  /* 行间分隔从标题的位置起（卡里 x 40） */
  .hm-line + .hm-line {
    border-top: 0;
  }

  .hm-line + .hm-line::before {
    position: absolute;
    top: 0;
    right: 0;
    left: 26px;
    height: 1px;
    background: var(--pt-border);
    content: "";
  }

  .hm-line__right,
  .hm-line__act {
    font-size: var(--pt-fz-sm);
  }
}
</style>
