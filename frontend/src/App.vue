<template>
  <div class="h-screen w-screen text-slate-900 dark:text-white font-sans transition-colors duration-300 overflow-hidden flex flex-col app-mica-surface">
    <header class="h-12 shrink-0 flex items-center justify-between bg-transparent window-drag">
      <div class="flex items-center gap-3 window-no-drag pl-6"
           @mouseenter="headerFanSpeed = isConnected ? '1.2s' : '10s'"
           @mouseleave="headerFanSpeed = isConnected ? '8s' : '10s'">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"
              class="h-6 w-6 shrink-0 transition-colors duration-500"
             :class="isConnected ? 'text-blue-500' : 'text-slate-400'"
             :style="{ animation: `fan-spin ${headerFanSpeed} linear infinite` }">
          <polygon points="12 2 20.66 7 20.66 17 12 22 3.34 17 3.34 7" stroke-opacity="0.35" />
          <circle cx="12" cy="12" r="3" />
          <path d="M12 8L12 2M16 12L22 12M12 16L12 22M8 12L2 12" />
          <path d="M14.8 9.2L18.5 5.5M14.8 14.8L18.5 18.5M9.2 14.8L5.5 18.5M9.2 9.2L5.5 5.5" stroke-opacity="0.35" />
        </svg>
        <div class="leading-none select-none flex items-center">
          <span class="text-sm font-bold tracking-wide text-slate-800 dark:text-slate-200">BS2PRO</span>
          <span class="ml-1.5 text-xs font-black tracking-wider text-blue-500">CONTROLLER</span>
        </div>
      </div>
      <div class="flex items-center h-full window-no-drag">
        <button @click="minimizeWindow"
                class="h-full w-12 flex items-center justify-center text-slate-500 dark:text-slate-400 hover:bg-slate-200/80 dark:hover:bg-slate-700/80 transition-colors">
          <Minus :size="12" />
        </button>
        <button @click="toggleMaximize"
                class="h-full w-12 flex items-center justify-center text-slate-500 dark:text-slate-400 hover:bg-slate-200/80 dark:hover:bg-slate-700/80 transition-colors">
          <svg v-if="isWindowMaximised" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" class="w-3.5 h-3.5">
            <rect x="5" y="8" width="11" height="11" rx="1.2" />
            <path d="M8 5h11v11" />
            <path d="M17 19h2" />
          </svg>
          <Square v-else :size="10" />
        </button>
        <button @click="closeWindow"
                class="h-full w-12 flex items-center justify-center text-slate-500 dark:text-slate-400 hover:bg-red-500 hover:text-white transition-colors">
          <X :size="12" />
        </button>
      </div>
    </header>

    <div class="flex flex-1 overflow-hidden">
      <aside @click="toggleSidebar"
             class="cursor-pointer transition-all duration-300 bg-transparent flex flex-col p-4 relative"
             :class="isCollapsed ? 'w-20' : 'w-56'">
        <div class="flex-1 space-y-2">
          <SidebarItem :icon="LayoutDashboard" label="设备概览" :active="currentView === 'dashboard'"
                       @click="setCurrentView('dashboard')" :is-dark="isDark" :is-collapsed="isCollapsed" />
          <SidebarItem :icon="LineChart" label="风扇曲线" :active="currentView === 'fan-curve'"
                       @click="setCurrentView('fan-curve')" :is-dark="isDark" :is-collapsed="isCollapsed" />
          <SidebarItem :icon="Sliders" label="设备参数" :active="currentView === 'device-params'"
                       @click="setCurrentView('device-params')" :is-dark="isDark" :is-collapsed="isCollapsed" />
          <SidebarItem :icon="Lightbulb" label="RGB 灯效" :active="currentView === 'rgb-light'"
                       @click="setCurrentView('rgb-light')" :is-dark="isDark" :is-collapsed="isCollapsed" />
          <SidebarItem :icon="Workflow" label="进程联动" :active="currentView === 'process-switch'"
                       @click="setCurrentView('process-switch')" :is-dark="isDark" :is-collapsed="isCollapsed" />
          <SidebarItem :icon="Settings" label="系统设置" :active="currentView === 'system-settings'"
                       @click="setCurrentView('system-settings')" :is-dark="isDark" :is-collapsed="isCollapsed" />
        </div>
        <div class="pt-4 border-t border-slate-100/50 dark:border-white/5">
        <div class="flex items-center transition-all"
             :class="isCollapsed ? 'flex-col space-y-2' : 'justify-between'">
          <button
            @click.stop="toggleDarkMode"
            class="flex-1 flex justify-center items-center p-2.5 rounded-xl transition-all font-semibold"
            :class="isDark ? 'text-slate-400 hover:text-slate-200 hover:bg-slate-700/30' : 'text-slate-600 hover:text-slate-700 hover:bg-slate-200/60'"
          >
            <Sun v-if="!isDark" :size="isCollapsed ? 22 : 18" />
            <Moon v-else :size="isCollapsed ? 22 : 18" />
          </button>
          <div :class="isCollapsed ? 'w-4 h-px' : 'w-px h-4'" class="bg-slate-300/50 dark:bg-slate-700/50 flex-shrink-0" />
          <button
            @click.stop="toggleSidebar"
            class="flex-1 flex justify-center items-center p-2.5 rounded-xl transition-all font-semibold"
            :class="isDark ? 'text-slate-400 hover:text-slate-200 hover:bg-slate-700/30' : 'text-slate-600 hover:text-slate-700 hover:bg-slate-200/60'"
          >
            <ChevronRight v-if="isCollapsed" :size="isCollapsed ? 22 : 18" />
            <ChevronLeft v-else :size="isCollapsed ? 22 : 18" />
          </button>
        </div>
          <SidebarItem :icon="Info" label="关于软件" :is-dark="isDark" :is-collapsed="isCollapsed"
                       :active="currentView === 'about'" @click="setCurrentView('about')" />
        </div>
      </aside>

      <div class="w-px my-6 self-stretch bg-gradient-to-b from-transparent via-slate-200/80 to-transparent dark:via-white/10 flex-shrink-0" />

      <main class="flex-1 overflow-hidden bg-transparent flex flex-col">
        <DashboardView v-if="currentView === 'dashboard'"
          :is-dark="isDark" :is-connected="isConnected"
          :device-model="deviceModel"
          :device-product-id="deviceProductId"
          :is-smart-freq="config.autoControl"
          :fan-data="fanData" :temperature="temperature"
          :config="config"
          @update:is-smart-freq="handleAutoControlChange"
          @disconnect="handleDisconnect"
          @connect="handleConnect" />
        <RGBLightView v-else-if="currentView === 'rgb-light'"
          :is-dark="isDark" :is-connected="isConnected"
          :saved-config="config.rgbConfig || null"
          :onSetRGBMode="handleSetRGBMode" />
        <FanCurveView v-else-if="currentView === 'fan-curve'"
          :is-dark="isDark" :is-connected="isConnected"
          :config="config" :fan-data="fanData" :temperature="temperature"
          @config-change="handleConfigChange" />
        <ProcessSwitchView v-else-if="currentView === 'process-switch'"
          :is-dark="isDark"
          :config="config"
          @config-change="handleConfigChange" />
        <DeviceParamsView v-else-if="currentView === 'device-params'"
          :is-dark="isDark" :is-connected="isConnected"
          :config="config"
          @config-change="handleConfigChange" />
        <SystemSettingsView v-else-if="currentView === 'system-settings'"
          :is-dark="isDark" :is-connected="isConnected"
          :config="config"
          @config-change="handleConfigChange" />
        <AboutView v-else-if="currentView === 'about'" :is-dark="isDark" />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted } from 'vue'
