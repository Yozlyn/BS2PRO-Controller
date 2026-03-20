<template>
  <div class="p-5 rounded-3xl border flex flex-col relative overflow-hidden surface-card">
    <div class="absolute -right-4 -bottom-4 w-32 h-32 rounded-full blur-[40px] opacity-20" :class="glow" />
    <div class="flex justify-between items-center mb-3 relative z-10">
      <div class="flex items-center space-x-2 font-bold" :class="color">
        <component :is="icon" :size="16" />
        <span class="text-xs uppercase tracking-wide opacity-80">{{ title }}</span>
      </div>
      <span class="text-[10px] font-black" :class="color">{{ status }}</span>
    </div>
    <div class="flex items-baseline space-x-1 mt-1 relative z-10">
      <span class="text-4xl font-black tracking-tighter" :class="color">{{ value }}</span>
      <span class="text-xs font-bold text-slate-500 dark:text-slate-500 uppercase">{{ unit }}</span>
    </div>
    <div class="absolute bottom-0 left-0 right-0 h-1 bg-slate-100/30 dark:bg-slate-700/30">
      <div class="h-full transition-all duration-1000" :class="barColor" :style="{ width: `${progress}%` }" />
    </div>
  </div>
</template>

<script setup lang="ts">
interface Props {
  title: string
  value: number | string
  unit: string
  status: string
  icon: any
  color: string
  glow: string
  barColor: string
  progress: number
  isDark: boolean
}

defineProps<Props>()
</script>
