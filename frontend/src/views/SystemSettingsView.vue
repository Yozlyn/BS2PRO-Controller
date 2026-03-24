<template>
  <div class="p-8 flex flex-col h-full space-y-6 overflow-y-auto">
    <InlineAlertDialog
      :visible="alertDialog.visible"
      :title="alertDialog.title"
      :message="alertDialog.message"
      :is-dark="isDark"
      @close="closeAlertDialog"
    />
    <header>
      <h2 class="text-xl font-bold tracking-tight text-slate-800 dark:text-white">系统首选项</h2>
    </header>

    <div class="space-y-6 max-w-4xl">
      <!-- 基础设置 -->
      <div class="p-1 rounded-[2.5rem] border surface-card">
        <SettingItem title="控制台开机自启动" desc="随系统启动 BS2PRO 控制台，保持后台静默运行"
                      :active="config.windowsAutoStart" @toggle="handleWindowsAutoStart"
                      :loading="loading.windowsAutoStart" />
        <div class="h-px bg-slate-50 dark:bg-white/6 mx-6" />
        <SettingItem title="Monitor 开机自启动" desc="随系统启动后台 Monitor，提供托盘、通知与快捷键常驻能力"
                     :active="config.monitorAutoStart ?? true" @toggle="handleMonitorAutoStart"
                     :loading="loading.monitorAutoStart" />
        <div class="h-px bg-slate-50 dark:bg-white/6 mx-6" />
        <SettingItem title="允许系统通知" desc="在后台运行或窗口未置前时展示设备与服务状态提醒"
                     :active="config.notificationsEnabled ?? true" @toggle="handleNotificationsEnabled" />
        <div class="h-px bg-slate-50 dark:bg-white/6 mx-6" />
        <SettingItem title="跟随系统主题" desc="启用后界面亮暗外观将自动跟随 Windows 系统主题切换"
                     :active="followSystemTheme" @toggle="handleFollowSystemTheme" />
        <div class="h-px bg-slate-50 dark:bg-white/6 mx-6" />
        <SettingItem title="挡位指示灯" desc="控制硬件设备上的挡位物理指示灯开关"
                     :active="config.gearLight" @toggle="handleGearLight"
                     :disabled="!isConnected" :loading="loading.gearLight" />
        <div class="h-px bg-slate-50 dark:bg-white/6 mx-6" />
        <SettingItem title="通电自启动" desc="硬件设备上电后自动进入工作状态"
                     :active="config.powerOnStart" @toggle="handlePowerOnStart"
                     :disabled="!isConnected" :loading="loading.powerOnStart" />
      </div>

      <!-- 连接设置 -->
      <div class="p-1 rounded-[2.5rem] border surface-card">
        <SettingItem title="断连保持配置" desc="当设备意外断开并重连后，继续沿用当前 APP 设置的参数"
                     :active="config.ignoreDeviceOnReconnect ?? true" @toggle="handleIgnoreReconnect" />
        <div class="h-px bg-slate-50 dark:bg-white/6 mx-6" />
        <div class="p-6 flex items-center justify-between">
          <div class="space-y-1">
            <h3 class="text-sm font-bold text-slate-700 dark:text-slate-300">智能启停策略</h3>
            <p class="text-xs text-slate-400">检测到负载降低时的风扇停转逻辑</p>
          </div>
          <UnifiedSelect :model-value="config.smartStartStop || 'delayed'"
                         @update:model-value="handleSmartStartStop(String($event))"
                         :options="smartStartStopOptions"
                         :disabled="!isConnected"
                         :is-dark="isDark"
                         width-class="w-52" />
        </div>
      </div>

      <div class="p-1 rounded-[2.5rem] border surface-card">
        <SettingItem title="启用快捷键" desc="统一控制全局快捷键与应用内快捷键总开关"
                     :active="hotkeysConfig.enabled ?? true" @toggle="handleHotkeysEnabled" />
        <div class="h-px bg-slate-50 dark:bg-white/6 mx-6" />
        <div class="px-6 py-5 space-y-4">
          <div v-if="conflictMessages.length" class="px-4 py-3 rounded-xl border border-red-200 bg-red-50 text-red-500 text-xs dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300">
            检测到快捷键冲突，请调整红色项后再保存。
            <div class="mt-2 space-y-1">
              <div v-for="msg in conflictMessages" :key="msg">{{ msg }}</div>
            </div>
          </div>
          <div>
            <div class="text-sm font-bold text-slate-700 dark:text-slate-300">全局快捷键</div>
            <div class="mt-3 space-y-2 text-xs text-slate-500 dark:text-slate-400">
              <div v-for="(item, index) in defaultGlobalBindings" :key="`${item.action}-${index}`" class="flex items-center justify-between gap-4 rounded-xl px-3 py-3 surface-tile">
                <div>
                  <div class="font-semibold text-slate-700 dark:text-slate-200">{{ item.description }}</div>
                  <div class="text-[11px] text-slate-400">{{ item.category || '全局' }}</div>
                </div>
                <div class="flex items-center gap-2">
                  <input
                    :value="editingPreview[`global-${index}`] || item.accelerator"
                    readonly
                    @focus="setEditingAction(`global-${index}`)"
                    @blur="clearEditingAction(`global-${index}`)"
                    @keydown.prevent="captureHotkey($event, item, 'global', index)"
                    :class="getHotkeyInputClass(`global-${index}`, 'w-36')"
                  />
                  <button @click="resetBinding(item, 'global', index)" class="px-3 py-2 rounded-xl text-xs surface-tile">重置</button>
                </div>
              </div>
            </div>
          </div>
          <div>
            <div class="flex items-center justify-between gap-3">
              <div>
                <div class="text-sm font-bold text-slate-700 dark:text-slate-300">自定义全局控制</div>
                <div class="text-[11px] text-slate-400 mt-1">在默认全局配置外，新增自定义功能与快捷键。</div>
              </div>
              <button @click="addCustomGlobalBinding" :disabled="customGlobalBindings.length >= customGlobalActionPresets.length"
                      class="px-3 py-2 rounded-xl text-xs surface-tile disabled:opacity-40 disabled:cursor-not-allowed">新增自定义</button>
            </div>
            <div v-if="customGlobalUiError" class="mt-3 px-4 py-3 rounded-xl border border-red-200 bg-red-50 text-red-500 text-xs dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300">
              {{ customGlobalUiError }}
            </div>
            <div v-if="customGlobalBindings.length >= customGlobalActionPresets.length" class="mt-2 text-[11px] text-slate-400">
              当前最多支持 {{ customGlobalActionPresets.length }} 个自定义快捷键位。
            </div>
            <div class="mt-3 space-y-2 text-xs text-slate-500 dark:text-slate-400">
              <div v-for="(item, customIndex) in customGlobalBindings" :key="`${item.action}-${defaultHotkeys.global.length + customIndex}`" class="flex items-center justify-between gap-4 rounded-xl px-3 py-3 surface-tile">
                <div>
                  <div class="font-semibold text-slate-700 dark:text-slate-200">{{ item.description || '自定义全局控制' }}</div>
                  <div class="text-[11px] text-slate-400">{{ item.category || '自定义' }}</div>
                </div>
                <div class="flex items-center gap-2">
                  <UnifiedSelect
                    :model-value="item.action"
                    @update:model-value="handleGlobalActionChange(defaultHotkeys.global.length + customIndex, String($event))"
                    :options="globalActionOptions"
                    :is-dark="isDark"
                    width-class="w-44"
                  />
                  <input
                    :value="editingPreview[`global-${defaultHotkeys.global.length + customIndex}`] || item.accelerator"
                    readonly
                    @focus="setEditingAction(`global-${defaultHotkeys.global.length + customIndex}`)"
                    @blur="clearEditingAction(`global-${defaultHotkeys.global.length + customIndex}`)"
                    @keydown.prevent="captureHotkey($event, item, 'global', defaultHotkeys.global.length + customIndex)"
                    :class="getHotkeyInputClass(`global-${defaultHotkeys.global.length + customIndex}`, 'w-36')"
                  />
                  <button @click="resetBinding(item, 'global', defaultHotkeys.global.length + customIndex)" class="px-3 py-2 rounded-xl text-xs surface-tile">重置</button>
                  <button @click="removeCustomGlobalBinding(customIndex)" class="px-3 py-2 rounded-xl text-xs surface-tile text-red-500 dark:text-red-300">删除</button>
                </div>
              </div>
            </div>
          </div>
          <div>
            <div class="text-sm font-bold text-slate-700 dark:text-slate-300">应用内快捷键</div>
            <div class="mt-3 grid grid-cols-1 md:grid-cols-2 gap-2 text-xs text-slate-500 dark:text-slate-400">
              <div v-for="(item, index) in hotkeysConfig.inApp || []" :key="`${item.action}-${index}`" class="flex items-center justify-between gap-4 rounded-xl px-3 py-3 surface-tile">
                <div>
                  <div class="font-semibold text-slate-700 dark:text-slate-200">{{ item.description }}</div>
                  <div class="text-[11px] text-slate-400">{{ item.category || '应用内' }}</div>
                </div>
                <div class="flex items-center gap-2">
                  <input
                    :value="editingPreview[`app-${index}`] || item.accelerator"
                    readonly
                    @focus="setEditingAction(`app-${index}`)"
                    @blur="clearEditingAction(`app-${index}`)"
                    @keydown.prevent="captureHotkey($event, item, 'app')"
                    :class="getHotkeyInputClass(`app-${index}`, 'w-32')"
                  />
                  <button @click="resetBinding(item, 'app', index)" class="px-3 py-2 rounded-xl text-xs surface-tile">重置</button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- 反馈工具 -->
      <div class="p-1 rounded-[2.5rem] border surface-card">
        <SettingItem title="调试模式" desc="启用后系统将记录更详细的运行日志用于故障排查"
                     :active="config.debugMode" @toggle="handleDebugMode" />
        <div class="h-px bg-slate-50 dark:bg-white/6 mx-6" />
        <div class="p-6 flex items-center justify-between gap-4">
          <div class="space-y-1">
            <h3 class="text-sm font-bold text-slate-700 dark:text-slate-300">反馈日志导出</h3>
            <p class="text-xs text-slate-400 setting-desc">一键打包最近7天的运行日志，便于提交问题反馈</p>
          </div>
          <button @click="exportRecentLogs" @mousedown="startButtonAnim('export-logs')" :disabled="exportLogsLoading"
                  class="fan-curve-action-btn flex items-center space-x-2 px-5 py-2.5 border rounded-xl text-xs font-bold transition-all cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed"
                  :class="[isDark ? 'surface-tile border-white/10 text-slate-300 hover:text-slate-200 hover:bg-slate-700/30' : 'surface-tile border-slate-200/70 text-slate-600 hover:text-slate-700 hover:bg-slate-200/60', clickedButton === 'export-logs' ? 'mode-clicked' : '']">
            <span>{{ exportLogsLoading ? '导出中...' : '导出日志包' }}</span>
          </button>
        </div>
      </div>

      <!-- 版本信息 -->
      <div class="pt-4 flex items-center justify-between">
        <div class="flex items-center space-x-2 text-[10px] text-slate-400 font-bold uppercase tracking-wider">
          <Terminal :size="12" />
          <span>App Version: {{ appVersion || '加载中...' }}</span>
        </div>
      </div>

      <!-- 调试信息 -->
      <pre v-if="debugInfo" class="p-4 rounded-2xl bg-slate-900 text-green-400 text-xs overflow-auto max-h-60 border border-slate-800">{{ JSON.stringify(debugInfo, null, 2) }}</pre>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted } from 'vue'
