import { createRouter, createWebHistory } from 'vue-router'
import { api } from '../api'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('../views/Login.vue'),
      meta: { public: true },
    },
    {
      path: '/',
      component: () => import('../layouts/DashboardLayout.vue'),
      children: [
        { path: '', redirect: '/overview' },
        { path: 'overview', name: 'overview', component: () => import('../views/Overview.vue') },
        { path: 'clients', name: 'clients', component: () => import('../views/Clients.vue') },
        { path: 'settings', name: 'settings', component: () => import('../views/Settings.vue') },
      ],
    },
  ],
})

// Simple auth guard: anything non-public requires a live session.
router.beforeEach(async (to) => {
  if (to.meta.public) return true
  try {
    const me = await api.me()
    if (me.username) return true
  } catch {
    // fall through to redirect
  }
  return { name: 'login', query: { redirect: to.fullPath } }
})

export default router
