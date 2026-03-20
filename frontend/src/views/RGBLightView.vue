<template>
  <div class="p-8 flex flex-col h-full space-y-5 overflow-visible panel-two-tone">
    <header class="flex justify-between items-center shrink-0">
      <div>
        <h2 class="text-xl font-bold tracking-tight text-slate-800 dark:text-white">RGB 灯效控制</h2>
      </div>
      <!-- 应用状态 -->
      <div class="relative">
        <div v-if="applying"
             class="flex items-center space-x-1.5 text-blue-500 bg-blue-50 dark:bg-blue-900/20 px-3 py-1 rounded-full border border-blue-100 dark:border-blue-800 text-[10px] font-black uppercase tracking-wider animate-pulse">
          <span>应用中...</span>
        </div>
        <div v-else-if="showSuccess"
             class="flex items-center space-x-1.5 text-emerald-500 bg-emerald-50 dark:bg-emerald-500/10 px-3 py-1 rounded-full border border-emerald-100 dark:border-emerald-500/20 text-[10px] font-black uppercase tracking-wider"
             :class="feedbackPulse ? 'animate-ping-once' : ''">
          <CheckCircle2 :size="12" />
          <span>配置已应用</span>
        </div>
        <div v-else-if="lastResult === false"
             class="text-[10px] font-black text-red-500 bg-red-50 dark:bg-red-900/20 border border-red-100 dark:border-red-700 px-3 py-1 rounded-full">应用失败</div>
        <span v-if="!isConnected" class="text-[10px] font-black text-slate-500 bg-slate-100 dark:bg-white/[0.06] px-3 py-1 rounded-full ml-2">设备未连接</span>
      </div>
    </header>

    <!-- 模式选择 -->
    <div class="grid grid-cols-7 gap-2 shrink-0 pb-2">
      <button v-for="mode in MODES" :key="mode.id"
              @click="selectMode(mode.id)"
              :disabled="!isConnected"
              class="p-3 rounded-2xl border transition-all flex flex-col items-center space-y-2 disabled:opacity-40 disabled:cursor-not-allowed"
              :class="[
                activeMode === mode.id
                  ? 'bg-blue-600 border-blue-600 text-white shadow-lg shadow-blue-200'
                  : 'surface-tile text-slate-600 dark:text-slate-300 border-slate-200/70 dark:border-white/10 hover:bg-white/80 dark:hover:bg-white/[0.12]',
                justClicked === mode.id ? 'mode-clicked' : ''
              ]"
              @mousedown="startClickAnim(mode.id)">
        <component :is="mode.icon" :size="16" />
        <span class="text-[10px] font-bold whitespace-nowrap">{{ mode.label }}</span>
      </button>
    </div>

    <!-- 主内容 -->
    <div class="flex-1 flex min-h-0 rounded-[2.5rem] border overflow-hidden surface-card">

      <!-- 预览区域 -->
      <div class="w-1/3 p-8 relative shrink-0 bg-transparent">

        <!-- 圆环预览 -->
        <div class="absolute top-1/2 left-1/2 transform -translate-x-1/2 -translate-y-1/2">
          <div class="relative w-40 h-40">
            <!-- 关闭模式预览 -->
            <template v-if="activeMode === 'off'">
              <div class="absolute inset-0 rounded-full preview-glow opacity-60 bg-slate-700"></div>
              <div class="relative w-full h-full rounded-full border-4 border-white/80 shadow-2xl overflow-hidden flex items-center justify-center bg-white/5 backdrop-blur-sm">
                <ZapOff class="text-slate-400" :size="32" />
              </div>
            </template>

            <!-- 智能模式预览 -->
            <template v-else-if="activeMode === 'smart'">
              <div class="absolute inset-0 rounded-full preview-glow opacity-60 animate-rainbow-spin"
                   style="background: conic-gradient(#ff0000, #00ff00, #0000ff, #ff0000)" />
              <div class="relative w-full h-full rounded-full border-4 border-white/80 shadow-2xl flex items-center justify-center bg-white/10 backdrop-blur-sm">
                <Activity class="text-white animate-pulse" :size="32" />
              </div>
            </template>

            <!-- 旋转模式预览 -->
            <template v-else-if="activeMode === 'rotation'">
              <div class="absolute inset-0 rounded-full preview-glow opacity-60 animate-rainbow-spin"
                   :style="{ background: rotationConicStyle }" />
              <div class="relative w-full h-full rounded-full border-4 border-white/80 shadow-2xl overflow-hidden flex items-center justify-center bg-white/10 backdrop-blur-sm">
                <RotateCw class="text-white animate-pulse" :size="32" />
              </div>
            </template>

            <!-- 呼吸模式预览 -->
            <template v-else-if="activeMode === 'breathing'">
              <div v-for="(c, i) in breathingDisplayColors" :key="i"
                   class="absolute inset-0 rounded-full preview-glow"
                   :style="{
                     backgroundColor: `rgb(${c.r},${c.g},${c.b})`,
                     animation: `rgb-breathing ${breathingDuration}ms linear ${i * breathingPerColor}ms infinite`,
                     opacity: 0
                   }" />
              <div class="relative w-full h-full rounded-full border-4 border-white/80 shadow-2xl overflow-hidden flex items-center justify-center bg-white/10 backdrop-blur-sm">
                <Heart class="text-white opacity-80" :size="28"
                       :style="{ animation: `breathing-scale ${breathingPerColor}ms ease-in-out infinite` }" />
              </div>
            </template>

            <!-- 单色模式预览 -->
            <template v-else-if="activeMode === 'static_single'">
              <div class="absolute inset-0 rounded-full preview-glow opacity-60"
                   :style="{ backgroundColor: staticSingleColor }" />
              <div class="relative w-full h-full rounded-full border-4 border-white/80 shadow-2xl flex items-center justify-center"
                   :style="{ backgroundColor: staticSingleColor + '88' }">
                <CircleDot class="text-white animate-pulse" :size="32" />
              </div>
            </template>

            <!-- 多色模式预览 -->
            <template v-else-if="activeMode === 'static_multi'">
              <div class="absolute inset-0 rounded-full preview-glow opacity-60"
                   :style="{ background: multiConicStyle }" />
              <div class="relative w-full h-full rounded-full border-4 border-white/80 shadow-2xl overflow-hidden flex items-center justify-center bg-white/10 backdrop-blur-sm">
                <Lightbulb class="text-white animate-pulse" :size="32" />
              </div>
            </template>

            <!-- 流光模式预览 -->
            <template v-else-if="activeMode === 'flowing'">
              <div class="absolute inset-0 rounded-full preview-glow opacity-60"
                   style="background: conic-gradient(#0000ff, #007f7f, #00ff00, #7f7f00, #ff0000, #7f007f, #0000ff);
                          animation: rainbow-spin 4s linear infinite" />
              <div class="relative w-full h-full rounded-full border-4 border-white/80 shadow-2xl overflow-hidden flex items-center justify-center bg-white/10 backdrop-blur-sm"
                   style="background: rgba(0,100,200,0.15)">
                <Flame class="text-white animate-pulse" :size="32" />
              </div>
            </template>
          </div>
        </div>

        <!-- 模式说明 -->
        <div class="absolute bottom-8 left-0 right-0 text-center space-y-1">
          <span class="text-xs font-black text-slate-700 dark:text-slate-300 uppercase tracking-wide">{{ currentModeConfig?.label }}</span>
          <p class="text-[10px] text-slate-500 dark:text-slate-400 max-w-[150px] mx-auto leading-relaxed italic opacity-80">{{ currentModeConfig?.desc }}</p>
        </div>
      </div>

      <!-- 控制区域 -->
      <div class="flex-1 p-8 overflow-y-auto flex flex-col rounded-r-[2.5rem]">
        <div class="flex-1 space-y-7">
          <template v-if="activeMode === 'off' || activeMode === 'smart'">
            <div class="h-full flex flex-col justify-center max-w-sm mx-auto px-4 cursor-default select-none">
              <!-- 状态指示点 -->
              <div class="flex items-center gap-3 mb-6">
                <div class="w-1.5 h-1.5 rounded-full bg-blue-500 animate-pulse shadow-[0_0_8px_rgba(59,130,246,0.6)]" v-if="activeMode === 'smart'"></div>
                <div class="w-1.5 h-1.5 rounded-full bg-slate-400" v-else></div>
                <span class="text-[10px] font-black uppercase tracking-widest" :class="isDark ? 'text-slate-500' : 'text-slate-500'">
                  {{ activeMode === 'smart' ? 'Hardware Engine' : 'Deep Hibernation' }}
                </span>
              </div>
              
              <!-- 模式标题 -->
              <h3 class="text-2xl font-black tracking-tight mb-5" :class="isDark ? 'text-slate-200' : 'text-slate-800'">
                {{ activeMode === 'smart' ? '色彩控制已交由底座接管' : '灯光组件已完全关闭' }}
              </h3>
              
              <!-- 模式描述 -->
              <div class="pl-5 border-l-2" :class="activeMode === 'smart' ? 'border-blue-500' : (isDark ? 'border-slate-700' : 'border-slate-300')">
                <p class="text-sm leading-relaxed" :class="isDark ? 'text-slate-400' : 'text-slate-500'">
                  {{ activeMode === 'smart'
                    ? '灯光色彩将随设备温度动态平滑改变，底座灯光将实时反映系统负载状态，无需手动干预。'
                    : '所有灯光模块已完全关闭，设备当前处于最高能效与绝对静默状态。' }}
                </p>
              </div>
            </div>
          </template>

          <template v-else>
            <!-- 亮度控制 -->
            <div class="space-y-3">
              <div class="flex justify-between items-center">
                <label class="text-[10px] font-black text-slate-500 uppercase tracking-widest">主亮度</label>
                <span class="text-xs font-black text-blue-600 dark:text-blue-400">{{ brightness }}%</span>
              </div>
              <div class="flex items-center space-x-4">
                <Sun :size="13" class="text-slate-300 dark:text-slate-600" />
                <input type="range" v-model.number="brightness" min="10" max="100" step="5"
                       :disabled="!isConnected" @change="debouncedApply()"
                       class="flex-1 h-1.5 bg-slate-100 dark:bg-white/[0.08] rounded-full appearance-none accent-blue-600 cursor-pointer disabled:opacity-40" />
                <Sun :size="18" class="text-slate-400 dark:text-slate-500" />
              </div>
            </div>

            <!-- 速度控制 -->
            <div v-if="currentModeConfig?.hasSpeed" class="space-y-3">
              <label class="text-[10px] font-black text-slate-500 uppercase tracking-widest">循环速度</label>
              <div class="flex surface-tile p-1.5 rounded-2xl w-fit border">
                <button v-for="s in SPEED_OPTIONS" :key="s.id"
                        @click="setSpeed(s.id)" :disabled="!isConnected"
                        class="px-8 py-2 rounded-xl text-xs font-bold transition-all disabled:opacity-40"
                        :class="speed === s.id
                          ? 'surface-tile text-blue-600 dark:text-blue-400 border border-slate-200/70 dark:border-white/10'
                          : 'text-slate-400 dark:text-slate-500 hover:text-slate-600 dark:hover:text-slate-300'">
                  {{ s.label }}
                </button>
              </div>
            </div>

            <!-- 颜色序列控制 -->
            <div v-if="currentModeConfig?.hasColors" class="space-y-4">
              <div class="flex justify-between items-center">
                <label class="text-[10px] font-black text-slate-500 uppercase tracking-widest">
                  颜色序列
                  <span v-if="currentModeConfig.minColors !== currentModeConfig.maxColors" class="ml-1 opacity-60">({{ currentModeConfig.minColors }}–{{ currentModeConfig.maxColors }})</span>
                </label>
                <span class="text-[10px] font-bold text-slate-500 dark:text-slate-600">{{ displayColors.length }} 个颜色</span>
              </div>
              <div class="grid grid-cols-6 gap-3">
                <div v-for="(c, i) in displayColors" :key="i" class="space-y-2 group relative">
                  <div @click="togglePicker(i)"
                       class="w-full aspect-square rounded-2xl shadow-md border-4 border-white dark:border-slate-700 cursor-pointer hover:scale-105 transition-all relative overflow-hidden"
                       :class="pickerIndex === i ? 'ring-2 ring-blue-500 ring-offset-2' : ''"
                       :style="{ backgroundColor: `rgb(${c.r},${c.g},${c.b})` }">
                    <div class="absolute inset-0 bg-black/0 hover:bg-black/10 flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity">
                      <Palette :size="14" class="text-white" />
                    </div>
                  </div>
                  <button v-if="canRemove(i)" @click.stop="removeColor(i)"
                          class="absolute -top-2 -right-2 z-10 w-6 h-6 rounded-full bg-slate-400 dark:bg-slate-500 hover:bg-red-500 text-white flex items-center justify-center transition-all shadow-md opacity-0 group-hover:opacity-100 hover:scale-110">
                    <X :size="11" />
                  </button>
                  <div class="text-[9px] font-mono font-bold text-slate-500 dark:text-slate-400 text-center uppercase tracking-tighter">{{ toHex(c) }}</div>
                </div>
                <div v-if="currentModeConfig.maxColors && currentModeConfig.maxColors > displayColors.length" class="space-y-2">
                  <button @click="addColor" :disabled="!isConnected"
                          class="w-full aspect-square rounded-2xl border-2 border-dashed border-slate-200 dark:border-slate-700 hover:border-blue-400 dark:hover:border-blue-500 hover:bg-blue-50 dark:hover:bg-blue-900/20 flex items-center justify-center transition-all disabled:opacity-40 group">
                    <Plus :size="20" class="text-slate-300 dark:text-slate-600 group-hover:text-blue-400 transition-colors" />
                  </button>
                  <div class="text-[9px] font-mono font-bold text-slate-500 dark:text-slate-600 text-center uppercase tracking-tighter">添加</div>
                </div>
              </div>

              <!-- 拾色器 -->
              <div v-if="pickerIndex !== null && pickerIndex < displayColors.length"
                   class="p-5 surface-tile rounded-[1.5rem] border">
                <div class="flex items-center justify-between mb-4">
                  <div class="flex items-center gap-2">
                    <div class="w-5 h-5 rounded-lg border border-white/30 shadow-sm"
                         :style="{ backgroundColor: `rgb(${displayColors[pickerIndex].r},${displayColors[pickerIndex].g},${displayColors[pickerIndex].b})` }" />
                    <span class="text-xs font-black text-slate-600 dark:text-slate-400 uppercase tracking-wider">拾色器</span>
                  </div>
                  <button @click="pickerIndex = null" class="w-6 h-6 rounded-lg flex items-center justify-center text-slate-400 hover:bg-slate-200 dark:hover:bg-slate-700 transition-colors">
                    <X :size="12" />
                  </button>
                </div>
                <HsvPicker :color="displayColors[pickerIndex]" @change="(c) => handleColorChange(pickerIndex!, c)" />
                <div class="flex items-center gap-2 mt-3">
                  <div class="w-8 h-8 rounded-lg shrink-0 border border-black/10 dark:border-white/10 shadow-inner"
                       :style="{ backgroundColor: `rgb(${displayColors[pickerIndex].r},${displayColors[pickerIndex].g},${displayColors[pickerIndex].b})` }" />
                  <div class="flex-1 relative">
                    <span class="absolute left-2.5 top-1/2 -translate-y-1/2 text-xs text-slate-500 dark:text-slate-400 font-mono select-none">#</span>
                    <input type="text" :value="toHex(displayColors[pickerIndex]).replace('#','').toUpperCase()"
                           @change="onHexInput(pickerIndex!, ($event.target as HTMLInputElement).value)"
                           class="w-full pl-6 pr-2 py-1.5 text-xs font-mono bg-white dark:bg-white/[0.06] text-slate-800 dark:text-slate-200 rounded-lg border border-slate-200 dark:border-white/10 focus:outline-none focus:ring-1 focus:ring-blue-400 uppercase"
                           maxlength="6" placeholder="十六进制颜色" />
                  </div>
                  <button @click="handleColorChange(pickerIndex!, randomColor())"
                          class="w-8 h-8 shrink-0 rounded-lg bg-white dark:bg-white/[0.05] hover:bg-slate-100 dark:hover:bg-white/[0.08] flex items-center justify-center transition-colors border border-slate-200 dark:border-white/10" title="随机颜色">
                    <RefreshCw :size="12" class="text-slate-400" />
                  </button>
                </div>
                <div class="space-y-2 mt-3 pt-3 border-t border-slate-200/60 dark:border-slate-700/60">
                  <div v-for="ch in (['r','g','b'] as const)" :key="ch" class="flex items-center gap-2">
                    <span class="text-[10px] font-bold w-3 uppercase text-center shrink-0"
                          :style="{ color: ch==='r' ? '#f87171' : ch==='g' ? '#4ade80' : '#60a5fa' }">{{ ch }}</span>
                    <input type="range" min="0" max="255" :value="displayColors[pickerIndex][ch]"
                           @input="handleColorChange(pickerIndex!, { ...displayColors[pickerIndex], [ch]: +($event.target as HTMLInputElement).value })"
                           @mousedown="handleSliderMouseDown(pickerIndex!, ch, $event)"
                           :data-channel="ch" :data-index="pickerIndex"
                           class="flex-1 h-2 rounded-full appearance-none cursor-pointer"
                           :style="{ background: rgbSliderBg(ch, displayColors[pickerIndex]) }" />
                    <span class="text-[10px] font-mono w-7 text-right text-slate-400 dark:text-slate-500 shrink-0">{{ displayColors[pickerIndex][ch] }}</span>
                  </div>
                </div>
              </div>
            </div>
          </template>
        </div>

      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onUnmounted } from 'vue'
