import { inject, type InjectionKey } from "vue";
export const Language: InjectionKey<boolean> = Symbol("mobileEnglish");
/** 编辑页是不是英文（App 的语言是英文时） */
export function useEnglish() {
  return inject(Language, false);
}
export function useWords() {
  const en = useEnglish();
  return (zh: string, english: string) => (en ? english : zh);
}
