<script setup lang="ts">
import { qbitApi, type QbitSettings } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { ElMessage } from "element-plus";
import { onMounted, ref } from "vue";

const loading = ref(false);
const saving = ref(false);

const form = ref<QbitSettings>({
  enabled: false,
  url: "",
  user: "",
  password: "",
});

onMounted(async () => {
  loading.value = true;
  try {
    form.value = await qbitApi.get();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "加载失败");
  } finally {
    loading.value = false;
  }
});

async function save() {
  if (form.value.enabled && (!form.value.url || !form.value.user || !form.value.password)) {
    ElMessage.error("启用时 URL、用户名、密码均为必填");
    return;
  }

  saving.value = true;
  try {
    await qbitApi.save(form.value);
    ElMessage.success("保存成功");
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "保存失败");
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <div class="qbit-page">
    <PtPanel v-loading="loading" title="qBittorrent 设置" icon="cloud-download">
      <template #actions>
        <PtStatusPill :tone="form.enabled ? 'ok' : 'neutral'" dot>
          {{ form.enabled ? "已启用" : "未启用" }}
        </PtStatusPill>
      </template>

      <el-form :model="form" label-position="top" class="pt-form">
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
          <div class="field-tip">启用后将自动推送种子到 qBittorrent</div>
        </el-form-item>

        <div class="field-rule" />

        <el-form-item label="URL">
          <el-input
            v-model="form.url"
            placeholder="http://192.168.1.10:8080"
            :disabled="!form.enabled">
            <template #prefix>
              <PtIcon name="link" :size="14" />
            </template>
          </el-input>
          <div class="field-tip">qBittorrent Web UI 地址</div>
        </el-form-item>

        <el-form-item label="用户名">
          <el-input v-model="form.user" placeholder="admin" :disabled="!form.enabled">
            <template #prefix>
              <PtIcon name="user" :size="14" />
            </template>
          </el-input>
        </el-form-item>

        <el-form-item label="密码">
          <el-input
            v-model="form.password"
            type="password"
            show-password
            placeholder="请输入密码"
            :disabled="!form.enabled">
            <template #prefix>
              <PtIcon name="lock" :size="14" />
            </template>
          </el-input>
        </el-form-item>
      </el-form>

      <template #footer>
        <span class="pt-foot-note">启用时 URL、用户名、密码均为必填</span>
        <el-button type="primary" :loading="saving" @click="save">
          <PtIcon name="save" :size="14" /><span>保存设置</span>
        </el-button>
      </template>
    </PtPanel>
  </div>
</template>

<style scoped>
.qbit-page {
  max-width: 520px;
}
</style>