import {
  CheckCircle2, ShieldCheck, Sun, Activity, RotateCw, Heart, Lightbulb,
  Palette, ZapOff, Flame, RotateCcw, X, Plus, RefreshCw, Cpu, Circle, Droplet, Paintbrush, Star, CircleDot
} from 'lucide-vue-next'
import HsvPicker from '../components/ui/HsvPicker.vue'
import { types } from '../../wailsjs/go/models'

interface RGBColor { r: number; g: number; b: number }
interface Props {
  isDark: boolean; isConnected: boolean; savedConfig: types.RGBConfig | null
  onSetRGBMode: (params: { mode: string; colors: RGBColor[]; speed: string; brightness: number }) => Promise<boolean>
}
const props = defineProps<Props>()

type LightMode = 'smart' | 'rotation' | 'breathing' | 'static_single' | 'static_multi' | 'flowing' | 'off'

const MODES = [
  { id: 'smart'         as LightMode, label: '智能温控', icon: Cpu,      desc: '颜色随设备温度实时变换', hasColors: false, hasSpeed: false },
  { id: 'rotation'      as LightMode, label: '旋转',     icon: RotateCw, desc: '色彩序列顺时针旋转，频率可调',    hasColors: true,  hasSpeed: true,  minColors: 1, maxColors: 6 },
  { id: 'breathing'     as LightMode, label: '呼吸',     icon: Heart,    desc: '颜色依次由明至暗，如同呼吸律动',  hasColors: true,  hasSpeed: true,  minColors: 1, maxColors: 5 },
  { id: 'static_single' as LightMode, label: '单色常亮', icon: CircleDot,desc: '最纯粹的基础照明，支持亮度调节',  hasColors: true,  hasSpeed: false, minColors: 1, maxColors: 1 },
  { id: 'static_multi'  as LightMode, label: '多色常亮', icon: Palette,  desc: '三色等分，固态色彩映射',          hasColors: true,  hasSpeed: false, minColors: 3, maxColors: 3 },
  { id: 'flowing'       as LightMode, label: '流光',     icon: Flame,    desc: '硬件内置六色流光，极速律动',      hasColors: false, hasSpeed: true  },
  { id: 'off'           as LightMode, label: '关闭',     icon: ZapOff,   desc: '关闭所有LED颜色输出', hasColors: false, hasSpeed: false },
]
const SPEED_OPTIONS = [{ id: 'slow', label: '慢' }, { id: 'medium', label: '中' }, { id: 'fast', label: '快' }]

