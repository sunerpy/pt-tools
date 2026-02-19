<script setup lang="ts">
import { reactive } from "vue";

const emit = defineEmits<{
  sync: [payload: { baseUrl: string; username?: string; password?: string }];
}>();

const form = reactive({
  baseUrl: "http://localhost:8080",
  username: "",
  password: "",
});

const props = defineProps<{
  busy: boolean;
}>();

function submit(): void {
  const payload: { baseUrl: string; username?: string; password?: string } = {
    baseUrl: form.baseUrl.trim(),
  };

  if (form.username.trim() && form.password.trim()) {
    payload.username = form.username.trim();
    payload.password = form.password.trim();
  }

  emit("sync", payload);
}
</script>

<template>
  <section class="panel">
    <h3>Cookie 同步</h3>
    <p class="muted">将已登录站点 Cookie 同步到 pt-tools。</p>
    <label class="field">
      <span>pt-tools 地址</span>
      <input v-model="form.baseUrl" type="url" placeholder="http://localhost:8080" />
    </label>
    <label class="field">
      <span>用户名（可选）</span>
      <input v-model="form.username" type="text" placeholder="admin" autocomplete="off" />
    </label>
    <label class="field">
      <span>密码（可选）</span>
      <input v-model="form.password" type="password" placeholder="password" autocomplete="off" />
    </label>
    <button type="button" class="btn" :disabled="props.busy" @click="submit">
      {{ props.busy ? "同步中..." : "同步全部站点 Cookie" }}
    </button>
  </section>
</template>
