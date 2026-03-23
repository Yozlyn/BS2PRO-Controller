<template>
  <div class="p-8 flex flex-col h-full space-y-6 overflow-hidden panel-two-tone">
    <header class="shrink-0 flex items-center justify-between">
      <div>
        <h2 class="text-xl font-bold tracking-tight text-slate-800 dark:text-white">风扇曲线配置</h2>
      </div>
      <div class="flex items-center gap-2">
        <span v-if="isDragging" class="text-[10px] font-black text-blue-500 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-700 px-2 py-1 rounded-full uppercase tracking-wider">编辑中</span>
        <span v-if="hasUnsavedChanges" class="text-[10px] font-black text-amber-500 bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-700 px-2 py-1 rounded-full uppercase tracking-wider">未保存</span>
        <div class="w-44">
          <input
            v-if="isRenamingProfile"
            ref="renameInputEl"
            v-model="renameValue"
            type="text"
            autofocus
            maxlength="20"
            class="h-11 w-full rounded-2xl px-4 text-xs font-bold border outline-none cursor-text select-text"
            :class="isDark ? 'bg-white/[0.04] border-white/10 text-slate-200' : 'bg-slate-50 border-slate-100 text-slate-700 shadow-sm'"
            @keyup.enter="submitRenameProfile"
            @keyup.esc="cancelRenameProfile"
            @blur="submitRenameProfile"
          />
          <UnifiedSelect
            v-else
            :model-value="selectedProfileId"
            :options="profileOptions"
            :is-dark="isDark"
            width-class="w-full"
            @update:model-value="switchProfile"
          />
        </div>
        <button
          @click="renameProfile"
          @mousedown="startButtonAnim('rename')"
          :disabled="selectedProfileId === DEVICE_PROFILE_ID"
          class="fan-curve-action-btn px-3 py-2 rounded-xl text-[11px] font-bold transition-all cursor-pointer disabled:cursor-not-allowed disabled:opacity-40"
          :class="[isDark ? 'surface-tile text-slate-300 hover:text-slate-200 hover:bg-slate-700/30' : 'surface-tile text-slate-600 hover:text-slate-700 hover:bg-slate-200/60', clickedButton === 'rename' ? 'mode-clicked' : '']"
        >
          重命名
        </button>
        <button
          @click="createProfile"
          @mousedown="startButtonAnim('create')"
          class="fan-curve-action-btn px-3 py-2 rounded-xl text-[11px] font-bold transition-all cursor-pointer"
          :class="[isDark ? 'surface-tile text-slate-300 hover:text-slate-200 hover:bg-slate-700/30' : 'surface-tile text-slate-600 hover:text-slate-700 hover:bg-slate-200/60', clickedButton === 'create' ? 'mode-clicked' : '']"
        >
          新建
        </button>
        <button
          @click="deleteProfile"
          @mousedown="startButtonAnim('delete')"
          :disabled="selectedProfileId === DEVICE_PROFILE_ID"
          class="fan-curve-action-btn px-3 py-2 rounded-xl text-[11px] font-bold transition-all cursor-pointer disabled:cursor-not-allowed disabled:opacity-40"
          :class="[isDark ? 'surface-tile text-slate-300 hover:text-slate-200 hover:bg-slate-700/30' : 'surface-tile text-slate-600 hover:text-slate-700 hover:bg-slate-200/60', clickedButton === 'delete' ? 'mode-clicked' : '']"
        >
          删除
        </button>
      </div>
    </header>

    <div class="relative flex-1 rounded-[2.5rem] border overflow-hidden flex flex-col min-h-0 surface-card">

      <div ref="chartContainer" class="flex-1 relative min-h-0"
           :class="isDragging ? 'cursor-ns-resize' : 'cursor-crosshair'"
           @mousemove="handleMouseMove" @mouseleave="handleMouseLeave"
           @mouseup="handleMouseUp">
        <svg ref="svgEl" width="100%" height="100%"
             :viewBox="`0 0 ${vbW} ${vbH}`"
             preserveAspectRatio="xMidYMid meet">
          <g v-for="rpm in rpmLabels" :key="'r'+rpm">
            <line :x1="PAD_X" :y1="sy(rpm)" :x2="PAD_X+PLOT_W" :y2="sy(rpm)"
                  :stroke="isDark ? '#334155' : '#f1f5f9'" stroke-dasharray="4" />
            <text :x="PAD_X-8" :y="sy(rpm)+4" font-size="10" fill="#94a3b8" text-anchor="end">{{ rpm }}</text>
          </g>
          <g v-for="(temp, i) in tempLabels" :key="'t'+temp">
            <line :x1="sx(temp)" :y1="PAD_Y" :x2="sx(temp)" :y2="PAD_Y+PLOT_H"
                  :stroke="isDark ? '#334155' : '#f1f5f9'" stroke-dasharray="4" />
            <text v-if="i%2===0" :x="sx(temp)" :y="PAD_Y+PLOT_H+18" font-size="10" fill="#94a3b8" text-anchor="middle">{{ temp }}°C</text>
          </g>

          <path v-if="config.fanCurveOffsetEnabled && offsetPath"
                :d="offsetPath" fill="none" stroke="#f59e0b" stroke-width="2"
                stroke-dasharray="6,3" opacity="0.8" />
          <path :d="mainPath" fill="none" stroke="#3b82f6" stroke-width="3"
                stroke-linecap="round" stroke-linejoin="round" />

          <g v-if="temperature && temperature.maxTemp > 0">
            <line :x1="sx(temperature.maxTemp)" :y1="PAD_Y"
                  :x2="sx(temperature.maxTemp)" :y2="PAD_Y+PLOT_H"
                  stroke="#ef4444" stroke-width="2" stroke-dasharray="4" />
            <rect :x="sx(temperature.maxTemp)-28" :y="PAD_Y-20" width="56" height="18" rx="4" fill="#ef4444" />
            <text :x="sx(temperature.maxTemp)" :y="PAD_Y-8" font-size="10" fill="white" text-anchor="middle" font-weight="bold">
              当前 {{ temperature.maxTemp }}°C
            </text>
          </g>

          <!-- 控制点高亮 -->
          <g v-for="(p, i) in localCurve" :key="'pt'+i" @mousedown.prevent="startDrag(i, $event)">
            <circle :cx="sx(p.temperature)" :cy="sy(p.rpm)"
                    r="14" fill="transparent" style="cursor:ns-resize" />
            <circle :cx="sx(p.temperature)" :cy="sy(p.rpm)"
                    :r="dragIndex === i ? 9 : (hoverSnap?.snappedIndex === i ? 8 : 6)"
                    :fill="dragIndex === i ? '#1d4ed8' : (hoverSnap?.snappedIndex === i ? '#2563eb' : '#3b82f6')"
                    stroke="white" stroke-width="2"
                    style="cursor:ns-resize;transition:r 0.15s,fill 0.1s"
                    :filter="dragIndex === i || hoverSnap?.snappedIndex === i ? 'url(#glow)' : ''" />
            <g v-if="dragIndex === i">
              <rect :x="sx(p.temperature)-36" :y="sy(p.rpm)-34" width="72" height="22" rx="4" fill="#1e40af" opacity="0.95" />
              <text :x="sx(p.temperature)" :y="sy(p.rpm)-19" text-anchor="middle" fill="white" font-size="12" font-weight="600">
                {{ p.rpm }} RPM
              </text>
            </g>
          </g>

          <defs>
            <filter id="glow"><feDropShadow dx="0" dy="0" stdDeviation="4" flood-color="#3b82f6" flood-opacity="0.7"/></filter>
          </defs>
        </svg>

        <!-- 悬停信息面板 -->
        <div v-if="hoverSnap && !isDragging"
             class="absolute pointer-events-none shadow-2xl border p-4 rounded-2xl z-20 text-[11px] backdrop-blur-md w-52"
             :class="isDark ? 'bg-[#1e2330] border-white/10 text-slate-200' : 'bg-white/95 border-slate-100 text-slate-800'"
             :style="hoverSnap.flipLeft
               ? { right: (containerWidth - hoverSnap.screenX + 12) + 'px', top: Math.max(8, hoverSnap.screenY - 110) + 'px' }
               : { left: (hoverSnap.screenX + 12) + 'px',                   top: Math.max(8, hoverSnap.screenY - 110) + 'px' }">
          <div class="font-bold border-b border-slate-100/20 pb-2 mb-2 text-xs">
            交互数据采样 · <span class="text-blue-500">{{ hoverSnap.temp }}°C</span>
          </div>
          <div class="space-y-1.5">
            <div class="flex justify-between gap-4">
              <span class="opacity-60">曲线基础转速:</span>
              <span class="font-bold text-blue-500">{{ hoverSnap.baseRpm }} RPM</span>
            </div>
            <div v-if="config.fanCurveOffsetEnabled" class="flex justify-between gap-4">
              <span class="opacity-60">实际转速(含偏移):</span>
              <span class="font-bold text-amber-500">{{ hoverSnap.effectiveRpm }} RPM</span>
            </div>
          </div>
        </div>
      </div>

      <!-- 底部操作栏 -->
      <div class="mt-4 flex items-center justify-between border-t border-slate-100/10 dark:border-slate-700/20 pt-4 px-6 pb-5 relative z-10 shrink-0">
        <div class="flex items-center space-x-2">
          <button @click="saveCurve" @mousedown="startButtonAnim('save')" :disabled="!hasUnsavedChanges || isSaving || !isConnected"
                  class="fan-curve-action-btn flex items-center space-x-2 px-6 py-2.5 rounded-xl font-bold text-xs transition-all cursor-pointer disabled:cursor-not-allowed disabled:opacity-40"
                  :class="[hasUnsavedChanges && isConnected
                    ? (isDark ? 'surface-tile text-slate-300 hover:text-slate-200 hover:bg-slate-700/30' : 'surface-tile text-slate-600 hover:text-slate-700 hover:bg-slate-200/60')
                    : 'bg-slate-100 dark:bg-slate-800 text-slate-400', clickedButton === 'save' ? 'mode-clicked' : '']">
            <span>{{ isSaving ? '保存中...' : '保存配置' }}</span>
          </button>
          <button @click="applyOffsetCurve" @mousedown="startButtonAnim('apply-offset')" :disabled="isApplyingOffset || !isConnected || !config.fanCurveOffsetEnabled"
                  class="fan-curve-action-btn px-5 py-2.5 rounded-xl text-xs font-bold transition-all cursor-pointer disabled:cursor-not-allowed disabled:opacity-40"
                  :class="[isConnected && config.fanCurveOffsetEnabled
                    ? (isDark ? 'surface-tile text-slate-300 hover:text-slate-200 hover:bg-slate-700/30' : 'surface-tile text-slate-600 hover:text-slate-700 hover:bg-slate-200/60')
                    : 'bg-slate-100 dark:bg-slate-800 text-slate-400', clickedButton === 'apply-offset' ? 'mode-clicked' : '']">
            {{ isApplyingOffset ? '应用中...' : '应用偏移' }}
          </button>
          <button @click="cancelChanges" @mousedown="startButtonAnim('cancel')" :disabled="!hasUnsavedChanges"
                  class="fan-curve-action-btn px-5 py-2.5 rounded-xl text-xs font-bold transition-all cursor-pointer disabled:cursor-not-allowed disabled:opacity-40"
                  :class="[isDark ? 'surface-tile text-slate-300 hover:text-slate-200 hover:bg-slate-700/30' : 'surface-tile text-slate-600 hover:text-slate-700 hover:bg-slate-200/60', clickedButton === 'cancel' ? 'mode-clicked' : '']">取消</button>
          <button @click="resetCurve" @mousedown="startButtonAnim('reset')"
                  class="fan-curve-action-btn flex items-center space-x-2 px-5 py-2.5 rounded-xl text-xs font-bold transition-all cursor-pointer"
                  :class="[isDark ? 'surface-tile text-slate-300 hover:text-slate-200 hover:bg-slate-700/30' : 'surface-tile text-slate-600 hover:text-slate-700 hover:bg-slate-200/60', clickedButton === 'reset' ? 'mode-clicked' : '']">
            <RotateCcw :size="14" /><span>重置默认</span>
          </button>
        </div>
        <div class="flex items-center space-x-2">
          <span v-if="transferMessage"
                class="text-[10px] font-black px-2 py-1 rounded-full uppercase tracking-wider"
                :class="transferMessageType === 'error'
                  ? 'text-red-500 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-700'
                  : 'text-emerald-500 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-700'">
            {{ transferMessage }}
          </span>
          <button @click="exportConfig" @mousedown="startButtonAnim('export')"
                  class="fan-curve-action-btn flex items-center space-x-2 px-5 py-2.5 border rounded-xl text-xs font-bold transition-all cursor-pointer"
                  :class="[isDark ? 'surface-tile border-white/10 text-slate-300 hover:text-slate-200 hover:bg-slate-700/30' : 'surface-tile border-slate-200/70 text-slate-600 hover:text-slate-700 hover:bg-slate-200/60', clickedButton === 'export' ? 'mode-clicked' : '']">
            <Download :size="14" /><span>导出数据</span>
          </button>
          <button @click="importConfig" @mousedown="startButtonAnim('import')"
                  class="fan-curve-action-btn flex items-center space-x-2 px-5 py-2.5 border rounded-xl text-xs font-bold transition-all cursor-pointer"
                  :class="[isDark ? 'surface-tile border-white/10 text-slate-300 hover:text-slate-200 hover:bg-slate-700/30' : 'surface-tile border-slate-200/70 text-slate-600 hover:text-slate-700 hover:bg-slate-200/60', clickedButton === 'import' ? 'mode-clicked' : '']">
            <Upload :size="14" /><span>导入备份</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { RotateCcw, Download, Upload } from 'lucide-vue-next'
