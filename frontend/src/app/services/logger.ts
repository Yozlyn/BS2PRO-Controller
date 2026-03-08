// 日志服务
import { LogFrontendError } from '../../../wailsjs/go/main/App';

export enum LogLevel {
  DEBUG = 'debug',
  INFO = 'info',
  WARN = 'warn',
  ERROR = 'error'
}

export interface LogEntry {
  level: LogLevel;
  message: string;
  timestamp: Date;
  context?: string;
  data?: any;
}

// 待 flush 的缓冲队列项
interface PendingEntry {
  level: string;
  source: string;
  message: string;
  stack: string;
}

class Logger {
  private static instance: Logger;
  private logLevel: LogLevel = LogLevel.INFO;
  private isDebugMode: boolean = false;

  // 后端不可用时的缓冲队列
  private pendingQueue: PendingEntry[] = [];
  private readonly MAX_PENDING = 200;
  private backendAvailable: boolean = true;
  private flushTimer: ReturnType<typeof setTimeout> | null = null;

  private constructor() {
    if (typeof window !== 'undefined') {
      const debugMode = (window as any).__DEBUG_MODE__ || false;
      this.isDebugMode = debugMode;
      this.logLevel = debugMode ? LogLevel.DEBUG : LogLevel.INFO;
    }
  }

  public static getInstance(): Logger {
    if (!Logger.instance) {
      Logger.instance = new Logger();
    }
    return Logger.instance;
  }

  public setDebugMode(enabled: boolean): void {
    this.isDebugMode = enabled;
    this.logLevel = enabled ? LogLevel.DEBUG : LogLevel.INFO;
  }

  // 通知logger后端已恢复，立即flush缓冲队列
  public notifyBackendAvailable(): void {
    this.backendAvailable = true;
    this.flushPending();
  }

  private shouldLog(level: LogLevel): boolean {
    const priority: Record<LogLevel, number> = {
      [LogLevel.DEBUG]: 0,
      [LogLevel.INFO]: 1,
      [LogLevel.WARN]: 2,
      [LogLevel.ERROR]: 3,
    };
    return priority[level] >= priority[this.logLevel];
  }

  private serializeData(data: any): string {
    if (data === null || data === undefined) return '';
    if (data instanceof Error) return `${data.name}: ${data.message}`;
    if (typeof data === 'object') {
      try {
        return JSON.stringify(data);
      } catch {
        return '[unserializable]';
      }
    }
    return String(data);
  }

  private sendToBackend(entry: PendingEntry): void {
    LogFrontendError(entry.level, entry.source, entry.message, entry.stack)
      .then(() => {
        // 发送成功，标记后端可用并尝试flush
        if (!this.backendAvailable) {
          this.backendAvailable = true;
          this.flushPending();
        }
      })
      .catch(() => {
        // 后端不可用，进入缓冲队列
        this.backendAvailable = false;
        if (this.pendingQueue.length < this.MAX_PENDING) {
          this.pendingQueue.push(entry);
        }
        this.scheduleFlush();
      });
  }

  private flushPending(): void {
    if (this.pendingQueue.length === 0) return;
    const batch = this.pendingQueue.splice(0);
    for (const entry of batch) {
      LogFrontendError(entry.level, entry.source, entry.message, entry.stack)
        .catch(() => {
          // 仍然失败，重新入队
          if (this.pendingQueue.length < this.MAX_PENDING) {
            this.pendingQueue.unshift(entry);
          }
          this.backendAvailable = false;
        });
    }
  }

  private scheduleFlush(): void {
    if (this.flushTimer !== null) return;
    this.flushTimer = setTimeout(() => {
      this.flushTimer = null;
      if (this.pendingQueue.length > 0) {
        this.flushPending();
        if (this.pendingQueue.length > 0) {
          this.scheduleFlush(); // 仍有未发出的，继续重试
        }
      }
    }, 5000);
  }

  private log(level: LogLevel, message: string, context?: string, data?: any): void {
    if (!this.shouldLog(level)) return;

    const source = context ?? 'frontend';
    const dataStr = data ? ` | ${this.serializeData(data)}` : '';
    const stack = data instanceof Error ? (data.stack ?? '') : '';

    // 控制台输出
    const consoleMsg = `[${source}] ${message}${dataStr}`;
    switch (level) {
      case LogLevel.DEBUG:
        if (this.isDebugMode) console.debug(`[DEBUG] ${consoleMsg}`);
        break;
      case LogLevel.INFO:
        console.info(`[INFO] ${consoleMsg}`);
        break;
      case LogLevel.WARN:
        console.warn(`[WARN] ${consoleMsg}`);
        break;
      case LogLevel.ERROR:
        console.error(`[ERROR] ${consoleMsg}`);
        break;
    }

    // 上报后端（带缓冲，后端不在时不丢失）
    this.sendToBackend({
      level,
      source,
      message: data ? `${message}${dataStr}` : message,
      stack,
    });
  }

  public debug(message: string, context?: string, data?: any): void {
    this.log(LogLevel.DEBUG, message, context, data);
  }

  public info(message: string, context?: string, data?: any): void {
    this.log(LogLevel.INFO, message, context, data);
  }

  public warn(message: string, context?: string, data?: any): void {
    this.log(LogLevel.WARN, message, context, data);
  }

  public error(message: string, context?: string, data?: any): void {
    this.log(LogLevel.ERROR, message, context, data);
  }
}

// 导出单例实例
export const logger = Logger.getInstance();

// 导出快捷方法
export const log = {
  debug: (message: string, context?: string, data?: any) => logger.debug(message, context, data),
  info:  (message: string, context?: string, data?: any) => logger.info(message, context, data),
  warn:  (message: string, context?: string, data?: any) => logger.warn(message, context, data),
  error: (message: string, context?: string, data?: any) => logger.error(message, context, data),
};

export default logger;