import { Terminal } from 'lucide-vue-next'
import InlineAlertDialog from '../components/ui/InlineAlertDialog.vue'
import SettingItem from '../components/ui/SettingItem.vue'
import UnifiedSelect from '../components/ui/UnifiedSelect.vue'
import { apiService } from '../services/api'
import { frontendLogger } from '../services/frontendLogger'
import { types } from '../../wailsjs/go/models'

interface Props {
  isDark: boolean
  isConnected: boolean
  followSystemTheme: boolean
  config: types.AppConfig
}
const props = defineProps<Props>()
const emit = defineEmits<{ 'config-change': [config: types.AppConfig], 'follow-system-theme-change': [enabled: boolean] }>()

const appVersion = ref('')
const debugInfo = ref<any>(null)
const exportLogsLoading = ref(false)
const clickedButton = ref<string | null>(null)
const editingAction = ref<string | null>(null)
const editingPreview = ref<Record<string, string>>({})
const customGlobalUiError = ref('')
const alertDialog = ref({ visible: false, title: '', message: '' })
let buttonAnimTimer: ReturnType<typeof setTimeout> | null = null
const loading = ref({ gearLight: false, powerOnStart: false, windowsAutoStart: false, monitorAutoStart: false })
const hotkeysConfig = computed(() => ((props.config as any).hotkeys ?? { enabled: true, global: [], inApp: [] }) as { enabled?: boolean; global?: Array<any>; inApp?: Array<any> })
const defaultHotkeys = {
	enabled: true,
	global: [
		{ action: 'show-main-window', accelerator: 'Ctrl+Alt+B', description: '显示或隐藏主窗口', category: '窗口' },
		{ action: 'toggle-auto-control', accelerator: 'Ctrl+Alt+A', description: '切换智能变频', category: '设备' },
		{ action: 'toggle-process-switch', accelerator: 'Ctrl+Alt+P', description: '切换进程联动', category: '设备' },
		{ action: 'cycle-rgb-mode', accelerator: 'Ctrl+Alt+R', description: '切换 RGB 灯光模式', category: 'RGB' },
	],
	inApp: [
		{ action: 'save-context', accelerator: 'Ctrl+S', description: '保存当前页面', category: '通用' },
		{ action: 'escape-local-interaction', accelerator: 'Escape', description: '取消页面交互', category: '通用' },
		{ action: 'toggle-sidebar', accelerator: 'Ctrl+B', description: '切换侧边栏', category: '通用' },
		{ action: 'navigate-dashboard', accelerator: 'Alt+1', description: '切换到设备概览', category: '导航' },
		{ action: 'navigate-fan-curve', accelerator: 'Alt+2', description: '切换到风扇曲线', category: '导航' },
		{ action: 'navigate-device-params', accelerator: 'Alt+3', description: '切换到设备参数', category: '导航' },
		{ action: 'navigate-rgb-light', accelerator: 'Alt+4', description: '切换到 RGB 灯效', category: '导航' },
		{ action: 'navigate-process-switch', accelerator: 'Alt+5', description: '切换到进程联动', category: '导航' },
		{ action: 'navigate-system-settings', accelerator: 'Alt+6', description: '切换到系统设置', category: '导航' },
		{ action: 'navigate-about', accelerator: 'Alt+7', description: '切换到关于软件', category: '导航' },
	],
} as const
const customGlobalActionPresets = [
	{ action: 'device-toggle-offset', accelerator: 'Ctrl+Alt+O', description: '切换自动曲线偏移', category: '设备参数' },
	{ action: 'device-gear-quiet', accelerator: 'Ctrl+Alt+1', description: '切换到静音挡', category: '设备参数' },
	{ action: 'device-gear-standard', accelerator: 'Ctrl+Alt+2', description: '切换到标准挡', category: '设备参数' },
	{ action: 'device-gear-strong', accelerator: 'Ctrl+Alt+3', description: '切换到强劲挡', category: '设备参数' },
	{ action: 'device-gear-overclock', accelerator: 'Ctrl+Alt+4', description: '切换到超频挡', category: '设备参数' },
	{ action: 'device-custom-speed-toggle', accelerator: 'Ctrl+Alt+C', description: '切换自定义转速', category: '设备参数' },
	{ action: 'device-custom-speed-apply', accelerator: 'Ctrl+Alt+V', description: '应用当前自定义转速', category: '设备参数' },
] as const
const conflictMap = computed(() => {
	const map = new Map<string, boolean>()
	const scopes = [
		...(hotkeysConfig.value.global || []).map((item: any, index: number) => ({ ...item, scope: 'global', rowKey: `global-${index}` })),
		...(hotkeysConfig.value.inApp || []).map((item: any, index: number) => ({ ...item, scope: 'app', rowKey: `app-${index}` })),
	]
	const grouped = new Map<string, any[]>()
	for (const item of scopes) {
		if (!item.enabled || !item.accelerator) continue
		const key = item.scope === 'global' ? `global:${item.accelerator}` : `app:${item.accelerator}`
		if (!grouped.has(key)) grouped.set(key, [])
		grouped.get(key)?.push(item)
	}
	for (const items of grouped.values()) {
		if (items.length < 2) continue
		for (const item of items) map.set(item.rowKey, true)
	}
	return map
})
const conflictMessages = computed(() => {
	const messages: string[] = []
	for (const item of (props.config as any).hotkeyConflicts || []) {
		const actions = Array.isArray(item.actions) ? item.actions.join('、') : ''
		const accelerator = item.accelerator || ''
		const message = item.message || '快捷键冲突'
		messages.push(`${accelerator} · ${actions} · ${message}`)
	}
	if (conflictMap.value.size) {
		messages.unshift('当前配置存在重复绑定')
	}
	return messages
})
const defaultGlobalBindings = computed(() => (hotkeysConfig.value.global || []).slice(0, defaultHotkeys.global.length))
const customGlobalBindings = computed(() => (hotkeysConfig.value.global || []).slice(defaultHotkeys.global.length))
const globalActionOptions = customGlobalActionPresets.map((item) => ({
	value: item.action,
	label: item.description,
}))
const smartStartStopOptions = [
  { value: 'off', label: '关闭' },
  { value: 'immediate', label: '即时' },
  { value: 'delayed', label: '延时' },
]