import { LayoutDashboard, Wind, Gauge, Zap, Settings, Info, Sun, Moon, ChevronRight, ChevronLeft, Minus, Square, X, Palette, LineChart, Sliders, Lightbulb, Workflow } from 'lucide-vue-next'
import SidebarItem from './components/ui/SidebarItem.vue'
import DashboardView from './views/DashboardView.vue'
import RGBLightView from './views/RGBLightView.vue'
import FanCurveView from './views/FanCurveView.vue'
import ProcessSwitchView from './views/ProcessSwitchView.vue'
import DeviceParamsView from './views/DeviceParamsView.vue'
import SystemSettingsView from './views/SystemSettingsView.vue'
import AboutView from './views/AboutView.vue'
import { apiService } from './services/api'
import { frontendLogger } from './services/frontendLogger'
import { types } from '../wailsjs/go/models'
import { HideWindow } from '../wailsjs/go/main/App'
import { WindowToggleMaximise, WindowIsMaximised } from '../wailsjs/runtime/runtime'

const isDark = ref(localStorage.theme === 'dark')
const isCollapsed = ref(true)
const currentView = ref('dashboard')
const isConnected = ref(false)
const deviceModel = ref('BS2PRO')
const deviceProductId = ref('')
const appVersion = ref('')
const headerFanSpeed = ref('10s')
const isWindowMaximised = ref(false)