import UnifiedSelect from '../components/ui/UnifiedSelect.vue'
import { apiService } from '../services/api'
import { frontendLogger } from '../services/frontendLogger'
import { types } from '../../wailsjs/go/models'
import { DEVICE_PROFILE_ID, type FanCurveProfile, loadCustomProfiles, saveCustomProfiles } from '../services/fanCurveProfiles'

interface Props {
  isDark: boolean; isConnected: boolean
  config: types.AppConfig; fanData: types.FanData | null; temperature: types.TemperatureData | null
}
const props = defineProps<Props>()
const emit = defineEmits<{ 'config-change': [config: types.AppConfig] }>()

// 图表范围常量定义
const TEMP_MIN = 30, TEMP_MAX = 100
const RPM_MIN = 500, RPM_MAX_DEFAULT = 4000
const PLOT_W = 720, PLOT_H = 300
const PAD_X = 60, PAD_Y = 40
const vbW = PLOT_W + PAD_X * 2
const vbH = PLOT_H + PAD_Y * 2

const rpmMax = computed(() => {
  const mg = props.fanData?.maxGear
  if (!mg) return RPM_MAX_DEFAULT
  return mg.includes('超频') ? 4000 : mg.includes('强劲') ? 3300 : 2760
})
const tempLabels = Array.from({ length: (TEMP_MAX - TEMP_MIN) / 5 + 1 }, (_, i) => TEMP_MIN + i * 5)
const rpmLabels  = computed(() =>
  Array.from({ length: Math.floor((rpmMax.value - RPM_MIN) / 500) + 1 }, (_, i) => RPM_MIN + i * 500)
)

