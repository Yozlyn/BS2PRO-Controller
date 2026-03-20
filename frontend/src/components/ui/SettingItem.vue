<template>
  <div class="p-6 flex items-center justify-between" :class="disabled ? 'opacity-60' : ''">
    <div class="space-y-1">
      <h3 class="text-sm font-bold text-slate-700 dark:text-slate-300">{{ title }}</h3>
      <p class="text-xs text-slate-400 setting-desc">{{ desc }}</p>
    </div>
    <LedToggle :active="active" @click="!disabled && $emit('toggle')" :disabled="disabled" />
  </div>
</template>

<script setup lang="ts">
import LedToggle from './LedToggle.vue'
defineProps<{ title: string; desc: string; active: boolean; disabled?: boolean; loading?: boolean }>()
defineEmits<{ toggle: [] }>()
</script>
