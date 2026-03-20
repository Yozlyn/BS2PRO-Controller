<template>
  <button
    @click="handleClick"
    :disabled="disabled"
    class="capsule-switch relative flex-shrink-0 w-11 h-6 rounded-full transition-all duration-200 p-1"
    :class="[
      active ? 'bg-blue-600' : 'bg-slate-200 dark:bg-slate-700',
      disabled ? 'opacity-40 cursor-not-allowed' : 'cursor-pointer'
    ]"
  >
    <!-- 滑块 -->
    <div class="w-4 h-4 bg-white rounded-full transition-all duration-200 shadow-md"
         :class="active ? 'translate-x-5' : 'translate-x-0'" />
    <!-- 底部 LED 发光条（暗色模式下更明显） -->
    <div class="absolute -bottom-1.5 left-1/2 -translate-x-1/2 h-[3px] rounded-full blur-sm transition-all duration-300 pointer-events-none"
         :class="active
           ? 'w-8 bg-blue-400 dark:bg-blue-300 opacity-80 dark:opacity-100'
           : 'w-0 opacity-0'" />
  </button>
</template>

<script setup lang="ts">
const props = withDefaults(defineProps<{
  active?: boolean
  disabled?: boolean
}>(), { active: false, disabled: false })

const emit = defineEmits<{ click: [] }>()
const handleClick = (e: Event) => {
  e.stopPropagation()
  if (!props.disabled) emit('click')
}
</script>
