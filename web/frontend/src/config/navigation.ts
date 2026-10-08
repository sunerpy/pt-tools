import type { LucideIconName } from "../icons/lucide";

/**
 * 导航结构 —— 与 Penpot 设计稿 G Cockpit 的 `D.nav` 一一对应（6 组 / 19 个路由）。
 * 分组顺序、图标名和文案都以设计稿为准；改这里等于改设计稿，先对一下画板。
 */
export interface NavItem {
  /** 路由 path，用于跳转与高亮 */
  path: string;
  label: string;
  icon: LucideIconName;
  /** 在新标签页打开（下载器 Web UI 是独立控制台，沿用旧行为） */
  external?: boolean;
  /** 右侧计数徽标取自哪个统计字段，没有就不显示 */
  badge?: "sites" | "tasks" | "paused";
  /**
   * 同时出现在深色 rail 的**主入口区**（设计稿 `D.rail` 的 8 个快捷入口）。
   * rail 底部分隔线之下那一格不走这个标记，见 RAIL_FOOT_ITEM。
   */
  rail?: boolean;
}

export interface NavGroup {
  title: string;
  items: NavItem[];
}

export const NAV_GROUPS: readonly NavGroup[] = [
  {
    title: "概览",
    items: [
      { path: "/userinfo", label: "用户统计", icon: "gauge", rail: true },
      { path: "/userinfo/export", label: "数据导出", icon: "file-down" },
    ],
  },
  {
    title: "站点",
    items: [
      { path: "/sites", label: "站点列表", icon: "globe", badge: "sites", rail: true },
      { path: "/supported-sites", label: "已支持站点", icon: "badge-check" },
      /* 路线图 M7：画板没有这一页，沿用站点组列表页的样式 */
      { path: "/cookiecloud", label: "CookieCloud 导入", icon: "cloud-download" },
    ],
  },
  {
    title: "下载",
    items: [
      { path: "/search", label: "种子搜索", icon: "search", rail: true },
      { path: "/tasks", label: "任务列表", icon: "list-checks", badge: "tasks", rail: true },
      { path: "/paused", label: "暂停任务", icon: "circle-pause", badge: "paused", rail: true },
      /* 路线图 M3：画板没有这一页，沿用下载组的列表页样式 */
      { path: "/brush", label: "刷流任务", icon: "trending-up" },
      /*
       * 画板 18 把控制台画在外壳里（head 64 → bar-64 → 左列 276 / 右 788），
       * 不再是新标签页里的独立控制台，所以这一项走站内跳转。
       */
      { path: "/downloader-hub", label: "下载器 Web UI", icon: "app-window" },
      { path: "/downloaders", label: "下载器设置", icon: "server" },
      /* 路线图 M4：画板没有这一页，沿用下载组的列表页样式 */
      { path: "/downloader-assistant", label: "下载器助手", icon: "wrench" },
      /* 路线图 M5：画板没有这一页，沿用下载组的列表页样式 */
      { path: "/transfer", label: "转移做种", icon: "arrow-right-left" },
      /* 路线图 M6：画板没有这一页，沿用下载组的列表页样式 */
      { path: "/reseed", label: "IYUU 辅种", icon: "share-2" },
    ],
  },
  {
    /* 路线图 M9–M11 的媒体自动化：画板没有这一组，沿用列表页的样式 */
    title: "媒体",
    items: [
      { path: "/media/explore", label: "探索", icon: "compass" },
      { path: "/media/subscriptions", label: "订阅", icon: "bookmark" },
      { path: "/media/library", label: "媒体库", icon: "folder-open" },
      { path: "/media/history", label: "整理历史", icon: "history" },
      { path: "/media/recognize", label: "媒体识别", icon: "scan" },
    ],
  },
  {
    title: "规则",
    items: [
      { path: "/filter-rules", label: "过滤规则", icon: "list-filter", rail: true },
      { path: "/cleanup", label: "自动清理", icon: "trash-2" },
    ],
  },
  {
    title: "ChatOps",
    items: [
      { path: "/chatops/notifications", label: "消息通知", icon: "bell", rail: true },
      { path: "/chatops/bindings", label: "ChatOps 绑定", icon: "link" },
      { path: "/chatops/audit", label: "操作审计", icon: "shield-check" },
      { path: "/chatops/rss-notifications", label: "RSS 通知日志", icon: "rss" },
    ],
  },
  {
    title: "系统",
    items: [
      { path: "/global", label: "全局设置", icon: "settings", rail: true },
      { path: "/cloak-config", label: "CloakBrowser", icon: "shield" },
      { path: "/logs", label: "运行日志", icon: "scroll-text" },
      { path: "/password", label: "修改密码", icon: "key-round" },
    ],
  },
] as const;