const DEFAULT_COLORS: Record<LightMode, RGBColor[]> = {
  smart:         [{ r:255,g:0,b:0 }, { r:0,g:255,b:0 }, { r:0,g:0,b:255 }],
  rotation:      [{ r:255,g:0,b:0 }, { r:255,g:165,b:0 }, { r:255,g:255,b:0 }, { r:0,g:255,b:0 }, { r:0,g:0,b:255 }, { r:128,g:0,b:128 }],
  breathing:     [{ r:0,g:150,b:255 }, { r:0,g:200,b:200 }, { r:0,g:255,b:150 }, { r:100,g:200,b:255 }, { r:150,g:150,b:255 }],
  static_single: [{ r:0,g:0,b:255 }],
  static_multi:  [{ r:255,g:0,b:0 }, { r:0,g:255,b:0 }, { r:0,g:0,b:255 }],
  flowing:       [],
  off:           [],
}

// 页面状态
const activeMode  = ref<LightMode>('smart')
const justClicked = ref<LightMode | null>(null)
let clickAnimTimer: ReturnType<typeof setTimeout> | null = null

function startClickAnim(mode: LightMode) {
  justClicked.value = mode
  if (clickAnimTimer) clearTimeout(clickAnimTimer)
  clickAnimTimer = setTimeout(() => { justClicked.value = null }, 200)
}
const brightness  = ref(100)
const speed       = ref('slow')
const modeColors  = ref<Record<LightMode, RGBColor[]>>(JSON.parse(JSON.stringify(DEFAULT_COLORS)))
const applying    = ref(false)
const lastResult  = ref<boolean | null>(null)
const showSuccess = ref(false)
const feedbackPulse = ref(false)
const pickerIndex = ref<number | null>(null)
const initialized = ref(false)