onMounted(async () => {
  try { appVersion.value = await apiService.getAppVersion() } catch {}
})

onUnmounted(() => {
  if (buttonAnimTimer) clearTimeout(buttonAnimTimer)
	void (window as any).go?.main?.App?.SetHotkeyEditMode?.(false)
})

function startButtonAnim(key: string) {
  clickedButton.value = key
  if (buttonAnimTimer) clearTimeout(buttonAnimTimer)
  buttonAnimTimer = setTimeout(() => { clickedButton.value = null }, 200)
}

function showAlertDialog(title: string, message: string) {
  alertDialog.value = { visible: true, title, message }
}

function closeAlertDialog() {
  alertDialog.value.visible = false
}

function handleFollowSystemTheme() {
  emit('follow-system-theme-change', !props.followSystemTheme)
}

async function handleGearLight() {
  if (!props.isConnected) return
  loading.value.gearLight = true
  try {
    const ok = await apiService.setGearLight(!props.config.gearLight)
    if (ok) emit('config-change', types.AppConfig.createFrom({ ...props.config, gearLight: !props.config.gearLight }))
  } catch (e) { frontendLogger.error('系统设置', '设置挡位指示灯失败', e) } finally { loading.value.gearLight = false }
}

async function handlePowerOnStart() {
  if (!props.isConnected) return
  loading.value.powerOnStart = true
  try {
    const ok = await apiService.setPowerOnStart(!props.config.powerOnStart)
    if (ok) emit('config-change', types.AppConfig.createFrom({ ...props.config, powerOnStart: !props.config.powerOnStart }))
  } catch (e) { frontendLogger.error('系统设置', '设置通电自启动失败', e) } finally { loading.value.powerOnStart = false }
}