// 坐标换算函数
const sx = (temp: number) => PAD_X + ((temp - TEMP_MIN) / (TEMP_MAX - TEMP_MIN)) * PLOT_W
const sy = (rpm: number)  => PAD_Y + PLOT_H - ((Math.min(rpmMax.value, Math.max(RPM_MIN, rpm)) - RPM_MIN) / (rpmMax.value - RPM_MIN)) * PLOT_H

// 视图状态数据
const localCurve = ref<types.FanCurvePoint[]>([])
const hasUnsavedChanges = ref(false)
const isSaving = ref(false)
const isApplyingOffset = ref(false)
const clickedButton = ref<string | null>(null)
let buttonAnimTimer: ReturnType<typeof setTimeout> | null = null
let transferMsgTimer: ReturnType<typeof setTimeout> | null = null
const initialized = ref(false)
const profilesInitialized = ref(false)
const chartContainer = ref<HTMLElement | null>(null)
const svgEl = ref<SVGSVGElement | null>(null)
const renameInputEl = ref<HTMLInputElement | null>(null)
const selectedProfileId = ref(DEVICE_PROFILE_ID)
const curveProfiles = ref<FanCurveProfile[]>([])
const isRenamingProfile = ref(false)
const renameValue = ref('')
const transferMessage = ref('')
const transferMessageType = ref<'success' | 'error'>('success')

