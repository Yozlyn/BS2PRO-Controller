import { apiService } from './api'

type LogLevel = 'info' | 'debug' | 'warn' | 'error' | 'crash'

class FrontendLogger {
  private debugEnabled = false
  private lastDebugAt = new Map<string, number>()
  private lastInfoAt = new Map<string, number>()

  private readonly levelLabelMap: Record<LogLevel, string> = {
    debug: '调试',
    info: '信息',
    warn: '警告',
    error: '错误',
    crash: '崩溃',
  }

  setDebugEnabled(enabled: boolean) {
    if (this.debugEnabled === enabled) return
    this.debugEnabled = enabled
    this.info('界面日志', `调试日志开关=${enabled ? '开启' : '关闭'}`)
  }

  info(source: string, message: string, meta?: unknown) {
    const now = Date.now()
    const key = `${source}:${message}`
    const last = this.lastInfoAt.get(key) || 0
    if (now - last < 3000) return
    this.lastInfoAt.set(key, now)
    this.emit('info', source, message, this.stringify(meta))
  }

  debug(source: string, message: string, meta?: unknown) {
    if (!this.debugEnabled) return
    const now = Date.now()
    const key = `${source}:${message}`
    const last = this.lastDebugAt.get(key) || 0
    if (now-last < 1500) return
    this.lastDebugAt.set(key, now)
    this.emit('debug', source, message, this.stringify(meta))
  }

  warn(source: string, message: string, meta?: unknown) {
    this.emit('warn', source, message, this.stringify(meta))
  }

  error(source: string, message: string, meta?: unknown) {
    this.emit('error', source, message, this.stringify(meta))
  }

  crash(source: string, message: string, stack?: string) {
    this.emit('crash', source, message, this.trim(stack || '', 3000))
  }

  private emit(level: LogLevel, source: string, message: string, stack: string) {
    const msg = this.trim(message, 1200)
    const levelLabel = this.levelLabelMap[level]
    const output = `[前端日志][${levelLabel}][来源:${source}] ${msg}`
    if (level === 'debug') console.debug(output)
    else if (level === 'info') console.info(output)
    else if (level === 'warn') console.warn(output)
    else console.error(output)
    apiService.reportFrontendLog(level, source, msg, stack)
  }

  private stringify(meta?: unknown): string {
    if (meta == null) return ''
    try {
      if (typeof meta === 'string') return this.trim(meta, 3000)
      return this.trim(JSON.stringify(meta), 3000)
    } catch {
      return '[元数据序列化失败]'
    }
  }

  private trim(s: string, max: number): string {
    return s.length > max ? `${s.slice(0, max)}...（已截断）` : s
  }
}

export const frontendLogger = new FrontendLogger()

