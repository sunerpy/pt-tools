/** App 的四套主题（apps/mobile/lib/src/app/theme.dart 的 PtTheme）。 */
export type AppTheme = "light" | "dark" | "warm" | "graphite";

const THEMES: readonly AppTheme[] = ["light", "dark", "warm", "graphite"];

/** App 传来的主题名；不认识或没传时按 dark 退回明亮或暗黑（旧版 App 只传 dark）。 */
export function appThemeOf(value: string | null, dark: boolean): AppTheme {
  const known = THEMES.find((t) => t === value);
  return known ?? (dark ? "dark" : "light");
}