const profileOptions = computed(() =>
  curveProfiles.value.map(profile => ({ value: profile.id, label: profile.name }))
)

const cloneCurve = (curve: types.FanCurvePoint[]) =>
  ensureCurve(curve).map(p => types.FanCurvePoint.createFrom({ temperature: p.temperature, rpm: p.rpm, offset: p.offset || 0 }))

function ensureCurve(curve: types.FanCurvePoint[]) {
  const c = [...(curve || [])]
  if (!c.length) return c
  if (c[c.length - 1].temperature < 100) {
    c.push({ temperature: 100, rpm: c[c.length - 1].rpm, offset: 0 } as types.FanCurvePoint)
  }
  return c
}

function curvesEqual(a: types.FanCurvePoint[], b: types.FanCurvePoint[]) {
  if (a.length !== b.length) return false
  return a.every((p, i) => p.temperature === b[i].temperature && p.rpm === b[i].rpm && (p.offset || 0) === (b[i].offset || 0))
}

function getCurrentProfileBaseline() {
  if (selectedProfileId.value === DEVICE_PROFILE_ID) {
    return props.config.fanCurve || []
  }
  const target = curveProfiles.value.find(profile => profile.id === selectedProfileId.value)
  return target?.curve || []
}

function refreshUnsavedFlag() {
  hasUnsavedChanges.value = !curvesEqual(ensureCurve(localCurve.value), ensureCurve(getCurrentProfileBaseline()))
}