const config = ref<types.AppConfig>(types.AppConfig.createFrom({
  autoControl: true, fanCurve: [], gearLight: true, powerOnStart: false,
  windowsAutoStart: false, smartStartStop: 'delayed', brightness: 100,
  tempUpdateRate: 1, tempSampleCount: 3, configPath: '', manualGear: '标准',
  manualLevel: '中', debugMode: false, guiMonitoring: false,
  customSpeedEnabled: false, customSpeedRPM: 2000,
  ignoreDeviceOnReconnect: true, fanCurveOffsetEnabled: false, rgbConfig: null,
}))
const fanData = ref<types.FanData | null>(null)
const temperature = ref<types.TemperatureData | null>(null)

watch(isConnected, (connected) => {
  headerFanSpeed.value = connected ? '8s' : '10s'
})

let unsubFanData: (() => void) | null = null
let unsubTemperature: (() => void) | null = null
let unsubConnected: (() => void) | null = null
let unsubDisconnected: (() => void) | null = null
let unsubConfigUpdate: (() => void) | null = null
let unsubWindowShown: (() => void) | null = null
let unsubWindowHidden: (() => void) | null = null
let clearHoverTimer: number | null = null
let hoverUnlockHandler: ((e: Event) => void) | null = null

const forceHoverReset = () => {
  const appRoot = document.getElementById('app') as HTMLElement | null
  if (appRoot) {
    const oldDisplay = appRoot.style.display
    appRoot.style.display = 'none'
    void appRoot.offsetHeight
    appRoot.style.display = oldDisplay
  }
  window.dispatchEvent(new Event('resize'))
  document.dispatchEvent(new MouseEvent('mousemove', { bubbles: true, clientX: -1, clientY: -1 }))
}

const detachHoverUnlockListeners = () => {
  if (!hoverUnlockHandler) return
  window.removeEventListener('mousemove', hoverUnlockHandler)
  window.removeEventListener('mousedown', hoverUnlockHandler)
  window.removeEventListener('wheel', hoverUnlockHandler)
  window.removeEventListener('keydown', hoverUnlockHandler)
  hoverUnlockHandler = null
}

const releaseHoverLock = (reason: string) => {
  document.documentElement.classList.remove('disable-hover')
  if (clearHoverTimer) {
    window.clearTimeout(clearHoverTimer)
    clearHoverTimer = null
  }
  detachHoverUnlockListeners()
}

const lockHoverImmediately = (reason: string) => {
  document.documentElement.classList.add('disable-hover')
}

const clearStickyHover = (reason: string) => {
  detachHoverUnlockListeners()
  document.documentElement.classList.add('disable-hover')
  forceHoverReset()
  requestAnimationFrame(() => forceHoverReset())

  hoverUnlockHandler = (e: Event) => {
    if (e.type === 'mousemove') {
      const ev = e as MouseEvent
      if (ev.clientX < 0 || ev.clientY < 0) return
    }
    releaseHoverLock(`interaction:${e.type}`)
  }
  window.addEventListener('mousemove', hoverUnlockHandler)
  window.addEventListener('mousedown', hoverUnlockHandler)
  window.addEventListener('wheel', hoverUnlockHandler)
  window.addEventListener('keydown', hoverUnlockHandler)

  if (clearHoverTimer) window.clearTimeout(clearHoverTimer)
  clearHoverTimer = window.setTimeout(() => {
    releaseHoverLock('timeout')
  }, 3000)
  uiStateLogger(reason, 'debug')
}

