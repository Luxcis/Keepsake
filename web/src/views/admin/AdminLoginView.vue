<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api, ApiError } from '@/api/client'

// 管理员登录（计划 §5.1）：账号密码 → POST /api/auth/login。
// 会话与限流在“后端基础”阶段接入；本页完成表单交互与错误呈现链路。
const router = useRouter()
const form = reactive({ username: '', password: '' })
const submitting = ref(false)
const notice = ref<string | null>(null)

async function submit() {
  if (submitting.value) return
  submitting.value = true
  notice.value = null
  try {
    await api.post('/api/auth/login', { username: form.username, password: form.password })
    await router.push('/admin')
  } catch (err) {
    if (err instanceof ApiError) {
      switch (err.code) {
        case 'invalid_credentials':
          notice.value = '用户名或密码不正确。'
          break
        case 'rate_limited':
          notice.value = '登录失败次数过多，请 15 分钟后再试。'
          break
        case 'backend_unavailable':
          notice.value = '会话服务暂不可用，请稍后再试。'
          break
        default:
          notice.value = err.message
      }
    } else {
      notice.value = '网络异常，请稍后再试。'
    }
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="min-h-dvh flex items-center justify-center px-5 sm:px-6 py-12 sm:py-16">
    <div class="w-full max-w-sm">
      <p class="text-center eyebrow mb-3">KEEPSAKE · 拾光集</p>
      <h1 class="section-title text-2xl sm:text-3xl text-center mb-6 sm:mb-8">管理员登录</h1>

      <form class="bg-paper rounded-sm shadow-soft p-6 sm:p-8 space-y-5" @submit.prevent="submit">
        <label class="block">
          <span class="text-[10px] tracking-[0.2em] text-muted">USERNAME</span>
          <input
            v-model="form.username"
            type="text"
            autocomplete="username"
            class="mt-2 w-full border border-line rounded-sm bg-white/60 px-4 py-3 text-sm outline-none focus:border-clay"
          />
        </label>
        <label class="block">
          <span class="text-[10px] tracking-[0.2em] text-muted">PASSWORD</span>
          <input
            v-model="form.password"
            type="password"
            autocomplete="current-password"
            class="mt-2 w-full border border-line rounded-sm bg-white/60 px-4 py-3 text-sm outline-none focus:border-clay"
          />
        </label>

        <p v-if="notice" class="text-xs text-clay leading-6">{{ notice }}</p>

        <button
          type="submit"
          :disabled="submitting"
          class="w-full bg-ink text-paper rounded-full py-3 text-xs tracking-wider hover:bg-moss transition disabled:opacity-50"
        >
          {{ submitting ? '登录中…' : '登录后台' }}
        </button>

        <p class="text-center text-[10px] text-muted">
          <RouterLink to="/" class="inline-flex min-h-[40px] items-center hover:text-ink">← 返回前台</RouterLink>
        </p>
      </form>
    </div>
  </div>
</template>