function onShortcutSave(event: Event) {
  const view = (event as CustomEvent).detail?.view
  if (view !== 'fan-curve') return
  void saveCurve()
}

function onShortcutEscape(event: Event) {
  const view = (event as CustomEvent).detail?.view
  if (view !== 'fan-curve') return
  cancelChanges()
}

async function persistProfiles() {
  try {
    await saveCustomProfiles(curveProfiles.value.map(profile => ({
      ...profile,
      curve: cloneCurve(profile.curve),
    })))
  } catch (e) {
    frontendLogger.error('风扇曲线', '保存多配置文件失败', e)
  }
}

async function initProfiles(baseCurve: types.FanCurvePoint[]) {
  if (profilesInitialized.value) return
  const defaultProfile: FanCurveProfile = {
    id: DEVICE_PROFILE_ID,
    name: '默认配置',
    curve: cloneCurve(baseCurve),
  }

  const custom = (await loadCustomProfiles()).map(profile => ({ ...profile, curve: cloneCurve(profile.curve) }))

  curveProfiles.value = [defaultProfile, ...custom]
  selectedProfileId.value = DEVICE_PROFILE_ID
  profilesInitialized.value = true
}

function updateSelectedProfileCurve() {
  const target = curveProfiles.value.find(profile => profile.id === selectedProfileId.value)
  if (!target) return
  target.curve = cloneCurve(localCurve.value)
}

function switchProfile(value: string | number) {
  updateSelectedProfileCurve()
  void persistProfiles()
  const target = curveProfiles.value.find(profile => profile.id === String(value))
  if (!target) return
  selectedProfileId.value = target.id
  localCurve.value = cloneCurve(target.curve)
  isRenamingProfile.value = false
  renameValue.value = ''
  refreshUnsavedFlag()
}

function createProfile() {
  updateSelectedProfileCurve()
  const customCount = curveProfiles.value.filter(p => p.id !== DEVICE_PROFILE_ID).length + 1
  const id = `profile-${Date.now()}`
  curveProfiles.value.push({ id, name: `配置 ${customCount}`, curve: cloneCurve(localCurve.value) })
  selectedProfileId.value = id
  void persistProfiles()
  refreshUnsavedFlag()
}

function deleteProfile() {
  if (selectedProfileId.value === DEVICE_PROFILE_ID) return
  curveProfiles.value = curveProfiles.value.filter(profile => profile.id !== selectedProfileId.value)
  selectedProfileId.value = DEVICE_PROFILE_ID
  isRenamingProfile.value = false
  renameValue.value = ''
  const defaultProfile = curveProfiles.value.find(profile => profile.id === DEVICE_PROFILE_ID)
  if (defaultProfile) localCurve.value = cloneCurve(defaultProfile.curve)
  void persistProfiles()
  refreshUnsavedFlag()
}

const selectedProfile = computed(() =>
  curveProfiles.value.find(profile => profile.id === selectedProfileId.value) || null
)

async function renameProfile() {
  if (selectedProfileId.value === DEVICE_PROFILE_ID) return
  if (!selectedProfile.value) return
  isRenamingProfile.value = true
  renameValue.value = selectedProfile.value.name || ''
  await nextTick()
  renameInputEl.value?.focus()
  renameInputEl.value?.select()
}

function submitRenameProfile() {
  if (!isRenamingProfile.value || !selectedProfile.value) return
  const name = renameValue.value.trim()
  if (!name) {
    cancelRenameProfile()
    return
  }
  const target = curveProfiles.value.find(profile => profile.id === selectedProfileId.value)
  if (!target) return
  target.name = name.slice(0, 20)
  isRenamingProfile.value = false
  renameValue.value = ''
  void persistProfiles()
}

function cancelRenameProfile() {
  isRenamingProfile.value = false
  renameValue.value = ''
}

// 拖拽状态数据
const dragIndex = ref<number | null>(null)
const isDragging = ref(false)
// 拖拽期间缓存坐标映射区域
const svgDragRect = ref<DOMRect | null>(null)

