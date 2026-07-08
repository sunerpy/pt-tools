/**
 * 登录态探测状态（ProbeStatus）前端映射工具。
 *
 * 后端枚举定义于 internal/sitelogin/result.go，`probeNow` 接口返回的
 * `last_probe_status` 为探测状态码而非成功标志：它可能是失败状态。
 * 后端 handler（web/api_site_login.go）在探测执行后恒返回 `ok: true`
 * （仅表示未发生锁冲突，探测已运行），因此 **`ok` 不是探测成功的可靠信号**，
 * 判定探测是否成功必须以 `last_probe_status === "OK"` 为准。
 */

/** 探测状态严重级别，与 ElMessage 的类型一一对应。 */
export type ProbeSeverity = "success" | "info" | "warning" | "error";

interface ProbeStatusMeta {
  /** 严重级别：仅 OK 为 success（绿色），真正的失败绝不为 success。 */
  severity: ProbeSeverity;
  /** 人类可读的中文标签。 */
  label: string;
}

const PROBE_STATUS_MAP: Record<string, ProbeStatusMeta> = {
  OK: { severity: "success", label: "正常" },
  NOT_APPLICABLE: { severity: "info", label: "不适用" },
  SESSION_EXPIRED: { severity: "warning", label: "会话已过期" },
  CHALLENGE: { severity: "warning", label: "被反爬拦截" },
  RATE_LIMITED: { severity: "warning", label: "请求过于频繁" },
  KEY_ERROR: { severity: "warning", label: "密钥错误" },
  NETWORK_ERROR: { severity: "error", label: "网络错误" },
  PARSE_ERROR: { severity: "error", label: "解析失败" },
  UNKNOWN: { severity: "error", label: "未知状态" },
};

/** 未识别 / 空状态统一按未知错误处理。 */
const UNKNOWN_META: ProbeStatusMeta = { severity: "error", label: "未知状态" };

function probeStatusMeta(status?: string): ProbeStatusMeta {
  if (!status) return UNKNOWN_META;
  return PROBE_STATUS_MAP[status] ?? UNKNOWN_META;
}

/**
 * 判定探测是否真正成功。
 * 以 `last_probe_status === "OK"` 为唯一权威信号（后端 `ok` 恒为 true，不可用于判定）。
 */
export function isProbeSuccess(status?: string): boolean {
  return status === "OK";
}

/** 获取探测状态的严重级别，用于选择 ElMessage 类型。 */
export function probeStatusSeverity(status?: string): ProbeSeverity {
  return probeStatusMeta(status).severity;
}

/**
 * 获取探测状态的展示文案，保留原始状态码以便排障，
 * 例如 "解析失败 (PARSE_ERROR)"；空状态返回 "未知状态"。
 */
export function probeStatusLabel(status?: string): string {
  const meta = probeStatusMeta(status);
  if (!status) return meta.label;
  return `${meta.label} (${status})`;
}
