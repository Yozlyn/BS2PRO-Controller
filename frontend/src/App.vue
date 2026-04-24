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
             class="cursor-pointer bg-transparent flex flex-col p-4 relative transition-[width,padding] duration-400 ease-[cubic-bezier(0.22,1,0.36,1)]"
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
        <div class="pt-4 border-t border-slate-100/50 dark:border-white/5 space-y-3">
        <div class="flex items-center transition-all duration-300 ease-[cubic-bezier(0.22,1,0.36,1)]"
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

      <div class="w-px my-6 self-stretch bg-gradient-to-b from-transparent via-slate-200/80 to-transparent dark:via-white/10 flex-shrink-0 transition-opacity duration-300 ease-[cubic-bezier(0.22,1,0.36,1)]" />

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
          :follow-system-theme="followSystemTheme"
          :config="config"
          @config-change="handleConfigChange"
          @follow-system-theme-change="handleFollowSystemThemeChange" />
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
import { HideWindow, QuitApp } from '../wailsjs/go/main/App'
import { WindowToggleMaximise, WindowIsMaximised } from '../wailsjs/runtime/runtime'

const THEME_STORAGE_KEY = 'theme'
const THEME_FOLLOW_SYSTEM_KEY = 'theme-follow-system'

const getFollowSystemTheme = (): boolean => localStorage.getItem(THEME_FOLLOW_SYSTEM_KEY) !== 'false'

const getStoredTheme = (): 'dark' | 'light' | null => {
  const storedTheme = localStorage.getItem(THEME_STORAGE_KEY)
  if (storedTheme === 'dark' || storedTheme === 'light') return storedTheme
  return null
}

const applyTheme = (dark: boolean) => {
  isDark.value = dark
  document.documentElement.classList.toggle('dark', dark)
}

const resolveInitialDarkMode = () => {
  if (getFollowSystemTheme()) return window.matchMedia('(prefers-color-scheme: dark)').matches
  const storedTheme = getStoredTheme()
  if (storedTheme === 'dark') return true
  if (storedTheme === 'light') return false
  return window.matchMedia('(prefers-color-scheme: dark)').matches
}

const systemThemeMedia = window.matchMedia('(prefers-color-scheme: dark)')

const followSystemTheme = ref(getFollowSystemTheme())
const isDark = ref(resolveInitialDarkMode())
applyTheme(isDark.value)
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
let unsubHotkeyAction: (() => void) | null = null
let clearHoverTimer: number | null = null
let hoverUnlockHandler: ((e: Event) => void) | null = null
let appHotkeyHandler: ((e: KeyboardEvent) => void) | null = null
let systemThemeChangeHandler: ((event: MediaQueryListEvent) => void) | null = null
let systemThemeLegacyChangeHandler: ((event: MediaQueryListEvent) => void) | null = null

const handleSystemThemeChange = (matches: boolean) => {
  if (!followSystemTheme.value) return
  applyTheme(matches)
}

const attachSystemThemeListener = () => {
  systemThemeChangeHandler = (event: MediaQueryListEvent) => handleSystemThemeChange(event.matches)
  if (typeof systemThemeMedia.addEventListener === 'function') {
    systemThemeMedia.addEventListener('change', systemThemeChangeHandler)
  }

  if (typeof systemThemeMedia.addListener === 'function') {
    systemThemeLegacyChangeHandler = (event: MediaQueryListEvent) => handleSystemThemeChange(event.matches)
    systemThemeMedia.addListener(systemThemeLegacyChangeHandler)
  }
}

const detachSystemThemeListener = () => {
  if (systemThemeChangeHandler && typeof systemThemeMedia.removeEventListener === 'function') {
    systemThemeMedia.removeEventListener('change', systemThemeChangeHandler)
  }
  if (systemThemeLegacyChangeHandler && typeof systemThemeMedia.removeListener === 'function') {
    systemThemeMedia.removeListener(systemThemeLegacyChangeHandler)
  }
  systemThemeChangeHandler = null
  systemThemeLegacyChangeHandler = null
}

