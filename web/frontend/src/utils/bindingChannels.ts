/*
 * 绑定页「通道」一栏的读法（接口见 internal/app/binding_channels.go 的 ChannelBindingView）。
 *
 * 一个通道要分开说三件事：机器人在不在线（运行状态）、没有指定收件人的通知（每日战报、签到…）发给谁、
 * 名单里每个账号绑定了没有。2026-10-28 用户以为「机器人接上了就是绑定了」：机器人在线、通知照常发，
 * 账号却在 10-21 发过 /unbind，命令要重新绑定才能用 —— 三件事混在一起时页面上看不出来。
 */

/** 通知收件人从哪来 */
const NOTIFY_VIA: Record<string, string> = {
  default_chat_id: "默认 Chat ID",
  admin: "第一个管理员",
  allowed: "第一个允许的账号",
  binding: "第一个绑定的账号",
};

export function notifyViaText(via: string | undefined): string {
  return (via && NOTIFY_VIA[via]) || "";
}

const ROLE_TEXT: Record<string, string> = { admin: "管理员", allowed: "允许" };

/** 名单里的身份：管理员、允许；都不是时为空数组 */
export function roleTexts(roles: string[] | undefined): string[] {
  return (roles ?? []).map((r) => ROLE_TEXT[r] ?? r);
}

const RESULT_TEXT: Record<string, string> = {
  success: "成功",
  "denied:not_bound": "被拒：没有绑定",
  "denied:not_allowed": "被拒：绑定已禁用",
  "denied:not_admin": "被拒：要管理员",
  "denied:rate_limit": "被拒：太频繁",
  "denied:unknown_command": "不认识的命令",
  "denied:not_in_whitelist": "被拒：不在名单里",
  "denied:already_bound": "已经绑定过",
};

/** 命令的写法：机器人命令前面加 /；空命令是普通消息 */
function commandText(command: string): string {
  if (!command) return "发了消息";
  if (command === "assistant") return "和 AI 助手对话";
  if (command === "user_message_hinted") return "发了消息";
  if (command.startsWith("session:")) return "回复了提问";
  return `/${command}`;
}

function resultText(result: string): string {
  // 绑定的账号发了不是命令的消息：机器人回了一句「只认命令」，没有成败可说
  if (result === "user_message_hinted") return "";
  if (RESULT_TEXT[result]) return RESULT_TEXT[result]!;
  const kind = result.split(":")[0];
  if (kind === "denied") return "被拒";
  if (kind === "error") return "出错";
  return result;
}

function shortTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const p = (n: number) => String(n).padStart(2, "0");
  return `${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

/** 最近一次往来：「/unbind 成功 · 10-21 20:57」；没有记录时为空串 */
export function activityText(
  last: { command: string; result: string; at: string } | undefined | null,
): string {
  if (!last) return "";
  const when = shortTime(last.at);
  const what = `${commandText(last.command)} ${resultText(last.result)}`.trim();
  return when ? `${what} · ${when}` : what;
}

/** 用户 ID 只露头尾（与已绑定用户表同一套） */
export function maskUserId(userId?: string): string {
  if (!userId) return "-";
  if (userId.length <= 6) return userId;
  return userId.slice(0, 2) + "***" + userId.slice(-4);
}