// 滑块拖动状态
const draggingSlider = ref<{
  pickerIndex: number,
  channel: 'r'|'g'|'b',
  startX: number,
  startValue: number,
  sliderWidth: number
} | null>(null)

let applyTimer: ReturnType<typeof setTimeout> | null = null
let successTimer: ReturnType<typeof setTimeout> | null = null

watch(() => props.savedConfig, (cfg) => {
  if (!cfg || initialized.value) return
  if (cfg.mode)              activeMode.value = cfg.mode as LightMode
  if (cfg.speed)             speed.value = cfg.speed
  if (cfg.brightness != null) brightness.value = cfg.brightness
  if (cfg.colors?.length)    modeColors.value[cfg.mode as LightMode] = cfg.colors.map(c => ({ r: c.r, g: c.g, b: c.b }))
  initialized.value = true
}, { immediate: true })

// 计算属性
const currentModeConfig = computed(() => MODES.find(m => m.id === activeMode.value))
const currentColors     = computed(() => modeColors.value[activeMode.value] || [])
const displayColors     = computed(() => {
  const cfg = currentModeConfig.value
  if (!cfg?.hasColors) return []
  return currentColors.value.slice(0, cfg.maxColors)
})

// 呼吸模式颜色
const breathingDisplayColors = computed(() => displayColors.value.length ? displayColors.value : DEFAULT_COLORS.breathing)
// 旋转模式渐变
const rotationConicStyle = computed(() => {
  const cols = rotationDisplayColors.value
  if (!cols.length) return 'conic-gradient(#ff0000, #00ff00, #0000ff, #ff0000)'
  const step = 360 / cols.length
  const stops = cols.map((c, i) => `rgb(${c.r},${c.g},${c.b}) ${i * step}deg ${(i + 1) * step}deg`)
  return `conic-gradient(${stops.join(', ')})`
})

