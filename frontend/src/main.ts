import { createApp } from 'vue'
import './styles/globals.css'
import App from './App.vue'
import { frontendLogger } from './services/frontendLogger'

const app = createApp(App)

app.config.errorHandler = (err, _instance, info) => {
  const stack = err instanceof Error ? (err.stack || '') : ''
  frontendLogger.crash('界面错误处理器', `组件信息=${info}，错误内容=${String(err)}`, stack)
}

window.addEventListener('error', (event) => {
  const stack = event.error instanceof Error ? (event.error.stack || '') : ''
  frontendLogger.crash('窗口错误事件', `错误消息=${event.message}，位置=${event.filename}:${event.lineno}:${event.colno}`, stack)
})

window.addEventListener('unhandledrejection', (event) => {
  const reason = event.reason
  if (reason instanceof Error) frontendLogger.crash('未处理的异步拒绝', reason.message, reason.stack || '')
  else frontendLogger.crash('未处理的异步拒绝', String(reason), '')
})

app.mount('#app')
