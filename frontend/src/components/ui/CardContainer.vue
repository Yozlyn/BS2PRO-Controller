<template>
  <div class="rounded-3xl p-6 border space-y-4 surface-card">
    <div class="flex items-center space-x-2 mb-2">
      <slot name="icon" />
      <h3 class="text-sm font-bold opacity-85 text-slate-700 dark:text-slate-300 uppercase tracking-tight">{{ title }}</h3>
    </div>
    <slot />
  </div>
</template>

<script setup lang="ts">
interface Props {
  title: string
  isDark: boolean
}

defineProps<Props>()
</script>
