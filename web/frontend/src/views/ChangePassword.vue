<script setup lang="ts">
import { passwordApi } from "@/api"
import { Key, Lock, User } from "@element-plus/icons-vue"
import { ElMessage, type FormInstance, type FormRules } from "element-plus"
import { reactive, ref } from "vue"

const formRef = ref<FormInstance>()
const saving = ref(false)

const form = reactive({
  username: "",
  oldPassword: "",
  newPassword: "",
  confirmPassword: ""
})

const rules: FormRules = {
  username: [{ required: true, message: "请输入用户名", trigger: "blur" }],
  oldPassword: [{ required: true, message: "请输入旧密码", trigger: "blur" }],
  newPassword: [
    { required: true, message: "请输入新密码", trigger: "blur" },
    { min: 6, message: "密码长度至少 6 位", trigger: "blur" }
  ],
  confirmPassword: [
    { required: true, message: "请确认新密码", trigger: "blur" },
    {
      validator: (_rule: unknown, value: string, callback: (error?: Error) => void) => {
        if (value !== form.newPassword) {
          callback(new Error("两次输入的密码不一致"))
        } else {
          callback()
        }
      },
      trigger: "blur"
    }
  ]
}

async function submit() {
  if (!formRef.value) return

  try {
    await formRef.value.validate()
  } catch {
    return
  }

  saving.value = true
  try {
    await passwordApi.change({
      username: form.username,
      old: form.oldPassword,
      new: form.newPassword
    })
    ElMessage.success("密码修改成功")
    formRef.value.resetFields()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "修改失败")
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="page-container">
    <div class="page-header">
      <div>
        <h1 class="page-title">修改密码</h1>
        <p class="page-subtitle">定期更换密码可以提高账户安全性</p>
      </div>
    </div>

    <div class="form-card" style="max-width: 600px">
      <div class="form-card-header">
        <h3>账号信息</h3>
      </div>

      <div class="form-section">
        <el-form
          ref="formRef"
          :model="form"
          :rules="rules"
          label-width="100px"
          label-position="top">
          <el-form-item label="用户名" prop="username">
            <el-input v-model="form.username" placeholder="请输入当前登录用户名">
              <template #prefix>
                <el-icon><User /></el-icon>
              </template>
            </el-input>
            <div class="form-tip">出于安全考虑，请再次输入您的用户名以验证身份。</div>
          </el-form-item>

          <el-form-item label="旧密码" prop="oldPassword">
            <el-input
              v-model="form.oldPassword"
              type="password"
              show-password
              placeholder="请输入当前密码">
              <template #prefix>
                <el-icon><Lock /></el-icon>
              </template>
            </el-input>
          </el-form-item>

          <div style="margin: var(--pt-space-6) 0; border-top: 1px dashed var(--pt-border-color)">
          </div>

          <el-form-item label="新密码" prop="newPassword">
            <el-input
              v-model="form.newPassword"
              type="password"
              show-password
              placeholder="请输入新密码（至少 6 位）">
              <template #prefix>
                <el-icon><Key /></el-icon>
              </template>
            </el-input>
          </el-form-item>

          <el-form-item label="确认新密码" prop="confirmPassword">
            <el-input
              v-model="form.confirmPassword"
              type="password"
              show-password
              placeholder="请再次输入新密码">
              <template #prefix>
                <el-icon><Key /></el-icon>
              </template>
            </el-input>
          </el-form-item>
        </el-form>
      </div>

      <div class="form-actions">
        <el-button type="primary" :loading="saving" size="large" @click="submit">
          保存修改
        </el-button>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* Component specific overrides if any */
:deep(.el-form-item__label) {
  font-weight: 600;
  padding-bottom: 8px !important;
}

:deep(.el-input__wrapper) {
  padding: 4px 12px;
}
</style>
