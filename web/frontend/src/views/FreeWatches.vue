<script setup lang="ts">
/*
 * 等免费（媒体中心 M2b）：搜索结果里点了「等免费」的种子。后台每 5 分钟看一次在等的种子的优惠，免费了（离免费结束还多于
 * 免费期结束前提前处理的分钟数）就推送到记下的下载器；到期还没免费就不再等。这一页能看每条在等的情况，取消，改到期时间。
 * 画板没有这一页，沿用下载组列表页的样式：页头 + 筛选 + 表格，手机是行卡。
 */
import {
  type DownloaderSetting,
  downloadersApi,
  type FreeWatch,
  freeWatchApi,
  type FreeWatchStatus,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { formatBytes, formatShortDateTime } from "@/utils/format";
import {
  checkText,
  expiryDisabled,
  expiryProblem,
  FREE_WATCH_FILTERS,
  filterLabel,
  freeWatchStatus,
} from "@/utils/freeWatch";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onBeforeUnmount, onMounted, ref } from "vue";

const isMobile = useIsMobile();
const errText = (e: unknown, fallback: string) => (e as Error)?.message || fallback;

/** 列表最多列几条（同服务端 freewatch.ListLimit） */
const LIST_LIMIT = 500;

const status = ref<"" | FreeWatchStatus>("");
const items = ref<FreeWatch[]>([]);
const counts = ref<Partial<Record<FreeWatchStatus, number>>>({});
const downloaders = ref<DownloaderSetting[]>([]);
const ds = useDataState({ filtered: () => Boolean(status.value) });

async function load() {
  const pending = ds.run(() => freeWatchApi.list(status.value || undefined));
  const got = await pending;
  if (ds.isStale(pending) || !got) return;
  items.value = got.items ?? [];
  counts.value = got.counts ?? {};
}

async function loadDownloaders() {
  try {
    downloaders.value = await downloadersApi.list();
  } catch {
    downloaders.value = [];
  }
}

/** 这个筛选下一共几条（列表最多列 LIST_LIMIT 条） */
const total = computed(() =>
  status.value
    ? (counts.value[status.value] ?? 0)
    : Object.values(counts.value).reduce((a, b) => a + (b ?? 0), 0),
);

const downloaderName = (id: number) =>
  downloaders.value.find((d) => d.id === id)?.name ?? `下载器 ${id}`;

/** 种子下面那一行：站点、体积、H&R */
function metaText(w: FreeWatch): string {
  return [
    w.site_name,
    w.size_bytes > 0 ? formatBytes(w.size_bytes) : "",
    w.has_hr ? (w.hr_seed_time_h > 0 ? `H&R ${w.hr_seed_time_h} 时` : "H&R") : "",
  ]
    .filter(Boolean)
    .join(" · ");
}

/** 推到哪里：路径没填时用下载器的默认路径 */
function targetText(w: FreeWatch): string {
  return [w.save_path || "默认路径", w.category, w.tags].filter(Boolean).join(" · ");
}

/** 在等的写下次什么时候看；别的写结束的时间 */
function timeText(w: FreeWatch): string {
  if (w.status === "watching")
    return w.next_check_at ? `下次 ${formatShortDateTime(w.next_check_at)}` : "马上看";
  return `结束于 ${formatShortDateTime(w.updated_at)}`;
}

// ---- 取消 ----
const cancelling = ref<number | null>(null);
async function cancel(w: FreeWatch) {
  try {
    await ElMessageBox.confirm(`不再等「${w.title || w.torrent_id}」免费了？`, "取消等免费", {
      type: "warning",
      confirmButtonText: "取消等免费",
      cancelButtonText: "接着等",
    });
  } catch {
    return;
  }
  cancelling.value = w.id;
  try {
    await freeWatchApi.cancel(w.id);
    ElMessage.success("已取消");
    await load();
  } catch (e) {
    ElMessage.error(errText(e, "取消失败"));
  } finally {
    cancelling.value = null;
  }
}

