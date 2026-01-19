import { ref } from "vue";

interface ToastMessage {
  id: number;
  message: string;
  type: "success" | "error";
}

const toasts = ref<ToastMessage[]>([]);
let nextId = 0;

export function useToast() {
  function show(message: string, type: "success" | "error" = "success") {
    const id = nextId++;
    toasts.value.push({ id, message, type });
    setTimeout(() => {
      const index = toasts.value.findIndex((t) => t.id === id);
      if (index > -1) {
        toasts.value.splice(index, 1);
      }
    }, 2000);
  }

  function success(message: string) {
    show(message, "success");
  }

  function error(message: string) {
    show(message, "error");
  }

  return { toasts, show, success, error };
}
