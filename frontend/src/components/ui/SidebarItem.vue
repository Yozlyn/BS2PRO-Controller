<template>
  <div
    @click="handleClick"
    class="group relative flex items-center rounded-2xl cursor-pointer font-semibold overflow-hidden sidebar-item-shell"
    :class="[
      isCollapsed ? 'justify-center px-0 py-2.5' : 'gap-3 px-4 py-2.5',
      active
        ? `${isDark ? 'bg-white/[0.10] text-slate-100 shadow-[inset_0_0_0_1px_rgba(255,255,255,0.12)]' : 'bg-white/72 text-slate-800 shadow-[inset_0_0_0_1px_rgba(203,213,225,0.9),0_8px_24px_rgba(15,23,42,0.06)]'}`
        : `${isDark ? 'text-slate-400 hover:text-slate-200 hover:bg-slate-700/30' : 'text-slate-600 hover:text-slate-700 hover:bg-slate-200/60'}`,
      className
    ]"
    :style="{ transition: 'background-color 240ms cubic-bezier(0.22, 1, 0.36, 1), color 220ms cubic-bezier(0.22, 1, 0.36, 1), box-shadow 260ms cubic-bezier(0.22, 1, 0.36, 1), transform 220ms cubic-bezier(0.22, 1, 0.36, 1)' }"
    :title="isCollapsed ? label : ''"
  >
    <div class="flex items-center justify-center shrink-0 transition-transform duration-300 ease-[cubic-bezier(0.22,1,0.36,1)]" :class="isCollapsed ? 'w-10 scale-100' : 'w-[18px] scale-100'">
      <component :is="icon" :size="isCollapsed ? 22 : 18" />
    </div>
    <span
      class="text-[13px] leading-5 tracking-[0.01em] whitespace-nowrap overflow-hidden transition-all duration-300 ease-[cubic-bezier(0.22,1,0.36,1)]"
      :class="isCollapsed ? 'max-w-0 opacity-0 -translate-x-1' : 'max-w-[120px] opacity-100 translate-x-0'"
    >{{ label }}</span>
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