async function handleWindowsAutoStart() {
  loading.value.windowsAutoStart = true
  try {
    await apiService.setWindowsAutoStart(!props.config.windowsAutoStart)
    emit('config-change', types.AppConfig.createFrom({ ...props.config, windowsAutoStart: !props.config.windowsAutoStart }))
  } catch (e) { frontendLogger.error('系统设置', '设置系统开机自启动失败', e); showAlertDialog('设置失败', `设置自启动失败：${e}`) }
  finally { loading.value.windowsAutoStart = false }
}

async function handleMonitorAutoStart() {
  loading.value.monitorAutoStart = true
  try {
    const next = !(props.config.monitorAutoStart ?? true)
    await apiService.setMonitorAutoStart(next)
    emit('config-change', types.AppConfig.createFrom({ ...props.config, monitorAutoStart: next }))
  } catch (e) { frontendLogger.error('系统设置', '设置 Monitor 开机自启动失败', e); showAlertDialog('设置失败', `设置 Monitor 自启动失败：${e}`) }
  finally { loading.value.monitorAutoStart = false }
}

async function handleIgnoreReconnect() {
  try {
    const nc = types.AppConfig.createFrom({ ...props.config, ignoreDeviceOnReconnect: !(props.config.ignoreDeviceOnReconnect ?? true) })
    await apiService.updateConfig(nc)
    emit('config-change', nc)
  } catch (e) { frontendLogger.error('系统设置', '设置断连保持配置失败', e) }
}