/**
 * 移动端底部 5 个 tab（设计稿 `D.mTabs`）。
 * 「我的」在设计稿板 34 上是一个设置聚合页；产品里没有这个路由，
 * 所以做成一张上拉面板，把「系统」「ChatOps」两组折进去，行为等价且不新增路由。
 */
export const MOBILE_TABS: readonly {
  key: string;
  label: string;
  icon: LucideIconName;
  path?: string;
}[] = [
  { key: "overview", label: "概览", icon: "gauge", path: "/userinfo" },
  { key: "sites", label: "站点", icon: "globe", path: "/sites" },
  { key: "tasks", label: "任务", icon: "list-checks", path: "/tasks" },
  { key: "search", label: "搜索", icon: "search", path: "/search" },
  { key: "me", label: "我的", icon: "user" },
] as const;

/** 所有导航项打平，供标题解析与高亮匹配使用 */
export const NAV_ITEMS: readonly NavItem[] = NAV_GROUPS.flatMap((g) => g.items);

/**
 * 深色 rail 上的快捷入口（设计稿 `D.rail`），直接从 NAV_GROUPS 里带 rail 标记的项派生。
 *
 * 这里原来另抄了一份 path/label/icon，改一次导航文案要记着改两处，迟早漂移。
 * 顺序跟着 NAV_GROUPS 走，与设计稿上那 8 项的次序一致。
 *
 * 导航列钉在旁边时这排图标是重复的，由 shell.css 藏掉；收起后它就是唯一的导航。
 */
export const RAIL_ITEMS: readonly NavItem[] = NAV_ITEMS.filter((i) => i.rail);

/** rail 底部那一格指向的路由（设计稿 `D.rail` 的 `rail-log`，在 foot 分隔线之下） */
export const RAIL_FOOT_PATH = "/logs";

/**
 * rail 底部分隔线之下那一个导航入口。
 *
 * 不给它打 `rail` 标记：带标记的项渲染在 rail 的主入口区（`.pt-rail__items`），
 * 而画板把运行日志画在 foot-rule 之下的底部那一组里。label 与 icon 仍然从
 * NAV_ITEMS 取，导航文案只有一处、不会漂移。
 *
 * 它和主入口区那 8 项同一档：导航列钉住时一起藏掉（shell.css 的
 * `.is-nav-docked .pt-rail__item`），否则和导航列「系统 › 运行日志」常态重复。
 */
export const RAIL_FOOT_ITEM: NavItem | undefined = NAV_ITEMS.find((i) => i.path === RAIL_FOOT_PATH);

/**
 * 路由 name → 导航项 path。详情页、子路由的 name 与导航 path 对不上，
 * 在这里显式映射，避免详情页把整组导航的高亮丢掉。
 */
const ROUTE_NAME_TO_NAV: Record<string, string> = {
  userinfo: "/userinfo",
  "userinfo-export": "/userinfo/export",
  sites: "/sites",
  "site-detail": "/sites",
  "supported-sites": "/supported-sites",
  search: "/search",
  tasks: "/tasks",
  paused: "/paused",
  "downloader-hub": "/downloader-hub",
  downloaders: "/downloaders",
  "filter-rules": "/filter-rules",
  cleanup: "/cleanup",
  notifications: "/chatops/notifications",
  "notification-detail": "/chatops/notifications",
  bindings: "/chatops/bindings",
  "audit-log": "/chatops/audit",
  "rss-notifications": "/chatops/rss-notifications",
  global: "/global",
  "cloak-config": "/cloak-config",
  logs: "/logs",
  password: "/password",
};

/**
 * 当前激活的导航项 path。
 * 路由未就绪（首屏第一帧）时返回空串 —— 返回默认值会让导航先高亮错的一项再跳回来，
 * 旧实现踩过这个闪烁。
 */
export function activeNavPath(routeName: string | symbol | null | undefined): string {
  if (typeof routeName !== "string" || !routeName) return "";
  return ROUTE_NAME_TO_NAV[routeName] ?? "";
}
