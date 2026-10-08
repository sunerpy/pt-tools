<script setup lang="ts">
/*
 * 订阅详情：剧集每一集的情况（已入库、下载中、缺、还没播），以及订阅下载过的种子（站点、集、分数、状态）。
 */
import { type SubscriptionDetail, subscribeApi } from "@/api";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { formatShortDateTime } from "@/utils/format";
import {
  episodeState,
  progressText,
  sourceLabel,
  subName,
  subStatus,
  torrentSpan,
  torrentStatus,
} from "@/utils/subscribe";
import { computed, ref, watch } from "vue";

const props = defineProps<{ modelValue: boolean; id: number | null }>();
const emit = defineEmits<{ (e: "update:modelValue", v: boolean): void }>();

const isMobile = useIsMobile();
const visible = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emit("update:modelValue", v),
});
const detail = ref<SubscriptionDetail | null>(null);
const ds = useDataState();

async function load() {
  if (!props.id) return;
  const pending = ds.run(() => subscribeApi.get(props.id as number));
  const got = await pending;
  if (ds.isStale(pending)) return;
  detail.value = got;
}

watch(
  () => [props.modelValue, props.id] as const,
  ([open]) => {
    if (open) {
      detail.value = null;
      void load();
    }
  },
  { immediate: true },
);
</script>

<template>
  <el-dialog
    v-model="visible"
    class="pt-dialog"
    title="订阅详情"
    :width="isMobile ? '94%' : '760px'"
    append-to-body
    align-center
    data-testid="sub-detail">
    <PtDataState
      v-if="!detail"
      :state="ds.state.value"
      :sub="ds.errorText.value"
      data-testid="sub-detail-state" />
    <div v-else class="sdd">
      <div class="sdd__head">
        <div class="sdd__name" data-testid="sub-detail-name">{{ subName(detail) }}</div>
        <div class="sdd__sub">
          <PtStatusPill :tone="subStatus(detail.status).tone" size="sm">{{
            subStatus(detail.status).label
          }}</PtStatusPill>
          <span>{{ sourceLabel(detail.source) }}</span>
          <span v-if="detail.upgrade">洗版</span>
          <span>{{ progressText(detail.media_type, detail.progress) }}</span>
        </div>
        <div v-if="detail.message" class="sdd__sub" data-testid="sub-detail-message">
          {{ detail.message }}
        </div>
        <div class="sdd__sub">
          <template v-if="detail.last_search_at"
            >上次搜索 {{ formatShortDateTime(detail.last_search_at) }}</template
          >
          <template v-if="detail.next_search_at && detail.status === 'active'">
            · 下次 {{ formatShortDateTime(detail.next_search_at) }}</template
          >
        </div>
      </div>

      <div v-if="detail.progress?.episodes?.length" class="sdd__section">
        <div class="sdd__label">第 {{ detail.season }} 季的分集</div>
        <div class="sdd__eps" data-testid="sub-detail-episodes">
          <div v-for="e in detail.progress.episodes" :key="e.number" class="sdd__ep">
            <span class="sdd__epno">E{{ String(e.number).padStart(2, "0") }}</span>
            <span class="sdd__epname">{{ e.name || "" }}</span>
            <span class="sdd__epdate">{{ e.air_date || "" }}</span>
            <PtStatusPill :tone="episodeState(e.state).tone" size="sm">{{
              episodeState(e.state).label
            }}</PtStatusPill>
          </div>
        </div>
      </div>

      <div class="sdd__section">
        <div class="sdd__label">下载过的种子</div>
        <p v-if="!detail.torrent_list.length" class="sdd__sub">还没有下载过</p>
        <div v-else class="sdd__torrents" data-testid="sub-detail-torrents">
          <div v-for="t in detail.torrent_list" :key="t.id" class="sdd__torrent">
            <div class="sdd__ttitle">{{ t.title }}</div>
            <div class="sdd__sub">
              <PtStatusPill :tone="torrentStatus(t.status).tone" size="sm">{{
                torrentStatus(t.status).label
              }}</PtStatusPill>
              <span>{{ t.site_name }}</span>
              <span v-if="torrentSpan(t)">{{ torrentSpan(t) }}</span>
              <span>{{ t.score }} 分</span>
              <span>{{ formatShortDateTime(t.created_at) }}</span>
            </div>
            <div v-if="t.message" class="sdd__sub">{{ t.message }}</div>
          </div>
        </div>
      </div>
    </div>
    <template #footer>
      <el-button @click="visible = false">关闭</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.sdd {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
}

.sdd__head {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.sdd__name {
  font-size: 15px;
  font-weight: 600;
  color: var(--pt-t1);
}

.sdd__sub {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 10px;
  font-size: 12px;
  color: var(--pt-t3);
  word-break: break-all;
}

.sdd__section {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.sdd__label {
  font-size: 12px;
  font-weight: 600;
  color: var(--pt-t2);
}

.sdd__eps {
  display: flex;
  flex-direction: column;
  border: 1px solid var(--pt-border);
  border-radius: 6px;
}

.sdd__ep {
  display: grid;
  grid-template-columns: 44px minmax(0, 1fr) 92px 64px;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  font-size: 13px;
  color: var(--pt-t2);
}

.sdd__ep + .sdd__ep {
  border-top: 1px solid var(--pt-border);
}

.sdd__epno {
  font-variant-numeric: tabular-nums;
  color: var(--pt-t1);
}

.sdd__epname {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sdd__epdate {
  font-size: 12px;
  color: var(--pt-t3);
  font-variant-numeric: tabular-nums;
}

.sdd__torrents {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.sdd__torrent {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.sdd__ttitle {
  font-size: 13px;
  color: var(--pt-t1);
  word-break: break-all;
}

@media (max-width: 767px) {
  .sdd__ep {
    grid-template-columns: 40px minmax(0, 1fr) 60px;
  }

  .sdd__epdate {
    display: none;
  }
}
</style>
