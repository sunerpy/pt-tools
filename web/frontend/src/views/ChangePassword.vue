<script setup lang="ts">
import { passwordApi } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtPasswordStrength from "@/components/ui/PtPasswordStrength.vue";
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
    画板 44：p-acct 624,92 520×456 一张居中的窄卡，下面一条分隔线加三张 344 宽的说明卡。
    外面这层 .change-password-page 是必需的单一根元素 —— App.vue 的 <router-view>
    外面套着 `<transition name="fade" mode="out-in">`，路由组件有多个根节点时
    Transition 认不出要过渡的元素，out-in 的 leave 回调不触发，**下一个页面就永远
    不挂载**，表现是跳到别的页时主区一片空白（从这一页跳站点详情实测中过）。
  -->
  <div class="change-password-page">
    <div class="pt-cards pt-cards--narrow">
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
            <!--
              画板 44 要求这里有一条强度条。只在有输入时才挂：空口令没有可估的
              强度，给个 0 长的条配「很好猜」纯属误导。
            -->
            <PtPasswordStrength
              v-if="form.newPassword"
              :password="form.newPassword"
              :username="form.username"
              class="pw-strength" />
            <div v-else class="field-tip">至少 6 位。开始输入后这里会给出强度估计。</div>
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

    <!--
      画板 44 在账号卡下面还有一条分隔线加三张 344 宽的卡：p-rules（口令规则）、
      p-msg（改完会发生什么）、p-note（忘记密码怎么办）。内容是固定说明，
      改密码这件事本身没有可查的数据，所以三张卡就是三段写死的文案。
    -->
    <div class="pw-rule" />

    <div class="pt-cards pt-cards--3">
      <PtPanel title="口令规则" icon="shield-check">
        <ul class="pw-list">
          <li>
            新口令至少 6 位 —— 这是<strong>本页表单</strong>的下限。
            <code>/api/password</code> 只拒绝空口令，不查复杂度；首尾空白会被去掉（登录时也一样）。
          </li>
          <li>新口令与确认口令必须一致，否则不提交。</li>
          <li>强度条只估「这个口令有多难猜」，不是提交条件，也不代表口令安全。</li>
          <li>
            用户名要再输一次：接口按「用户名 + 原口令」一起校验，对不上会返回
            <code>原密码错误</code>。
          </li>
        </ul>
      </PtPanel>

      <PtPanel title="改完会发生什么" icon="log-out">
        <ul class="pw-list">
          <li>
            新口令立刻生效。<strong>当前会话不会被踢掉</strong>，手上这个登录态继续有效；
            其他浏览器和设备上的登录会话全部失效，需要用新口令重新登录。
          </li>
          <li>站点 Cookie、下载器口令、CloakBrowser token 都不受影响，它们是另一套凭据。</li>
          <li>
            浏览器扩展不保存这个账号口令（设置面板里填的口令保存时就清空了），它靠浏览器里的
            pt-tools 登录 Cookie 调接口，所以改完不用去扩展里重填。
          </li>
        </ul>
      </PtPanel>

      <!-- 图标名必须存在于 src/icons/lucide.ts：原来写的 life-buoy 不在表里，
           PtIcon 找不到就画一个空 svg，开发环境只有一句 console 警告 -->
      <PtPanel title="忘记密码怎么办" icon="rotate-ccw">
        <ul class="pw-list">
          <li>Web 端没有找回入口：账号只存在本机库里，没有邮箱可以发信。</li>
          <li>
            带 <code>PT_ADMIN_RESET=1</code> 与 <code>PT_ADMIN_USER</code> /
            <code>PT_ADMIN_PASS</code> 重启一次，启动时会把该用户的口令改成
            <code>PT_ADMIN_PASS</code>。
          </li>
          <li>
            重置成功后<strong>去掉 <code>PT_ADMIN_RESET</code> 再启动一次</strong>，
            否则每次启动都会按环境变量把口令覆盖回去。完整命令见
            <code>docs/configuration.md</code> 的「重置管理员密码」。
          </li>
        </ul>
      </PtPanel>
    </div>
  </div>
</template>

<style scoped>
/* 520 的宽度与居中都由 .pt-cards--narrow 给（画板 44 的 p-acct 就是 520） */
.change-password-page {
  display: flex;
  flex-direction: column;
}

/* 与 .pt-form .field-tip 一样占满 el-form-item 的内容行，免得和输入框挤在一行 */
.pw-strength {
  margin-top: 6px;
}

/* 画板 44 在账号卡与下面三张说明卡之间有一条分隔线 */
.pw-rule {
  height: 1px;
  margin: 0 var(--pt-pad);
  background: var(--pt-border);
}

.pw-list {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  margin: 0;
  padding-left: 18px;
  font-size: var(--pt-fz-sm);
  line-height: var(--pt-lh-body);
  color: var(--pt-t2);
}

.pw-list code {
  padding: 1px 5px;
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-label);
  background: var(--pt-hover);
  border-radius: var(--pt-r-sm);
}
</style>
