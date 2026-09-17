<script setup lang="ts">
import { computed } from "vue";
import { useLogLevelStore } from "../../stores/logLevel";
import { useThemeStore } from "../../stores/theme";
import PtIcon from "../PtIcon";

/**
 * 偏好面板 —— 设计稿板 41 把「深色开关 / 配色 / 日志级别 / 退出登录」四项
 * 都收进 rail 头像的 popover 里，这里就是那个 popover 的内容；移动端「我的」
 * 面板复用同一个组件，两处行为必须一致，所以做成组件而不是抄两遍模板。
 *
 * 板 41 写的是「深色开关（el-switch）」，但 themeStore 现在有第三档
 * auto（跟随系统），开关表达不了三态，这里改成三段选择器 —— 是超集，
 * 交互目标不变。
 */
const themeStore = useThemeStore();
const logLevelStore = useLogLevelStore();

const LEVEL_LABEL: Record<string, string> = {
  debug: "调试",
  info: "信息",
  warn: "警告",
  error: "错误",
};

const levels = computed(() =>
  logLevelStore.availableLevels.map((v) => ({ value: v, label: LEVEL_LABEL[v] ?? v })),
);

function logout() {
  window.location.href = "/logout";
}
</script>

<template>
  <div class="pt-prefs">
    <section class="pt-prefs__block">
      <div class="pt-prefs__label">外观</div>
      <div class="pt-prefs__seg" role="group" aria-label="明暗模式">
        <button
          v-for="m in themeStore.modes"
          :key="m.value"
          type="button"
          class="pt-prefs__seg-btn"
          :class="{ 'is-active': themeStore.mode === m.value }"
          :aria-pressed="themeStore.mode === m.value"
          @click="themeStore.setMode(m.value)">
          <PtIcon :name="m.icon" :size="14" />
          <span>{{ m.label }}</span>
        </button>
      </div>
    </section>

    <section class="pt-prefs__block">
      <div class="pt-prefs__label">配色</div>
      <div class="pt-prefs__palettes">
        <button
          v-for="p in themeStore.palettes"
          :key="p.value"
          type="button"
          class="pt-prefs__palette"
          :class="{ 'is-active': themeStore.palette === p.value }"
          :aria-pressed="themeStore.palette === p.value"
          :title="p.hint"
          @click="themeStore.setPalette(p.value)">
          <span class="pt-prefs__swatch" aria-hidden="true">
            <i v-for="(c, i) in p.swatch" :key="i" :style="{ background: c }" />
          </span>
          <span class="pt-prefs__palette-name">{{ p.label }}</span>
          <PtIcon v-if="themeStore.palette === p.value" name="check" :size="14" />
        </button>
      </div>
    </section>

    <section class="pt-prefs__block">
      <div class="pt-prefs__label">日志级别</div>
      <div class="pt-prefs__seg pt-prefs__seg--4" role="group" aria-label="日志级别">
        <button
          v-for="l in levels"
          :key="l.value"
          type="button"
          class="pt-prefs__seg-btn"
          :class="{ 'is-active': logLevelStore.currentLevel === l.value }"
          :aria-pressed="logLevelStore.currentLevel === l.value"
          :disabled="logLevelStore.loading"
          @click="logLevelStore.setLogLevel(l.value)">
          {{ l.label }}
        </button>
      </div>
    </section>

    <footer class="pt-prefs__foot">
      <span class="pt-prefs__user">
        <PtIcon name="user" :size="14" />
        <span>admin</span>
      </span>
      <button type="button" class="pt-prefs__logout" @click="logout">
        <PtIcon name="log-out" :size="14" />
        <span>退出登录</span>
      </button>
    </footer>
  </div>
</template>

<style scoped>
.pt-prefs {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-4);
  min-width: 236px;
}

.pt-prefs__block {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
}

.pt-prefs__label {
  font-size: var(--pt-fz-label);
  font-weight: 600;
  color: var(--pt-t3);
}

.pt-prefs__seg {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 2px;
  padding: 2px;
  background: var(--pt-hover);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-md);
}

.pt-prefs__seg--4 {
  grid-template-columns: repeat(4, 1fr);
}

.pt-prefs__seg-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  height: 26px;
  padding: 0 6px;
  font-family: inherit;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
  background: transparent;
  border: 0;
  border-radius: calc(var(--pt-r-md) - 2px);
  cursor: pointer;
  transition:
    background var(--pt-transition-fast),
    color var(--pt-transition-fast);
}

.pt-prefs__seg-btn:hover:not(:disabled) {
  color: var(--pt-t1);
  background: var(--pt-surface);
}

.pt-prefs__seg-btn.is-active {
  color: var(--pt-on-p);
  background: var(--pt-p);
}

.pt-prefs__seg-btn:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

.pt-prefs__palettes {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.pt-prefs__palette {
  display: flex;
  align-items: center;
  gap: var(--pt-space-2);
  height: 32px;
  padding: 0 var(--pt-space-2);
  font-family: inherit;
  font-size: var(--pt-fz-body);
  color: var(--pt-t2);
  text-align: left;
  background: transparent;
  border: 0;
  border-radius: var(--pt-r-sm);
  cursor: pointer;
  transition:
    background var(--pt-transition-fast),
    color var(--pt-transition-fast);
}

.pt-prefs__palette:hover {
  color: var(--pt-t1);
  background: var(--pt-hover);
}

.pt-prefs__palette.is-active {
  color: var(--pt-p);
  background: var(--pt-p-soft);
}

.pt-prefs__palette-name {
  flex: 1;
}

.pt-prefs__swatch {
  display: inline-flex;
  overflow: hidden;
  border: 1px solid var(--pt-border-strong);
  border-radius: 4px;
}

.pt-prefs__swatch i {
  display: block;
  width: 8px;
  height: 16px;
}

.pt-prefs__foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-top: var(--pt-space-3);
  border-top: 1px solid var(--pt-border);
}

.pt-prefs__user {
  display: inline-flex;
  gap: 6px;
  align-items: center;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
}

.pt-prefs__logout {
  display: inline-flex;
  gap: 4px;
  align-items: center;
  height: 26px;
  padding: 0 8px;
  font-family: inherit;
  font-size: var(--pt-fz-sm);
  color: var(--pt-dang);
  background: transparent;
  border: 1px solid transparent;
  border-radius: var(--pt-r-sm);
  cursor: pointer;
  transition:
    background var(--pt-transition-fast),
    border-color var(--pt-transition-fast);
}

.pt-prefs__logout:hover {
  background: var(--pt-dang-soft);
  border-color: color-mix(in srgb, var(--pt-dang) 32%, transparent);
}
</style>
