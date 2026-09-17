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
}

export interface NavGroup {
  title: string;
  items: NavItem[];
}

export const NAV_GROUPS: readonly NavGroup[] = [
  {
    title: "概览",
    items: [
      { path: "/userinfo", label: "用户统计", icon: "gauge" },
      { path: "/userinfo/export", label: "数据导出", icon: "file-down" },
    ],
  },
  {
    title: "站点",
    items: [
      { path: "/sites", label: "站点列表", icon: "globe", badge: "sites" },
      { path: "/supported-sites", label: "已支持站点", icon: "badge-check" },
    ],
  },
  {
    title: "下载",
    items: [
      { path: "/search", label: "种子搜索", icon: "search" },
      { path: "/tasks", label: "任务列表", icon: "list-checks", badge: "tasks" },
      { path: "/paused", label: "暂停任务", icon: "circle-pause", badge: "paused" },
      { path: "/downloader-hub", label: "下载器 Web UI", icon: "app-window", external: true },
      { path: "/downloaders", label: "下载器设置", icon: "server" },
    ],
  },
  {
    title: "规则",
    items: [
      { path: "/filter-rules", label: "过滤规则", icon: "list-filter" },
      { path: "/cleanup", label: "自动清理", icon: "trash-2" },
    ],
  },
  {
    title: "ChatOps",
    items: [
      { path: "/chatops/notifications", label: "消息通知", icon: "bell" },
      { path: "/chatops/bindings", label: "ChatOps 绑定", icon: "link" },
      { path: "/chatops/audit", label: "操作审计", icon: "shield-check" },
      { path: "/chatops/rss-notifications", label: "RSS 通知日志", icon: "rss" },
    ],
  },
  {
    title: "系统",
    items: [
      { path: "/global", label: "全局设置", icon: "settings" },
      { path: "/cloak-config", label: "CloakBrowser", icon: "shield" },
      { path: "/logs", label: "运行日志", icon: "scroll-text" },
      { path: "/password", label: "修改密码", icon: "key-round" },
    ],
  },
] as const;

/** 深色 rail 上的 8 个快捷入口（设计稿 `D.rail`），是 NAV_GROUPS 的子集。 */
export const RAIL_ITEMS: readonly { path: string; icon: LucideIconName; label: string }[] = [
  { path: "/userinfo", icon: "gauge", label: "用户统计" },
  { path: "/sites", icon: "globe", label: "站点列表" },
  { path: "/search", icon: "search", label: "种子搜索" },
  { path: "/tasks", icon: "list-checks", label: "任务列表" },
  { path: "/paused", icon: "circle-pause", label: "暂停任务" },
  { path: "/filter-rules", icon: "list-filter", label: "过滤规则" },
  { path: "/chatops/notifications", icon: "bell", label: "消息通知" },
  { path: "/global", icon: "settings", label: "全局设置" },
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
