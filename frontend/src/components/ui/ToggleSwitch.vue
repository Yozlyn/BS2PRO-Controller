<template>
  <button
    @click="handleClick"
    :disabled="disabled"
    class="capsule-switch w-11 h-6 rounded-full transition-all relative p-1 disabled:opacity-40 disabled:cursor-not-allowed"
    :class="active ? color : 'bg-slate-200 dark:bg-slate-700'"
  >
    <div class="w-4 h-4 bg-white rounded-full transition-all shadow-md"
         :class="active ? 'translate-x-5' : 'translate-x-0'" />
  </button>
</template>

<script setup lang="ts">
const props = withDefaults(defineProps<{ active?: boolean; color?: string; disabled?: boolean }>(), {
  active: false, color: 'bg-blue-600', disabled: false
})
const emit = defineEmits<{ click: [] }>()
const handleClick = (e: Event) => { e.stopPropagation(); if (!props.disabled) emit('click') }
</script>
