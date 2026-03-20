<template>
  <div ref="rootEl" class="relative" :class="widthClass">
    <button type="button"
            :disabled="disabled"
            @click="toggleOpen"
            class="no-global-press w-full h-11 border rounded-2xl px-4 pr-10 text-xs font-bold outline-none transition-all disabled:opacity-40 disabled:cursor-not-allowed"
            :class="[
              isDark
                ? 'bg-white/[0.04] border-white/10 text-slate-300 hover:bg-white/[0.07]'
                : 'bg-slate-50 border-slate-100 text-slate-600 hover:bg-slate-100 shadow-sm',
              open ? (isDark ? 'border-blue-500/50' : 'border-blue-300 bg-white') : ''
            ]">
      <span class="block truncate text-left">{{ selectedLabel }}</span>
      <ChevronRight :size="14"
                    class="absolute right-4 top-1/2 -translate-y-1/2 text-slate-400 transition-transform"
                    :class="open ? 'rotate-[-90deg]' : 'rotate-90'" />
    </button>

    <Transition name="select-dropdown">
      <div v-if="open"
           class="absolute z-40 mt-2 w-full rounded-2xl border p-1 shadow-xl backdrop-blur-md max-h-72 overflow-y-auto"
           :class="[
             isDark ? 'bg-[#1e2330]/95 border-white/10' : 'bg-white/95 border-slate-100',
             dropdownClass,
           ]">
        <button v-for="option in options"
                :key="String(option.value)"
                type="button"
                @click="selectOption(option.value)"
                class="no-global-press w-full text-left px-3 py-2 rounded-xl text-xs font-bold transition-colors"
                :class="[
                  modelValue === option.value
                    ? (isDark ? 'bg-blue-500/20 text-blue-300' : 'bg-blue-50 text-blue-600')
                    : (isDark ? 'text-slate-300 hover:bg-white/[0.06]' : 'text-slate-600 hover:bg-slate-50')
                ]">
          {{ option.label }}
        </button>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { ChevronRight } from 'lucide-vue-next'

interface SelectOption {
  value: string | number
  label: string
}

interface Props {
  modelValue: string | number
  options: SelectOption[]
  disabled?: boolean
  isDark?: boolean
  widthClass?: string
  dropdownClass?: string
  placeholder?: string
}

const props = withDefaults(defineProps<Props>(), {
  disabled: false,
  isDark: false,
  widthClass: '',
  dropdownClass: '',
  placeholder: '请选择',
})

const emit = defineEmits<{ 'update:modelValue': [value: string | number] }>()

const open = ref(false)
const rootEl = ref<HTMLElement | null>(null)

const selectedLabel = computed(() => {
  const selected = props.options.find(o => o.value === props.modelValue)
  return selected?.label ?? props.placeholder
})

function toggleOpen() {
  if (props.disabled) return
  open.value = !open.value
}

function selectOption(value: string | number) {
  emit('update:modelValue', value)
  open.value = false
}

function handleClickOutside(e: MouseEvent) {
  if (!open.value || !rootEl.value) return
  if (!rootEl.value.contains(e.target as Node)) open.value = false
}

function handleEscape(e: KeyboardEvent) {
  if (e.key === 'Escape') open.value = false
}

onMounted(() => {
  document.addEventListener('mousedown', handleClickOutside)
  document.addEventListener('keydown', handleEscape)
})

onUnmounted(() => {
  document.removeEventListener('mousedown', handleClickOutside)
  document.removeEventListener('keydown', handleEscape)
})
</script>

<style scoped>
.select-dropdown-enter-active,
.select-dropdown-leave-active {
  transition: opacity 0.18s ease, transform 0.18s cubic-bezier(0.2, 0.8, 0.2, 1);
}

.select-dropdown-enter-from,
.select-dropdown-leave-to {
  opacity: 0;
  transform: translateY(-6px) scale(0.98);
}

.select-dropdown-enter-to,
.select-dropdown-leave-from {
  opacity: 1;
  transform: translateY(0) scale(1);
}
</style>
