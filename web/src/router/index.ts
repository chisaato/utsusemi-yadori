import { createRouter, createWebHashHistory } from 'vue-router'

// hash 模式：KSU WebView file-ish 场景最稳（契约 §6）
export const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', name: 'overview', component: () => import('../pages/OverviewPage.vue') },
    { path: '/apps', name: 'apps', component: () => import('../pages/AppsPage.vue') },
    { path: '/bin', name: 'bin', component: () => import('../pages/BinariesPage.vue') },
    { path: '/adb', name: 'adb', component: () => import('../pages/AdbPage.vue') },
    { path: '/remote', name: 'remote', component: () => import('../pages/RemotePage.vue') },
    { path: '/logs', name: 'logs', component: () => import('../pages/LogsPage.vue') },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})
