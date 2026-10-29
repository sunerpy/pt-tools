/*
 * 后台通知（签到、追剧、每日战报、保号提醒…）的叫法，以及它们的投递写进操作审计时的读法。
 *
 * 来源是 scheduler.MonitorNotifyEntry 的 Source；投递写进审计时命令是 `notify:<来源>:<种类>`，
 * 参数里有标题、第几次投递，失败时还有原因、下一次重试的时间或 final（不再重试），
 * 见 cmd/login_reminder_wiring.go 的 monitorDeliveryAudit。
 */

/** 通知来源的叫法：RSS 以外都是后台监控发的 */
export const NOTIFY_SOURCE_LABELS: Record<string, string> = {
  rss: "RSS",
  login: "保号提醒",
  attendance: "签到",
  daily_report: "每日战报",
  downloader_assistant: "失效种子",
  free_end: "免费到期",
  media: "订阅与入库",
  remote: "远程访问",
};

/** 同一来源下要分开叫的种类（来源:种类）；没列的用来源的叫法 */
const KIND_LABELS: Record<string, string> = {
  "login:test": "保号提醒（测试）",
  "login:recovery": "保号提醒：已恢复",
  "login:failure": "保号提醒：探测失败",
  "free_end:pause_failed": "免费到期：暂停失败",
  "remote:paired": "远程访问：新设备配对",
  "media:organized": "入库通知",
  "media:subscribe_downloaded": "订阅：已下载",
  "media:douban_abnormal": "豆瓣想看同步异常",
  "media:followshow_found": "追剧：下载了新的集",
  "media:followshow_waiting": "追剧：锁定的版本久等不到",
  "media:followshow_failed": "追剧：连续几次没搜成",
};

export const NOTIFY_COMMAND_PREFIX = "notify:";

export function notifySourceLabel(source: string): string {
  return NOTIFY_SOURCE_LABELS[source] ?? source;
}

/** 审计里这一行是不是后台通知的投递 */
export function isNotifyCommand(command: string | undefined | null): boolean {
  return (command ?? "").startsWith(NOTIFY_COMMAND_PREFIX);
}

/** `notify:daily_report:daily` → 「每日战报」；不是通知的命令原样返回 */
export function notifyCommandLabel(command: string): string {
  if (!isNotifyCommand(command)) return command;
  const rest = command.slice(NOTIFY_COMMAND_PREFIX.length);
  const [source = "", kind = ""] = rest.split(":");
  return KIND_LABELS[`${source}:${kind}`] ?? notifySourceLabel(source);
}

/** 通知投递写进审计的参数（见文件头）；读不出来时是空对象 */
export interface NotifyAuditArgs {
  title?: string;
  subject?: string;
  attempt?: number;
  error?: string;
  final?: boolean;
  next_retry_at?: string;
}

export function notifyAuditArgs(argsJSON: string | undefined | null): NotifyAuditArgs {
  if (!argsJSON) return {};
  try {
    const v = JSON.parse(argsJSON) as unknown;
    return v && typeof v === "object" && !Array.isArray(v) ? (v as NotifyAuditArgs) : {};
  } catch {
    return {};
  }
}

function hhmm(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const p = (n: number) => String(n).padStart(2, "0");
  return `${p(d.getHours())}:${p(d.getMinutes())}`;
}

/**
 * 失败的那次投递为什么没送到：原因，加上第几次、什么时候再试（或不再试）。
 * 送达的、或参数里没有原因的返回空串。
 */
export function notifyFailureText(args: NotifyAuditArgs): string {
  if (!args.error) return "";
  const tail: string[] = [];
  if (args.attempt) tail.push(`第 ${args.attempt} 次`);
  if (args.final) tail.push("不再重试");
  else if (args.next_retry_at) {
    const at = hhmm(args.next_retry_at);
    if (at) tail.push(`${at} 重试`);
  }
  return tail.length ? `${args.error}（${tail.join("，")}）` : args.error;
}