// 多色模式渐变
const multiConicStyle = computed(() => {
  const cols = multiDisplayColors.value
  const c0 = cols[0] || { r:255,g:0,b:0 }
  const c1 = cols[1] || { r:0,g:255,b:0 }
  const c2 = cols[2] || { r:0,g:0,b:255 }
  return `conic-gradient(rgb(${c0.r},${c0.g},${c0.b}) 0deg 120deg, rgb(${c1.r},${c1.g},${c1.b}) 120deg 240deg, rgb(${c2.r},${c2.g},${c2.b}) 240deg 360deg)`
})

// 每色持续时长
const breathingPerColor = computed(() => speed.value === 'fast' ? 1000 : speed.value === 'medium' ? 2000 : 6000)
const breathingDuration = computed(() => breathingPerColor.value * breathingDisplayColors.value.length)
const breathingDelay    = computed(() => breathingPerColor.value)

// 注入呼吸动画
function injectBreathingKeyframe() {
  const N   = breathingDisplayColors.value.length || 1
  const slot = +(100 / N).toFixed(3)
  const fadeIn  = +(slot * 0.25).toFixed(3)
  const peak    = +(slot * 0.65).toFixed(3)
  const fadeOut = +slot.toFixed(3)
  let el = document.getElementById('rgb-breathing-kf') as HTMLStyleElement | null
  if (!el) {
    el = document.createElement('style')
    el.id = 'rgb-breathing-kf'
    document.head.appendChild(el)
  }
  el.textContent = `@keyframes rgb-breathing {
    0%           { opacity: 0; }
    ${fadeIn}%   { opacity: 0.75; }
    ${peak}%     { opacity: 0.75; }
    ${fadeOut}%  { opacity: 0; }
    100%         { opacity: 0; }
  }`
}
watch([breathingDisplayColors, breathingPerColor], injectBreathingKeyframe, { immediate: true })
onUnmounted(() => { document.getElementById('rgb-breathing-kf')?.remove() })

