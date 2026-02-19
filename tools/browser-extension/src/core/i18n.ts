import enUS from "./locales/en-US";
import zhCN from "./locales/zh-CN";

export type MessageMap = Record<keyof typeof zhCN, string>;
export type MessageKey = keyof MessageMap;
export type Locale = "zh-CN" | "en-US";

const LOCALES: Record<Locale, MessageMap> = {
  "zh-CN": zhCN,
  "en-US": enUS,
};

function detectLocale(): Locale {
  const lang =
    typeof chrome !== "undefined" && chrome.i18n ? chrome.i18n.getUILanguage() : navigator.language;
  return lang.startsWith("zh") ? "zh-CN" : "en-US";
}

let currentLocale: Locale = detectLocale();

export function initI18n(): void {
  currentLocale = detectLocale();
}

export function t(key: MessageKey, ...args: Array<string | number>): string {
  const messages = LOCALES[currentLocale] ?? LOCALES["zh-CN"];
  let text: string = messages[key] ?? key;
  for (let i = 0; i < args.length; i += 1) {
    text = text.replace(`{${i}}`, String(args[i]));
  }
  return text;
}

export function getLocale(): Locale {
  return currentLocale;
}

export function getMessages(locale: Locale): MessageMap {
  return LOCALES[locale];
}
