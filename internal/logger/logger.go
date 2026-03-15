// Package logger 提供基于 zap 的日志记录功能
package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// csvEncoder 自定义CSV格式编码器
type csvEncoder struct {
	zapcore.Encoder
	pool buffer.Pool
}

// NewCSVEncoder 创建CSV格式编码器
func NewCSVEncoder(cfg zapcore.EncoderConfig) zapcore.Encoder {
	return &csvEncoder{
		Encoder: zapcore.NewConsoleEncoder(cfg),
		pool:    buffer.NewPool(),
	}
}

// EncodeEntry 编码日志条目为CSV格式
func (e *csvEncoder) EncodeEntry(entry zapcore.Entry, fields []zapcore.Field) (*buffer.Buffer, error) {
	buf := e.pool.Get()

	// 编码级别
	buf.AppendString(`"`)
	buf.AppendString(entry.Level.CapitalString())
	buf.AppendString(`",`)

	// 编码时间
	buf.AppendString(`"`)
	buf.AppendString(entry.Time.Format("2006-01-02 15:04:05"))
	buf.AppendString(`",`)

	// 编码调用者
	if entry.Caller.Defined {
		buf.AppendString(`"`)
		buf.AppendString(entry.Caller.TrimmedPath())
		buf.AppendString(`",`)
	} else {
		buf.AppendString(`"",`)
	}

	// 编码消息
	buf.AppendString(`"`)
	escapedMsg := strings.ReplaceAll(entry.Message, `"`, `\"`)
	buf.AppendString(escapedMsg)
	buf.AppendString(`"`)

	buf.AppendByte('\n')

	return buf, nil
}

// Clone 克隆编码器
func (e *csvEncoder) Clone() zapcore.Encoder {
	return &csvEncoder{
		Encoder: e.Encoder.Clone(),
		pool:    e.pool,
	}
}

// CustomLogger zap 日志记录器封装
type CustomLogger struct {
	logger    *zap.Logger
	sugar     *zap.SugaredLogger
	debugMode bool
	logDir    string
	atom      zap.AtomicLevel
}

// NewCustomLogger 创建新的日志记录器
func NewCustomLogger(debugMode bool, installDir string, prefix string) (*CustomLogger, error) {
	logDir := filepath.Join(installDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %v", err)
	}

	if prefix == "" {
		prefix = "core"
	}

	// 主日志文件路径
	logFilePath := filepath.Join(logDir, fmt.Sprintf("%s_%s.log", prefix, time.Now().Format("2006-01-02")))

	// 创建主日志轮转配置
	appLogRotate := &lumberjack.Logger{
		Filename:   logFilePath,
		MaxSize:    10, // MB
		MaxBackups: 7,
		MaxAge:     7, // 天
		Compress:   true,
	}

	// 编码器配置
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	consoleEncoderConfig := encoderConfig
	consoleEncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder

	// 设置日志级别
	atom := zap.NewAtomicLevel()
	if debugMode {
		atom.SetLevel(zapcore.DebugLevel)
	} else {
		atom.SetLevel(zapcore.InfoLevel)
	}

	// 创建多个核心
	consoleEncoder := zapcore.NewConsoleEncoder(consoleEncoderConfig)
	csvEncoderConfig := zapcore.EncoderConfig{
		TimeKey:       "time",
		LevelKey:      "level",
		NameKey:       "logger",
		CallerKey:     "caller",
		FunctionKey:   zapcore.OmitKey,
		MessageKey:    "msg",
		StacktraceKey: "stacktrace",
		LineEnding:    zapcore.DefaultLineEnding,
		EncodeLevel: func(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
			enc.AppendString(`"` + l.CapitalString() + `"`)
		},
		EncodeTime: func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
			enc.AppendString(`"` + t.Format("2006-01-02 15:04:05") + `"`)
		},
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller: func(caller zapcore.EntryCaller, enc zapcore.PrimitiveArrayEncoder) {
			enc.AppendString(`"` + caller.TrimmedPath() + `"`)
		},
	}

	fileEncoder := NewCSVEncoder(csvEncoderConfig)

	// 总是创建主日志核心
	appCore := zapcore.NewCore(
		fileEncoder,
		zapcore.AddSync(appLogRotate),
		atom,
	)

	// 控制台输出核心
	consoleCore := zapcore.NewCore(
		consoleEncoder,
		zapcore.AddSync(os.Stdout),
		atom,
	)

	// 动态构建核心列表
	cores := []zapcore.Core{appCore, consoleCore}

	// 只有在debug模式开启时才创建debug日志文件
	if debugMode {
		debugFilePath := filepath.Join(logDir, fmt.Sprintf("%s_debug_%s.log", prefix, time.Now().Format("2006-01-02")))
		debugLogRotate := &lumberjack.Logger{
			Filename:   debugFilePath,
			MaxSize:    10,
			MaxBackups: 7,
			MaxAge:     7,
			Compress:   true,
		}

		debugCore := zapcore.NewCore(
			fileEncoder,
			zapcore.AddSync(debugLogRotate),
			atom,
		)
		cores = append(cores, debugCore)
	}

	// 合并核心
	core := zapcore.NewTee(cores...)

	// 创建 logger
	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(2))
	sugar := logger.Sugar()

	return &CustomLogger{
		logger:    logger,
		sugar:     sugar,
		debugMode: debugMode,
		logDir:    logDir,
		atom:      atom,
	}, nil
}

// Info 记录信息日志
func (l *CustomLogger) Info(format string, v ...any) {
	l.sugar.Infof(format, v...)
}

// Error 记录错误日志
func (l *CustomLogger) Error(format string, v ...any) {
	l.sugar.Errorf(format, v...)
}

// Debug 记录调试日志
func (l *CustomLogger) Debug(format string, v ...any) {
	l.sugar.Debugf(format, v...)
}

// Warn 记录警告日志
func (l *CustomLogger) Warn(format string, v ...any) {
	l.sugar.Warnf(format, v...)
}

// GetLogDir 获取日志目录
func (l *CustomLogger) GetLogDir() string {
	return l.logDir
}

// GetSugaredLogger 获取底层的zap.SugaredLogger
func (l *CustomLogger) GetSugaredLogger() *zap.SugaredLogger {
	return l.sugar
}

// Close 关闭日志
func (l *CustomLogger) Close() {
	if l.logger != nil {
		l.logger.Sync()
	}
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
	l.debugMode = enabled
	if enabled {
		l.atom.SetLevel(zapcore.DebugLevel)
	} else {
		l.atom.SetLevel(zapcore.InfoLevel)
	}
}
