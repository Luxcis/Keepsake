<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '@/api/client'

// 管理后台布局：会话态 + 模块导航；各维护页以子路由渲染。
interface Me {
  username: string
  role: string
}

const route = useRoute()
const me = ref<Me | null>(null)
const checked = ref(false)

async function checkMe() {
  try {
    me.value = await api.get<Me>('/api/auth/me')
  } catch {
    me.value = null
  } finally {
    checked.value = true
  }
}

async function logout() {
  try {
    await api.post('/api/auth/logout')
  } finally {
    me.value = null
  }
}

onMounted(checkMe)

const tabs = [
  { to: '/admin/trips', label: '旅行管理', name: 'admin-trips' },
  { to: '/admin/photos', label: '相册与照片', name: 'admin-photos' },
  { to: '/admin/letter', label: '生日信', name: 'admin-letter' },
  { to: '/admin/backup', label: '备份', name: 'admin-backup' },
  { to: '/admin/settings', label: '站点设置', name: 'admin-settings' },
]
</script>

<template>
  <div class="min-h-screen bg-paper text-ink">
    <div class="max-w-5xl mx-auto px-6 py-10">
      <!-- 未登录态 -->
      <div v-if="checked && !me" class="max-w-md mx-auto bg-paper rounded-sm shadow-soft p-10 text-center mt-10">
        <p class="eyebrow mb-3">SIGNED OUT</p>
        <h1 class="section-title text-2xl mb-3">需要先登录管理员账号。</h1>
        <p class="text-xs text-muted mb-7">会话有效期 24 小时，过期后需重新登录。</p>
        <RouterLink
          to="/admin/login"
          class="inline-flex bg-ink text-paper rounded-full px-6 py-3 text-xs tracking-wider hover:bg-moss transition"
          >前往登录</RouterLink
        >
      </div>

      <!-- 已登录布局 -->
      <template v-if="me">
        <header class="flex items-end justify-between mb-8">
          <div>
            <p class="eyebrow mb-2">ADMIN CONSOLE</p>
            <h1 class="section-title text-3xl">管理后台</h1>
          </div>
          <div class="flex items-center gap-4 text-xs text-muted">
            <span>{{ me.username }}</span>
            <button class="underline hover:text-ink" @click="logout">退出</button>
            <RouterLink to="/" class="hover:text-ink">← 返回前台</RouterLink>
          </div>
        </header>

        <nav class="flex flex-wrap gap-2 border-b border-line pb-4 mb-8">
          <RouterLink
            v-for="t in tabs"
            :key="t.name"
            :to="t.to"
            class="pill"
            :class="{ active: route.name === t.name }"
          >
            {{ t.label }}
          </RouterLink>
        </nav>

        <RouterView :key="route.fullPath" />
      </template>
    </div>
  </div>
</template>
