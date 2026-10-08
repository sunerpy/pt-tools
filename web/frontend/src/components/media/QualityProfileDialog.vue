<script setup lang="ts">
/*
 * 质量档案弹窗：分辨率、来源、编码按选的先后排偏好（不选不限）；Remux、HDR、中字、免费选不限、优先、必须有
 * （Remux 与 HDR 还能选不要）；偏好的制作组、体积区间（剧集按每集算）、最少做种数、不要 H&R。
 */
import { type QualityProfile, type QualityProfileInput, subscribeApi } from "@/api";
import { useIsMobile } from "@/composables/useIsMobile";
import { prefOptions } from "@/utils/subscribe";
import { ElMessage } from "element-plus";
import { computed, ref, watch } from "vue";

const props = defineProps<{
  modelValue: boolean;
  profile?: QualityProfile | null;
  resolutions: string[];
  sources: string[];
  codecs: string[];
}>();
const emit = defineEmits<{
  (e: "update:modelValue", v: boolean): void;
  (e: "saved", v: QualityProfile): void;
}>();

const isMobile = useIsMobile();
const visible = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emit("update:modelValue", v),
});
const blank = (): QualityProfileInput => ({
  name: "",
  resolutions: [],
  sources: [],
  codecs: [],
  remux: "",
  hdr: "",
  chinese_subs: "",
  free: "",
  groups: [],
  min_size_gb: 0,
  max_size_gb: 0,
  min_seeders: 0,
  exclude_hr: false,
});
const form = ref<QualityProfileInput>(blank());
const saving = ref(false);

watch(
  () => props.modelValue,
  (open) => {
    if (!open) return;
    const p = props.profile;
    if (!p) {
      form.value = blank();
      return;
    }
    const { id: _id, ...rest } = p;
    form.value = {
      ...rest,
      resolutions: [...p.resolutions],
      sources: [...p.sources],
      codecs: [...p.codecs],
      groups: [...p.groups],
    };
  },
  { immediate: true },
);

async function save() {
  saving.value = true;
  try {
    const saved = props.profile
      ? await subscribeApi.updateProfile(props.profile.id, form.value)
      : await subscribeApi.createProfile(form.value);
    ElMessage.success("已保存质量档案");
    emit("saved", saved);
    visible.value = false;
  } catch (e) {
    ElMessage.error((e as Error)?.message || "保存失败");
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <el-dialog
    v-model="visible"
    class="pt-dialog"
    :title="profile ? '修改质量档案' : '添加质量档案'"
    :width="isMobile ? '94%' : '620px'"
    append-to-body
    align-center
    data-testid="profile-dialog">
    <el-form class="pt-form" label-position="top" @submit.prevent>
      <el-form-item label="名称">
        <el-input
          v-model="form.name"
          maxlength="64"
          placeholder="如 4K 原盘"
          data-testid="qp-name" />
      </el-form-item>
      <el-form-item label="分辨率（按选的先后排偏好，不选不限）">
        <el-select v-model="form.resolutions" multiple placeholder="不限" data-testid="qp-res">
          <el-option v-for="r in resolutions" :key="r" :value="r" :label="r" />
        </el-select>
      </el-form-item>
      <el-form-item label="来源（按选的先后排偏好，不选不限）">
        <el-select v-model="form.sources" multiple placeholder="不限" data-testid="qp-src">
          <el-option v-for="r in sources" :key="r" :value="r" :label="r" />
        </el-select>
      </el-form-item>
      <el-form-item label="编码（按选的先后排偏好，不选不限）">
        <el-select v-model="form.codecs" multiple placeholder="不限" data-testid="qp-codec">
          <el-option v-for="r in codecs" :key="r" :value="r" :label="r" />
        </el-select>
      </el-form-item>
      <div class="qp-row">
        <el-form-item label="Remux">
          <el-select v-model="form.remux" :empty-values="[null, undefined]" data-testid="qp-remux">
            <el-option
              v-for="o in prefOptions(true)"
              :key="o.value || 'any'"
              :value="o.value"
              :label="o.label" />
          </el-select>
        </el-form-item>
        <el-form-item label="HDR">
          <el-select v-model="form.hdr" :empty-values="[null, undefined]" data-testid="qp-hdr">
            <el-option
              v-for="o in prefOptions(true)"
              :key="o.value || 'any'"
              :value="o.value"
              :label="o.label" />
          </el-select>
        </el-form-item>
        <el-form-item label="中字">
          <el-select
            v-model="form.chinese_subs"
            :empty-values="[null, undefined]"
            data-testid="qp-subs">
            <el-option
              v-for="o in prefOptions(false)"
              :key="o.value || 'any'"
              :value="o.value"
              :label="o.label" />
          </el-select>
        </el-form-item>
        <el-form-item label="免费">
          <el-select v-model="form.free" :empty-values="[null, undefined]" data-testid="qp-free">
            <el-option
              v-for="o in prefOptions(false)"
              :key="o.value || 'any'"
              :value="o.value"
              :label="o.label" />
          </el-select>
        </el-form-item>
      </div>
      <el-form-item label="偏好的制作组（加分，不是必须）">
        <el-select
          v-model="form.groups"
          multiple
          filterable
          allow-create
          default-first-option
          :reserve-keyword="false"
          placeholder="输入后回车，如 FRDS"
          data-testid="qp-groups" />
      </el-form-item>
      <div class="qp-row">
        <el-form-item label="最小体积（GB）">
          <el-input-number
            v-model="form.min_size_gb"
            :min="0"
            :max="10000"
            :step="1"
            controls-position="right"
            data-testid="qp-min" />
        </el-form-item>
        <el-form-item label="最大体积（GB，0 不限）">
          <el-input-number
            v-model="form.max_size_gb"
            :min="0"
            :max="10000"
            :step="1"
            controls-position="right"
            data-testid="qp-max" />
        </el-form-item>
        <el-form-item label="最少做种数">
          <el-input-number
            v-model="form.min_seeders"
            :min="0"
            :max="100000"
            controls-position="right"
            data-testid="qp-seeders" />
        </el-form-item>
      </div>
      <div class="field-tip qp-tip">
        体积剧集按每集算；RSS 里刚发布的种子不看做种数。免费、做种数只在分数相同时排先后。
      </div>
      <el-form-item label="H&R">
        <div class="qp-switch">
          <el-switch v-model="form.exclude_hr" data-testid="qp-hr" />
          <span>不要有 H&R 要求的种子</span>
        </div>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="saving" data-testid="qp-save" @click="save"
        >保存</el-button
      >
    </template>
  </el-dialog>
</template>

<style scoped>
/* 这行说明夹在两行表单项之间：和下一项隔开，不贴着「H&R」 */
.qp-tip {
  margin-bottom: 16px;
}

.qp-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(130px, 1fr));
  gap: 0 14px;
}

.qp-switch {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--pt-t2);
}
</style>
