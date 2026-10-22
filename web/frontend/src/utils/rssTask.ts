import type { RssTaskState, RssTaskStatus } from "@/api";

/**
 * RSS 任务（原「调度器」）的状态文案。状态来自 GET /api/control/status：
 * 后端按正在运行的任务、手动启停、自动启动开关和上一次没启动的原因给出 state，这里只负责说人话。
 *
 * RSS 任务只管 RSS 订阅的定时拉取与推送；刷流、辅种、自动删种、签到等功能各自运行，不受它影响。
 */
export type RssTone = "ok" | "warn" | "neutral";

export interface RssTaskView {
  /** 状态栏格子里的短文案 */
  label: string;
  /** 浮层胶囊里的状态 */
  pill: string;
  /** 浮层里那句说明：在跑什么，或者为什么没在跑 */
  detail: string;
  tone: RssTone;
  icon: string;
}

/** RSS 任务管什么、不管什么：浮层和提示条都写这一句 */
export const RSS_TASK_SCOPE =
  "RSS 任务按间隔拉取 RSS 订阅并推送种子；刷流、辅种、自动删种、签到等功能不受它影响。";

const pad = (n: number) => String(n).padStart(2, "0");

function startedText(iso: string | undefined, now: Date): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const hm = `${pad(d.getHours())}:${pad(d.getMinutes())}`;
  const sameDay =
    d.getFullYear() === now.getFullYear() &&
    d.getMonth() === now.getMonth() &&
    d.getDate() === now.getDate();
  return sameDay ? `今天 ${hm}` : `${d.getMonth() + 1} 月 ${d.getDate()} 日 ${hm}`;
}

const UNKNOWN: RssTaskView = {
  label: "RSS 任务",
  pill: "状态未知",
  detail: "还没有取到 RSS 任务的状态",
  tone: "neutral",
  icon: "activity",
};

const NOT_RUNNING = {
  label: "RSS 任务未运行",
  pill: "未运行",
  tone: "warn",
  icon: "circle-alert",
} as const;

export function rssTaskView(st: RssTaskStatus | null, now: Date = new Date()): RssTaskView {
  if (!st) return UNKNOWN;
  switch (st.state) {
    case "running": {
      const when = startedText(st.started_at, now);
      return {
        label: "RSS 任务运行中",
        pill: "运行中",
        detail: `${st.jobs} 个订阅在运行${when ? `，${when} 启动` : ""}`,
        tone: "ok",
        icon: "circle-check",
      };
    }
    case "starting":
      return {
        label: "RSS 任务启动中",
        pill: "启动中",
        detail: "正在按配置启动，连接默认下载器可能要等一会儿",
        tone: "neutral",
        icon: "loader-circle",
      };
    case "paused":
      return {
        label: "RSS 任务已停止",
        pill: "已停止",
        detail: `已手动停止，再次启动前不会拉取 RSS 订阅${st.auto_start ? "；程序重启后会自动运行" : ""}`,
        tone: "warn",
        icon: "circle-pause",
      };
    case "manual":
      return {
        label: "RSS 任务未启动",
        pill: "未启动",
        detail: "没有打开自动启动：每次程序启动（包括重启容器）后都要手动启动一次",
        tone: "warn",
        icon: "circle-alert",
      };
    case "not_ready":
      return { ...NOT_RUNNING, detail: "全局设置里的下载目录为空，RSS 任务不会启动" };
    case "no_downloader":
      return { ...NOT_RUNNING, detail: "没有设置默认下载器，RSS 任务没有启动" };
    case "downloader_unhealthy":
      return {
        ...NOT_RUNNING,
        detail: "默认下载器连不上，RSS 任务没有启动；下载器恢复后点「启动」",
      };
    case "stopped":
      return { ...NOT_RUNNING, detail: "没有在运行的 RSS 任务" };
    case "no_subscriptions":
      return {
        label: "RSS 任务无订阅",
        pill: "无订阅",
        detail: "已启用的站点下还没有 RSS 订阅",
        tone: "neutral",
        icon: "rss",
      };
    default:
      return UNKNOWN;
  }
}

export interface RssBanner {
  text: string;
  /** 主按钮：启动 RSS 任务，或者先去设置下载器 */
  action: "start" | "downloaders";
}

/** 提示条只在这几种状态出现：RSS 任务没在跑，而且不是用户自己刚停的 */
const BANNER_STATES: readonly RssTaskState[] = [
  "manual",
  "no_downloader",
  "downloader_unhealthy",
  "stopped",
];

/**
 * 页面顶上那条「RSS 任务没有运行」：没开自动启动、有订阅、RSS 任务又没在跑时才出现。
 * 手动停止（paused）不提示：那是用户刚做的决定，状态栏已经写着「已停止」。
 */
export function rssBanner(st: RssTaskStatus | null): RssBanner | null {
  if (!st || st.auto_start || st.subscriptions <= 0 || !BANNER_STATES.includes(st.state))
    return null;
  switch (st.state) {
    case "manual":
      return {
        text: "RSS 订阅不会拉取新种子。自动启动没有打开，每次程序启动（包括重启容器）后都要手动启动。",
        action: "start",
      };
    case "no_downloader":
      return { text: "没有设置默认下载器，RSS 订阅不会拉取新种子。", action: "downloaders" };
    case "downloader_unhealthy":
      return { text: "默认下载器连不上，RSS 订阅不会拉取新种子。", action: "start" };
    default:
      return { text: "RSS 订阅不会拉取新种子。", action: "start" };
  }
}

/** 状态没取到时两个按钮都能点：不因为一次查询失败把人挡在外面 */
export function canStopRss(st: RssTaskStatus | null): boolean {
  return !st || st.jobs > 0 || st.state === "starting";
}

export function canStartRss(st: RssTaskStatus | null): boolean {
  return !st || st.state !== "running";
}
