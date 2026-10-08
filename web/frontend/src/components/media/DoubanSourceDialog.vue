<script setup lang="ts">
/*
 * 豆瓣想看来源：填豆瓣用户编号（个人主页地址 douban.com/people/ 后面那一段），定时拉公开的「收藏」RSS，
 * 给「想看」的电影与剧集建订阅；可以设成先待确认，选建订阅用的质量档案。
 */
import {
  type DoubanSource,
  type DoubanSourceInput,
  type QualityProfile,
  subscribeApi,
} from "@/api";
import { useIsMobile } from "@/composables/useIsMobile";
import { ElMessage } from "element-plus";
import { computed, ref, watch } from "vue";

const props = defineProps<{
  modelValue: boolean;
  source?: DoubanSource | null;
  profiles: QualityProfile[];
}>();
const emit = defineEmits<{
  (e: "update:modelValue", v: boolean): void;
  (e: "saved", v: DoubanSource): void;
}>();

const isMobile = useIsMobile();
const visible = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emit("update:modelValue", v),
});
const blank = (): DoubanSourceInput => ({
  user_id: "",
  name: "",
  enabled: true,
  confirm: true,
  profile_id: 0,
});
const form = ref<DoubanSourceInput>(blank());
const saving = ref(false);

watch(
  () => props.modelValue,
  (open) => {
    if (!open) return;
    const s = props.source;
    form.value = s
      ? {
          user_id: s.user_id,
          name: s.name,
          enabled: s.enabled,
          confirm: s.confirm,
          profile_id: s.profile_id,
        }
      : blank();
  },
  { immediate: true },
);

async function save() {
  saving.value = true;
  try {
    const saved = props.source
      ? await subscribeApi.updateDouban(props.source.id, form.value)
      : await subscribeApi.createDouban(form.value);
    ElMessage.success("已保存豆瓣来源");
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
    :title="source ? '修改豆瓣来源' : '添加豆瓣来源'"
    :width="isMobile ? '94%' : '520px'"
    append-to-body
    align-center
    data-testid="douban-dialog">
    <el-form class="pt-form" label-position="top" @submit.prevent>
      <el-form-item label="豆瓣用户编号">
        <el-input v-model="form.user_id" placeholder="如 ahbei" data-testid="db-user" />
        <div class="field-tip">
          个人主页地址 douban.com/people/ 后面那一段；收藏要公开（豆瓣的隐私设置里）。
        </div>
      </el-form-item>
      <el-form-item label="名字（可以不填）">
        <el-input
          v-model="form.name"
          maxlength="64"
          placeholder="界面上显示"
          data-testid="db-name" />
      </el-form-item>
      <el-form-item label="建订阅用的质量档案">
        <el-select v-model="form.profile_id" data-testid="db-profile">
          <el-option :value="0" label="用设置里的默认档案" />
          <el-option v-for="p in profiles" :key="p.id" :value="p.id" :label="p.name" />
        </el-select>
      </el-form-item>
      <el-form-item label="启用">
        <div class="db-switch">
          <el-switch v-model="form.enabled" data-testid="db-enabled" />
          <span>每 6 小时拉一次「想看」</span>
        </div>
      </el-form-item>
      <el-form-item label="先确认">
        <div class="db-switch">
          <el-switch v-model="form.confirm" data-testid="db-confirm" />
          <span>建出的订阅先是待确认，确认后才开始找资源</span>
        </div>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="saving" data-testid="db-save" @click="save"
        >保存</el-button
      >
    </template>
  </el-dialog>
</template>

<style scoped>
.db-switch {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--pt-t2);
}
</style>
