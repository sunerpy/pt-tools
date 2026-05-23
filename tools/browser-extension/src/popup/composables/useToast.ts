import { ref } from "vue";

export type ToastSeverity = "success" | "error" | "warning" | "info";

export interface Toast {
  id: number;
  severity: ToastSeverity;
  message: string;
  timeoutMs: number;
}

const toasts = ref<Toast[]>([]);
let nextId = 1;

const DEFAULT_TIMEOUTS: Record<ToastSeverity, number> = {
  success: 3500,
  info: 3500,
  warning: 6000,
  error: 8000,
};

function push(severity: ToastSeverity, message: string, timeoutMs?: number): number {
  const id = nextId++;
  const ttl = timeoutMs ?? DEFAULT_TIMEOUTS[severity];
  toasts.value = [...toasts.value, { id, severity, message, timeoutMs: ttl }];
  if (ttl > 0) {
    setTimeout(() => dismiss(id), ttl);
  }
  return id;
}

function dismiss(id: number): void {
  toasts.value = toasts.value.filter((toast) => toast.id !== id);
}

export function useToast(): {
  toasts: typeof toasts;
  success: (message: string, timeoutMs?: number) => number;
  error: (message: string, timeoutMs?: number) => number;
  warning: (message: string, timeoutMs?: number) => number;
  info: (message: string, timeoutMs?: number) => number;
  dismiss: (id: number) => void;
} {
  return {
    toasts,
    success: (msg, ttl) => push("success", msg, ttl),
    error: (msg, ttl) => push("error", msg, ttl),
    warning: (msg, ttl) => push("warning", msg, ttl),
    info: (msg, ttl) => push("info", msg, ttl),
    dismiss,
  };
}