const uiStateLogger = (reason: string, level: 'info' | 'debug' = 'debug') => {
  const appRoot = document.getElementById('app')
  const rect = appRoot?.getBoundingClientRect()
  const appSize = rect ? `${Math.round(rect.width)}x${Math.round(rect.height)}` : 'nil'
  const reasonLabel = formatUiReason(reason)
  const msg = `触发原因=${reasonLabel}，页面隐藏=${document.hidden}，可见状态=${document.visibilityState}，窗口焦点=${document.hasFocus()}，窗口尺寸=${window.innerWidth}x${window.innerHeight}，应用区域=${appSize}，当前视图=${currentView.value}`
  if (level === 'info') frontendLogger.info('界面状态', msg)
  else frontendLogger.debug('界面状态', msg)
}

const formatUiReason = (reason: string) => {
  const map: Record<string, string> = {
    'visibility-hidden': '页面不可见',
    visibilitychange: '可见性变化',
    focus: '窗口获得焦点',
    blur: '窗口失去焦点',
    resize: '窗口尺寸变化',
    pageshow: '页面显示',
    'window-shown': '窗口显示',
    'window-hidden': '窗口隐藏',
    'window-shown+50ms': '窗口显示后50毫秒',
    'window-shown+300ms': '窗口显示后300毫秒',
    mounted: '应用挂载完成',
    timeout: '悬停锁超时解除',
    'click-minimize': '点击最小化按钮',
    'click-close-as-minimize': '点击关闭按钮(映射最小化)',
  }
  if (reason.startsWith('interaction:')) {
    const action = reason.replace('interaction:', '')
    return `用户交互恢复(${action})`
  }
  return map[reason] || reason
}

const onVisibilityChange = () => {
  if (document.hidden) lockHoverImmediately('visibility-hidden')
  uiStateLogger('visibilitychange')
}
const onFocus = () => {
  uiStateLogger('focus')
  void syncWindowMaxState()
}
const onBlur = () => uiStateLogger('blur')
const onResize = () => {
  uiStateLogger('resize')
  void syncWindowMaxState()
}
const onPageShow = () => uiStateLogger('pageshow')

async function syncWindowMaxState() {
  try { isWindowMaximised.value = await WindowIsMaximised() } catch {}
}

onMounted(async () => {
  if (isDark.value) document.documentElement.classList.add('dark')

  document.addEventListener('visibilitychange', onVisibilityChange)
  window.addEventListener('focus', onFocus)
  window.addEventListener('blur', onBlur)
  window.addEventListener('resize', onResize)
  window.addEventListener('pageshow', onPageShow)

  unsubWindowShown = apiService.onWindowShown(() => {
    clearStickyHover('window-shown')
    setTimeout(() => uiStateLogger('window-shown+50ms', 'debug'), 50)
    setTimeout(() => uiStateLogger('window-shown+300ms', 'debug'), 300)
  })
  unsubWindowHidden = apiService.onWindowHidden(() => uiStateLogger('window-hidden', 'debug'))

  unsubFanData = apiService.onFanDataUpdate((data) => { fanData.value = data })
  unsubTemperature = apiService.onTemperatureUpdate((data) => { temperature.value = data })
  unsubConnected = apiService.onDeviceConnected((data: any) => {
    isConnected.value = true
    deviceModel.value = (data?.model || 'BS2PRO').toString()
    deviceProductId.value = (data?.productId || '').toString()
    loadConfig()
  })
  unsubDisconnected = apiService.onDeviceDisconnected(() => {
    isConnected.value = false
    deviceModel.value = 'BS2PRO'
    deviceProductId.value = ''
    fanData.value = null
  })
  unsubConfigUpdate = apiService.onConfigUpdate((cfg) => {
    config.value = cfg
    frontendLogger.setDebugEnabled(!!cfg.debugMode)
    frontendLogger.debug('配置', '收到配置更新', { debugMode: cfg.debugMode })
  })

  try {
    const [cfg, ver, status] = await Promise.allSettled([
      apiService.getConfig(), apiService.getAppVersion(), apiService.getDeviceStatus(),
    ])
    if (cfg.status === 'fulfilled') {
      config.value = cfg.value
      frontendLogger.setDebugEnabled(!!cfg.value.debugMode)
    }
    if (ver.status === 'fulfilled') appVersion.value = ver.value
    if (status.status === 'fulfilled') {
      const s = (status.value as any) || {}
      isConnected.value = !!s.connected
      deviceModel.value = (s.model || 'BS2PRO').toString()
      deviceProductId.value = (s.productId || '').toString()
    }
    if (isConnected.value) {
      const [td, fd] = await Promise.allSettled([apiService.getTemperature(), apiService.getCurrentFanData()])
      if (td.status === 'fulfilled') temperature.value = td.value
      if (fd.status === 'fulfilled') fanData.value = fd.value
    }
  } catch (e) { frontendLogger.error('应用初始化', '应用初始化失败', e) }

  await syncWindowMaxState()
  uiStateLogger('mounted', 'debug')
})

