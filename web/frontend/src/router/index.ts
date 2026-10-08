import { createRouter, createWebHashHistory } from "vue-router";

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {
      path: "/",
      redirect: "/userinfo",
    },
    {
      path: "/userinfo",
      name: "userinfo",
      component: () => import("@/views/UserInfoDashboard.vue"),
      meta: { title: "用户统计" },
    },
    {
      path: "/userinfo/export",
      name: "userinfo-export",
      component: () => import("@/views/UserDataExport.vue"),
      meta: { title: "数据导出" },
    },
    {
      path: "/global",
      name: "global",
      component: () => import("@/views/GlobalSettings.vue"),
      meta: { title: "全局设置" },
    },
    {
      path: "/api-tokens",
      name: "api-tokens",
      component: () => import("@/views/ApiTokens.vue"),
      meta: { title: "API 令牌" },
    },
    {
      path: "/mcp",
      name: "mcp",
      component: () => import("@/views/McpAccess.vue"),
      meta: { title: "MCP 接入" },
    },
    {
      path: "/cloak-config",
      name: "cloak-config",
      component: () => import("@/views/CloakBrowserConfig.vue"),
      meta: { title: "CloakBrowser 配置" },
    },
    {
      path: "/cleanup",
      name: "cleanup",
      component: () => import("@/views/AutoCleanup.vue"),
      meta: { title: "自动清理" },
    },
    // 旧的 qBittorrent 设置页面（已隐藏）
    // {
    //   path: '/qbit',
    //   name: 'qbit',
    //   component: () => import('@/views/QbitSettings.vue')
    // },
    {
      path: "/downloaders",
      name: "downloaders",
      component: () => import("@/views/DownloaderSettings.vue"),
      meta: { title: "下载器设置" },
    },
    {
      path: "/sites",
      name: "sites",
      component: () => import("@/views/SiteList.vue"),
      meta: { title: "站点列表" },
    },
    {
      path: "/supported-sites",
      name: "supported-sites",
      component: () => import("@/views/SupportedSites.vue"),
      meta: { title: "已支持站点" },
    },
    {
      path: "/cookiecloud",
      name: "cookiecloud",
      component: () => import("@/views/CookieCloud.vue"),
      meta: { title: "CookieCloud 导入" },
    },
    {
      path: "/media/recognize",
      name: "media-recognize",
      component: () => import("@/views/MediaRecognize.vue"),
      meta: { title: "媒体识别" },
    },
    {
      path: "/media/library",
      name: "media-library",
      component: () => import("@/views/MediaLibrary.vue"),
      meta: { title: "媒体库" },
    },
    {
      path: "/media/history",
      name: "media-history",
      component: () => import("@/views/MediaHistory.vue"),
      meta: { title: "整理历史" },
    },
    {
      path: "/media/explore",
      name: "media-explore",
      component: () => import("@/views/MediaExplore.vue"),
      meta: { title: "探索" },
    },
    {
      path: "/media/subscriptions",
      name: "media-subscriptions",
      component: () => import("@/views/MediaSubscriptions.vue"),
      meta: { title: "订阅" },
    },
    {
      path: "/search",
      name: "search",
      component: () => import("@/views/TorrentSearch.vue"),
      meta: { title: "种子搜索" },
    },
    // 动态站点页面（已隐藏）
    // {
    //   path: '/sites/dynamic',
    //   name: 'dynamic-sites',
    //   component: () => import('@/views/DynamicSiteSettings.vue')
    // },
    {
      path: "/sites/:name",
      name: "site-detail",
      component: () => import("@/views/SiteDetail.vue"),
      meta: { title: "站点详情" },
    },
    {
      path: "/filter-rules",
      name: "filter-rules",
      component: () => import("@/views/FilterRules.vue"),
      meta: { title: "过滤规则" },
    },
    {
      path: "/tasks",
      name: "tasks",
      component: () => import("@/views/TaskList.vue"),
      meta: { title: "任务列表" },
    },
    {
      path: "/brush",
      name: "brush",
      component: () => import("@/views/BrushTasks.vue"),
      meta: { title: "刷流任务" },
    },
    {
      path: "/downloader-assistant",
      name: "downloader-assistant",
      component: () => import("@/views/DownloaderAssistant.vue"),
      meta: { title: "下载器助手" },
    },
    {
      path: "/transfer",
      name: "transfer",
      component: () => import("@/views/TorrentTransfer.vue"),
      meta: { title: "转移做种" },
    },
    {
      path: "/reseed",
      name: "reseed",
      component: () => import("@/views/Reseed.vue"),
      meta: { title: "IYUU 辅种" },
    },
    {
      path: "/qbit-compat",
      name: "qbit-compat",
      component: () => import("@/views/QbitCompat.vue"),
      meta: { title: "qB 兼容入口" },
    },
    {
      path: "/paused",
      name: "paused",
      component: () => import("@/views/PausedTorrents.vue"),
      meta: { title: "暂停任务" },
    },
    {
      path: "/logs",
      name: "logs",
      component: () => import("@/views/LogViewer.vue"),
      meta: { title: "运行日志" },
    },
    {
      path: "/password",
      name: "password",
      component: () => import("@/views/ChangePassword.vue"),
      meta: { title: "修改密码" },
    },
    {
      path: "/downloader-hub",
      name: "downloader-hub",
      component: () => import("@/views/DownloaderHub.vue"),
      meta: { title: "下载器 Web UI" },
    },

    {
      path: "/chatops/notifications",
      name: "notifications",
      component: () => import("@/views/chatops/Notifications.vue"),
      meta: { title: "消息通知配置" },
    },
    {
      path: "/chatops/notifications/:id",
      name: "notification-detail",
      component: () => import("@/views/chatops/NotificationDetail.vue"),
      meta: { title: "通知通道详情" },
    },
    {
      path: "/chatops/bindings",
      name: "bindings",
      component: () => import("@/views/chatops/Bindings.vue"),
      meta: { title: "ChatOps 绑定" },
    },
    {
      path: "/chatops/audit",
      name: "audit-log",
      component: () => import("@/views/chatops/AuditLog.vue"),
      meta: { title: "操作审计日志" },
    },
    {
      path: "/chatops/rss-notifications",
      name: "rss-notifications",
      component: () => import("@/views/chatops/RSSNotifications.vue"),
      meta: { title: "RSS 通知日志" },
    },
  ],
});

export default router;
