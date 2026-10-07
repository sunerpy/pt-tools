<script setup lang="ts">
/*
 * 转移做种对话框（路线图 M5）：选目标下载器 → 预览（能不能转、换算后的路径、种子文件从哪来）→ 建任务。
 * 下载器 Web UI 与任务列表的批量操作共用。任务建好后由后台一步步做：暂停加入目标、校验到 100% 再从源移除。
 */
import {
  type DownloaderSetting,
  downloadersApi,
  type TransferItem,
  type TransferPreviewItem,
  transferApi,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useIsMobile } from "@/composables/useIsMobile";
import { formatBytes } from "@/utils/format";
import { transferPreviewSummary, transferSourceLabel } from "@/utils/transfer";
import { ElMessage } from "element-plus";
import { computed, ref, watch } from "vue";

const props = defineProps<{ items: TransferItem[] }>();
const visible = defineModel<boolean>({ default: false });
const emit = defineEmits<{ created: [count: number] }>();

const isMobile = useIsMobile();
const downloaders = ref<DownloaderSetting[]>([]);
const targetId = ref<number | undefined>(undefined);
const preview = ref<TransferPreviewItem[]>([]);
const previewedFor = ref<number | undefined>(undefined);
const loading = ref(false);
const creating = ref(false);
const error = ref("");

const sourceIds = computed(() => new Set(props.items.map((i) => i.source_id)));
/** 目标候选：已启用、不是唯一那台源下载器 */
const targets = computed(() =>
  downloaders.value.filter((d) => !(sourceIds.value.size === 1 && sourceIds.value.has(d.id ?? 0))),
);
const okCount = computed(() => preview.value.filter((p) => p.ok).length);
const stale = computed(
  () => previewedFor.value !== undefined && previewedFor.value !== targetId.value,
);

watch(visible, async (open) => {
  if (!open) return;
  preview.value = [];
  previewedFor.value = undefined;
  error.value = "";
  try {
    downloaders.value = ((await downloadersApi.list()) ?? []).filter((d) => d.enabled);
  } catch (e) {
    downloaders.value = [];
    error.value = (e as Error).message || "下载器列表没读到";
  }
  if (!targets.value.some((d) => d.id === targetId.value)) targetId.value = targets.value[0]?.id;
});

/** 预览请求的序号：只认最新一次、并且目标没换过的结果 */
let previewSeq = 0;

async function runPreview() {
  const target = targetId.value;
  if (!target || !props.items.length) return;
  const seq = ++previewSeq;
  loading.value = true;
  error.value = "";
  try {
    const res = await transferApi.preview(target, props.items);
    // 请求途中换了目标或又点了一次预览：这次的结果作废
    if (seq !== previewSeq || target !== targetId.value) return;
    preview.value = res?.items ?? [];
    previewedFor.value = target;
  } catch (e) {
    if (seq !== previewSeq) return;
    preview.value = [];
    previewedFor.value = undefined;
    error.value = (e as Error).message || "预览失败";
  } finally {
    if (seq === previewSeq) loading.value = false;
  }
}

async function create() {
  if (!targetId.value || stale.value || !okCount.value) return;
  creating.value = true;
  try {
    const items = preview.value
      .filter((p) => p.ok)
      .map((p) => ({ source_id: p.source_id, hash: p.hash }));
    const res = await transferApi.create(targetId.value, items);
    const n = res?.created?.length ?? 0;
    const skipped = res?.skipped ?? [];
    const first = skipped[0];
    ElMessage({
      type: n ? "success" : "warning",
      message:
        `已建 ${n} 个转移任务，到「下载 → 转移做种」看进度` +
        (first
          ? `；跳过 ${skipped.length} 个（${first.name || first.hash}：${first.reason}）`
          : ""),
      duration: 6000,
    });
    if (n) {
      emit("created", n);
      visible.value = false;
    }
  } catch (e) {
    ElMessage.error((e as Error).message || "建任务失败");
  } finally {
    creating.value = false;
  }
}
</script>

