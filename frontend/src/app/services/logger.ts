// 日志服务

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

class Logger {
  private static instance: Logger;
  private logLevel: LogLevel = LogLevel.INFO;
  private isDebugMode: boolean = false;
  private logs: LogEntry[] = [];
  private readonly STORAGE_KEY = 'bs2pro_frontend_logs';
  private readonly MAX_LOCAL_STORAGE_ENTRIES = 100;
  private readonly MAX_MEMORY_ENTRIES = 1000;

  private constructor() {
    // 从环境变量或配置中读取日志级别
    if (typeof window !== 'undefined') {
      const debugMode = (window as any).__DEBUG_MODE__ || false;
      this.isDebugMode = debugMode;
      this.logLevel = debugMode ? LogLevel.DEBUG : LogLevel.INFO;
      
      // 从本地存储加载历史日志
      this.loadFromLocalStorage();
    }
  }

  public static getInstance(): Logger {
    if (!Logger.instance) {
      Logger.instance = new Logger();
    }
    return Logger.instance;
  }

  public setLogLevel(level: LogLevel): void {
    this.logLevel = level;
  }

  public setDebugMode(enabled: boolean): void {
    this.isDebugMode = enabled;
    this.logLevel = enabled ? LogLevel.DEBUG : LogLevel.INFO;
  }

  private shouldLog(level: LogLevel): boolean {
    const levelPriority = {
      [LogLevel.DEBUG]: 0,
      [LogLevel.INFO]: 1,
      [LogLevel.WARN]: 2,
      [LogLevel.ERROR]: 3
    };

    return levelPriority[level] >= levelPriority[this.logLevel];
  }

  private formatMessage(message: string, context?: string): string {
    const timestamp = new Date().toISOString();
    const contextStr = context ? `[${context}] ` : '';
    return `${timestamp} ${contextStr}${message}`;
  }

  private saveToLocalStorage(): void {
    try {
      if (typeof window === 'undefined' || !window.localStorage) {
        return;
      }

      // 只保存最近的日志
      const recentLogs = this.logs.slice(-this.MAX_LOCAL_STORAGE_ENTRIES);
      const serializableLogs = recentLogs.map(log => ({
        ...log,
        timestamp: log.timestamp.toISOString(),
        // 简化数据，避免循环引用
        data: this.serializeData(log.data)
      }));

      localStorage.setItem(this.STORAGE_KEY, JSON.stringify(serializableLogs));
    } catch (error) {
      // 本地存储可能已满或不可用，静默失败
      console.warn('无法保存日志到本地存储:', error);
    }
  }

  private loadFromLocalStorage(): void {
    try {
      if (typeof window === 'undefined' || !window.localStorage) {
        return;
      }

      const stored = localStorage.getItem(this.STORAGE_KEY);
      if (stored) {
        const parsedLogs = JSON.parse(stored);
        const restoredLogs = parsedLogs.map((log: any) => ({
          ...log,
          timestamp: new Date(log.timestamp)
        }));
        
        // 合并到内存中，但不超过限制
        this.logs = [...restoredLogs, ...this.logs].slice(-this.MAX_MEMORY_ENTRIES);
      }
    } catch (error) {
      // 本地存储可能损坏，清除它
      console.warn('无法从本地存储加载日志:', error);
      this.clearLocalStorage();
    }
  }

  private clearLocalStorage(): void {
    try {
      if (typeof window !== 'undefined' && window.localStorage) {
        localStorage.removeItem(this.STORAGE_KEY);
      }
    } catch (error) {
      // 忽略错误
    }
  }

  private serializeData(data: any): any {
    if (data === null || data === undefined) {
      return data;
    }

    if (data instanceof Error) {
      return {
        name: data.name,
        message: data.message,
        stack: data.stack
      };
    }

    if (typeof data === 'object') {
      try {
        // 尝试序列化，如果失败则返回简化版本
        return JSON.parse(JSON.stringify(data));
      } catch {
        return { _type: 'unserializable_object' };
      }
    }

    return data;
  }

  private log(level: LogLevel, message: string, context?: string, data?: any): void {
    if (!this.shouldLog(level)) {
      return;
    }

    const formattedMessage = this.formatMessage(message, context);
    const entry: LogEntry = {
      level,
      message: formattedMessage,
      timestamp: new Date(),
      context,
      data
    };

    // 添加到内存日志
    this.logs.push(entry);

    // 保存到本地存储（异步，避免阻塞）
    setTimeout(() => this.saveToLocalStorage(), 0);

    // 根据级别输出到控制台
    switch (level) {
      case LogLevel.DEBUG:
        if (this.isDebugMode) {
          console.debug(`[DEBUG] ${formattedMessage}`, data || '');
        }
        break;
      case LogLevel.INFO:
        console.info(`[INFO] ${formattedMessage}`, data || '');
        break;
      case LogLevel.WARN:
        console.warn(`[WARN] ${formattedMessage}`, data || '');
        break;
      case LogLevel.ERROR:
        console.error(`[ERROR] ${formattedMessage}`, data || '');
        break;
    }

    // 限制内存中的日志数量
    if (this.logs.length > this.MAX_MEMORY_ENTRIES) {
      this.logs = this.logs.slice(-500);
    }
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

  public getLogs(): LogEntry[] {
    return [...this.logs];
  }

  public getLogsAsText(): string {
    return this.logs.map(log => {
      const level = log.level.toUpperCase();
      const context = log.context ? ` [${log.context}]` : '';
      return `${log.timestamp.toISOString()} ${level}${context}: ${log.message}`;
    }).join('\n');
  }

  public clearLogs(): void {
    this.logs = [];
    this.clearLocalStorage();
  }

  public exportLogs(): void {
    try {
      const logText = this.getLogsAsText();
      const blob = new Blob([logText], { type: 'text/plain' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `bs2pro_logs_${new Date().toISOString().replace(/[:.]/g, '-')}.txt`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (error) {
      console.error('导出日志失败:', error);
    }
  }

  // 兼容旧版console.log的快捷方法
  public logError(error: any, context?: string): void {
    const message = error instanceof Error ? error.message : String(error);
    const stack = error instanceof Error ? error.stack : undefined;
    
    this.error(message, context, { 
      error: error,
      stack: stack
    });
  }
}

// 导出单例实例
export const logger = Logger.getInstance();

// 导出快捷方法
export const log = {
  debug: (message: string, context?: string, data?: any) => logger.debug(message, context, data),
  info: (message: string, context?: string, data?: any) => logger.info(message, context, data),
  warn: (message: string, context?: string, data?: any) => logger.warn(message, context, data),
  error: (message: string, context?: string, data?: any) => logger.error(message, context, data),
  errorObj: (error: any, context?: string) => logger.logError(error, context),
  export: () => logger.exportLogs(),
  clear: () => logger.clearLogs()
};

export default logger;