// ---- 改到期时间 ----
const expiryDialog = ref(false);
const expiryTarget = ref<FreeWatch | null>(null);
const expiryAt = ref<Date | null>(null);
const savingExpiry = ref(false);
const expiryError = computed(() => (expiryDialog.value ? expiryProblem(expiryAt.value) : ""));

function openExpiry(w: FreeWatch) {
  expiryTarget.value = w;
  expiryAt.value = new Date(w.expires_at);
  expiryDialog.value = true;
}

async function saveExpiry() {
  const w = expiryTarget.value;
  if (!w || !expiryAt.value || expiryError.value) return;
  savingExpiry.value = true;
  try {
    await freeWatchApi.setExpiry(w.id, expiryAt.value.toISOString());
    ElMessage.success(`改好了：等到 ${formatShortDateTime(expiryAt.value)}`);
    expiryDialog.value = false;
    await load();
  } catch (e) {
    ElMessage.error(errText(e, "改到期时间失败"));
  } finally {
    savingExpiry.value = false;
  }
}

// 后台每 5 分钟看一轮：页面开着时每分钟静默刷新一次，切到别的标签页、开着弹窗、上一次还没回来时这一拍不刷
const AUTO_REFRESH_MS = 60_000;
let timer: ReturnType<typeof setInterval> | undefined;

onMounted(() => {
  void load();
  void loadDownloaders();
  timer = setInterval(() => {
    if (document.hidden || expiryDialog.value || ds.loading.value) return;
    void load();
  }, AUTO_REFRESH_MS);
});

onBeforeUnmount(() => clearInterval(timer));
</script>

