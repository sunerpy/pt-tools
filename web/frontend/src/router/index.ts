import { createRouter, createWebHashHistory } from 'vue-router'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {
      path: '/',
      redirect: '/global'
    },
    {
      path: '/global',
      name: 'global',
      component: () => import('@/views/GlobalSettings.vue')
    },
    {
      path: '/qbit',
      name: 'qbit',
      component: () => import('@/views/QbitSettings.vue')
    },
    {
      path: '/sites',
      name: 'sites',
      component: () => import('@/views/SiteList.vue')
    },
    {
      path: '/sites/:name',
      name: 'site-detail',
      component: () => import('@/views/SiteDetail.vue')
    },
    {
      path: '/tasks',
      name: 'tasks',
      component: () => import('@/views/TaskList.vue')
    },
    {
      path: '/logs',
      name: 'logs',
      component: () => import('@/views/LogViewer.vue')
    },
    {
      path: '/password',
      name: 'password',
      component: () => import('@/views/ChangePassword.vue')
    }
  ]
})

export default router