// 悬停吸附信息定义
interface HoverSnap {
  screenX: number; screenY: number; flipLeft: boolean
  temp: number; baseRpm: number; effectiveRpm: number; snappedIndex: number
}
const hoverSnap = ref<HoverSnap | null>(null)
const containerWidth = ref(800)

// 通过单调三次插值生成平滑路径
function smoothPath(pts: { x: number; y: number }[]): string {
  const n = pts.length
  if (n === 0) return ''
  if (n === 1) return `M ${pts[0].x},${pts[0].y}`
  if (n === 2) return `M ${pts[0].x},${pts[0].y} L ${pts[1].x},${pts[1].y}`

  // 计算分段斜率
  const dx = [], dy = [], m: number[] = []
  for (let i = 0; i < n - 1; i++) {
    dx[i] = pts[i + 1].x - pts[i].x
    dy[i] = pts[i + 1].y - pts[i].y
    m[i + 1 < n - 1 ? i + 1 : i] = dy[i] / (dx[i] || 1e-9)
  }

  // 计算端点斜率
  const slopes: number[] = new Array(n)
  slopes[0] = dy[0] / (dx[0] || 1e-9)
  slopes[n - 1] = dy[n - 2] / (dx[n - 2] || 1e-9)
  for (let i = 1; i < n - 1; i++) {
    slopes[i] = (dy[i - 1] / (dx[i - 1] || 1e-9) + dy[i] / (dx[i] || 1e-9)) / 2
  }

  // 修正斜率以避免过冲
  for (let i = 0; i < n - 1; i++) {
    const s = dy[i] / (dx[i] || 1e-9)
    if (Math.abs(s) < 1e-9) {
      slopes[i] = 0; slopes[i + 1] = 0; continue
    }
    const a = slopes[i] / s, b = slopes[i + 1] / s
    const h = Math.sqrt(a * a + b * b)
    if (h > 3) {
      slopes[i]     = (3 / h) * a * s
      slopes[i + 1] = (3 / h) * b * s
    }
  }

  // 生成三次贝塞尔路径
  let d = `M ${pts[0].x.toFixed(2)},${pts[0].y.toFixed(2)}`
  for (let i = 0; i < n - 1; i++) {
    const x0 = pts[i].x,     y0 = pts[i].y
    const x1 = pts[i + 1].x, y1 = pts[i + 1].y
    const t = dx[i] / 3
    const cp1x = x0 + t
    const cp1y = y0 + slopes[i] * t
    const cp2x = x1 - t
    const cp2y = y1 - slopes[i + 1] * t
    d += ` C ${cp1x.toFixed(2)},${cp1y.toFixed(2)} ${cp2x.toFixed(2)},${cp2y.toFixed(2)} ${x1.toFixed(2)},${y1.toFixed(2)}`
  }
  return d
}

// 曲线路径数据
const mainPath = computed(() => {
  if (!localCurve.value.length) return ''
  return smoothPath(localCurve.value.map(p => ({ x: sx(p.temperature), y: sy(p.rpm) })))
})
const offsetPath = computed(() => {
  if (!localCurve.value.length) return ''
  return smoothPath(localCurve.value.map(p => {
    const eff = Math.min(rpmMax.value, Math.max(RPM_MIN, Math.round((p.rpm + (p.offset || 0)) / 100) * 100))
    return { x: sx(p.temperature), y: sy(eff) }
  }))
})

// 初始化曲线并同步偏移
watch(() => props.config.fanCurve, (curve) => {
  if (initialized.value || !curve?.length) return
  const c = cloneCurve(curve)
  localCurve.value = c
  void initProfiles(c)
  initialized.value = true
}, { immediate: true })

watch(() => props.config.fanCurve, (curve) => {
  if (!profilesInitialized.value || !curve?.length) return
  const defaultProfile = curveProfiles.value.find(profile => profile.id === DEVICE_PROFILE_ID)
  if (!defaultProfile) return
  defaultProfile.curve = cloneCurve(curve)
  if (selectedProfileId.value === DEVICE_PROFILE_ID && !isDragging.value) {
    localCurve.value = cloneCurve(defaultProfile.curve)
  }
  refreshUnsavedFlag()
}, { deep: true })

watch(() => props.config.fanCurve, (curve) => {
  if (!initialized.value || !props.config.fanCurveOffsetEnabled) return
  localCurve.value = localCurve.value.map((p, i) => {
    const cfgOffset = curve[i]?.offset ?? 0
    return p.offset === cfgOffset ? p : { ...p, offset: cfgOffset }
  })
}, { deep: true })