const syncThemePreferenceToNative = async () => {
  try {
    await (window as any).go?.main?.App?.SaveThemePreference?.(followSystemTheme.value, isDark.value ? 'dark' : 'light')
  } catch (e) {
    frontendLogger.debug('主题', '同步原生主题偏好失败', e)
  }
}

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

const releaseHoverLock = () => {
  document.documentElement.classList.remove('disable-hover')
  if (clearHoverTimer) {
    window.clearTimeout(clearHoverTimer)
    clearHoverTimer = null
  }
  detachHoverUnlockListeners()
}

const lockHoverImmediately = () => {
  document.documentElement.classList.add('disable-hover')
}

const clearStickyHover = () => {
  detachHoverUnlockListeners()
  document.documentElement.classList.add('disable-hover')
  forceHoverReset()
  requestAnimationFrame(() => forceHoverReset())

  hoverUnlockHandler = (e: Event) => {
    if (e.type === 'mousemove') {
      const ev = e as MouseEvent
      if (ev.clientX < 0 || ev.clientY < 0) return
    }
    releaseHoverLock()
  }
  window.addEventListener('mousemove', hoverUnlockHandler)
  window.addEventListener('mousedown', hoverUnlockHandler)
  window.addEventListener('wheel', hoverUnlockHandler)
  window.addEventListener('keydown', hoverUnlockHandler)

  if (clearHoverTimer) window.clearTimeout(clearHoverTimer)
  clearHoverTimer = window.setTimeout(() => {
    releaseHoverLock()
  }, 3000)
}

const onVisibilityChange = () => {
  if (document.hidden) lockHoverImmediately()
}
const onFocus = () => {
  void syncWindowMaxState()
}
const onResize = () => {
  void syncWindowMaxState()
}

async function syncWindowMaxState() {
  try { isWindowMaximised.value = await WindowIsMaximised() } catch {}
}

onMounted(async () => {
  applyTheme(resolveInitialDarkMode())
  void syncThemePreferenceToNative()

  document.addEventListener('visibilitychange', onVisibilityChange)
  appHotkeyHandler = (e: KeyboardEvent) => handleAppHotkeys(e)
  document.addEventListener('keydown', appHotkeyHandler)
  window.addEventListener('focus', onFocus)
  window.addEventListener('resize', onResize)
  attachSystemThemeListener()

  unsubWindowShown = apiService.onWindowShown(() => {
    clearStickyHover()
  })
  unsubHotkeyAction = apiService.onHotkeyAction((action) => { void runHotkeyAction(action) })

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
})

onUnmounted(() => {
  unsubFanData?.(); unsubTemperature?.(); unsubConnected?.()
  unsubDisconnected?.(); unsubConfigUpdate?.()
  unsubWindowShown?.(); unsubHotkeyAction?.()
  if (clearHoverTimer) {
    window.clearTimeout(clearHoverTimer)
    clearHoverTimer = null
  }
  detachHoverUnlockListeners()
  document.documentElement.classList.remove('disable-hover')
  document.removeEventListener('visibilitychange', onVisibilityChange)
  if (appHotkeyHandler) document.removeEventListener('keydown', appHotkeyHandler)
  window.removeEventListener('focus', onFocus)
  window.removeEventListener('resize', onResize)
  detachSystemThemeListener()
})

async function loadConfig() {
  try {
    config.value = await apiService.getConfig()
    frontendLogger.setDebugEnabled(!!config.value.debugMode)
  } catch (e) { frontendLogger.error('加载配置', '加载配置失败', e) }
}

const toggleDarkMode = () => {
  if (followSystemTheme.value) {
    followSystemTheme.value = false
    localStorage.setItem(THEME_FOLLOW_SYSTEM_KEY, 'false')
  }
  const nextDark = !isDark.value
  applyTheme(nextDark)
  localStorage.setItem(THEME_STORAGE_KEY, nextDark ? 'dark' : 'light')
  void syncThemePreferenceToNative()
}