// 旋转模式时序
const rotationDisplayColors = computed(() => displayColors.value.length ? displayColors.value : DEFAULT_COLORS.rotation)
const rotationDuration = computed(() => speed.value === 'fast' ? 800 : speed.value === 'medium' ? 2000 : 4000)
const rotationDelay    = computed(() => rotationDuration.value / rotationDisplayColors.value.length)

// 流光模式颜色
const FLOWING_COLORS = [
  { r:0,g:0,b:255 }, { r:0,g:127,b:127 }, { r:0,g:255,b:0 },
  { r:127,g:127,b:0 }, { r:255,g:0,b:0 }, { r:127,g:0,b:127 }
]
const flowingDuration = computed(() => speed.value === 'fast' ? 600 : speed.value === 'medium' ? 1200 : 2400)
const flowingDelay    = computed(() => flowingDuration.value / FLOWING_COLORS.length)

// 多色模式展示色
const multiDisplayColors = computed(() => {
  const cols = displayColors.value
  return cols.length >= 3 ? cols.slice(0,3) : DEFAULT_COLORS.static_multi.slice(0,3)
})

// 单色模式展示色
const staticSingleColor = computed(() => {
  const c = displayColors.value[0]
  return c ? `rgb(${c.r},${c.g},${c.b})` : '#007AFF'
})



