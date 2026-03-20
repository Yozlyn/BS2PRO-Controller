<template>
  <div class="p-8 flex flex-col h-full space-y-6 overflow-y-auto">
    <header>
      <h2 class="text-xl font-bold tracking-tight text-slate-800 dark:text-white">系统首选项</h2>
    </header>

    <div class="space-y-6 max-w-4xl">
      <!-- 基础设置 -->
      <div class="p-1 rounded-[2.5rem] border surface-card">
        <SettingItem title="Windows 开机自启动" desc="随系统启动 BS2PRO 控制台，保持后台静默运行"
                     :active="config.windowsAutoStart" @toggle="handleWindowsAutoStart"
                     :loading="loading.windowsAutoStart" />
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

      <!-- 调试设置 -->
      <div class="p-1 rounded-[2.5rem] border surface-card">
        <SettingItem title="调试模式" desc="启用后系统将记录更详细的运行日志用于故障排查"
                     :active="config.debugMode" @toggle="handleDebugMode" />
      </div>

      <!-- 反馈工具 -->
      <div class="p-6 rounded-[2.5rem] border surface-card flex items-center justify-between gap-4">
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
import { ref, onMounted, onUnmounted } from 'vue'
import { Terminal } from 'lucide-vue-next'
import SettingItem from '../components/ui/SettingItem.vue'
import UnifiedSelect from '../components/ui/UnifiedSelect.vue'
import { apiService } from '../services/api'
import { frontendLogger } from '../services/frontendLogger'
import { types } from '../../wailsjs/go/models'

interface Props {
  isDark: boolean
  isConnected: boolean
  config: types.AppConfig
}
const props = defineProps<Props>()
const emit = defineEmits<{ 'config-change': [config: types.AppConfig] }>()

const appVersion = ref('')
const debugInfo = ref<any>(null)
const exportLogsLoading = ref(false)
const clickedButton = ref<string | null>(null)
let buttonAnimTimer: ReturnType<typeof setTimeout> | null = null
const loading = ref({ gearLight: false, powerOnStart: false, windowsAutoStart: false })
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
})

function startButtonAnim(key: string) {
  clickedButton.value = key
  if (buttonAnimTimer) clearTimeout(buttonAnimTimer)
  buttonAnimTimer = setTimeout(() => { clickedButton.value = null }, 200)
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
  } catch (e) { frontendLogger.error('系统设置', '设置系统开机自启动失败', e); alert(`设置自启动失败：${e}`) }
  finally { loading.value.windowsAutoStart = false }
}

async function handleIgnoreReconnect() {
  try {
    const nc = types.AppConfig.createFrom({ ...props.config, ignoreDeviceOnReconnect: !(props.config.ignoreDeviceOnReconnect ?? true) })
    await apiService.updateConfig(nc)
    emit('config-change', nc)
  } catch (e) { frontendLogger.error('系统设置', '设置断连保持配置失败', e) }
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
    alert(`导出日志包失败：${e}`)
  } finally {
    exportLogsLoading.value = false
  }
}
</script>