onUnmounted(() => {
  unsubFanData?.(); unsubTemperature?.(); unsubConnected?.()
  unsubDisconnected?.(); unsubConfigUpdate?.()
  unsubWindowShown?.(); unsubWindowHidden?.()
  if (clearHoverTimer) {
    window.clearTimeout(clearHoverTimer)
    clearHoverTimer = null
  }
  detachHoverUnlockListeners()
  document.documentElement.classList.remove('disable-hover')
  document.removeEventListener('visibilitychange', onVisibilityChange)
  window.removeEventListener('focus', onFocus)
  window.removeEventListener('blur', onBlur)
  window.removeEventListener('resize', onResize)
  window.removeEventListener('pageshow', onPageShow)
})

async function loadConfig() {
  try {
    config.value = await apiService.getConfig()
    frontendLogger.setDebugEnabled(!!config.value.debugMode)
  } catch (e) { frontendLogger.error('加载配置', '加载配置失败', e) }
}

const toggleDarkMode = () => {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.theme = isDark.value ? 'dark' : 'light'
}
const toggleSidebar = () => { isCollapsed.value = !isCollapsed.value }
const setCurrentView = (view: string) => { currentView.value = view }
const handleConfigChange = (newConfig: types.AppConfig) => { config.value = newConfig }

const handleAutoControlChange = async (enabled: boolean) => {
  try {
    if (enabled && config.value.customSpeedEnabled) {
      await apiService.setCustomSpeed(false, config.value.customSpeedRPM || 2000)
    }
    await apiService.setAutoControl(enabled)
    config.value = types.AppConfig.createFrom({
      ...config.value,
      autoControl: enabled,
      customSpeedEnabled: enabled ? false : config.value.customSpeedEnabled,
    })
  } catch (e) { frontendLogger.error('智能变频', '设置智能变频失败', e) }
}
const handleSetRGBMode = async (params: any): Promise<boolean> => {
  try { return await apiService.setRGBMode(params) } catch (e) { return false }
}
const handleDisconnect = async () => {
  try { await apiService.disconnectDevice() } catch (e) { frontendLogger.error('设备', '断开设备失败', e) }
}
const handleConnect = async () => {
  try { await apiService.connectDevice() } catch (e) { frontendLogger.error('设备', '连接设备失败', e) }
}

const minimizeWindow = () => {
  frontendLogger.info('窗口', '点击最小化')
  lockHoverImmediately('click-minimize')
  HideWindow().catch((e) => frontendLogger.error('窗口', '最小化失败', e))
}
const toggleMaximize = () => {
  WindowToggleMaximise()
  window.setTimeout(() => { void syncWindowMaxState() }, 80)
}
const closeWindow = () => {
  frontendLogger.info('窗口', '点击关闭(映射最小化)')
  lockHoverImmediately('click-close-as-minimize')
  HideWindow().catch((e) => frontendLogger.error('窗口', '关闭映射最小化失败', e))
}
</script>