<template>
  <div class="pt-cards pt-cards--wide">
    <PtHeadSub>免费了才推送：每 5 分钟看一次优惠，到期还没免费就不再等</PtHeadSub>
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button data-testid="fw-refresh" @click="load">
        <PtIcon name="refresh-cw" :size="15" /><span>刷新</span>
      </el-button>
    </Teleport>

    <PtPanel title="等免费" icon="hourglass" :count="total">
      <div class="fw-bar">
        <el-radio-group v-model="status" size="small" data-testid="fw-status" @change="load">
          <el-radio-button
            v-for="f in FREE_WATCH_FILTERS"
            :key="f.value || 'all'"
            :value="f.value"
            >{{ filterLabel(f, counts) }}</el-radio-button
          >
        </el-radio-group>
      </div>

      <div
        v-if="items.length && ds.error.value"
        class="pt-note pt-note--warn fw-note"
        data-testid="fw-stale">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>刷新失败：{{ ds.errorText.value }}。下面是上次读到的。</span>
      </div>
      <p v-if="total > items.length && items.length >= LIST_LIMIT" class="fw-more">
        一共 {{ total }} 条，只列出最近 {{ LIST_LIMIT }} 条
      </p>
      <PtDataState
        v-if="!items.length"
        :state="ds.state.value"
        :title="ds.state.value === 'empty' ? '没有在等免费的种子' : ''"
        :sub="
          ds.errorText.value ||
          (ds.state.value === 'empty' ? '在种子搜索的结果里点「等免费」：免费了才推送到下载器' : '')
        "
        data-testid="fw-state" />
      <el-table
        v-else-if="!isMobile"
        :data="items"
        row-key="id"
        class="pt-grid"
        data-testid="fw-table">
        <el-table-column label="种子" min-width="280" class-name="pt-cell-1line">
          <template #default="{ row }">
            <div class="fw-strong" :title="row.title">{{ row.title || row.torrent_id }}</div>
            <div class="fw-sub">{{ metaText(row) }}</div>
          </template>
        </el-table-column>
        <el-table-column label="推到" min-width="160" class-name="pt-cell-1line">
          <template #default="{ row }">
            <div>{{ downloaderName(row.downloader_id) }}</div>
            <div class="fw-sub">{{ targetText(row) }}</div>
          </template>
        </el-table-column>
        <el-table-column label="状态" min-width="220">
          <template #default="{ row }">
            <PtStatusPill :tone="freeWatchStatus(row.status).tone" size="sm">{{
              freeWatchStatus(row.status).label
            }}</PtStatusPill>
            <div v-if="row.message" class="fw-sub">{{ row.message }}</div>
          </template>
        </el-table-column>
        <el-table-column label="检查" width="168">
          <template #default="{ row }">
            <div>{{ timeText(row) }}</div>
            <div class="fw-sub">{{ checkText(row) }}</div>
          </template>
        </el-table-column>
        <el-table-column label="到期" width="120">
          <template #default="{ row }">
            <span v-if="row.status === 'watching'">{{ formatShortDateTime(row.expires_at) }}</span>
            <span v-else class="fw-sub">—</span>
          </template>
        </el-table-column>
        <el-table-column label="" width="150" align="right">
          <template #default="{ row }">
            <template v-if="row.status === 'watching'">
              <el-button link :data-testid="`fw-expiry-${row.id}`" @click="openExpiry(row)"
                >改到期</el-button
              >
              <el-button
                link
                type="danger"
                :loading="cancelling === row.id"
                :data-testid="`fw-cancel-${row.id}`"
                @click="cancel(row)"
                >取消</el-button
              >
            </template>
          </template>
        </el-table-column>
      </el-table>
      <div v-else class="fw-cards">
        <PtRowCard v-for="w in items" :key="w.id">
          <template #title>{{ w.title || w.torrent_id }}</template>
          <template #meta>
            <span>{{ metaText(w) }}</span>
            <span>推到 {{ downloaderName(w.downloader_id) }}</span>
            <span v-if="w.message" class="fw-full">{{ w.message }}</span>
            <span>{{ timeText(w) }} · {{ checkText(w) }}</span>
            <span v-if="w.status === 'watching'">等到 {{ formatShortDateTime(w.expires_at) }}</span>
          </template>
          <template #status>
            <PtStatusPill dot :tone="freeWatchStatus(w.status).tone" size="sm">{{
              freeWatchStatus(w.status).label
            }}</PtStatusPill>
          </template>
          <template v-if="w.status === 'watching'" #actions>
            <el-button size="small" @click="openExpiry(w)">改到期</el-button>
            <el-button
              size="small"
              type="danger"
              plain
              :loading="cancelling === w.id"
              @click="cancel(w)"
              >取消</el-button
            >
          </template>
        </PtRowCard>
      </div>
    </PtPanel>

    <el-dialog
      v-model="expiryDialog"
      class="pt-dialog"
      title="改到期时间"
      :width="isMobile ? '94%' : '420px'"
      align-center>
      <el-form class="pt-form" label-position="top" @submit.prevent>
        <p class="fw-dtitle">{{ expiryTarget?.title || expiryTarget?.torrent_id }}</p>
        <el-form-item label="等到">
          <el-date-picker
            v-model="expiryAt"
            type="datetime"
            format="YYYY-MM-DD HH:mm"
            :disabled-date="(d: Date) => expiryDisabled(d)"
            :clearable="false"
            style="width: 100%"
            data-testid="fw-expiry-at" />
          <div class="field-tip" :class="{ 'field-tip--warn': expiryError }">
            {{ expiryError || "最多从现在起再等 30 天；到点还没免费就不再等" }}
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="expiryDialog = false">取消</el-button>
        <el-button
          type="primary"
          :loading="savingExpiry"
          :disabled="Boolean(expiryError)"
          data-testid="fw-expiry-save"
          @click="saveExpiry"
          >保存</el-button
        >
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.fw-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
  margin-bottom: 12px;
}

.fw-note {
  margin-bottom: 12px;
}

.fw-more {
  margin: 0 0 8px;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
}

.fw-strong {
  overflow: hidden;
  font-weight: 600;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.fw-sub {
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
}

.fw-cards {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.fw-full {
  flex-basis: 100%;
  word-break: break-all;
}

.fw-dtitle {
  margin: 0 0 12px;
  font-size: var(--pt-fz-body);
  font-weight: 600;
  color: var(--pt-t1);
  word-break: break-all;
}
</style>