// 交互操作
function selectMode(mode: LightMode) {
  activeMode.value = mode
  pickerIndex.value = null
  const cfg = MODES.find(m => m.id === mode)!
  const colors = cfg.hasColors ? modeColors.value[mode].slice(0, cfg.maxColors) : []
  triggerApply(mode, colors, speed.value, brightness.value)
}

function setSpeed(s: string) {
  speed.value = s
  triggerApply(activeMode.value, displayColors.value, s, brightness.value)
}

function debouncedApply() {
  if (applyTimer) clearTimeout(applyTimer)
  applyTimer = setTimeout(() => triggerApply(activeMode.value, displayColors.value, speed.value, brightness.value), 300)
}

async function triggerApply(mode: LightMode, colors: RGBColor[], spd: string, br: number) {
  if (!props.isConnected) return
  if (applyTimer) { clearTimeout(applyTimer); applyTimer = null }
  applying.value = true
  showSuccess.value = false
  try {
    const ok = await props.onSetRGBMode({ mode, colors, speed: spd, brightness: br })
    lastResult.value = ok
    if (ok) {
      showSuccess.value = true
      feedbackPulse.value = true
      if (successTimer) clearTimeout(successTimer)
      successTimer = setTimeout(() => { feedbackPulse.value = false }, 600)
    }
  } catch { lastResult.value = false }
  finally { applying.value = false }
}

function handleColorChange(idx: number, c: RGBColor) {
  const next = [...currentColors.value]; next[idx] = c
  modeColors.value = { ...modeColors.value, [activeMode.value]: next }
  if (applyTimer) clearTimeout(applyTimer)
  applyTimer = setTimeout(() => triggerApply(activeMode.value, displayColors.value, speed.value, brightness.value), 300)
}

function addColor() {
  const cfg = currentModeConfig.value
  if (!cfg?.maxColors || currentColors.value.length >= cfg.maxColors) return
  const next = [...currentColors.value, { r: 255, g: 255, b: 255 }]
  modeColors.value = { ...modeColors.value, [activeMode.value]: next }
  debouncedApply()
}
function removeColor(idx: number) {
  const cfg = currentModeConfig.value
  if (!cfg?.minColors || currentColors.value.length <= cfg.minColors) return
  const next = currentColors.value.filter((_, i) => i !== idx)
  modeColors.value = { ...modeColors.value, [activeMode.value]: next }
  if (pickerIndex.value === idx) pickerIndex.value = null
  debouncedApply()
}
function canRemove(idx: number) {
  const cfg = currentModeConfig.value
  return cfg && cfg.minColors !== cfg.maxColors && currentColors.value.length > (cfg.minColors || 1)
}
function togglePicker(idx: number) { pickerIndex.value = pickerIndex.value === idx ? null : idx }
function onHexInput(idx: number, raw: string) {
  const hex = raw.replace(/[^0-9a-fA-F]/g, '').slice(0, 6)
  if (hex.length === 6) {
    const n = parseInt(hex, 16)
    handleColorChange(idx, { r: (n >> 16) & 255, g: (n >> 8) & 255, b: n & 255 })
  }
}

