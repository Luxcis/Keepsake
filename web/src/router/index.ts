import { createRouter, createWebHistory } from 'vue-router'

// 页面路由（计划 §2 产品结构）。
// meta.title 用于 document.title；admin 段在认证阶段加守卫。
export const routes = [
  { path: '/', name: 'home', component: () => import('@/views/HomeView.vue'), meta: { title: '首页' } },
  { path: '/trips', name: 'trips', component: () => import('@/views/TripsView.vue'), meta: { title: '足迹' } },
  { path: '/trips/:id', name: 'trip-detail', component: () => import('@/views/TripDetailView.vue'), meta: { title: '旅行详情' } },
  // 照片收藏页已移除，照片改在旅行详情中浏览；旧链接重定向到足迹。
  { path: '/photos', redirect: { name: 'trips' } },
  { path: '/timeline', name: 'timeline', component: () => import('@/views/TimelineView.vue'), meta: { title: '时光轴' } },
  { path: '/letter', name: 'letter', component: () => import('@/views/LetterView.vue'), meta: { title: '生日信' } },
  { path: '/admin/login', name: 'admin-login', component: () => import('@/views/admin/AdminLoginView.vue'), meta: { title: '管理员登录', plain: true } },
  {
    path: '/admin',
    component: () => import('@/views/admin/AdminView.vue'),
    meta: { title: '管理后台', plain: true },
    children: [
      { path: '', redirect: { name: 'admin-trips' } },
      { path: 'trips', name: 'admin-trips', component: () => import('@/views/admin/AdminTripsView.vue'), meta: { title: '旅行管理' } },
      { path: 'photos', name: 'admin-photos', component: () => import('@/views/admin/AdminPhotosView.vue'), meta: { title: '相册与照片' } },
      { path: 'letter', name: 'admin-letter', component: () => import('@/views/admin/AdminLetterView.vue'), meta: { title: '生日信' } },
      { path: 'backup', name: 'admin-backup', component: () => import('@/views/admin/AdminBackupView.vue'), meta: { title: '备份' } },
      { path: 'settings', name: 'admin-settings', component: () => import('@/views/admin/AdminSettingsView.vue'), meta: { title: '站点设置' } },
    ],
  },
  { path: '/:pathMatch(.*)*', name: 'not-found', component: () => import('@/views/NotFoundView.vue'), meta: { title: '页面不存在', plain: true } },
]

export const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior(_to, _from, saved) {
    if (saved) return saved
    return { top: 0 }
  },
})

router.afterEach((to) => {
  const base = '我们的旅行纪念册'
  document.title = to.meta.title ? `${to.meta.title} · ${base}` : base
})
