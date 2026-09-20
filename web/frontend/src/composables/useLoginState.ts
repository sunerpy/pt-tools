import type { Ref } from "vue";
import type { SiteLoginState } from "@/api";

export type ReminderTier =
  | "none"
  | "pre-warn"
  | "30d"
  | "14d"
  | "7d"
  | "3d"
  | "1d"
  | "banned-imminent"
  | "unknown";

export function useLoginState(loginStates: Ref<Record<string, SiteLoginState>>) {
  function loginState(name: string): SiteLoginState | undefined {
    return loginStates.value[name];
  }

  function effectiveLastActive(name: string): number {
    return loginState(name)?.effective_last_active_at ?? 0;
  }

  function lastAccess(name: string): number {
    return loginState(name)?.last_access_at ?? 0;
  }

  function lastLogin(name: string): number {
    return loginState(name)?.last_login_at ?? 0;
  }

  function daysRemaining(name: string): number | null {
    const st = loginState(name);
    if (!st || st.tier === "unknown") return null;
    return st.days_remaining;
  }

  function reminderTier(name: string): ReminderTier {
    return (loginState(name)?.tier as ReminderTier) ?? "unknown";
  }

  function probeModeOf(name: string): "auto" | "manual" | "disabled" {
    return (loginState(name)?.probe_mode as "auto" | "manual" | "disabled") ?? "auto";
  }

  /*
   * 只有**明确的** none（后端算出来「离阈值还远」）才回空串（= 正常 / 绿）。
   * 认不出来的档位一律走 info：它和 unknown 同一档，界面上是中性灰。
   * 此前 default 回空串，于是任何没见过的档位字符串都被染成绿色的「正常」，
   * 而 tierLabel 的 default 印的是「未知」—— 一枚绿底的「未知」胶囊，
   * 把「没有数据」说成了「站点健康」。后端将来新加一档就会踩到。
   */
  function tierTagType(tier: ReminderTier): "" | "info" | "primary" | "warning" | "danger" {
    switch (tier) {
      case "none":
        return "";
      case "30d":
      case "pre-warn":
        return "primary";
      case "14d":
      case "7d":
        return "warning";
      case "3d":
      case "1d":
      case "banned-imminent":
        return "danger";
      case "unknown":
      default:
        return "info";
    }
  }

  function tierLabel(tier: ReminderTier): string {
    switch (tier) {
      case "none":
        return "正常";
      case "pre-warn":
      case "30d":
        return "30 天内";
      case "14d":
        return "14 天内";
      case "7d":
        return "7 天内";
      case "3d":
        return "3 天内";
      case "1d":
        return "1 天内";
      case "banned-imminent":
        return "即将封禁";
      case "unknown":
      default:
        return "未知";
    }
  }

  function daysCellClass(name: string): string[] {
    const classes = ["days-remaining-value"];
    const st = loginState(name);
    if (!st || st.tier === "unknown") return classes;
    if (st.days_remaining <= 1) classes.push("days-remaining--critical");
    else if (st.tier !== "none") classes.push("days-remaining--warn");
    return classes;
  }

  return {
    loginState,
    effectiveLastActive,
    lastAccess,
    lastLogin,
    daysRemaining,
    reminderTier,
    probeModeOf,
    tierTagType,
    tierLabel,
    daysCellClass,
  };
}
