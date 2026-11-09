<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { api, isGateRequired, type Letter as LetterData } from '@/api/client'
import PageState from '@/components/PageState.vue'
import GatePrompt from '@/components/GatePrompt.vue'
import LetterCard from '@/components/LetterCard.vue'
import { bindMotion } from '@/compositions/motion'

// 生日信（计划 §2/§4.2）：门禁后可读。拆信动画分四段 —— 翻开封口 → 露出折起的
// 空白信纸 → 信纸向上抽出、信封下移淡出 → 信纸展开并淡入正文。
// reduced-motion 或 GSAP 加载失败时点击直接呈现信纸；折叠态只显示纸面，
// 正文在展开阶段淡入，避免出现「折起来还能读正文」的错位。
const letter = ref<LetterData | null>(null)
const loading = ref(true)
const error = ref<string | null>(null)
const gate = ref(false)
const phase = ref<'closed' | 'animating' | 'done'>('closed')

const scene = ref<HTMLElement | null>(null)
const paperLayer = ref<HTMLElement | null>(null)
const paperClip = ref<HTMLElement | null>(null)
const paperInner = ref<HTMLElement | null>(null)
const crease = ref<HTMLElement | null>(null)
const spacer = ref<HTMLElement | null>(null)
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
const needEnvelope = computed(
  () => !!letter.value && letter.value.reveal_enabled && phase.value !== 'done',
)

async function openLetter() {
  if (phase.value !== 'closed') return // 动画中/已完成，重复点击直接忽略
  // 先锁住入口并立刻隐藏按钮：bindMotion 的动态 import 是异步的，避免这段时间内重复进入。
  phase.value = 'animating'
  await nextTick() // 等按钮隐藏后的布局生效，占位高度才按最终版式计算

  const bound = await bindMotion(scene)
  const clip = paperClip.value
  const inner = paperInner.value
  const layer = paperLayer.value
  const creaseEl = crease.value
  const spacerEl = spacer.value
  const card = scene.value?.querySelector<HTMLElement>('.envelope-stage')
  if (!bound || !clip || !inner || !layer || !creaseEl || !spacerEl || !card) {
    phase.value = 'done' // 完全降级：直接呈现信纸
    return
  }

  // 折叠高度跟信封卡片高度联动（约 40%），折叠与展开同宽，避免展开时文字重排。
  const cardH = card.offsetHeight
  const foldedH = Math.round(cardH * 0.4)
  const fullH = Math.max(Math.round(inner.getBoundingClientRect().height), foldedH)
  // 展开前用占位高度把页面撑到最终版式高度，收尾换成真实信纸时不再跳变。
  const spacerH = Math.max(0, Math.round(fullH - scene.value!.offsetHeight))
  const insideY = cardH - foldedH - 16 // 折好的信纸藏在信封里
  const revealY = -Math.round(foldedH * 0.5) // 露出一半，信封保持可见
  const outY = -foldedH - 20 // 完全抽出：信纸下沿高于信封上沿

  const { gsap } = bound.modules
  gsap.set(clip, { height: foldedH })
  gsap.set(layer, { y: insideY })
  gsap.set(inner, { opacity: 0 })

  ctx = gsap.context(() => {
    // 信封各层独立参与淡出（不额外包一层，否则会把信纸一起放进信封的堆叠上下文）。
    const envelope =
      '.envelope-inside, .envelope-front, .border-envelope-bottom, .envelope-flap, .envelope-label'
    gsap
      .timeline({
        onComplete: () => {
          phase.value = 'done'
        },
      })
      .to('.envelope-flap', {
        rotateX: -180,
        transformOrigin: '50% top',
        duration: 0.5,
        ease: 'power2.inOut',
      }, 0)
      // 封口翻到侧面（不可见）时切到信纸下方，之后抽纸不会被翻盖挡住。
      .set('.envelope-flap', { zIndex: 0 }, 0.25)
      .to(layer, { y: revealY, duration: 0.5, ease: 'power2.out' }, 0.42)
      .to(layer, { y: outY, duration: 0.6, ease: 'power2.inOut' }, 0.98)
      .to(envelope, { y: 56, opacity: 0, duration: 0.65, ease: 'power1.in' }, 0.98)
      .to(clip, { height: fullH, duration: 0.85, ease: 'power2.inOut' }, 1.5)
      .to(layer, { y: 0, duration: 0.85, ease: 'power2.inOut' }, 1.5)
      .to(spacerEl, { height: spacerH, duration: 0.85, ease: 'power2.inOut' }, 1.5)
      .to(inner, { opacity: 1, duration: 0.4, ease: 'power1.out' }, 1.68)
      .to(creaseEl, { opacity: 0, duration: 0.35, ease: 'power1.out' }, 1.68)
  }, bound.el)
}

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
          <!-- 信封与揭晓：信纸始终是同一份 LetterCard，折叠态只是裁切 + 位移 -->
          <div v-if="needEnvelope" ref="scene" class="relative w-full max-w-md mx-auto">
            <div class="envelope-stage relative h-60 sm:h-64 md:h-72" style="perspective: 800px">
              <div class="envelope-inside absolute inset-0 rounded-sm shadow-soft"></div>

              <!-- 折好的空白信纸：先藏在信封里，再露出一部分、整体抽出 -->
              <div ref="paperLayer" class="paper-layer absolute inset-x-0 top-0">
                <div ref="paperClip" class="paper-clip relative overflow-hidden letter-paper">
                  <div ref="paperInner" class="paper-inner" aria-hidden="true">
                    <LetterCard :letter="letter" />
                  </div>
                  <div ref="crease" class="paper-crease" aria-hidden="true"></div>
                </div>
              </div>

              <div class="envelope-front absolute inset-0 rounded-sm linear-envelope"></div>
              <div class="absolute bottom-0 left-0 right-0 h-[42%] border-envelope-bottom"></div>
              <div class="envelope-flap absolute top-0 left-0 right-0 h-[42%] border-envelope-flap"></div>
              <div class="envelope-label absolute inset-0 flex flex-col items-center justify-center pt-5">
                <span class="text-3xl text-[#8f7050]" aria-hidden="true">✳</span>
                <span class="section-title text-2xl text-[#6e5b44] mt-2">For You</span>
                <span class="text-[9px] tracking-[0.25em] text-[#8e775b] mt-2">WITH LOVE</span>
              </div>
            </div>

            <button
              v-show="phase === 'closed'"
              class="envelope-button mt-6 mx-auto block rounded-full bg-ink text-paper px-7 py-3 text-xs tracking-wider hover:bg-moss transition disabled:cursor-default"
              :disabled="phase !== 'closed'"
              @click="openLetter"
            >
              拆开这封信 ✉
            </button>

            <!-- 展开时撑开页面，收尾换成真实信纸时不产生位置突跳 -->
            <div ref="spacer" class="paper-spacer" aria-hidden="true"></div>
          </div>

          <!-- 信纸正文（揭晓关闭或动画完成后） -->
          <LetterCard v-else :letter="letter" class="max-w-md mx-auto mt-2" />

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
/* 信封各层：层序固定为「内衬 < 信纸 < 正面 < 封口 < 标签」，
   信纸因此能从信封里升出来，又不会在折叠时露在正面之上。 */
