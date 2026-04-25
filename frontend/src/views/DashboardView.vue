<template>
  <div class="flex-1 flex flex-col overflow-y-auto">
    <header class="p-8 flex justify-between items-start shrink-0">
      <div class="space-y-3">
        <h2 class="text-xl font-bold tracking-tight text-slate-800 dark:text-white">设备概览</h2>
        <div class="flex items-center pt-1 select-none">
          <div class="flex gap-[3px] mr-3.5">
            <div class="w-1.5 h-3.5 rounded-[2px]"
                 :class="isConnected ? 'bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.6)] animate-pulse' : 'bg-slate-400/60'" />
            <div class="w-1.5 h-3.5 rounded-[2px]" :class="isConnected ? 'bg-emerald-500/40' : 'bg-slate-400/35'" />
            <div class="w-1.5 h-3.5 rounded-[2px]" :class="isConnected ? 'bg-emerald-500/20' : 'bg-slate-400/20'" />
          </div>
          <div class="flex items-center">
            <span class="text-[11px] font-black tracking-[0.2em] uppercase mt-px"
                  :class="isConnected ? 'text-emerald-600 dark:text-emerald-400' : 'text-slate-500 dark:text-slate-500'">
              {{ isConnected ? 'Link Active' : 'Link Offline' }}
            </span>
            <span class="text-slate-300 dark:text-slate-700 font-light mx-3">/</span>
            <div class="flex items-center gap-1.5"
                 :class="isConnected ? 'text-slate-400 dark:text-slate-500' : 'text-slate-400 dark:text-slate-600'">
              <Server :size="14" :stroke-width="2" />
              <span class="text-[10px] font-bold uppercase tracking-[0.15em] mt-px">
                {{ isConnected ? resolvedDeviceModel : 'BS2PRO' }}
              </span>
            </div>
          </div>
        </div>
      </div>
        <div class="flex items-center space-x-4">
        <!-- 智能变频开关 -->
        <div class="flex items-center space-x-3 px-4 py-2 rounded-2xl border"
             :class="isDark
               ? 'surface-tile border-white/10 text-slate-200'
               : 'surface-tile border-slate-200/70 text-slate-700'">
          <span class="text-xs font-bold text-slate-500 dark:text-slate-400">智能变频</span>
          <LedToggle :active="isSmartFreq"
                     @click="$emit('update:isSmartFreq', !isSmartFreq)"
                     :disabled="!isConnected" />
        </div>
        <!-- 断开/连接 -->
        <button @click="isConnected ? $emit('disconnect') : $emit('connect')"
                class="h-10 flex items-center space-x-2 px-4 rounded-2xl text-xs font-bold transition-all border cursor-pointer"
                :class="isConnected
                  ? 'surface-tile border-slate-200/70 dark:border-white/10 text-slate-700 dark:text-slate-200 hover:text-red-500 hover:bg-slate-200/60 dark:hover:bg-slate-700/30'
                  : 'bg-blue-600 border-blue-600 text-white hover:bg-blue-700'">
          <Power :size="14" />
          <span>{{ isConnected ? '断开连接' : '连接设备' }}</span>
        </button>
      </div>
    </header>

    <div class="px-8 pb-8 space-y-6">
      <div class="grid grid-cols-3 gap-6">
        <CompactMonitor title="CPU 温度" :value="isConnected ? (temperature?.cpuTemp ?? '--') : '--'" unit="°C"
                        :status="isConnected ? cpuStatus : '离线'" :icon="Cpu"
                        :color="isConnected ? cpuColor : 'text-slate-400'"
                        :glow="isConnected ? cpuGlow : 'bg-slate-400/10'"
                        :bar-color="isConnected ? cpuBarColor : 'bg-slate-300'"
                        :progress="isConnected ? cpuProgress : 0" :is-dark="isDark" />
        <CompactMonitor title="GPU 温度" :value="isConnected ? (temperature?.gpuTemp ?? '--') : '--'" unit="°C"
                        :status="isConnected ? gpuStatus : '离线'" :icon="Activity"
                        :color="isConnected ? gpuColor : 'text-slate-400'"
                        :glow="isConnected ? gpuGlow : 'bg-slate-400/10'"
                        :bar-color="isConnected ? gpuBarColor : 'bg-slate-300'"
                        :progress="isConnected ? gpuProgress : 0" :is-dark="isDark" />
        <CompactMonitor title="风扇转速" :value="isConnected ? (fanData?.currentRpm ?? '--') : '--'" unit="RPM"
                        :status="isConnected ? fanStatus : '离线'" :icon="Tornado"
                        :color="isConnected ? 'text-blue-500' : 'text-slate-400'"
                        :glow="isConnected ? 'bg-blue-500/20' : 'bg-slate-400/10'"
                        :bar-color="isConnected ? 'bg-blue-500' : 'bg-slate-300'"
                        :progress="isConnected ? fanProgress : 0" :is-dark="isDark" />
      </div>

      <div class="grid grid-cols-2 gap-6">
        <CardContainer title="设备状态" :is-dark="isDark">
          <template #icon><div class="w-1.5 h-1.5 rounded-full animate-pulse" :class="isConnected ? 'bg-blue-500' : 'bg-slate-400'" /></template>
          <InfoRow label="连接状态" :value="isConnected ? '已连接' : '未连接'"
                   :has-dot="true" :dot-color="isConnected ? 'bg-emerald-500' : 'bg-slate-400'"
                   :value-color="isConnected ? 'text-emerald-500' : 'text-slate-400'" />
          <InfoRow label="当前挡位"
                    :value="isConnected ? maxSupportedGearText : '--'"
                    :value-color="isConnected ? gearColor : 'text-slate-400'"
                   :has-dot="isConnected" :dot-color="gearDot" value-bold />
          <InfoRow label="最高转速"
                   :value="isConnected ? `${fanMaxRpm} RPM` : '--'"
                   value-bold value-color="text-rose-500" />
        </CardContainer>
        <CardContainer title="曲线偏移状态" :is-dark="isDark">
          <template #icon>
            <div class="w-1.5 h-1.5 rounded-full animate-pulse"
                 :class="isConnected && config?.fanCurveOffsetEnabled ? 'bg-emerald-500' : 'bg-slate-400'" />
          </template>
          <InfoRow label="引擎收敛状态"
                   :value="isConnected ? engineConvergenceStatus : '离线'"
                   :value-color="isConnected && config?.fanCurveOffsetEnabled ? 'text-emerald-500' : 'text-slate-400'"
                   value-bold />
          <InfoRow label="自动偏移补偿"
                   :value="isConnected && temperature && config?.fanCurveOffsetEnabled
                     ? `${temperature.autoOffset >= 0 ? '+' : ''}${temperature.autoOffset} RPM` : '--'"
                   value-bold :value-color="offsetValueColor" />
          <InfoRow label="引擎当前状态"
                   :value="isConnected && config?.fanCurveOffsetEnabled ? (temperature?.engineState || '稳定') : '--'"
                   :value-color="isConnected && config?.fanCurveOffsetEnabled ? engineCurrentStatusColor : 'text-slate-400'"
                   value-bold />
        </CardContainer>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Cpu, Activity, Wind, Power, Tornado, Server } from 'lucide-vue-next'