<template>
  <el-dialog
    v-model="visible"
    class="pt-dialog"
    title="转移到其他下载器"
    :width="isMobile ? '94%' : '820px'"
    append-to-body>
    <div class="tr-dlg__bar">
      <el-select
        v-model="targetId"
        class="tr-dlg__target"
        placeholder="目标下载器"
        aria-label="目标下载器"
        data-testid="tr-target">
        <el-option v-for="d in targets" :key="d.id" :label="d.name" :value="d.id" />
      </el-select>
      <el-button
        type="primary"
        :loading="loading"
        :disabled="!targetId || !items.length"
        data-testid="tr-preview"
        @click="runPreview">
        <PtIcon name="scan" :size="14" /><span>预览</span>
      </el-button>
    </div>
    <p class="tr-dlg__tip">
      已选 {{ items.length }} 个种子。转移时先把种子暂停加入目标下载器（保存路径按「转移做种 →
      路径映射」换算）， 校验到 100% 才在目标开始做种、从源下载器移除（数据文件不删）；没到 100%
      就从目标移除，源下载器里的种子不动。
    </p>
    <div v-if="error" class="pt-note pt-note--warn">
      <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
      <span>{{ error }}</span>
    </div>
    <div v-if="stale" class="pt-note">
      <PtIcon name="info" :size="14" class="pt-note__icon" />
      <span>换了目标下载器，请重新预览。</span>
    </div>

    <template v-if="preview.length">
      <el-table v-if="!isMobile" :data="preview" class="pt-grid" max-height="360" row-key="hash">
        <el-table-column label="种子" min-width="200" class-name="pt-cell-strong">
          <template #default="{ row }">
            <div class="tr-dlg__name">{{ row.name || row.hash }}</div>
            <div class="tr-dlg__sub">{{ row.source_name }} · {{ formatBytes(row.size) }}</div>
          </template>
        </el-table-column>
        <el-table-column label="源路径 → 目标路径" min-width="260">
          <template #default="{ row }">
            <template v-if="row.save_path">
              <div class="tr-dlg__path">{{ row.save_path }}</div>
              <div class="tr-dlg__path tr-dlg__path--to">
                → {{ row.target_path
                }}<span v-if="!row.mapped" class="tr-dlg__same">（同一路径）</span>
              </div>
            </template>
          </template>
        </el-table-column>
        <el-table-column label="结果" min-width="200">
          <template #default="{ row }">
            <PtStatusPill v-if="row.ok" tone="ok" size="sm">可以转</PtStatusPill>
            <span v-if="row.ok" class="tr-dlg__sub tr-dlg__src">{{
              transferSourceLabel(row)
            }}</span>
            <span v-else class="tr-dlg__reason">{{ row.reason }}</span>
          </template>
        </el-table-column>
      </el-table>
      <div v-else class="tr-dlg__cards">
        <PtRowCard v-for="p in preview" :key="`${p.source_id}|${p.hash}`">
          <template #title>{{ p.name || p.hash }}</template>
          <template #meta>
            <span>{{ p.source_name }} · {{ formatBytes(p.size) }}</span>
            <span v-if="p.save_path" class="tr-dlg__path tr-dlg__full"
              >{{ p.save_path }} → {{ p.target_path }}</span
            >
            <span v-if="!p.ok" class="tr-dlg__reason tr-dlg__full">{{ p.reason }}</span>
          </template>
          <template #status>
            <PtStatusPill :tone="p.ok ? 'ok' : 'dang'" size="sm">{{
              p.ok ? "可以转" : "跳过"
            }}</PtStatusPill>
          </template>
        </PtRowCard>
      </div>
      <p class="tr-dlg__tip">{{ transferPreviewSummary(preview) }}。</p>
    </template>

    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button
        type="primary"
        :loading="creating"
        :disabled="!okCount || stale"
        data-testid="tr-create"
        @click="create">
        建 {{ okCount }} 个转移任务
      </el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.tr-dlg__bar {
  display: flex;
  gap: 8px;
  align-items: center;
}

.tr-dlg__target {
  flex: 1;
  min-width: 0;
  max-width: 320px;
}

.tr-dlg__tip {
  margin: 8px 0 12px;
  font-size: 12px;
  line-height: 1.6;
  color: var(--pt-t3);
}

.tr-dlg__name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tr-dlg__sub {
  font-size: 12px;
  color: var(--pt-t3);
}

.tr-dlg__src {
  margin-left: 6px;
}

.tr-dlg__path {
  font-family: var(--pt-font-mono);
  font-size: 12px;
  color: var(--pt-t2);
  word-break: break-all;
}

.tr-dlg__path--to {
  color: var(--pt-t1);
}

.tr-dlg__same {
  font-family: var(--pt-font-family);
  color: var(--pt-t3);
}

.tr-dlg__reason {
  font-size: 12px;
  color: var(--pt-dang);
}

.tr-dlg__cards {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.tr-dlg__full {
  flex-basis: 100%;
}
</style>
