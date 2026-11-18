<script setup lang="ts">
/*
 * 「不要」这个种子（媒体中心 M2b）：写一个原因（可以不写），拉黑以后订阅不再选它，按条目搜索把它标成已拉黑。
 * 按条目搜索、关键字搜索与追剧详情三处共用；作用范围（这个条目还是所有条目）由打开的地方给出。
 * 追剧详情里还能顺手从下载器删掉这个种子、把制作组加进订阅的标题排除词。
 */
import { computed, ref, watch } from "vue";

import PtTag from "@/components/ui/PtTag.vue";
import { useIsMobile } from "@/composables/useIsMobile";

export interface BlockChoice {
  reason: string;
  excludeGroup: boolean;
  deleteTorrent: boolean;
  deleteFiles: boolean;
}

const props = defineProps<{
  modelValue: boolean;
  torrentTitle: string;
  /** 一起拉黑的站点（同一资源在几个站点上的种子） */
  sites: string[];
  /** 作用范围的说明 */
  scope: string;
  /** 能加进标题排除词的制作组：字符串时写出名字，true 时不知道名字（由服务端认），不给时没有这一项 */
  excludeGroup?: string | true;
  /** 能顺手从下载器删掉时给出下载器的名字 */
  deleteFrom?: string;
  busy?: boolean;
}>();

const emit = defineEmits<{
  "update:modelValue": [open: boolean];
  confirm: [choice: BlockChoice];
}>();

const isMobile = useIsMobile();
const visible = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emit("update:modelValue", v),
});

const reason = ref("");
const wantExclude = ref(false);
const wantDelete = ref(false);
const wantDeleteFiles = ref(false);

watch(
  () => props.modelValue,
  (open) => {
    if (!open) return;
    reason.value = "";
    wantExclude.value = false;
    wantDelete.value = false;
    wantDeleteFiles.value = false;
  },
);

const groupLabel = computed(() =>
  typeof props.excludeGroup === "string"
    ? `以后也不要这个制作组（${props.excludeGroup}）`
    : "以后也不要这个种子的制作组",
);

function confirm() {
  emit("confirm", {
    reason: reason.value.trim(),
    excludeGroup: Boolean(props.excludeGroup) && wantExclude.value,
    deleteTorrent: Boolean(props.deleteFrom) && wantDelete.value,
    deleteFiles: Boolean(props.deleteFrom) && wantDelete.value && wantDeleteFiles.value,
  });
}
</script>

<template>
  <el-dialog
    v-model="visible"
    class="pt-dialog"
    title="不要这个种子"
    :width="isMobile ? '94%' : '480px'"
    append-to-body
    align-center
    data-testid="block-dialog">
    <el-form class="pt-form" label-position="top" @submit.prevent>
      <p class="bd-title" :title="torrentTitle">{{ torrentTitle }}</p>
      <div class="bd-sites">
        <PtTag v-for="s in sites" :key="s">{{ s }}</PtTag>
        <span v-if="sites.length > 1" class="bd-note">同一资源在这几个站点上的种子一起拉黑</span>
      </div>
      <el-form-item label="原因（可以不写）">
        <el-input
          v-model="reason"
          maxlength="100"
          show-word-limit
          placeholder="比如：音画不同步、画质差、假种"
          data-testid="block-reason"
          @keyup.enter="confirm" />
      </el-form-item>
      <p class="bd-scope" data-testid="block-scope">{{ scope }}</p>
      <div v-if="excludeGroup || deleteFrom" class="bd-opts">
        <el-checkbox v-if="excludeGroup" v-model="wantExclude" data-testid="block-exclude-group">
          {{ groupLabel }}<span class="bd-hint">：加进这个订阅的标题排除词</span>
        </el-checkbox>
        <template v-if="deleteFrom">
          <el-checkbox v-model="wantDelete" data-testid="block-delete">
            同时从下载器 {{ deleteFrom }} 删掉这个种子
          </el-checkbox>
          <el-checkbox
            v-if="wantDelete"
            v-model="wantDeleteFiles"
            class="bd-sub"
            data-testid="block-delete-files">
            连同已下载的文件一起删（删后不能恢复）
          </el-checkbox>
        </template>
      </div>
    </el-form>
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="busy" data-testid="block-confirm" @click="confirm"
        >不要了</el-button
      >
    </template>
  </el-dialog>
</template>

<style scoped>
.bd-title {
  margin: 0 0 6px;
  overflow: hidden;
  font-size: var(--pt-fz-body);
  font-weight: 600;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bd-sites {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 6px;
  align-items: center;
  margin-bottom: 14px;
}

.bd-note,
.bd-hint {
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

.bd-scope {
  margin: -6px 0 0;
  font-size: var(--pt-fz-label);
  line-height: var(--pt-lh-row);
  color: var(--pt-t3);
}

.bd-opts {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-top: 12px;
}

.bd-sub {
  margin-left: 24px;
}
</style>