import LedToggle from '../components/ui/LedToggle.vue'
import CompactMonitor from '../components/ui/CompactMonitor.vue'
import CardContainer from '../components/ui/CardContainer.vue'
import InfoRow from '../components/ui/InfoRow.vue'
import { types } from '../../wailsjs/go/models'

interface Props {
  isDark: boolean; isConnected: boolean; isSmartFreq: boolean
  deviceModel?: string; deviceProductId?: string
  fanData: types.FanData | null; temperature: types.TemperatureData | null; config: types.AppConfig
}
const props = defineProps<Props>()
defineEmits<{ 'update:isSmartFreq': [v: boolean]; disconnect: []; connect: [] }>()

const cpuTemp = computed(() => props.temperature?.cpuTemp ?? 0)
const gpuTemp = computed(() => props.temperature?.gpuTemp ?? 0)

const cpuColor    = computed(() => cpuTemp.value > 85 ? 'text-red-500' : cpuTemp.value > 75 ? 'text-orange-500' : cpuTemp.value > 60 ? 'text-yellow-500' : 'text-emerald-500')
const cpuGlow     = computed(() => cpuTemp.value > 85 ? 'bg-red-500/20' : cpuTemp.value > 75 ? 'bg-orange-500/20' : cpuTemp.value > 60 ? 'bg-yellow-500/20' : 'bg-emerald-500/20')
const cpuBarColor = computed(() => cpuTemp.value > 85 ? 'bg-red-500' : cpuTemp.value > 75 ? 'bg-orange-500' : cpuTemp.value > 60 ? 'bg-yellow-500' : 'bg-emerald-500')
const cpuStatus   = computed(() => cpuTemp.value > 85 ? '过热' : cpuTemp.value > 75 ? '偏高' : cpuTemp.value > 60 ? '正常' : '良好')
const cpuProgress = computed(() => Math.min(100, (cpuTemp.value / 100) * 100))

