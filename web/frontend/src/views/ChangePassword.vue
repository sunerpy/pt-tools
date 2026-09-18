<script setup lang="ts">
import { passwordApi } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtPanel from "@/components/ui/PtPanel.vue";
import { ElMessage, type FormInstance, type FormRules } from "element-plus";
import { reactive, ref } from "vue";

const formRef = ref<FormInstance>();
const saving = ref(false);
const form = reactive({
  username: "",
  oldPassword: "",
  newPassword: "",
  confirmPassword: "",
});

const rules: FormRules = {
  username: [
    {
      required: true,
      message: "请输入用户名",
      trigger: "blur",
    },
  ],
  oldPassword: [
    {
      required: true,
      message: "请输入旧密码",
      trigger: "blur",
    },
  ],
  newPassword: [
    {
      required: true,
      message: "请输入新密码",
      trigger: "blur",
    },
    {
      min: 6,
      message: "密码长度至少 6 位",
      trigger: "blur",
    },
  ],
  confirmPassword: [
    {
      required: true,
      message: "请确认新密码",
      trigger: "blur",
    },
    {
      validator: (_rule: unknown, value: string, callback: (error?: Error) => void) => {
        if (value !== form.newPassword) {
          callback(new Error("两次输入的密码不一致"));
        } else {
          callback();
        }
      },
      trigger: "blur",
    },
  ],
};

async function submit() {
  if (!formRef.value) return;

  try {
    await formRef.value.validate();
  } catch {
    return;
  }

  saving.value = true;
  try {
    await passwordApi.change({
      username: form.username,
      old: form.oldPassword,
      new: form.newPassword,
    });
    ElMessage.success("密码修改成功");
    formRef.value.resetFields();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "修改失败");
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <!--
    画板 44：p-acct 624,92 520×456 —— 一张 520 宽的卡片居中，不是通栏。
    6 个字段横铺在 1080 里会变成一行一个输入框加一大片空白。
  -->
  <div class="change-password-page pt-cards pt-cards--narrow">
    <PtPanel title="账号信息" icon="shield-check">
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top" class="pt-form">
        <el-form-item label="用户名" prop="username">
          <el-input v-model="form.username" placeholder="请输入当前登录用户名">
            <template #prefix>
              <PtIcon name="user" :size="14" />
            </template>
          </el-input>
          <div class="field-tip">出于安全考虑，请再次输入您的用户名以验证身份。</div>
        </el-form-item>

        <el-form-item label="旧密码" prop="oldPassword">
          <el-input
            v-model="form.oldPassword"
            type="password"
            show-password
            placeholder="请输入当前密码">
            <template #prefix>
              <PtIcon name="lock" :size="14" />
            </template>
          </el-input>
        </el-form-item>

        <div class="field-rule" />

        <el-form-item label="新密码" prop="newPassword">
          <el-input
            v-model="form.newPassword"
            type="password"
            show-password
            placeholder="请输入新密码（至少 6 位）">
            <template #prefix>
              <PtIcon name="key-round" :size="14" />
            </template>
          </el-input>
          <div class="field-tip">至少 6 位；只校验长度，不做复杂度要求。</div>
        </el-form-item>

        <el-form-item label="确认新密码" prop="confirmPassword">
          <el-input
            v-model="form.confirmPassword"
            type="password"
            show-password
            placeholder="请再次输入新密码">
            <template #prefix>
              <PtIcon name="key-round" :size="14" />
            </template>
          </el-input>
        </el-form-item>
      </el-form>

      <template #footer>
        <span class="pt-foot-note">定期更换密码可以提高账户安全性</span>
        <el-button type="primary" size="large" :loading="saving" @click="submit">
          <PtIcon name="save" :size="14" /><span>保存修改</span>
        </el-button>
      </template>
    </PtPanel>
  </div>
</template>

<style scoped>
/* 单栏表单页限宽：输入框拉满 1600px 宽屏时字段与标签会离得太远读不成一组 */
/* 520 的宽度与居中都由 .pt-cards--narrow 给（画板 44 的 p-acct 就是 520） */
</style>
