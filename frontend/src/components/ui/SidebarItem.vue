<template>
  <div
    @click="handleClick"
    class="flex items-center rounded-xl cursor-pointer transition-all font-semibold"
    :class="[
      isCollapsed ? 'justify-center p-2.5' : 'space-x-3 px-4 py-2.5',
      active
        ? `${isDark ? 'bg-white/[0.10] text-slate-100 border border-white/12' : 'bg-white/72 text-slate-800 border border-slate-200/75 shadow-sm'}`
        : `${isDark ? 'text-slate-400 hover:text-slate-200 hover:bg-slate-700/30' : 'text-slate-600 hover:text-slate-700 hover:bg-slate-200/60'}`,
      className
    ]"
    :title="isCollapsed ? label : ''"
  >
    <component :is="icon" :size="isCollapsed ? 22 : 18" />
    <span v-if="!isCollapsed" class="text-[13px] leading-5 tracking-[0.01em] whitespace-nowrap">{{ label }}</span>
  </div>
</template>

<script setup lang="ts">
interface Props {
  icon: any
  label: string
  active?: boolean
  isDark?: boolean
  isCollapsed?: boolean
  className?: string
}

const props = withDefaults(defineProps<Props>(), {
  active: false,
  isDark: false,
  isCollapsed: false,
  className: ''
})

const emit = defineEmits<{
  click: []
}>()

const handleClick = (e: Event) => {
  e.stopPropagation()
  emit('click')
}
</script>