const handleFollowSystemThemeChange = (enabled: boolean) => {
  followSystemTheme.value = enabled
  localStorage.setItem(THEME_FOLLOW_SYSTEM_KEY, enabled ? 'true' : 'false')
  if (enabled) {
    localStorage.removeItem(THEME_STORAGE_KEY)
    applyTheme(systemThemeMedia.matches)
    void syncThemePreferenceToNative()
    return
  }
  localStorage.setItem(THEME_STORAGE_KEY, isDark.value ? 'dark' : 'light')
  void syncThemePreferenceToNative()
}
const toggleSidebar = () => { isCollapsed.value = !isCollapsed.value }
const setCurrentView = (view: string) => { currentView.value = view }
const handleConfigChange = (newConfig: types.AppConfig) => { config.value = newConfig }

const appViewActionMap: Record<string, string> = {
  'navigate-dashboard': 'dashboard',
  'navigate-fan-curve': 'fan-curve',
  'navigate-device-params': 'device-params',
  'navigate-rgb-light': 'rgb-light',
  'navigate-process-switch': 'process-switch',
  'navigate-system-settings': 'system-settings',
  'navigate-about': 'about',
}

const buildAccelerator = (e: KeyboardEvent) => {
  const parts: string[] = []
  if (e.ctrlKey || e.metaKey) parts.push('Ctrl')
  if (e.altKey) parts.push('Alt')
  if (e.shiftKey) parts.push('Shift')
  const key = e.key === 'Escape' ? 'Escape' : e.key.length === 1 ? e.key.toUpperCase() : e.key
  parts.push(key)
  return parts.join('+')
}

const isEditableTarget = (target: EventTarget | null) => {
  const el = target as HTMLElement | null
  if (!el) return false
  const tag = el.tagName?.toLowerCase()
  return el.isContentEditable || tag === 'input' || tag === 'textarea' || tag === 'select'
}

const runContextSave = async () => {
  document.dispatchEvent(new CustomEvent('app-shortcut-save', { detail: { view: currentView.value } }))
}

const runHotkeyAction = async (action: string) => {
  if (!action) return
  if (action === 'save-context') {
    void runContextSave()
    return
  }
  if (action === 'toggle-sidebar') {
    toggleSidebar()
    return
  }
  if (action === 'escape-local-interaction') {
    document.dispatchEvent(new CustomEvent('app-shortcut-escape', { detail: { view: currentView.value } }))
    return
  }
  const targetView = appViewActionMap[action]
  if (targetView) setCurrentView(targetView)
}

const handleAppHotkeys = (e: KeyboardEvent) => {
  const hotkeys = (config.value as any).hotkeys as { enabled?: boolean; inApp?: Array<{ enabled?: boolean; accelerator?: string; action?: string }> } | undefined
  if (!hotkeys?.enabled) return
  const accelerator = buildAccelerator(e)
  const binding = (hotkeys.inApp || []).find((item: { enabled?: boolean; accelerator?: string; action?: string }) => item.enabled && item.accelerator === accelerator)
  if (!binding) return
  if (isEditableTarget(e.target) && binding.action !== 'escape-local-interaction') return

  if (binding.action === 'save-context') {
    e.preventDefault()
    void runHotkeyAction(binding.action)
    return
  }
  if (binding.action === 'toggle-sidebar') {
    e.preventDefault()
    void runHotkeyAction(binding.action)
    return
  }
  if (binding.action === 'escape-local-interaction') {
    e.preventDefault()
    void runHotkeyAction(binding.action)
    return
  }

  const action = binding.action
  if (!action) return
  const targetView = appViewActionMap[action]
  if (targetView) {
    e.preventDefault()
    void runHotkeyAction(action)
  }
}

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
  lockHoverImmediately()
  HideWindow().catch((e) => frontendLogger.error('窗口', '最小化失败', e))
}
const toggleMaximize = () => {
  WindowToggleMaximise()
  window.setTimeout(() => { void syncWindowMaxState() }, 80)
}
const closeWindow = () => {
  frontendLogger.info('窗口', '点击关闭(直接退出 GUI)')
  lockHoverImmediately()
  QuitApp().catch((e) => frontendLogger.error('窗口', '关闭 GUI 失败', e))
}
</script>