// 计算悬停吸附点
function computeHoverSnap(clientX: number, clientY: number): HoverSnap | null {
  if (!svgEl.value || !chartContainer.value || !localCurve.value.length) return null

  const svgRect = svgEl.value.getBoundingClientRect()
  // 换算为图表坐标
  const svgX = (clientX - svgRect.left) / svgRect.width * vbW
  const svgY = (clientY - svgRect.top) / svgRect.height * vbH

  // 限制在绘图区内
  if (svgX < PAD_X || svgX > PAD_X + PLOT_W || svgY < PAD_Y || svgY > PAD_Y + PLOT_H) return null

  // 按温度方向吸附最近控制点
  const rawTemp = TEMP_MIN + ((svgX - PAD_X) / PLOT_W) * (TEMP_MAX - TEMP_MIN)
  let nearestIdx = 0
  let minDist = Infinity
  localCurve.value.forEach((p, i) => {
    const d = Math.abs(p.temperature - rawTemp)
    if (d < minDist) { minDist = d; nearestIdx = i }
  })

  const snapPoint = localCurve.value[nearestIdx]
  const snapTemp = snapPoint.temperature
  const baseRpm = snapPoint.rpm
  const offsetVal = props.config.fanCurveOffsetEnabled ? (snapPoint.offset || 0) : 0
  const effectiveRpm = Math.min(rpmMax.value, Math.max(RPM_MIN, Math.round((baseRpm + offsetVal) / 100) * 100))

  const containerRect = chartContainer.value.getBoundingClientRect()
  const screenX = clientX - containerRect.left
  const screenY = clientY - containerRect.top
  containerWidth.value = containerRect.width

  // 靠近右侧时将提示框切换至左侧
  const flipLeft = screenX > containerRect.width * 0.58

  return { screenX, screenY, flipLeft, temp: snapTemp, baseRpm, effectiveRpm, snappedIndex: nearestIdx }
}

// 拖拽更新逻辑
function startDrag(i: number, e: MouseEvent) {
  e.preventDefault()
  dragIndex.value = i
  isDragging.value = true
  hoverSnap.value = null
  // 记录拖拽开始时的坐标区域
  svgDragRect.value = svgEl.value?.getBoundingClientRect() ?? null
}

function handleMouseMove(e: MouseEvent) {
  if (isDragging.value && dragIndex.value !== null && svgDragRect.value) {
    // 将鼠标纵坐标映射为图表坐标
    const svgY = (e.clientY - svgDragRect.value.top) / svgDragRect.value.height * vbH
    // 反算目标转速
    const rpm = RPM_MIN + ((PAD_Y + PLOT_H - svgY) / PLOT_H) * (rpmMax.value - RPM_MIN)
    updatePoint(dragIndex.value, rpm)
    return
  }
  hoverSnap.value = computeHoverSnap(e.clientX, e.clientY)
}

function handleMouseLeave() {
  hoverSnap.value = null
}
function handleMouseUp() {
  if (isDragging.value) {
    dragIndex.value = null
    isDragging.value = false
    svgDragRect.value = null
  }
}
function globalMouseUp() { if (isDragging.value) handleMouseUp() }
onMounted(() => {
  document.addEventListener('mouseup', globalMouseUp)
  document.addEventListener('app-shortcut-save', onShortcutSave as EventListener)
  document.addEventListener('app-shortcut-escape', onShortcutEscape as EventListener)
})
onUnmounted(() => {
  document.removeEventListener('mouseup', globalMouseUp)
  document.removeEventListener('app-shortcut-save', onShortcutSave as EventListener)
  document.removeEventListener('app-shortcut-escape', onShortcutEscape as EventListener)
  if (buttonAnimTimer) clearTimeout(buttonAnimTimer)
  if (transferMsgTimer) clearTimeout(transferMsgTimer)
})

function startButtonAnim(key: string) {
  clickedButton.value = key
  if (buttonAnimTimer) clearTimeout(buttonAnimTimer)
  buttonAnimTimer = setTimeout(() => { clickedButton.value = null }, 200)
}

function showTransferMessage(msg: string, type: 'success' | 'error' = 'success') {
  transferMessage.value = msg
  transferMessageType.value = type
  if (transferMsgTimer) clearTimeout(transferMsgTimer)
  transferMsgTimer = setTimeout(() => { transferMessage.value = '' }, 2500)
}

function updatePoint(index: number, newRpm: number) {
  const clamped = Math.max(RPM_MIN, Math.min(rpmMax.value, Math.round(newRpm / 100) * 100))
  if (localCurve.value[index]?.rpm === clamped) return
  const next = [...localCurve.value]
  next[index] = { ...next[index], rpm: clamped }
  localCurve.value = next
  refreshUnsavedFlag()
}

