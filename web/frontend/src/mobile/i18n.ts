import { inject, type InjectionKey } from "vue";
export const Language: InjectionKey<boolean> = Symbol("mobileEnglish");
export function useWords() {
  const en = inject(Language, false);
  return (zh: string, english: string) => (en ? english : zh);
}
