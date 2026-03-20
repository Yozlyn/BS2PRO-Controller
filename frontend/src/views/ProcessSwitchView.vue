<template>
  <div class="p-8 flex flex-col h-full space-y-6 overflow-y-auto">
    <header>
      <h2 class="text-xl font-bold tracking-tight text-slate-800 dark:text-white">进程联动风扇配置</h2>
      <p class="text-xs text-slate-400 mt-1">开启后自动按前台窗口进程匹配风扇配置，适合游戏或全屏应用场景</p>
    </header>

    <div class="space-y-6 max-w-5xl">
      <div class="p-1 rounded-[2.5rem] border surface-card">
        <SettingItem
          title="启用进程联动"
          desc="按规则扫描进程并自动应用对应风扇配置，启动并设置监控探针程序自启动"
          :active="localEnabled"
          :loading="saving"
          @toggle="handleToggleEnabled"
        />

        <div class="h-px bg-slate-50 dark:bg-white/6 mx-6" />

        <div class="p-6 flex items-center justify-between gap-4">
          <div class="space-y-1">
            <h3 class="text-sm font-bold text-slate-700 dark:text-slate-300">扫描周期</h3>
            <p class="text-xs text-slate-400">每隔多久同步一次前台进程匹配状态</p>
          </div>
          <UnifiedSelect
            :model-value="localInterval"
            :options="intervalOptions"
            :is-dark="isDark"
            width-class="w-40"
            :disabled="saving"
            @update:model-value="handleIntervalChange"
          />
        </div>
      </div>

      <div class="p-6 rounded-[2.5rem] border surface-card space-y-4">
        <div class="flex items-center justify-between">
          <h3 class="text-sm font-bold text-slate-700 dark:text-slate-300">联动规则</h3>
          <button
            @click="addRule"
            :disabled="saving"
            @mousedown="startButtonAnim('add-rule')"
            class="fan-curve-action-btn px-3 py-2 rounded-xl text-[11px] font-bold transition-all cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed"
            :class="[isDark ? 'surface-tile text-slate-300 hover:text-slate-200 hover:bg-slate-700/30' : 'surface-tile text-slate-600 hover:text-slate-700 hover:bg-slate-200/60', clickedButton === 'add-rule' ? 'mode-clicked' : '']"
          >新增规则</button>
        </div>

        <div v-if="localRules.length === 0" class="text-xs text-slate-400 py-4">暂无规则，点击右上角新增规则</div>

        <div v-for="(rule, idx) in localRules" :key="idx" class="rounded-2xl border border-slate-100/60 dark:border-white/10 p-4 space-y-3">
          <div class="grid grid-cols-12 gap-3 items-center">
            <label class="col-span-2 text-xs font-bold text-slate-500">进程名</label>
            <UnifiedSelect
              :model-value="rule.processName || ''"
              :options="buildRuleProcessOptions(rule)"
              :is-dark="isDark"
              width-class="col-span-10"
              dropdown-class="max-h-56"
              placeholder="选择运行中的进程"
              :disabled="saving"
              @update:model-value="(value) => { rule.processName = String(value || '').trim() }"
            />
          </div>
          <div class="grid grid-cols-12 gap-3 items-center">
            <label class="col-span-2 text-xs font-bold text-slate-500">配置文件</label>
            <UnifiedSelect
              :model-value="rule.profilePath || ''"
              :options="buildRuleProfileOptions(rule)"
              :is-dark="isDark"
              width-class="col-span-10"
              placeholder="选择已有风扇配置"
              :disabled="saving"
              @update:model-value="(value) => { rule.profilePath = String(value || '').trim() }"
            />
          </div>
          <div class="flex items-center justify-between">
            <button
              @click="toggleRuleEnabled(idx)"
              class="px-3 py-1.5 rounded-lg text-xs font-bold transition-all"
              :class="rule.enabled
                ? (isDark ? 'bg-emerald-500/20 text-emerald-300' : 'bg-emerald-50 text-emerald-600')
                : (isDark ? 'surface-tile text-slate-300' : 'surface-tile text-slate-600')"
            >{{ rule.enabled ? '已启用' : '已禁用' }}</button>

            <button
              @click="removeRule(idx)"
              :disabled="saving"
              class="px-3 py-1.5 rounded-lg text-xs font-bold text-red-500 hover:bg-red-50 dark:hover:bg-red-500/10 transition-all disabled:opacity-40"
            >删除</button>
          </div>
        </div>

        <div class="pt-2 flex justify-end">
          <button
            @click="saveRules"
            :disabled="saving"
            class="fan-curve-action-btn px-5 py-2.5 rounded-xl text-xs font-bold transition-all cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed"
            :class="[isDark ? 'surface-tile text-slate-300 hover:text-slate-200 hover:bg-slate-700/30' : 'surface-tile text-slate-600 hover:text-slate-700 hover:bg-slate-200/60', clickedButton === 'save-rules' ? 'mode-clicked' : '']"
            @mousedown="startButtonAnim('save-rules')"
          >
            {{ saving ? '保存中...' : '保存规则' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import SettingItem from '../components/ui/SettingItem.vue'
import UnifiedSelect from '../components/ui/UnifiedSelect.vue'
import { apiService } from '../services/api'
import { frontendLogger } from '../services/frontendLogger'
import { main, types } from '../../wailsjs/go/models'

interface Props {
  isDark: boolean
  config: types.AppConfig
}

const props = defineProps<Props>()
const emit = defineEmits<{ 'config-change': [config: types.AppConfig] }>()

const saving = ref(false)
const clickedButton = ref<string | null>(null)
const localEnabled = ref(false)
const localInterval = ref(3)
const localRules = ref<types.ProcessFanRule[]>([])
const processOptions = ref<{ value: string; label: string }[]>([])
const profileOptions = ref<{ value: string; label: string }[]>([])
let buttonAnimTimer: ReturnType<typeof setTimeout> | null = null

const intervalOptions = [
  { value: 1, label: '1 秒' },
  { value: 2, label: '2 秒' },
  { value: 3, label: '3 秒' },
  { value: 5, label: '5 秒' },
  { value: 10, label: '10 秒' },
]

watch(
  () => props.config,
  (cfg) => {
    localEnabled.value = !!cfg.processSwitchEnabled
    localInterval.value = cfg.processSwitchInterval > 0 ? cfg.processSwitchInterval : 3
    localRules.value = (cfg.processSwitchRules || []).map((rule) =>
      types.ProcessFanRule.createFrom({
        processName: rule.processName || '',
        profilePath: rule.profilePath || '',
        enabled: rule.enabled !== false,
      })
    )
  },
  { immediate: true, deep: true }
)

function buildNextConfig(patch: Partial<types.AppConfig>) {
  return types.AppConfig.createFrom({ ...props.config, ...patch })
}

function startButtonAnim(key: string) {
  clickedButton.value = key
  if (buttonAnimTimer) clearTimeout(buttonAnimTimer)
  buttonAnimTimer = setTimeout(() => { clickedButton.value = null }, 200)
}

async function updateConfig(patch: Partial<types.AppConfig>) {
  saving.value = true
  try {
    const next = buildNextConfig(patch)
    await apiService.updateConfig(next)
    emit('config-change', next)
  } catch (e) {
    frontendLogger.error('进程联动', '更新配置失败', e)
  } finally {
    saving.value = false
  }
}

function sanitizeRules() {
  return localRules.value
    .map((rule) =>
      types.ProcessFanRule.createFrom({
        processName: rule.processName?.trim(),
        profilePath: rule.profilePath?.trim(),
        enabled: rule.enabled !== false,
      })
    )
    .filter((rule) => !!rule.processName && !!rule.profilePath)
}

async function handleToggleEnabled() {
  localEnabled.value = !localEnabled.value
  await updateConfig({ processSwitchEnabled: localEnabled.value })
}

async function handleIntervalChange(value: string | number) {
  localInterval.value = Number(value) || 3
  await updateConfig({ processSwitchInterval: localInterval.value })
}

function addRule() {
  localRules.value.push(types.ProcessFanRule.createFrom({ processName: '', profilePath: '', enabled: true }))
}

function buildRuleProcessOptions(rule: types.ProcessFanRule) {
  const options = [...processOptions.value]
  const current = (rule.processName || '').trim()
  if (current && !options.some((opt) => opt.value.toLowerCase() === current.toLowerCase())) {
    options.unshift({ value: current, label: `🧩 当前: ${current}` })
  }
  return options
}

function processIcon(name: string) {
  const n = name.toLowerCase()
  if (n.includes('chrome')) return '🌐'
  if (n.includes('msedge') || n.includes('edge')) return '🌀'
  if (n.includes('firefox')) return '🦊'
  if (n.includes('qq')) return '🐧'
  if (n.includes('wechat') || n.includes('weixin')) return '💬'
  if (n.includes('code') || n.includes('devenv') || n.includes('idea')) return '🧑‍💻'
  if (n.includes('explorer')) return '📁'
  if (n.includes('taskmgr')) return '📊'
  if (n.includes('game') || n.includes('steam')) return '🎮'
  return '🧩'
}

function processPriority(name: string) {
  const n = name.toLowerCase()
  if (n.includes('chrome') || n.includes('msedge') || n.includes('firefox')) return 100
  if (n.includes('qq') || n.includes('wechat') || n.includes('weixin')) return 95
  if (n.includes('code') || n.includes('idea') || n.includes('devenv')) return 90
  if (n.includes('explorer') || n.includes('taskmgr')) return 85
  if (n.includes('steam') || n.includes('game')) return 80
  if (n.includes('svchost') || n.includes('service') || n.includes('host')) return 10
  return 50
}

async function loadRuleProcessOptions() {
  try {
    const processes = await apiService.listRunningProcessNames()
    const grouped = new Map<string, { value: string; label: string; count: number }>()
    for (const rawName of processes || []) {
      const value = String(rawName || '').trim()
      if (!value) continue
      const key = value.toLowerCase()
      const current = grouped.get(key)
      if (current) {
        current.count += 1
        continue
      }
      grouped.set(key, {
        value,
        label: value.replace(/\.exe$/i, ''),
        count: 1,
      })
    }

    processOptions.value = Array.from(grouped.values())
      .sort((a, b) => {
        const priorityDiff = processPriority(b.value) - processPriority(a.value)
        if (priorityDiff !== 0) return priorityDiff
        if (b.count !== a.count) return b.count - a.count
        return a.value.localeCompare(b.value, 'zh-CN')
      })
      .map((item) => ({
        value: item.value,
        label: `${processIcon(item.value)} ${item.label} (${item.count})`,
      }))
  } catch (e) {
    frontendLogger.error('进程联动', '加载进程列表失败', e)
    processOptions.value = []
  }
}

function buildRuleProfileOptions(rule: types.ProcessFanRule) {
  const options = [...profileOptions.value]
  const current = (rule.profilePath || '').trim()
  if (current && !options.some((opt) => opt.value === current)) {
    options.unshift({ value: current, label: `当前: ${current}` })
  }
  return options
}

async function loadRuleProfileOptions() {
  try {
    const profiles = await apiService.getFanCurveProfileConfigs()
    profileOptions.value = (profiles || [])
      .map((profile: main.FanCurveProfileConfig) => ({
        name: String(profile.name || '').trim() || '未命名配置',
        path: String(profile.profilePath || '').trim(),
      }))
      .filter((item) => !!item.path)
      .map((item) => ({
        value: item.path,
        label: `${item.name} (${item.path})`,
      }))
  } catch (e) {
    frontendLogger.error('进程联动', '加载风扇配置列表失败', e)
    profileOptions.value = []
  }
}

onMounted(() => {
  void loadRuleProcessOptions()
  void loadRuleProfileOptions()
})

function removeRule(index: number) {
  localRules.value.splice(index, 1)
}

function toggleRuleEnabled(index: number) {
  const rule = localRules.value[index]
  if (!rule) return
  localRules.value[index] = types.ProcessFanRule.createFrom({ ...rule, enabled: !rule.enabled })
}

async function saveRules() {
  await updateConfig({ processSwitchRules: sanitizeRules() as any })
}

</script>

