<script setup lang="ts">
import { computed } from "vue";
import PtIcon from "../PtIcon";
import { scorePassword } from "./passwordStrength";
import PtMeter from "./PtMeter.vue";

/**
 * 口令强度条 —— 画板 44 的「表单 + 强度条 + 规则清单」里的中间那件。
 *
 * 条本身直接复用 PtMeter（标签 + 读数 + 5 高进度条），不另画一套进度轨：
 * 这一页要的正是「一个名字 + 一个读数 + 一个占比」那种形状。
 *
 * 这个组件**不参与校验**，没有 emit、也不动表单：算分只是提示，
 * 判据与措辞的理由见 passwordStrength.ts 的开头。
 */
const props = withDefaults(
  defineProps<{
    password: string;
    /** 当前表单里的用户名，用于判断口令是否把它嵌了进去 */
    username?: string;
  }>(),
  { username: "" },
);

const strength = computed(() => scorePassword(props.password, props.username));
</script>

<template>
  <div class="pt-pwstrength">
    <!--
      aria-live 让读数变化被念出来 —— 强度只体现在颜色和条长上，读屏器拿不到。
      percent 是从 level 派生的（见 passwordStrength.ts 的 LEVELS），所以档位不跳
      时读数文本一字不变，Vue 会跳过这次文本补丁，不至于每敲一个字符都刷一遍。
    -->
    <PtMeter
      label="口令强度"
      :value="strength.label"
      :percent="strength.percent"
      :tone="strength.tone"
      aria-live="polite" />

    <ul class="pt-pwstrength__list">
      <li v-for="c in strength.checks" :key="c.label" :class="{ 'is-met': c.met }">
        <PtIcon :name="c.met ? 'check' : 'minus'" :size="12" />
        <!-- 满足与否只靠图标和颜色区分，读屏器两样都拿不到，所以补一个隐藏文本 -->
        <span class="pt-pwstrength__state">{{ c.met ? "已满足：" : "未满足：" }}</span>
        <span>{{ c.label }}</span>
      </li>
    </ul>

    <p class="pt-pwstrength__note">强度只是「多难猜」的估计，不决定能不能提交。</p>
  </div>
</template>

<style scoped>
.pt-pwstrength {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  width: 100%;
}

.pt-pwstrength__list {
  display: flex;
  flex-direction: column;
  gap: 3px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.pt-pwstrength__list li {
  display: flex;
  gap: 5px;
  /* 图标对齐首行文字而不是整块居中：清单项换行时居中会把勾拉到两行中间 */
  align-items: flex-start;
  font-size: var(--pt-fz-label);
  line-height: var(--pt-lh-body);
  color: var(--pt-t3);
}

.pt-pwstrength__list li .pt-icon {
  flex: 0 0 auto;
  margin-top: 2px;
}

.pt-pwstrength__list li.is-met {
  color: var(--pt-t2);
}

.pt-pwstrength__list li.is-met .pt-icon {
  color: var(--pt-ok);
}

/* 只给读屏器的状态词。项目里还没有全局的视觉隐藏类，所以就地写一份 */
.pt-pwstrength__state {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip-path: inset(50%);
  white-space: nowrap;
}

.pt-pwstrength__note {
  margin: 0;
  font-size: var(--pt-fz-label);
  line-height: var(--pt-lh-body);
  color: var(--pt-t3);
}
</style>