async function handleNotificationsEnabled() {
  try {
    const nc = types.AppConfig.createFrom({ ...props.config, notificationsEnabled: !(props.config.notificationsEnabled ?? true) })
    await apiService.updateConfig(nc)
    emit('config-change', nc)
  } catch (e) { frontendLogger.error('系统设置', '设置系统通知失败', e) }
}

async function handleHotkeysEnabled() {
  try {
    const current = hotkeysConfig.value
    const nc = types.AppConfig.createFrom({ ...props.config, hotkeys: { ...current, enabled: !current.enabled } })
    await apiService.updateConfig(nc)
    emit('config-change', nc)
  } catch (e) { frontendLogger.error('系统设置', '设置快捷键总开关失败', e) }
}

function normalizeAccelerator(event: KeyboardEvent) {
	const parts: string[] = []
	if (event.ctrlKey) parts.push('Ctrl')
	if (event.altKey) parts.push('Alt')
	if (event.shiftKey) parts.push('Shift')
	if (event.metaKey) parts.push('Meta')
	const key = event.key === 'Escape' ? 'Escape' : event.key.length === 1 ? event.key.toUpperCase() : event.key
	if (['Control', 'Shift', 'Alt', 'Meta'].includes(key)) return parts.join('+')
	parts.push(key)
	return parts.join('+')
}

