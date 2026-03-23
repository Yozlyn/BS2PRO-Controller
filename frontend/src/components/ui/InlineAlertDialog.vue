<template>
  <Teleport to="body">
    <div v-if="visible" class="fixed inset-0 z-[1200] flex items-center justify-center px-4 pointer-events-none">
      <div class="relative w-full max-w-md rounded-[2rem] border shadow-2xl p-6 bg-white dark:bg-slate-900 border-slate-200/80 dark:border-white/10 pointer-events-auto">
        <div class="space-y-2">
          <h3 class="text-lg font-black text-slate-800 dark:text-slate-100">{{ title }}</h3>
          <p class="text-sm leading-6 text-slate-500 dark:text-slate-300 whitespace-pre-wrap">{{ message }}</p>
        </div>
        <div class="mt-6 flex justify-end">
          <button
            @click="emit('close')"
            class="px-5 py-2.5 rounded-xl text-sm font-bold transition-all"
            :class="isDark ? 'bg-blue-500 text-white hover:bg-blue-400' : 'bg-blue-600 text-white hover:bg-blue-500'"
          >确定</button>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
defineProps<{
  visible: boolean
  title: string
  message: string
  isDark: boolean
}>()

const emit = defineEmits<{ close: [] }>()
</script>
