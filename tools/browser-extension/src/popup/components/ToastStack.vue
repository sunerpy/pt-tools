<script setup lang="ts">
import { useToast } from "../composables/useToast";

const { toasts, dismiss } = useToast();

const ICON: Record<string, string> = {
  success: "✓",
  error: "✕",
  warning: "⚠",
  info: "ℹ",
};
</script>

<template>
  <div class="toast-stack" role="status" aria-live="polite">
    <transition-group name="toast">
      <div
        v-for="toast in toasts"
        :key="toast.id"
        :class="['toast', `toast-${toast.severity}`]"
        @click="dismiss(toast.id)">
        <span class="toast-icon">{{ ICON[toast.severity] }}</span>
        <span class="toast-message">{{ toast.message }}</span>
      </div>
    </transition-group>
  </div>
</template>

<style scoped>
.toast-stack {
  position: fixed;
  left: 12px;
  right: 12px;
  bottom: 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  z-index: 9999;
  pointer-events: none;
}

.toast {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 10px 12px;
  border-radius: 8px;
  font-size: 12px;
  line-height: 1.4;
  cursor: pointer;
  pointer-events: auto;
  background: var(--surface, #fff);
  border: 1px solid var(--border, #e5e5e5);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.12);
  word-break: break-word;
}

.toast-icon {
  flex-shrink: 0;
  font-weight: 700;
  font-size: 14px;
  line-height: 1.2;
}

.toast-message {
  flex: 1;
  min-width: 0;
}

.toast-success {
  background: #e8f7ee;
  border-color: #b9e1c5;
  color: #0f5132;
}

.toast-success .toast-icon {
  color: #1a7f44;
}

.toast-error {
  background: #fdecec;
  border-color: #f3b6b6;
  color: #842029;
}

.toast-error .toast-icon {
  color: #c0392b;
}

.toast-warning {
  background: #fff7e0;
  border-color: #ffd980;
  color: #7a5a00;
}

.toast-warning .toast-icon {
  color: #b07d00;
}

.toast-info {
  background: #e8f1fb;
  border-color: #b9d4f0;
  color: #0c4a82;
}

.toast-info .toast-icon {
  color: #1c6dc7;
}

.toast-enter-active,
.toast-leave-active {
  transition:
    opacity 0.18s ease,
    transform 0.22s ease;
}

.toast-enter-from {
  opacity: 0;
  transform: translateY(8px);
}

.toast-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
</style>