function setEditingAction(action: string) {
	editingAction.value = action
	editingPreview.value[action] = ''
	void (window as any).go?.main?.App?.SetHotkeyEditMode?.(true)
}

function clearEditingAction(action: string) {
	if (editingAction.value === action) {
		editingAction.value = null
		delete editingPreview.value[action]
		void (window as any).go?.main?.App?.SetHotkeyEditMode?.(false)
	}
}

function getHotkeyInputClass(rowKey: string, widthClass: string) {
	const hasConflict = conflictMap.value.has(rowKey)
	const isEditing = editingAction.value === rowKey
	return [
		widthClass,
		'px-3 py-2 rounded-xl border text-center text-xs outline-none transition-all',
		hasConflict
			? 'bg-red-50 border-red-400 text-red-600 shadow-[0_0_0_3px_rgba(248,113,113,0.12)] dark:bg-red-500/10 dark:border-red-500/50 dark:text-red-300'
			: isEditing
				? 'bg-blue-50 border-blue-400 text-blue-700 shadow-[0_0_0_3px_rgba(96,165,250,0.16)] dark:bg-blue-500/10 dark:border-blue-400/50 dark:text-blue-200'
				: 'bg-slate-50 dark:bg-slate-800 border-slate-200/70 dark:border-white/10 text-slate-700 dark:text-slate-200',
	]
}

function buildNextHotkeys() {
	return {
		...hotkeysConfig.value,
		global: [...(hotkeysConfig.value.global || [])],
		inApp: [...(hotkeysConfig.value.inApp || [])],
	}
}

function ensureGlobalDefaults(next: any) {
	for (let i = 0; i < defaultHotkeys.global.length; i += 1) {
		if (!next.global[i]) {
			next.global[i] = { ...defaultHotkeys.global[i], scope: 'global', enabled: true, editable: true }
		}
	}
}

async function commitHotkeys(next: any) {
	customGlobalUiError.value = ''
	const nc = types.AppConfig.createFrom({ ...props.config, hotkeys: next })
	await apiService.updateConfig(nc)
	emit('config-change', nc)
}

async function updateBinding(target: any, scope: 'global' | 'app', accelerator: string, indexOverride?: number) {
	const next = buildNextHotkeys()
	const list = scope === 'global' ? next.global : next.inApp
	if (scope === 'global') ensureGlobalDefaults(next)
	const index = indexOverride ?? list.findIndex((item: any) => item.action === target.action)
	if (index < 0) return
	list[index] = { ...list[index], accelerator }
	const scopeItems = [
		...(next.global || []).map((item: any) => ({ ...item, scope: 'global' })),
		...(next.inApp || []).map((item: any) => ({ ...item, scope: 'app' })),
	]
	const duplicated = scopeItems.some((item: any, idx: number) => {
		if (!item.enabled || !item.accelerator) return false
		return scopeItems.findIndex((other: any) => other.scope === item.scope && other.accelerator === item.accelerator && other.enabled) !== idx
	})
	if (duplicated) return
	await commitHotkeys(next)
}

async function captureHotkey(event: KeyboardEvent, item: any, scope: 'global' | 'app', index?: number) {
	try {
		const rowKey = scope === 'global' ? `global-${index}` : `app-${index}`
		const accelerator = normalizeAccelerator(event)
		editingPreview.value[rowKey] = accelerator
		if (!accelerator) return
		if (['Ctrl', 'Alt', 'Shift', 'Meta'].includes(accelerator)) return
		await updateBinding(item, scope, accelerator, index)
		delete editingPreview.value[rowKey]
	} catch (e) {
		frontendLogger.error('系统设置', '更新快捷键失败', e)
	}
}