// 保存与导入导出
async function saveCurve() {
  if (isSaving.value) return
  try {
    isSaving.value = true
    await apiService.setFanCurve(localCurve.value)
    const defaultProfile = curveProfiles.value.find(profile => profile.id === DEVICE_PROFILE_ID)
    if (defaultProfile) defaultProfile.curve = cloneCurve(localCurve.value)
    updateSelectedProfileCurve()
    void persistProfiles()
    emit('config-change', types.AppConfig.createFrom({ ...props.config, fanCurve: localCurve.value }))
    refreshUnsavedFlag()
  } catch (e) { frontendLogger.error('风扇曲线', '保存风扇曲线失败', e) }
  finally { isSaving.value = false }
}

async function applyOffsetCurve() {
  if (isApplyingOffset.value) return
  try {
    isApplyingOffset.value = true
    await apiService.applyOffsetToCurve()
    const curve = await apiService.getFanCurve()
    const nextCurve = [...curve]
    if (nextCurve.length && nextCurve[nextCurve.length - 1].temperature < 100)
      nextCurve.push({ temperature: 100, rpm: nextCurve[nextCurve.length - 1].rpm, offset: 0 } as types.FanCurvePoint)
    localCurve.value = nextCurve
    const defaultProfile = curveProfiles.value.find(profile => profile.id === DEVICE_PROFILE_ID)
    if (defaultProfile) defaultProfile.curve = cloneCurve(nextCurve)
    updateSelectedProfileCurve()
    emit('config-change', types.AppConfig.createFrom({ ...props.config, fanCurve: nextCurve }))
    refreshUnsavedFlag()
  } catch (e) { frontendLogger.error('风扇曲线', '应用偏移曲线失败', e) }
  finally { isApplyingOffset.value = false }
}

function cancelChanges() {
  localCurve.value = cloneCurve(getCurrentProfileBaseline())
  refreshUnsavedFlag()
}
function resetCurve() {
  const m = rpmMax.value
  localCurve.value = [
    { temperature: 30, rpm: 500, offset: 0 }, { temperature: 35, rpm: 1200, offset: 0 },
    { temperature: 40, rpm: 1400, offset: 0 }, { temperature: 45, rpm: 1600, offset: 0 },
    { temperature: 50, rpm: 1800, offset: 100 }, { temperature: 55, rpm: 2000, offset: 100 },
    { temperature: 60, rpm: Math.min(2300, m), offset: 100 }, { temperature: 65, rpm: Math.min(2600, m), offset: 200 },
    { temperature: 70, rpm: Math.min(2900, m), offset: 200 }, { temperature: 75, rpm: Math.min(3200, m), offset: 200 },
    { temperature: 80, rpm: Math.min(3500, m), offset: 300 }, { temperature: 85, rpm: Math.min(3800, m), offset: 200 },
    { temperature: 90, rpm: m, offset: 0 }, { temperature: 95, rpm: m, offset: 0 },
    { temperature: 100, rpm: m, offset: 0 },
  ].map(p => types.FanCurvePoint.createFrom(p))
  refreshUnsavedFlag()
}
function exportConfig() {
  apiService.exportFanCurveProfilesZip()
    .then(() => {})
    .catch((e) => {
      frontendLogger.error('风扇曲线', '导出配置包失败', e)
      showTransferMessage('导出失败', 'error')
      alert('导出失败：请检查保存路径权限或磁盘可用空间')
    })
}
function importConfig() {
  apiService.importFanCurveProfilesZip()
    .then(async () => {
      const cfg = await apiService.getConfig()
      emit('config-change', types.AppConfig.createFrom(cfg))
      localCurve.value = cloneCurve(cfg.fanCurve)
      const baseCurve = cloneCurve(cfg.fanCurve)
      const custom = await loadCustomProfiles()
      curveProfiles.value = [
        { id: DEVICE_PROFILE_ID, name: '默认配置', curve: baseCurve },
        ...custom.map((p) => ({ ...p, curve: cloneCurve(p.curve) })),
      ]
      selectedProfileId.value = DEVICE_PROFILE_ID
      refreshUnsavedFlag()
    })
    .catch((e) => {
      frontendLogger.error('风扇曲线', '导入配置包失败', e)
      showTransferMessage('导入失败', 'error')
      alert(`导入失败：${e}`)
    })
}
</script>
