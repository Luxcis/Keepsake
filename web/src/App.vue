<script setup lang="ts">
import { onMounted } from 'vue'
import AppHeader from '@/components/AppHeader.vue'
import AppFooter from '@/components/AppFooter.vue'
import { useSiteStore } from '@/stores/site'

const site = useSiteStore()
onMounted(() => site.load())
</script>

<template>
  <div class="grain min-h-screen bg-paper text-ink font-sans antialiased">
    <AppHeader v-if="!$route.meta.plain" />

    <main class="min-h-[70vh]">
      <RouterView />
    </main>

    <!-- 全站页脚：系统观测/管理入口，正式视觉阶段优化。 -->
    <AppFooter v-if="!$route.meta.plain" />
  </div>
</template>
