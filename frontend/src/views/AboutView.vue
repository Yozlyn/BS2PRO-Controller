<template>
  <div class="flex-1 flex flex-col overflow-y-auto">
    <header class="px-8 pt-8 pb-4 shrink-0">
      <h2 class="text-xl font-bold tracking-tight text-slate-800 dark:text-white">关于软件</h2>
    </header>

    <div class="px-8 pb-8 space-y-5">
      <!-- 应用身份卡片 -->
      <div class="rounded-[2.5rem] border p-8 relative overflow-hidden select-none surface-card">
        <div class="absolute -right-10 -top-10 w-48 h-48 rounded-full blur-[60px] opacity-20 bg-blue-500 pointer-events-none" />
        <div class="absolute -left-6 -bottom-6 w-32 h-32 rounded-full blur-[40px] opacity-10 bg-blue-400 pointer-events-none" />

        <div class="relative z-10 flex items-center gap-8">
          <div class="shrink-0 cursor-default"
               @mouseenter="fanSpeed = '1.5s'"
               @mouseleave="fanSpeed = '10s'">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"
                 class="w-20 h-20 text-blue-500"
                 :style="{ animation: `fan-spin ${fanSpeed} linear infinite` }">
              <polygon points="12 2 20.66 7 20.66 17 12 22 3.34 17 3.34 7" stroke-opacity="0.35" />
              <circle cx="12" cy="12" r="3" />
              <path d="M12 8L12 2M16 12L22 12M12 16L12 22M8 12L2 12" />
              <path d="M14.8 9.2L18.5 5.5M14.8 14.8L18.5 18.5M9.2 14.8L5.5 18.5M9.2 9.2L5.5 5.5" stroke-opacity="0.35"/>
            </svg>
          </div>
          <div class="space-y-2">
            <h3 class="text-2xl font-black tracking-widest text-slate-900 dark:text-white">
              BS2PRO <span class="text-blue-500">CONTROLLER</span>
            </h3>
            <span class="inline-block text-sm font-medium text-slate-600 dark:text-slate-400
                         bg-slate-100 dark:bg-white/8 px-4 py-1.5 rounded-full
                         border border-slate-200 dark:border-white/10">
              版本 {{ appVersion || '加载中...' }}
            </span>
            <p class="text-xs text-slate-400 dark:text-slate-500 pt-1">
              飞智空间站的 BS2 / BS2PRO 替代控制器
            </p>
          </div>
        </div>
      </div>

      <!-- 开发者 + 关于链接 两栏 -->
      <div class="grid grid-cols-2 gap-5">
        <!-- 开发者 -->
        <div class="rounded-[2rem] border p-6 flex flex-col gap-4 surface-card">
          <p class="text-[10px] font-black text-slate-400 uppercase tracking-widest">Developer</p>
          <div class="flex items-center gap-3 flex-1">
            <img src="https://q1.qlogo.cn/g?b=qq&nk=507249007&s=640"
                 alt="TIANLI"
                 class="w-12 h-12 rounded-2xl border-2 border-white dark:border-white/20 shadow-md shrink-0"
                 @error="(e) => (e.target as HTMLImageElement).style.display='none'" />
            <div>
              <div class="font-black text-sm text-slate-800 dark:text-slate-100 tracking-widest">TIANLI</div>
              <button @click="openUrl('mailto:wutianli@tianli0.top')"
                      class="text-xs text-blue-500 hover:text-blue-600 dark:hover:text-blue-300 transition-colors mt-0.5 block">
                wutianli@tianli0.top
              </button>
            </div>
          </div>
        </div>

        <!-- 链接 -->
        <div class="rounded-[2rem] border p-6 flex flex-col gap-4 surface-card">
          <p class="text-[10px] font-black text-slate-400 uppercase tracking-widest">Links</p>
          <div class="space-y-3 flex-1">
            <button @click="openUrl('https://blog.tianli0.top/pages/bs2pro')"
                    class="flex items-center gap-2.5 text-sm font-bold text-slate-700 dark:text-slate-300 hover:text-blue-500 dark:hover:text-blue-400 transition-colors w-full text-left group">
              <div class="w-7 h-7 rounded-xl flex items-center justify-center bg-blue-50 dark:bg-blue-500/10 group-hover:bg-blue-100 dark:group-hover:bg-blue-500/20 transition-colors shrink-0">
                <svg class="w-3.5 h-3.5 text-blue-500" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <circle cx="12" cy="12" r="10"/><path d="M12 8v4l3 3"/>
                </svg>
              </div>
              <span>更新日志</span>
            </button>
            <button @click="openUrl('https://github.com/TIANLI0/BS2PRO-Controller')"
                    class="flex items-center gap-2.5 text-sm font-bold text-slate-700 dark:text-slate-300 hover:text-blue-500 dark:hover:text-blue-400 transition-colors w-full text-left group">
              <div class="w-7 h-7 rounded-xl flex items-center justify-center bg-slate-100 dark:bg-white/8 group-hover:bg-slate-200 dark:group-hover:bg-white/12 transition-colors shrink-0">
                <svg class="w-3.5 h-3.5 text-slate-600 dark:text-slate-300" viewBox="0 0 24 24" fill="currentColor">
                  <path d="M12 0C5.374 0 0 5.373 0 12c0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23A11.509 11.509 0 0112 5.803c1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576C20.566 21.797 24 17.3 24 12c0-6.627-5.373-12-12-12z"/>
                </svg>
              </div>
              <span>GitHub仓库</span>
            </button>
          </div>
        </div>

      </div>

      <!-- 版权/许可 - 移到页面底部 -->
      <div class="mt-5">
        <div class="rounded-[2rem] border p-6 surface-card">
          <div class="flex items-center gap-3 mb-3">
            <div class="w-8 h-8 rounded-xl flex items-center justify-center bg-emerald-50 dark:bg-emerald-500/10 shrink-0">
              <svg class="w-4 h-4 text-emerald-500" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" stroke-linecap="round" stroke-linejoin="round"/>
              </svg>
            </div>
            <div>
              <h4 class="text-sm font-bold text-slate-700 dark:text-slate-300">MIT License</h4>
              <p class="text-xs text-slate-400 dark:text-slate-500">开源许可协议</p>
            </div>
          </div>
          <div class="space-y-2 text-xs text-slate-500 dark:text-slate-400 pl-11">
            <p>© 2026 TIANLI. All rights reserved.</p>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
import { apiService } from '../services/api'

interface Props { isDark: boolean }
defineProps<Props>()

const appVersion = ref('')
const fanSpeed   = ref('10s')

onMounted(async () => {
  try { appVersion.value = await apiService.getAppVersion() } catch {}
})

function openUrl(url: string) {
  try { BrowserOpenURL(url) } catch {}
}
</script>
