import React, { Component, ErrorInfo, ReactNode, useEffect } from 'react'
import ReactDOM from 'react-dom/client'
import './app/globals.css'
import App from './app/page'
import { LogFrontendError } from '../wailsjs/go/main/App'

function reportError(level: string, source: string, message: string, stack: string) {
  try { LogFrontendError(level, source, message, stack).catch(() => {}) } catch {}
}

function GlobalErrorSetup() {
  useEffect(() => {
    const onError = (e: ErrorEvent) =>
      reportError('error', e.filename ?? 'window.onerror', e.message, e.error?.stack ?? '')
    const onUnhandled = (e: PromiseRejectionEvent) => {
      const err = e.reason instanceof Error ? e.reason : new Error(String(e.reason))
      reportError('error', 'unhandledrejection', err.message, err.stack ?? '')
    }
    window.addEventListener('error', onError)
    window.addEventListener('unhandledrejection', onUnhandled)
    return () => {
      window.removeEventListener('error', onError)
      window.removeEventListener('unhandledrejection', onUnhandled)
    }
  }, [])
  return null
}

interface EBState { hasError: boolean; error: Error | null }
class ErrorBoundary extends Component<{ children: ReactNode }, EBState> {
  constructor(props: { children: ReactNode }) {
    super(props)
    this.state = { hasError: false, error: null }
  }
  static getDerivedStateFromError(error: Error) {
    return { hasError: true, error }
  }
  componentDidCatch(error: Error, info: ErrorInfo) {
    reportError('crash', 'ErrorBoundary', error.message, (error.stack ?? '') + '\n' + info.componentStack)
  }
  render() {
    if (this.state.hasError) return (
      <div style={{ padding: 32, color: '#ef4444', fontFamily: 'monospace' }}>
        <h2 style={{ marginBottom: 8 }}>界面发生异常</h2>
        <pre style={{ fontSize: 12, whiteSpace: 'pre-wrap', opacity: 0.8 }}>{this.state.error?.message}</pre>
        <button
          onClick={() => this.setState({ hasError: false, error: null })}
          style={{ marginTop: 16, padding: '6px 16px', cursor: 'pointer' }}
        >重试</button>
      </div>
    )
    return this.props.children
  }
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <GlobalErrorSetup />
    <ErrorBoundary>
      <App />
    </ErrorBoundary>
  </React.StrictMode>
)
