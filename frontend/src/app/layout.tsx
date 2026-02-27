'use client';

import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import "./globals.css";
import React, { Component, ErrorInfo, ReactNode, useEffect } from "react";
import { LogFrontendError } from "../../wailsjs/go/main/App";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

// 安全上报：如果 Wails 桥接还未就绪就静默失败
function reportError(level: string, source: string, message: string, stack: string) {
  try {
    LogFrontendError(level, source, message, stack).catch(() => {});
  } catch {
    // Wails 未初始化时静默忽略
  }
}

// ---- 全局 JS 错误捕获（非 React 渲染错误）----
function GlobalErrorSetup() {
  useEffect(() => {
    const onError = (e: ErrorEvent) => {
      reportError("error", e.filename ?? "window.onerror", e.message, e.error?.stack ?? "");
    };
    const onUnhandled = (e: PromiseRejectionEvent) => {
      const err = e.reason instanceof Error ? e.reason : new Error(String(e.reason));
      reportError("error", "unhandledrejection", err.message, err.stack ?? "");
    };
    window.addEventListener("error", onError);
    window.addEventListener("unhandledrejection", onUnhandled);
    return () => {
      window.removeEventListener("error", onError);
      window.removeEventListener("unhandledrejection", onUnhandled);
    };
  }, []);
  return null;
}

// ---- React 渲染错误边界 ----
interface ErrorBoundaryState { hasError: boolean; error: Error | null }
class ErrorBoundary extends Component<{ children: ReactNode }, ErrorBoundaryState> {
  constructor(props: { children: ReactNode }) {
    super(props);
    this.state = { hasError: false, error: null };
  }
  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { hasError: true, error };
  }
  componentDidCatch(error: Error, info: ErrorInfo) {
    reportError("crash", "ErrorBoundary", error.message, (error.stack ?? "") + "\n" + info.componentStack);
  }
  render() {
    if (this.state.hasError) {
      return (
        <div style={{ padding: 32, color: "#ef4444", fontFamily: "monospace" }}>
          <h2 style={{ marginBottom: 8 }}>界面发生异常</h2>
          <pre style={{ fontSize: 12, whiteSpace: "pre-wrap", opacity: 0.8 }}>
            {this.state.error?.message}
          </pre>
          <button
            onClick={() => this.setState({ hasError: false, error: null })}
            style={{ marginTop: 16, padding: "6px 16px", cursor: "pointer" }}
          >
            重试
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="zh">
      <body className={`${geistSans.variable} ${geistMono.variable} antialiased`}>
        <GlobalErrorSetup />
        <ErrorBoundary>
          {children}
        </ErrorBoundary>
      </body>
    </html>
  );
}
