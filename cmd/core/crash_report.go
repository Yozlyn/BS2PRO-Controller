package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/TIANLI0/BS2PRO-Controller/internal/config"
)

func capturePanic(app *CoreApp, source string, recovered any) {
	stack := debug.Stack()
	logDir := resolveCrashLogDir(app)

	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "创建崩溃日志目录失败: %v\n", err)
		fmt.Fprintf(os.Stderr, "异常来源: %s, 异常: %v\n%s\n", source, recovered, string(stack))
		return
	}

	fileName := fmt.Sprintf("crash_%s.log", time.Now().Format("2006-01-02_15-04-05.000"))
	filePath := filepath.Join(logDir, fileName)

	var builder strings.Builder
	builder.WriteString("=== BS2PRO 核心服务崩溃报告 ===\n")
	fmt.Fprintf(&builder, "时间:      %s\n", time.Now().Format(time.RFC3339Nano))
	fmt.Fprintf(&builder, "来源:      %s\n", source)
	fmt.Fprintf(&builder, "异常:      %v\n", recovered)
	fmt.Fprintf(&builder, "进程ID:    %d\n", os.Getpid())
	fmt.Fprintf(&builder, "启动参数:  %v\n", os.Args)
	builder.WriteString("\n--- 调用栈 ---\n")
	builder.Write(stack)
	builder.WriteString("\n")

	if err := os.WriteFile(filePath, []byte(builder.String()), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "写入崩溃报告失败: %v\n", err)
		fmt.Fprintf(os.Stderr, "异常来源: %s, 异常: %v\n%s\n", source, recovered, string(stack))
		return
	}

	if app != nil {
		app.logError("捕获到异常", "source", source, "error", recovered)
		app.logError("异常调用栈", "source", source, "stack", string(stack))
		if app.logger != nil {
			app.logger.Close()
		}
	}

	fmt.Fprintf(os.Stderr, "程序发生异常，崩溃报告已写入: %s\n", filePath)
}

func resolveCrashLogDir(app *CoreApp) string {
	if app != nil && app.logger != nil {
		if logDir := app.logger.GetLogDir(); logDir != "" {
			return logDir
		}
	}
	return config.GetLogDir()
}