function toHex(c: RGBColor) { return '#' + [c.r,c.g,c.b].map(v => v.toString(16).padStart(2,'0')).join('') }
function randomColor(): RGBColor { return { r: Math.floor(Math.random()*256), g: Math.floor(Math.random()*256), b: Math.floor(Math.random()*256) } }
function rgbSliderBg(ch: 'r'|'g'|'b', c: RGBColor) {
  const v = (n: number) => ch==='r' ? `rgb(${n},${c.g},${c.b})` : ch==='g' ? `rgb(${c.r},${n},${c.b})` : `rgb(${c.r},${c.g},${n})`
  return `linear-gradient(to right, ${v(0)}, ${v(255)})`
}

// 滑块按下处理
function handleSliderMouseDown(pickerIdx: number, channel: 'r'|'g'|'b', e: MouseEvent) {
  const slider = e.target as HTMLInputElement
  const rect = slider.getBoundingClientRect()
  const currentValue = displayColors.value[pickerIdx][channel]
  
  draggingSlider.value = {
    pickerIndex: pickerIdx,
    channel,
    startX: e.clientX,
    startValue: currentValue,
    sliderWidth: rect.width
  }
  
  // 更新光标
  document.body.style.cursor = 'grabbing'
  document.body.style.userSelect = 'none'
  
  document.addEventListener('mousemove', handleGlobalMouseMove)
  document.addEventListener('mouseup', handleGlobalMouseUp)
}

function handleGlobalMouseMove(e: MouseEvent) {
  if (!draggingSlider.value || pickerIndex.value === null) return
  
  const { pickerIndex: idx, channel, startX, startValue, sliderWidth } = draggingSlider.value
  if (idx !== pickerIndex.value) return
  
  // 计算新值
  const deltaX = e.clientX - startX
  const deltaValue = Math.round((deltaX / sliderWidth) * 255)
  const newValue = Math.max(0, Math.min(255, startValue + deltaValue))
  
  // 更新颜色
  handleColorChange(idx, { ...displayColors.value[idx], [channel]: newValue })
}

function handleGlobalMouseUp() {
  draggingSlider.value = null
  document.removeEventListener('mousemove', handleGlobalMouseMove)
  document.removeEventListener('mouseup', handleGlobalMouseUp)
}

// 卸载时清理事件
onUnmounted(() => {
  document.removeEventListener('mousemove', handleGlobalMouseMove)
  document.removeEventListener('mouseup', handleGlobalMouseUp)
  document.getElementById('rgb-breathing-kf')?.remove()
})
</script>

<style>
/* 呼吸旋转关键帧 */
@keyframes breathing-fade {
  0%   { opacity: 0; }
  15%  { opacity: 0.7; }
  35%  { opacity: 0.7; }
  50%  { opacity: 0; }
  100% { opacity: 0; }
}
@keyframes breathing-scale {
  0%, 100% { transform: scale(0.85); opacity: 0.5; }
  50%       { transform: scale(1.1);  opacity: 1; }
}
/* 成功反馈关键帧 */
@keyframes ping-once {
  0%   { transform: scale(1); opacity: 1; }
  50%  { transform: scale(1.06); opacity: 0.85; }
  100% { transform: scale(1); opacity: 1; }
}
.animate-ping-once { animation: ping-once 0.5s ease-out; }

/* 模式按钮关键帧 */
@keyframes mode-press {
  0%   { transform: scale(1); }
  40%  { transform: scale(0.88); }
  100% { transform: scale(1); }
}
.mode-clicked { animation: mode-press 0.18s cubic-bezier(0.4, 0, 0.2, 1); }

/* 旧版脉冲动画已移除 */
</style>

