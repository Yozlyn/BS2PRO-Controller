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
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

// CustomLogger slog 日志记录器封装
type CustomLogger struct {
	logger     *slog.Logger
	levelVar   *slog.LevelVar
	debugMode  bool
	logDir     string
	fileWriter managedFileWriter
}

type managedFileWriter interface {
	io.Writer
	Close() error
	CleanOldLogs()
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

const (
	logMaxSizeMB     = 10
	logRetentionDays = 7
	projectSourceTag = "BS2PRO-Controller/"
	moduleCacheTag   = "/pkg/mod/"
)

type framePriority uint8

const (
	framePriorityNone framePriority = iota
	framePriorityStdlib
	framePriorityExternal
	framePriorityProject
)

var (
	projectSourcePrefix = detectProjectSourcePrefix()
	goRootSourcePrefix  = detectGoRootSourcePrefix()
	wrapperFuncSuffixes = []string{
		".logInfo",
		".logError",
		".logWarn",
		".logDebug",
		".monitorInfo",
		".monitorWarn",
		".monitorError",
		".monitorDebug",
	}
)

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
	opts.Format = normalizeFormat(opts.Format)

	fileWriter, err := newDailyFileWriter(logDir, prefix, opts.Format)
	if err != nil {
		return nil, fmt.Errorf("创建日志文件失败: %v", err)
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

type dailyFileWriter struct {
	mu          sync.Mutex
	logDir      string
	format      Format
	prefix      string
	currentDate string
	logger      *lumberjack.Logger
}

func newDailyFileWriter(logDir string, prefix string, format Format) (*dailyFileWriter, error) {
	writer := &dailyFileWriter{
		logDir: logDir,
		format: normalizeFormat(format),
		prefix: prefix,
	}
	if err := writer.rotateIfNeeded(time.Now()); err != nil {
		return nil, err
	}
	return writer, nil
}

func (w *dailyFileWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.rotateIfNeededLocked(time.Now()); err != nil {
		return 0, err
	}
	return w.logger.Write(p)
}

func (w *dailyFileWriter) Close() error {
	if w == nil {
		return nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.logger == nil {
		return nil
	}
	err := w.logger.Close()
	w.logger = nil
	return err
}

func (w *dailyFileWriter) CleanOldLogs() {
	if w == nil {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	_ = cleanupOldLogs(w.logDir, time.Now())
}

func (w *dailyFileWriter) rotateIfNeeded(now time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.rotateIfNeededLocked(now)
}

func (w *dailyFileWriter) rotateIfNeededLocked(now time.Time) error {
	date := now.Format("2006-01-02")
	if w.logger != nil && w.currentDate == date {
		return nil
	}

	nextLogger := &lumberjack.Logger{
		Filename:   filepath.Join(w.logDir, runtimeLogFilename(w.prefix, date, w.format)),
		MaxSize:    logMaxSizeMB,
		MaxBackups: 0,
		MaxAge:     logRetentionDays,
		Compress:   false,
	}
	prevLogger := w.logger
	w.logger = nextLogger
	w.currentDate = date
	if prevLogger != nil {
		_ = prevLogger.Close()
	}
	return cleanupOldLogs(w.logDir, now)
}

func newHandler(fileWriter io.Writer, levelVar *slog.LevelVar, prefix string, opts Options) slog.Handler {
	format := opts.Format
	if format == "" {
		format = FormatText
	}
	console := opts.Console

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
				return slog.String(slog.SourceKey, trimSource(source.File, source.Line, source.Function))
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
func (l *CustomLogger) Info(msg any, args ...any) {
	l.emit(slog.LevelInfo, stringifyMessage(msg), args...)
}

// Error 记录错误日志
func (l *CustomLogger) Error(msg any, args ...any) {
	l.emit(slog.LevelError, stringifyMessage(msg), args...)
}

// Debug 记录调试日志
func (l *CustomLogger) Debug(msg any, args ...any) {
	l.emit(slog.LevelDebug, stringifyMessage(msg), args...)
}

// Warn 记录警告日志
func (l *CustomLogger) Warn(msg any, args ...any) {
	l.emit(slog.LevelWarn, stringifyMessage(msg), args...)
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
	if l == nil {
		return
	}
	if l.fileWriter != nil {
		l.fileWriter.CleanOldLogs()
		return
	}
	_ = cleanupOldLogs(l.logDir, time.Now())
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
	pcs := make([]uintptr, 32)
	n := runtime.Callers(3, pcs)
	if n == 0 {
		return 0
	}
	frames := runtime.CallersFrames(pcs[:n])
	var bestPC uintptr
	bestPriority := framePriorityNone
	for {
		frame, more := frames.Next()
		priority := framePriorityOf(frame)
		if priority > bestPriority {
			bestPC = frame.PC
			bestPriority = priority
			if priority == framePriorityProject {
				return bestPC
			}
		}
		if !more {
			break
		}
	}
	return bestPC
}

func framePriorityOf(frame runtime.Frame) framePriority {
	if shouldSkipFrame(frame) {
		return framePriorityNone
	}
	if isProjectFrame(frame.File) {
		return framePriorityProject
	}
	if isStdlibFrame(frame) {
		return framePriorityStdlib
	}
	return framePriorityExternal
}

func shouldSkipFrame(frame runtime.Frame) bool {
	file := filepath.ToSlash(frame.File)
	if file == "" {
		return true
	}
	if strings.Contains(file, "/internal/logger/") {
		return true
	}
	if strings.Contains(frame.Function, "monitorTrayLoggerAdapter") {
		return true
	}
	for _, suffix := range wrapperFuncSuffixes {
		if strings.HasSuffix(frame.Function, suffix) {
			return true
		}
	}
	if isRuntimeBoilerplateFrame(file, frame.Function) {
		return true
	}
	if isStdlibBridgeFrame(file, frame.Function) {
		return true
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

func stringifyMessage(msg any) string {
	if s, ok := msg.(string); ok {
		return s
	}
	return fmt.Sprint(msg)
}

func trimSource(file string, line int, function string) string {
	normalized := normalizeSourcePath(file)
	if shouldUseFunctionSource(file) {
		if fallback := normalizeSourceFunction(function); fallback != "" {
			return fallback
		}
	}
	if normalized == "" {
		if fallback := normalizeSourceFunction(function); fallback != "" {
			return fallback
		}
		normalized = filepath.Base(file)
	}
	if line <= 0 {
		return normalized
	}
	return fmt.Sprintf("%s:%d", normalized, line)
}

func normalizeFormat(format Format) Format {
	if format == FormatJSON {
		return FormatJSON
	}
	return FormatText
}

func runtimeLogFilename(prefix string, date string, format Format) string {
	return fmt.Sprintf("%s.%s.%s.log", prefix, date, format)
}

func detectProjectSourcePrefix() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
	return filepath.ToSlash(root) + "/"
}

func detectGoRootSourcePrefix() string {
	goRoot := strings.TrimSpace(os.Getenv("GOROOT"))
	if goRoot == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Join(goRoot, "src")) + "/"
}

func isProjectFrame(file string) bool {
	normalized := filepath.ToSlash(file)
	switch {
	case normalized == "":
		return false
	case projectSourcePrefix != "" && strings.HasPrefix(normalized, projectSourcePrefix):
		return true
	default:
		return strings.Contains(normalized, "/"+projectSourceTag)
	}
}

func isStdlibFrame(frame runtime.Frame) bool {
	if goRootSourcePrefix == "" {
		return false
	}
	return strings.HasPrefix(filepath.ToSlash(frame.File), goRootSourcePrefix)
}

func isStdlibBridgeFrame(file string, function string) bool {
	if !strings.Contains(function, ".") {
		return false
	}
	if strings.Contains(file, "/reflect/") {
		switch function {
		case "reflect.Value.Call", "reflect.Value.call", "reflect.makeFuncStub":
			return true
		}
	}
	switch function {
	case "syscall.Syscall", "syscall.Syscall6", "syscall.Syscall9", "syscall.Syscall12", "syscall.Syscall15", "syscall.Syscall18", "syscall.SyscallN", "syscall.RawSyscall", "syscall.RawSyscall6":
		return true
	}
	return false
}

func isRuntimeBoilerplateFrame(file string, function string) bool {
	if !strings.Contains(file, "/runtime/") {
		return false
	}
	base := filepath.Base(file)
	if strings.HasPrefix(base, "asm_") || strings.HasPrefix(base, "signal_") {
		return true
	}
	switch function {
	case "runtime.goexit", "runtime.main", "runtime.sigpanic":
		return true
	}
	switch base {
	case "panic.go", "proc.go":
		return true
	}
	return false
}

func normalizeSourcePath(file string) string {
	normalized := filepath.ToSlash(file)
	switch {
	case projectSourcePrefix != "" && strings.HasPrefix(normalized, projectSourcePrefix):
		return strings.TrimPrefix(normalized, projectSourcePrefix)
	case goRootSourcePrefix != "" && strings.HasPrefix(normalized, goRootSourcePrefix):
		return strings.TrimPrefix(normalized, goRootSourcePrefix)
	}
	if modulePath := trimModuleCacheSource(normalized); modulePath != "" {
		return modulePath
	}
	if idx := strings.LastIndex(normalized, projectSourceTag); idx >= 0 {
		return normalized[idx+len(projectSourceTag):]
	}
	return normalized
}

func trimModuleCacheSource(path string) string {
	idx := strings.Index(path, moduleCacheTag)
	if idx < 0 {
		return ""
	}
	trimmed := path[idx+len(moduleCacheTag):]
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, "/")
	for i, part := range parts {
		if at := strings.Index(part, "@"); at >= 0 {
			parts[i] = part[:at]
		}
	}
	return strings.Join(parts, "/")
}

func normalizeSourceFunction(function string) string {
	function = strings.TrimSpace(function)
	if function == "" {
		return ""
	}
	function = strings.TrimSuffix(function, "-fm")
	if idx := strings.LastIndex(function, projectSourceTag); idx >= 0 {
		return function[idx+len(projectSourceTag):]
	}
	return function
}

func shouldUseFunctionSource(file string) bool {
	normalized := filepath.ToSlash(file)
	if normalized == "" {
		return true
	}
	if projectSourcePrefix != "" && strings.HasPrefix(normalized, projectSourcePrefix) {
		return false
	}
	return goRootSourcePrefix != "" && strings.HasPrefix(normalized, goRootSourcePrefix)
}

func cleanupOldLogs(logDir string, now time.Time) error {
	files, err := os.ReadDir(logDir)
	if err != nil {
		return err
	}

	cutoff := now.AddDate(0, 0, -logRetentionDays)
	for _, file := range files {
		if !isManagedLogFile(file.Name()) {
			continue
		}
		info, err := file.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(logDir, file.Name()))
		}
	}
	return nil
}

func isManagedLogFile(name string) bool {
	return strings.HasSuffix(name, ".log") || strings.HasSuffix(name, ".log.gz")
}