const gpuColor    = computed(() => gpuTemp.value > 85 ? 'text-red-500' : gpuTemp.value > 75 ? 'text-orange-500' : gpuTemp.value > 60 ? 'text-yellow-500' : 'text-emerald-500')
const gpuGlow     = computed(() => gpuTemp.value > 85 ? 'bg-red-500/20' : gpuTemp.value > 75 ? 'bg-orange-500/20' : gpuTemp.value > 60 ? 'bg-yellow-500/20' : 'bg-emerald-500/20')
const gpuBarColor = computed(() => gpuTemp.value > 85 ? 'bg-red-500' : gpuTemp.value > 75 ? 'bg-orange-500' : gpuTemp.value > 60 ? 'bg-yellow-500' : 'bg-emerald-500')
const gpuStatus   = computed(() => gpuTemp.value > 85 ? '过热' : gpuTemp.value > 75 ? '偏高' : gpuTemp.value > 60 ? '正常' : '良好')
const gpuProgress = computed(() => Math.min(100, (gpuTemp.value / 100) * 100))

const fanMaxRpm   = computed(() => {
  const mg = props.fanData?.maxGear
  if (!mg) return 4000
  return mg.includes('超频') ? 4000 : mg.includes('强劲') ? 3300 : 2760
})

const lastTargetRpm = ref<number | null>(null)
const lastTargetAt = ref(0)

watch(() => props.fanData?.targetRpm, (rpm) => {
  if (rpm && rpm > 0) {
    lastTargetRpm.value = rpm
    lastTargetAt.value = Date.now()
  }
}, { immediate: true })

watch(() => props.isSmartFreq, (enabled) => {
  if (!enabled) {
    lastTargetRpm.value = null
    lastTargetAt.value = 0
  }
})

const fanStatus   = computed(() => {
  const d = props.fanData
  if (!d) return '--'
  const modeTag = props.isSmartFreq ? '自动' : '挡位'
  if (d.targetRpm && d.targetRpm > 0) return `目标 ${d.targetRpm} · ${modeTag}`

  if (props.isSmartFreq) {
    if (lastTargetRpm.value) return `目标 ${lastTargetRpm.value} · 自动`
    return `目标 -- · 自动`
  }
  return d.workMode || '--'
})
const fanProgress = computed(() => Math.min(100, ((props.fanData?.currentRpm ?? 0) / fanMaxRpm.value) * 100))

const resolvedDeviceModel = computed(() => {
  const pid = (props.deviceProductId || '').toLowerCase()
  if (pid.includes('0x1002')) return 'BS2 PRO'
  if (pid.includes('0x1001')) return 'BS2'

  const model = (props.deviceModel || '').toUpperCase()
  if (model.includes('BS2PRO') || model.includes('BS2 PRO')) return 'BS2 PRO'
  if (model.includes('BS2')) return 'BS2'

  const mg = props.fanData?.maxGear || ''
  if (mg.includes('超频')) return 'BS2 PRO'
  if (mg) return 'BS2'
  return 'BS2 PRO'
})

function decodeUnknownGearLabel(raw: string, map: Record<number, string>) {
  if (!raw.startsWith('未知(')) return raw
  const match = raw.match(/未知\(0x([0-9A-Fa-f]+)\)/)
  if (!match) return raw
  const code = parseInt(match[1], 16)
  return map[code] || raw
}

const currentGearDisplayText = computed(() => {
  const sg = props.fanData?.setGear
  if (!sg) return '--'
  return decodeUnknownGearLabel(sg, {
    0x08: '静音',
    0x0A: '标准',
    0x0C: '强劲',
    0x0E: '超频',
  })
})

const maxSupportedGearText = computed(() => {
  const mg = props.fanData?.maxGear
  if (!mg) return '--'
  return decodeUnknownGearLabel(mg, {
    0x02: '标准',
    0x04: '强劲',
    0x06: '超频',
  })
})

const gearColor = computed(() => {
  const t = maxSupportedGearText.value
  if (t.includes('超频')) return 'text-purple-600 dark:text-purple-400'
  if (t.includes('强劲')) return 'text-orange-600 dark:text-orange-400'
  return 'text-blue-600 dark:text-blue-400'
})
const gearDot = computed(() => {
  const t = maxSupportedGearText.value
  if (t.includes('超频')) return 'bg-purple-500'
  if (t.includes('强劲')) return 'bg-orange-500'
  return 'bg-blue-500'
})

const engineConvergenceStatus = computed(() =>
  props.config?.fanCurveOffsetEnabled ? '自动调节中' : '已停用'
)

const engineCurrentStatusColor = computed(() => {
  const state = props.temperature?.engineState || '稳定'
  if (state === '补偿中') return 'text-blue-500'
  if (state === '回收中') return 'text-orange-500'
  return 'text-emerald-500'
})

const offsetValueColor = computed(() =>
  props.isDark ? 'text-slate-300' : 'text-blue-600'
)
</script>
