<script setup lang="ts">
// 访问门禁卡片（计划 §5.1 站点访问）：私密页捕获 gate_required(401) 后展示，
// 输入正确的访问口令即发放 HttpOnly 会话 Cookie 并触发本页重新加载。
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { api, ApiError } from '@/api/client'

const emit = defineEmits<{ unlocked: [] }>()
const props = defineProps<{ hint?: string }>()
void props
const password = ref('')
const busy = ref(false)
const message = ref<string | null>(null)
// 非空时展示未开放弹窗：口令正确但未到开放日期，卡片内不再重复该文案。
const notOpenMessage = ref<string | null>(null)
const closeButton = ref<HTMLButtonElement | null>(null)

function closeNotOpen() {
  notOpenMessage.value = null
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && notOpenMessage.value) closeNotOpen()
}

onMounted(() => window.addEventListener('keydown', onKeydown))
onBeforeUnmount(() => window.removeEventListener('keydown', onKeydown))

async function submit() {
  if (busy.value || password.value === '') return
  busy.value = true
  message.value = null
  try {
    await api.post('/api/site/unlock', { password: password.value })
    password.value = ''
    emit('unlocked')
  } catch (err) {
    if (err instanceof ApiError) {
      switch (err.code) {
        case 'gate_mismatch':
          message.value = '访问口令不正确。'
          break
        case 'rate_limited':
          message.value = '尝试次数过多，请稍后再试。'
          break
        case 'gate_not_set':
          message.value = '访问口令尚未启用：管理员在后台设置后，这里即可输入。'
          break
        case 'not_open_yet':
          // 口令正确但未到开放日期：改用弹窗展示提示，卡片内不再内联重复。
          password.value = ''
          notOpenMessage.value = err.message || '还没到打开的时候哦，再耐心等一等吧。'
          await nextTick()
          closeButton.value?.focus()
          break
        default:
          message.value = err.message
      }
    } else {
      message.value = '网络异常，请稍后再试。'
    }
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="w-full max-w-md mx-auto bg-paper rounded-sm shadow-soft p-6 sm:p-8 md:p-10 text-center">
    <p class="eyebrow mb-3">A PRIVATE PLACE</p>
    <h2 class="section-title text-2xl mb-3">这是只属于你们的地方。</h2>
    <p v-if="hint" class="text-xs text-muted leading-6 mb-5">{{ hint }}</p>
    <p v-else class="text-xs text-muted leading-6 mb-5">
      输入访问口令，解锁旅行、照片与那封未拆的信。
    </p>

    <form class="flex gap-2" @submit.prevent="submit">
      <input
        v-model="password"
        type="password"
        inputmode="numeric"
        autocomplete="current-password"
        placeholder="访问口令"
        class="flex-1 min-w-0 border border-line rounded-full px-5 py-3 text-sm bg-white/70 outline-none focus:border-clay"
      />
      <button
        type="submit"
        :disabled="busy"
        class="bg-ink text-paper rounded-full px-5 sm:px-6 py-3 min-h-[44px] text-xs tracking-wider hover:bg-moss transition disabled:opacity-50 whitespace-nowrap"
      >
        {{ busy ? '解锁中…' : '解锁' }}
      </button>
    </form>

    <p v-if="message" class="mt-4 text-xs text-clay leading-6">{{ message }}</p>
  </div>

  <Teleport to="body">
    <div
      v-if="notOpenMessage"
      class="fixed inset-0 z-50 flex items-center justify-center bg-[#101715]/60 p-4 sm:p-5"
      @click.self="closeNotOpen"
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="gate-not-open-title"
        class="w-full max-w-sm bg-paper rounded-sm shadow-soft p-6 sm:p-8 text-center"
      >
        <p class="eyebrow mb-3">NOT YET</p>
        <h2 id="gate-not-open-title" class="section-title text-xl mb-3">还没到打开的时候</h2>
        <p class="text-xs text-muted leading-6">{{ notOpenMessage }}</p>
        <button
          ref="closeButton"
          type="button"
          class="mt-6 min-h-[44px] rounded-full bg-ink px-6 py-3 text-xs tracking-wider text-paper transition hover:bg-moss"
          @click="closeNotOpen"
        >
          我知道了
        </button>
      </div>
    </div>
  </Teleport>
</template>