async function handleGlobalActionChange(index: number, nextAction: string) {
	try {
		const preset = customGlobalActionPresets.find((item) => item.action === nextAction)
		if (!preset) return
		const next = buildNextHotkeys()
		ensureGlobalDefaults(next)
		const current = next.global?.[index]
		if (!current) return
		next.global[index] = {
			...current,
			action: preset.action,
			description: preset.description,
			category: preset.category,
		}
		await commitHotkeys(next)
	} catch (e) {
		frontendLogger.error('系统设置', '更新全局快捷键功能失败', e)
		customGlobalUiError.value = `更新全局快捷键功能失败：${e}`
	}
}

async function resetBinding(item: any, scope: 'global' | 'app', index: number) {
	try {
		const defaults = scope === 'global' ? defaultHotkeys.global : defaultHotkeys.inApp
		const fallback = scope === 'global'
			? (index < defaultHotkeys.global.length ? defaultHotkeys.global[index] : customGlobalActionPresets.find((entry: any) => entry.action === item.action))
			: defaults[index] || defaults.find((entry: any) => entry.action === item.action)
		if (!fallback?.accelerator) return
		const next = buildNextHotkeys()
		if (scope === 'global') ensureGlobalDefaults(next)
		const list = scope === 'global' ? next.global : next.inApp
		if (!list[index]) return
		list[index] = {
			...list[index],
			...fallback,
			scope,
			enabled: list[index].enabled,
			editable: list[index].editable,
		}
		await commitHotkeys(next)
	} catch (e) {
		frontendLogger.error('系统设置', '重置快捷键失败', e)
		customGlobalUiError.value = `重置快捷键失败：${e}`
	}
}

async function addCustomGlobalBinding() {
	try {
		const next = buildNextHotkeys()
		ensureGlobalDefaults(next)
		if ((next.global || []).length-defaultHotkeys.global.length >= customGlobalActionPresets.length) {
			customGlobalUiError.value = `当前最多只能添加 ${customGlobalActionPresets.length} 个自定义功能位`
			return
		}
		const usedAccelerators = new Set((next.global || []).filter((item: any) => item.enabled && item.accelerator).map((item: any) => item.accelerator))
		const preset = customGlobalActionPresets.find((item) => !usedAccelerators.has(item.accelerator)) || customGlobalActionPresets[0]
		next.global.push({ ...preset, scope: 'global', enabled: true, editable: true })
		await commitHotkeys(next)
	} catch (e) {
		frontendLogger.error('系统设置', '新增自定义全局快捷键失败', e)
		customGlobalUiError.value = `新增自定义失败：${e}`
	}
}

async function removeCustomGlobalBinding(customIndex: number) {
	try {
		const next = buildNextHotkeys()
		ensureGlobalDefaults(next)
		const customItems = [...(next.global || []).slice(defaultHotkeys.global.length)]
		customItems.splice(customIndex, 1)
		next.global = [...(next.global || []).slice(0, defaultHotkeys.global.length), ...customItems]
		await commitHotkeys(next)
	} catch (e) {
		frontendLogger.error('系统设置', '删除自定义全局快捷键失败', e)
		customGlobalUiError.value = `删除自定义失败：${e}`
	}
}


async function handleSmartStartStop(mode: string) {
  if (!props.isConnected) return
  try {
    const ok = await apiService.setSmartStartStop(mode)
    if (ok) emit('config-change', types.AppConfig.createFrom({ ...props.config, smartStartStop: mode }))
  } catch (e) { frontendLogger.error('系统设置', '设置智能启停策略失败', e) }
}

async function handleDebugMode() {
  try {
    await apiService.setDebugMode(!props.config.debugMode)
    emit('config-change', types.AppConfig.createFrom({ ...props.config, debugMode: !props.config.debugMode }))
  } catch (e) { frontendLogger.error('系统设置', '切换调试模式失败', e) }
}

async function exportRecentLogs() {
  exportLogsLoading.value = true
  try {
    await apiService.exportRecentLogsZip()
  } catch (e) {
    frontendLogger.error('系统设置', '导出日志包失败', e)
    showAlertDialog('导出失败', `导出日志包失败：${e}`)
  } finally {
    exportLogsLoading.value = false
  }
}
</script>
