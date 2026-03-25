// Package logger 提供基于 slog 的日志记录功能
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

// CustomLogger slog 日志记录器封装
type CustomLogger struct {
	logger     *slog.Logger
	levelVar   *slog.LevelVar
	debugMode  bool
	logDir     string
	fileWriter *lumberjack.Logger
}

type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
)

type Options struct {
	Format  Format
	Console bool
}

// NewCustomLogger 创建新的日志记录器
func NewCustomLogger(debugMode bool, installDir string, prefix string) (*CustomLogger, error) {
	return NewCustomLoggerWithOptions(debugMode, installDir, prefix, Options{})
}

// NewCustomLoggerWithOptions 创建带选项的日志记录器
func NewCustomLoggerWithOptions(debugMode bool, installDir string, prefix string, opts Options) (*CustomLogger, error) {
	logDir := filepath.Join(installDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %v", err)
	}

	if prefix == "" {
		prefix = "core"
	}

	logFilePath := filepath.Join(logDir, fmt.Sprintf("%s_%s.log", prefix, time.Now().Format("2006-01-02")))
	fileWriter := &lumberjack.Logger{
		Filename:   logFilePath,
		MaxSize:    10, // MB
		MaxBackups: 7,
		MaxAge:     7, // 天
		Compress:   true,
	}

	levelVar := &slog.LevelVar{}
	if debugMode {
		levelVar.Set(slog.LevelDebug)
	} else {
		levelVar.Set(slog.LevelInfo)
	}

	handler := newHandler(fileWriter, levelVar, prefix, opts)
	logger := slog.New(handler)

	return &CustomLogger{
		logger:     logger,
		levelVar:   levelVar,
		debugMode:  debugMode,
		logDir:     logDir,
		fileWriter: fileWriter,
	}, nil
}

func newHandler(fileWriter io.Writer, levelVar *slog.LevelVar, prefix string, opts Options) slog.Handler {
	format := opts.Format
	if format == "" {
		format = FormatText
	}
	console := true
	if !opts.Console {
		console = false
	}

	writers := []io.Writer{fileWriter}
	if console {
		writers = append(writers, os.Stdout)
	}
	target := io.MultiWriter(writers...)

	handlerOptions := &slog.HandlerOptions{
		AddSource: true,
		Level:     levelVar,
		ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
			switch attr.Key {
			case slog.TimeKey:
				if t, ok := attr.Value.Any().(time.Time); ok {
					return slog.String(slog.TimeKey, t.Format("2006-01-02 15:04:05"))
				}
			case slog.LevelKey:
				return slog.String(slog.LevelKey, strings.ToUpper(attr.Value.String()))
			case slog.SourceKey:
				source, ok := attr.Value.Any().(*slog.Source)
				if !ok || source == nil {
					return slog.Attr{}
				}
				return slog.String(slog.SourceKey, trimSource(source.File, source.Line))
			}
			return attr
		},
	}

	var handler slog.Handler
	switch format {
	case FormatJSON:
		handler = slog.NewJSONHandler(target, handlerOptions)
	default:
		handler = slog.NewTextHandler(target, handlerOptions)
	}

	return handler.WithAttrs([]slog.Attr{slog.String("service", prefix)})
}

// GetSlogLogger 获取底层 slog.Logger
func (l *CustomLogger) GetSlogLogger() *slog.Logger {
	if l == nil {
		return nil
	}
	return l.logger
}

// Info 记录信息日志
func (l *CustomLogger) Info(msg string, args ...any) {
	l.emit(slog.LevelInfo, msg, args...)
}

// Error 记录错误日志
func (l *CustomLogger) Error(msg string, args ...any) {
	l.emit(slog.LevelError, msg, args...)
}

// Debug 记录调试日志
func (l *CustomLogger) Debug(msg string, args ...any) {
	l.emit(slog.LevelDebug, msg, args...)
}

// Warn 记录警告日志
func (l *CustomLogger) Warn(msg string, args ...any) {
	l.emit(slog.LevelWarn, msg, args...)
}

// GetLogDir 获取日志目录
func (l *CustomLogger) GetLogDir() string {
	return l.logDir
}

// Sync 保留旧兼容接口；当前实现为直写输出，无额外缓冲需要刷新。
func (l *CustomLogger) Sync() {}

// Close 关闭日志
func (l *CustomLogger) Close() {
	if l == nil || l.fileWriter == nil {
		return
	}
	_ = l.fileWriter.Close()
}

// CleanOldLogs 清理旧日志文件（保留7天）
func (l *CustomLogger) CleanOldLogs() {
	files, err := os.ReadDir(l.logDir)
	if err != nil {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -7)
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".log") || strings.HasSuffix(file.Name(), ".log.gz") {
			info, err := file.Info()
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoff) {
				os.Remove(filepath.Join(l.logDir, file.Name()))
			}
		}
	}
}

// SetDebugMode 设置调试模式
func (l *CustomLogger) SetDebugMode(enabled bool) {
	if l == nil || l.levelVar == nil {
		return
	}
	l.debugMode = enabled
	if enabled {
		l.levelVar.Set(slog.LevelDebug)
	} else {
		l.levelVar.Set(slog.LevelInfo)
	}
}

func (l *CustomLogger) emit(level slog.Level, msg string, args ...any) {
	if l == nil || l.logger == nil {
		return
	}
	if len(args) == 0 {
		l.writeRecord(level, msg)
		return
	}
	if strings.Contains(msg, "%") || !looksStructured(args) {
		l.emitf(level, msg, args...)
		return
	}
	l.writeRecord(level, msg, args...)
}

func (l *CustomLogger) emitf(level slog.Level, format string, args ...any) {
	if l == nil || l.logger == nil {
		return
	}
	l.writeRecord(level, fmt.Sprintf(format, args...))
}

func (l *CustomLogger) writeRecord(level slog.Level, msg string, args ...any) {
	record := slog.NewRecord(time.Now(), level, msg, callerPC())
	record.Add(args...)
	_ = l.logger.Handler().Handle(context.Background(), record)
}

func callerPC() uintptr {
	pcs := make([]uintptr, 16)
	n := runtime.Callers(3, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		if !shouldSkipFrame(frame) {
			return frame.PC
		}
		if !more {
			break
		}
	}
	return 0
}

func shouldSkipFrame(frame runtime.Frame) bool {
	file := filepath.ToSlash(frame.File)
	if strings.Contains(file, "/internal/logger/") {
		return true
	}
	for _, suffix := range []string{".logInfo", ".logError", ".logWarn", ".logDebug"} {
		if strings.HasSuffix(frame.Function, suffix) {
			return true
		}
	}
	return false
}

func looksStructured(args []any) bool {
	if len(args) == 0 {
		return false
	}
	allAttrs := true
	for _, arg := range args {
		if _, ok := arg.(slog.Attr); !ok {
			allAttrs = false
			break
		}
	}
	if allAttrs {
		return true
	}
	if len(args)%2 != 0 {
		return false
	}
	for i := 0; i < len(args); i += 2 {
		if _, ok := args[i].(string); !ok {
			return false
		}
	}
	return true
}

func trimSource(file string, line int) string {
	normalized := filepath.ToSlash(file)
	marker := "BS2PRO-Controller/"
	if idx := strings.LastIndex(normalized, marker); idx >= 0 {
		normalized = normalized[idx+len(marker):]
	}
	return fmt.Sprintf("%s:%d", normalized, line)
}
