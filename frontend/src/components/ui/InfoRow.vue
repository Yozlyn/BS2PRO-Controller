<template>
  <div class="flex items-center justify-between p-3 rounded-2xl surface-tile transition-all cursor-default">
    <span class="text-xs font-bold uppercase tracking-tight text-slate-600 dark:text-slate-400">{{ label }}</span>
    <div class="flex items-center space-x-2">
      <div v-if="hasDot" class="w-1.5 h-1.5 rounded-full" :class="dotColor" />
      <span class="text-xs font-black px-1" 
            :class="[
              valueColor || '',
              italic ? 'italic font-medium' : '',
              valueBold ? 'font-black' : ''
            ]">
        {{ value }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
interface Props {
  label: string
  value: string
  valueColor?: string
  italic?: boolean
  hasDot?: boolean
  dotColor?: string
  valueBold?: boolean
}

withDefaults(defineProps<Props>(), {
  valueColor: '',
  italic: false,
  hasDot: false,
  dotColor: '',
  valueBold: false
})
</script>
