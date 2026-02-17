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
    },
    {
      path: "/cleanup",
      name: "cleanup",
      component: () => import("@/views/AutoCleanup.vue"),
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
    },
    {
      path: "/sites",
      name: "sites",
      component: () => import("@/views/SiteList.vue"),
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
    },
    {
      path: "/filter-rules",
      name: "filter-rules",
      component: () => import("@/views/FilterRules.vue"),
    },
    {
      path: "/tasks",
      name: "tasks",
      component: () => import("@/views/TaskList.vue"),
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
    },
    {
      path: "/password",
      name: "password",
      component: () => import("@/views/ChangePassword.vue"),
    },
  ],
});

export default router;
