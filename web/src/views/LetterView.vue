<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { api, isGateRequired, type Letter as LetterData } from '@/api/client'
import PageState from '@/components/PageState.vue'
import GatePrompt from '@/components/GatePrompt.vue'
import { bindMotion, loadMotion } from '@/compositions/motion'

// 生日信（计划 §2/§4.2）：门禁后可读；信封展开动画（GSAP；可跳过 ——
// reduced-motion/加载失败时点击直接露出信纸）。语音彩蛋依赖 COS 签名 URL。
const letter = ref<LetterData | null>(null)
const loading = ref(true)
const error = ref<string | null>(null)
const gate = ref(false)
const revealed = ref(false)
const envelope = ref<HTMLElement | null>(null)
let ctx: { revert(): void } | null = null

async function load() {
  loading.value = true
  error.value = null
  try {
    letter.value = await api.get<LetterData>('/api/letter')
  } catch (err) {
    if (isGateRequired(err)) {
      gate.value = true
    } else {
      error.value = err instanceof Error ? err.message : '加载失败'
    }
  } finally {
    loading.value = false
  }
}

// 信封逻辑默认仅在 reveal_enabled 时出现；用户可点按钮揭晓。
const needEnvelope = computed(() => !!letter.value && letter.value.reveal_enabled)
const paragraphs = computed<string[]>(() =>
  letter.value?.body ? letter.value.body.split('\n').filter((line) => line.trim() !== '') : [],
)

async function openLetter() {
  const bound = await bindMotion(envelope)
  if (!bound) {
    revealed.value = true // 完全降级：直接呈现信纸
    return
  }
  ctx = bound.modules.gsap.context(() => {
    bound.modules.gsap
      .timeline({
        onComplete: () => {
          revealed.value = true
        },
      })
      .to('.border-envelope-flap', {
        rotateX: -180,
        transformOrigin: '50% top',
        duration: 0.55,
        ease: 'power2.inOut',
      })
  }, bound.el)
}

// 信纸淡入（仅 GSAP 分支；降级路径无动画直接可见）
watch(revealed, async (val) => {
  if (!val) return
  const mods = await loadMotion()
  if (!mods) return
  await nextTick()
  ctx?.revert()
  ctx = mods.gsap.context(() => {
    mods.gsap.fromTo(
      '.letter-paper',
      { y: 34, opacity: 0 },
      { y: 0, opacity: 1, duration: 0.5, ease: 'power2.out' },
    )
  })
})

onMounted(load)
onBeforeUnmount(() => ctx?.revert())
</script>

<template>
  <section>
    <div class="max-w-4xl mx-auto px-5 sm:px-6 md:px-12 py-16 sm:py-20 md:py-28">
      <div class="text-center mb-12">
        <p class="eyebrow mb-4">A LETTER FOR YOU</p>
        <h1 class="section-title text-3xl sm:text-4xl md:text-5xl leading-tight">
          旅程还没有结束，<br />
          <span class="text-clay italic">还有一封信给你。</span>
        </h1>
        <p class="text-sm text-muted leading-8 mt-6">
          有些话，平时不一定说得出口。今天想把它们认真写下来，<br class="hidden md:block" />
          作为这份小小礼物的最后一页。
        </p>
      </div>

      <GatePrompt v-if="gate" hint="信件是最私密的一部分，解锁口令后即可阅读。" @unlocked="gate = false;load()" />

      <template v-else>
        <PageState
          :loading="loading"
          :error="error"
          :empty="!loading && !error && !letter"
          empty-title="这封信还没有写好。"
          empty-hint="它在管理后台的“生日信”里等待落笔。"
          @retry="load"
        />

        <template v-if="letter">
          <!-- 信封与揭晓（信件已配置时） -->
          <div v-if="needEnvelope && !revealed" ref="envelope" class="w-full max-w-md mx-auto">
            <div
              class="relative h-60 sm:h-64 md:h-72 rounded-sm shadow-soft overflow-hidden"
              style="perspective: 800px"
            >
              <div class="absolute inset-0 linear-envelope"></div>
              <div class="absolute bottom-0 left-0 right-0 h-[42%] border-envelope-bottom"></div>
              <div class="envelope-flap absolute top-0 left-0 right-0 h-[42%] border-envelope-flap"></div>
              <div class="absolute inset-0 flex flex-col items-center justify-center pt-5">
                <span class="text-3xl text-[#8f7050]" aria-hidden="true">✳</span>
                <span class="section-title text-2xl text-[#6e5b44] mt-2">For You</span>
                <span class="text-[9px] tracking-[0.25em] text-[#8e775b] mt-2">WITH LOVE</span>
              </div>
            </div>
            <button
              class="mt-6 mx-auto block rounded-full bg-ink text-paper px-7 py-3 text-xs tracking-wider hover:bg-moss transition"
              @click="openLetter"
            >
              拆开这封信 ✉
            </button>
          </div>

          <!-- 信纸正文 -->
          <article
            v-else
            class="letter-paper w-full max-w-md mx-auto mt-2 p-5 sm:p-7 md:p-10"
          >
            <p class="eyebrow mb-4">DEAR YOU,</p>
            <h2 class="section-title text-2xl mb-5">{{ letter.title }}</h2>

            <div class="space-y-4 text-sm leading-8 text-[#68685f]">
              <p v-for="(para, i) in paragraphs" :key="i">{{ para }}</p>
              <p v-if="!paragraphs.length" class="text-muted">(信纸留白中)</p>
            </div>

            <div class="border-t border-[#ddd3c2] mt-7 pt-5 flex flex-wrap items-center justify-between gap-2 text-right">
              <span class="text-[10px] tracking-[0.18em] text-muted">
                {{ letter.updated_at.slice(0, 10).replace(/-/g, '.') }}
              </span>
              <span class="text-xs text-muted">永远爱你的那个人</span>
            </div>
          </article>

          <!-- 语音彩蛋占位（COS 签名 URL 阶段接入播放） -->
          <div v-if="letter.audio_key" class="w-full max-w-md mx-auto mt-6 text-center">
            <span
              class="inline-flex items-center gap-3 border border-[#d9d2c6] rounded-full px-4 sm:px-5 py-3 text-xs leading-6 text-muted text-left"
            >
              <span aria-hidden="true">◉</span> 随信语音彩蛋 · COS 链路上线后可播放
            </span>
          </div>
        </template>
      </template>
    </div>
  </section>
</template>

<style scoped>
.linear-envelope {
  background: linear-gradient(145deg, #e7d8bf, #c8ae8a);
}
/* 用百分比裁切代替固定像素三角，信封始终与卡片同宽，手机上也不会两侧漏空。 */
.border-envelope-bottom {
  background: #bfa581;
  clip-path: polygon(0 100%, 50% 0, 100% 100%);
}
.border-envelope-flap {
  background: #e9dbc4;
  clip-path: polygon(0 0, 50% 100%, 100% 0);
  transform-origin: top;
  filter: drop-shadow(0 3px 2px rgba(70, 50, 30, 0.08));
}
</style>