.envelope-inside {
  z-index: 0;
  background: #b9a081;
}
.linear-envelope {
  z-index: 2;
  background: linear-gradient(145deg, #e7d8bf, #c8ae8a);
}
/* 用百分比裁切代替固定像素三角，信封始终与卡片同宽，手机上也不会两侧漏空。 */
.border-envelope-bottom {
  z-index: 2;
  background: #bfa581;
  clip-path: polygon(0 100%, 50% 0, 100% 100%);
}
.border-envelope-flap {
  z-index: 3;
  background: #e9dbc4;
  clip-path: polygon(0 0, 50% 100%, 100% 0);
  transform-origin: top;
  filter: drop-shadow(0 3px 2px rgba(70, 50, 30, 0.08));
}
.envelope-label {
  z-index: 4;
}
.paper-layer {
  z-index: 1;
}
/* 折叠态的高度与正文淡入由 GSAP 接管；未播放时高度为 0，信纸不露出来。 */
.paper-clip {
  height: 0;
}
.paper-inner {
  opacity: 0;
}
/* 折痕：贴在裁切下沿，展开过程中随正文淡入一起消失。 */
.paper-crease {
  position: absolute;
  inset-inline: 0;
  bottom: 0;
  height: 22px;
  pointer-events: none;
  background: linear-gradient(to top, rgba(70, 50, 30, 0.16), rgba(70, 50, 30, 0.02) 62%, transparent);
  box-shadow: inset 0 -1px 0 rgba(70, 50, 30, 0.18);
}
.paper-spacer {
  height: 0;
}
</style>
